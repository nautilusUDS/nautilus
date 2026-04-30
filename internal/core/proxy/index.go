package proxy

import (
	"log"
	"nautilus/internal/core/builtins"
	"nautilus/internal/core/builtins/builtinsmware"
	"nautilus/internal/core/builtins/virtualservices"
	"nautilus/internal/core/forwarder"
	"nautilus/internal/core/metrics"
	"nautilus/internal/core/registry"
	"nautilus/internal/rtree"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
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
	NodeLock sync.RWMutex
	Nodes    map[string][]string
	Indices  map[string]*uint32
	Registry *registry.Registry

	builtinCache sync.Map // map[string]http.HandlerFunc
	virtualCache sync.Map // map[string]http.HandlerFunc
	Forwarder    *forwarder.Forwarder
}

func NewManager(reg *registry.Registry) *Manager {
	m := &Manager{
		Nodes:     make(map[string][]string),
		Indices:   make(map[string]*uint32),
		Registry:  reg,
		Forwarder: forwarder.NewForwarder(reg.OnFailure),
	}
	m.Tree.Store(&rtree.RouteTree{})
	return m
}

func (m *Manager) UpdateTree(newTree *rtree.RouteTree) {
	m.Tree.Store(newTree)
	metrics.Global.IncUpdates()
}

func (m *Manager) UpdateNodes(nodes map[string][]string) {
	m.NodeLock.Lock()
	defer m.NodeLock.Unlock()

	m.Nodes = nodes
	for svc := range nodes {
		if _, exists := m.Indices[svc]; !exists {
			var i uint32
			m.Indices[svc] = &i
		}
	}
	metrics.Global.IncUpdates()
}

func (m *Manager) UpdateServiceNodes(serviceName string, nodes []string) {
	m.NodeLock.Lock()
	defer m.NodeLock.Unlock()

	if len(nodes) == 0 {
		delete(m.Nodes, serviceName)
	} else {
		m.Nodes[serviceName] = nodes
		if _, exists := m.Indices[serviceName]; !exists {
			var i uint32
			m.Indices[serviceName] = &i
		}
	}
	metrics.Global.IncUpdates()
}

// resolveBuiltinMiddleware parses and caches functional middleware expressions.
func (m *Manager) resolveBuiltinMiddleware(expr string) http.HandlerFunc {
	if h, ok := m.builtinCache.Load(expr); ok {
		return h.(http.HandlerFunc)
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
	urlPath := host + r.URL.Path
	node, exists := tree.Search(rtree.ReverseHost(urlPath))
	if !exists {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	serviceName := tree.ServicePool[node.ServiceID]

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
	rs := &responseState{ResponseWriter: w}

	// 3. Middleware Execution
	for _, mwID := range node.Middlewares {
		mwExpr := tree.MiddlewarePool[mwID]
		handled := false

		// Try Built-in
		if strings.HasPrefix(mwExpr, "$") {
			if handler := m.resolveBuiltinMiddleware(mwExpr); handler != nil {
				handler(rs, r)
				handled = true
			}
		} else {
			// Try External UDS
			if !m.Forwarder.ForwardMiddleware(rs, r, r.URL.Path, mwExpr) {
				return
			}
			handled = true
		}

		if handled && rs.wroteHeader {
			return
		}
	}

	// 4. Virtual Service Check
	if strings.HasPrefix(serviceName, "$") {
		if handler := m.resolveVirtualService(serviceName); handler != nil {
			handler(rs, r)
			return
		}
		http.Error(rs, "Unknown Virtual Service: "+serviceName, http.StatusInternalServerError)
		return
	}

	// 5. Load Balancing (Protected by RLock)
	m.NodeLock.RLock()
	nodes, ok := m.Nodes[serviceName]
	indexPtr := m.Indices[serviceName]
	m.NodeLock.RUnlock()

	if !ok || len(nodes) == 0 {
		log.Printf("No healthy nodes for service: %s", serviceName)
		metrics.Global.IncErrors()
		http.Error(rs, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}

	idx := atomic.AddUint32(indexPtr, 1) % uint32(len(nodes))
	targetNode := nodes[idx]

	// 6. Final Backend Forwarding
	m.Forwarder.Forward(rs, r, serviceName, targetNode)
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
	log.Printf("Nautilus Core listening on UDS: %s", socketPath)
	return http.Serve(listener, m)
}
