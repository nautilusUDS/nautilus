package registry

import (
	"log"
	"nautilus/internal/core/registry/forwarder"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
)

type Registry struct {
	mu      sync.RWMutex
	baseDir string

	services map[string]*ServiceSet  // serviceName -> node list & load balanced index
	nodeMap  map[string]*nodeContext // nodePath -> forwarder & service name

	failureChan chan forwarder.FailureForwarder
}

type ServiceSet struct {
	nodes []string
	index uint32
}

type nodeContext struct {
	serviceName string
	forwarder   *forwarder.Forwarder
}

func NewRegistry(baseDir string) (*Registry, error) {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, err
	}

	r := &Registry{
		services:    make(map[string]*ServiceSet),
		nodeMap:     make(map[string]*nodeContext),
		baseDir:     strings.TrimRight(filepath.ToSlash(absBase), "/"),
		failureChan: make(chan forwarder.FailureForwarder, 100),
	}

	go r.listenFailures()

	return r, nil
}

func (r *Registry) BaseDir() string {
	return r.baseDir
}

func (r *Registry) listenFailures() {
	for failure := range r.failureChan {
		log.Printf("[Registry] Node failure detected: %s, error: %v", failure.SocketPath, failure.Error)
		r.RemoveNode(failure.SocketPath, true)
	}
}

// GetNodes returns a copy of the current physical nodes for a service
func (r *Registry) GetNodes(serviceName string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ss, ok := r.services[serviceName]
	if !ok {
		return nil
	}
	return slices.Clone(ss.nodes)
}

// GetState returns a snapshot of all services and their current nodes.
func (r *Registry) GetState() map[string][]string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	state := make(map[string][]string)
	for name, ss := range r.services {
		state[name] = slices.Clone(ss.nodes)
	}
	return state
}

func (r *Registry) GetForwarder(serviceName string) (*forwarder.Forwarder, error) {
	r.mu.RLock()
	ss, exists := r.services[serviceName]
	r.mu.RUnlock()

	if !exists || len(ss.nodes) == 0 {
		return nil, os.ErrNotExist
	}

	idx := atomic.AddUint32(&ss.index, 1) % uint32(len(ss.nodes))
	nodePath := ss.nodes[idx]

	r.mu.RLock()
	ctx, ok := r.nodeMap[nodePath]
	r.mu.RUnlock()

	if !ok {
		return nil, os.ErrNotExist
	}

	return ctx.forwarder, nil
}

// Scan satisfies the Watcher's expectation. It performs either a full scan
// or a targeted scan based on the provided target string.
func (r *Registry) Scan(target string) error {
	// If target is empty or matches baseDir, perform full scan
	if target == "" || target == r.baseDir {
		return r.fullScan()
	}

	return r.ScanService(target)
}

func (r *Registry) fullScan() error {
	scannedState := make(map[string][]string)
	baseLen := len(r.baseDir) + 1

	err := filepath.WalkDir(r.baseDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && strings.HasSuffix(d.Name(), ".sock") {
			rel := filepath.ToSlash(path[baseLen:])
			if !strings.Contains(rel, "/") {
				return nil
			}

			serviceName := filepath.Dir(rel)
			scannedState[serviceName] = append(scannedState[serviceName], path)
		}
		return nil
	})

	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// 1. Remove services/nodes no longer present
	for path, ctx := range r.nodeMap {
		discovered, found := scannedState[ctx.serviceName]
		if !found || !slices.Contains(discovered, path) {
			r.removeNodeUnsafe(path, false)
		}
	}

	// 2. Add new nodes and update services
	for svcName, nodes := range scannedState {
		for _, node := range nodes {
			if _, exists := r.nodeMap[node]; !exists {
				r.nodeMap[node] = &nodeContext{
					serviceName: svcName,
					forwarder:   forwarder.New(node, r.failureChan),
				}
			}
		}

		if ss, exists := r.services[svcName]; exists {
			ss.nodes = nodes
		} else {
			r.services[svcName] = &ServiceSet{nodes: nodes}
		}
	}

	return nil
}

// ScanService performs a targeted scan of a single service directory
func (r *Registry) ScanService(serviceName string) error {
	serviceDir := filepath.Join(r.baseDir, serviceName)
	var discoveredNodes []string

	err := filepath.WalkDir(serviceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".sock") {
			discoveredNodes = append(discoveredNodes, path)
		}
		return nil
	})

	if err != nil && !os.IsNotExist(err) {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// 1. Identify nodes to remove
	currentSet, exists := r.services[serviceName]
	if exists {
		for _, oldNode := range currentSet.nodes {
			if !slices.Contains(discoveredNodes, oldNode) {
				r.removeNodeUnsafe(oldNode, false)
			}
		}
	}

	// 2. Identify and add new nodes
	if len(discoveredNodes) == 0 {
		delete(r.services, serviceName)
		return nil
	}

	for _, newNode := range discoveredNodes {
		if _, exists := r.nodeMap[newNode]; !exists {
			r.nodeMap[newNode] = &nodeContext{
				serviceName: serviceName,
				forwarder:   forwarder.New(newNode, r.failureChan),
			}
		}
	}

	// 3. Update service set
	if !exists {
		r.services[serviceName] = &ServiceSet{nodes: discoveredNodes}
	} else {
		r.services[serviceName].nodes = discoveredNodes
	}

	return nil
}

func (r *Registry) RemoveNode(nodePath string, shouldDeleteFile bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.removeNodeUnsafe(nodePath, shouldDeleteFile)
}

func (r *Registry) removeNodeUnsafe(nodePath string, shouldDeleteFile bool) {
	ctx, ok := r.nodeMap[nodePath]
	if !ok {
		return
	}

	serviceName := ctx.serviceName
	delete(r.nodeMap, nodePath)

	if ss, ok := r.services[serviceName]; ok {
		for i, n := range ss.nodes {
			if n == nodePath {
				ss.nodes = slices.Delete(ss.nodes, i, i+1)
				break
			}
		}
		if len(ss.nodes) == 0 {
			delete(r.services, serviceName)
		}
	}

	if shouldDeleteFile {
		go os.Remove(nodePath)
	}
}
