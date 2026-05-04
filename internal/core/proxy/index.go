package proxy

import (
	"nautilus/internal/core/builtins"
	"nautilus/internal/core/builtins/builtinsmware"
	"nautilus/internal/core/builtins/virtualservices"
	"nautilus/internal/core/logs"
	"nautilus/internal/core/metrics"
	"nautilus/internal/core/registry"
	"nautilus/internal/interpolate"
	"nautilus/internal/rtree"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"go.uber.org/zap"
)

// responseState wraps http.ResponseWriter to track if a response has been initiated.
type responseState struct {
	http.ResponseWriter
	wroteHeader bool
}

func (rs *responseState) WriteHeader(code int) {
	rs.wroteHeader = true
	rs.ResponseWriter.WriteHeader(code)
}

func (rs *responseState) Write(b []byte) (int, error) {
	rs.wroteHeader = true
	return rs.ResponseWriter.Write(b)
}

type Manager struct {
	Tree     atomic.Pointer[rtree.RouteTree]
	Registry *registry.Registry

	builtinCache sync.Map // map[string]http.HandlerFunc
	virtualCache sync.Map // map[string]http.HandlerFunc
}

func NewManager(reg *registry.Registry) *Manager {
	m := &Manager{
		Registry: reg,
	}
	m.Tree.Store(&rtree.RouteTree{})
	return m
}

func (m *Manager) UpdateTree(newTree *rtree.RouteTree) {
	m.Tree.Store(newTree)
	metrics.Global.IncUpdates()
}

// resolveBuiltinMiddleware parses and caches functional middleware expressions.
func (m *Manager) resolveBuiltinMiddleware(expr string) builtinsmware.HandlerFunc {
	if h, ok := m.builtinCache.Load(expr); ok {
		return h.(builtinsmware.HandlerFunc)
	}

	funcName, args, err := builtins.ParseDirective(expr)
	if err != nil {
		return nil
	}

	if factory, ok := builtinsmware.Registry[funcName]; ok {
		handler := factory(args...)
		m.builtinCache.Store(expr, handler)
		return handler
	}

	return nil
}

// resolveVirtualService parses and caches functional virtual service expressions.
func (m *Manager) resolveVirtualService(expr string) http.HandlerFunc {
	// Special case for $services which needs access to registry state
	if expr == "$services" {
		return virtualservices.Discovery(m.Registry.GetState())
	}

	funcName, args, err := builtins.ParseDirective(expr)
	if err != nil {
		return nil
	}

	// Special case for $ping which needs access to registry and arguments
	if funcName == "$ping" {
		targetSvc := ""
		if len(args) > 0 {
			targetSvc = args[0]
		}
		nodes := m.Registry.GetNodes(targetSvc)
		return virtualservices.Ping(targetSvc, nodes)
	}

	if h, ok := m.virtualCache.Load(expr); ok {
		return h.(http.HandlerFunc)
	}

	if factory, ok := virtualservices.Registry[funcName]; ok {
		handler := factory(args...)
		m.virtualCache.Store(expr, handler)
		return handler
	}

	return nil
}

func (m *Manager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	metrics.Global.IncRequests()
	metrics.Global.AddActive(1)
	defer metrics.Global.AddActive(-1)

	tree := m.Tree.Load()

	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		host = r.Host
	}

	// 1. Route Lookup
	lookupPath := host + r.URL.Path
	node, exists := tree.Search(rtree.ReverseHost(lookupPath))
	if !exists {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	// 2. Method Validation
	methodBit := rtree.HTTPMethodMap[r.Method]
	if methodBit == 0 {
		methodBit = rtree.MethodAny
	}
	if node.Methods&methodBit == 0 {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Wrap ResponseWriter to track activity
	trackedWriter := &responseState{ResponseWriter: w}
	interpolator := interpolate.New(r)

	serviceMetadataIndex := tree.ActionMetadata[node.ActionIndex]
	targetServiceID := tree.ActionMetadata[serviceMetadataIndex]
	rawServiceName := tree.GetActionName(targetServiceID)

	mwCount := tree.ActionMetadata[node.ActionIndex+1]
	resolvedMiddlewares := make([]string, mwCount)

	if mwCount > 0 {
		baseOffset := node.ActionIndex + 2
		for i := range mwCount {
			mwMetaIndex := tree.ActionMetadata[baseOffset+i]
			rawMwName := tree.GetActionName(tree.ActionMetadata[mwMetaIndex])

			opLen := tree.ActionMetadata[mwMetaIndex+1]
			if opLen > 0 {
				opOffset := mwMetaIndex + 2
				ops := tree.ActionMetadata[opOffset : opOffset+opLen]
				resolvedMiddlewares[i] = interpolator.Replace(rawMwName, ops)
			} else {
				resolvedMiddlewares[i] = rawMwName
			}
		}
	}
	// 3. Middleware Execution
	for _, mwExpr := range resolvedMiddlewares {
		isHandled := false

		if strings.HasPrefix(mwExpr, "$") {
			// Internal built-in middleware logic
			if handler := m.resolveBuiltinMiddleware(mwExpr); handler != nil {
				resp := builtinsmware.NewResponseWriter()
				handler(resp, r)
				if resp.GetCode() != http.StatusOK {
					resp.WriteTo(trackedWriter)
					return
				}
				isHandled = true
			} else {
				logs.Out.Error("Failed to resolve built-in middleware", zap.String("middleware", mwExpr))
				http.Error(trackedWriter, "Internal Middleware Error", http.StatusInternalServerError)
				return
			}
		} else {
			mw, err := m.Registry.GetForwarder(mwExpr)
			if err != nil {
				http.Error(trackedWriter, "Middleware Node Not Found", http.StatusBadGateway)
				return
			}

			resp := builtinsmware.NewResponseWriter()
			// External middleware (usually over UDS)

			if !mw.ForwardMiddleware(resp, r, "") {
				// If forwarding fails or is intercepted, stop execution
				resp.WriteTo(trackedWriter)
				return
			}
			isHandled = true
		}

		// If a middleware has already sent a response, terminate the chain
		if isHandled && trackedWriter.wroteHeader {
			return
		}
	}

	finalServiceName := rawServiceName
	if opCount := tree.ActionMetadata[serviceMetadataIndex+1]; opCount > 0 {
		offset := serviceMetadataIndex + 2
		ops := tree.ActionMetadata[offset : offset+opCount]
		finalServiceName = interpolator.Replace(rawServiceName, ops)
	}

	// 4. Virtual Service Check
	if strings.HasPrefix(finalServiceName, "$") {
		if handler := m.resolveVirtualService(finalServiceName); handler != nil {
			handler(trackedWriter, r)
			return
		}
		logs.Out.Error("Failed to resolve virtual service", zap.String("virtualService", finalServiceName))
		http.Error(trackedWriter, "Unknown Virtual Service: "+finalServiceName, http.StatusInternalServerError)
		return
	}

	// 5. Load Balancing (Protected by RLock)
	service, err := m.Registry.GetForwarder(finalServiceName)
	if err != nil {
		http.Error(trackedWriter, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}
	service.Forward(w, r)
}

func (m *Manager) StartUDSListener(socketPath string) error {
	if _, err := os.Stat(socketPath); err == nil {
		if err := os.Remove(socketPath); err != nil {
			return err
		}
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	if err := os.Chmod(socketPath, 0666); err != nil {
		return err
	}
	logs.Out.Info("Nautilus Core listening on UDS", zap.String("socketPath", socketPath))
	return http.Serve(listener, m)
}
