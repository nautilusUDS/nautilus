package registry

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

type NodesChangedCallback func(serviceName string, nodes []string)
type NodesChangedState struct {
	ServiceName string
	Nodes       []string
}

type Registry struct {
	mu       sync.RWMutex
	services map[string]*ServiceNodes
	baseDir  string

	unhealthy   map[string]time.Time
	unhealthyMu sync.RWMutex

	subMu       sync.RWMutex
	subscribers []NodesChangedCallback
}

type ServiceNodes struct {
	nodes []string
	index uint32
}

func (s *ServiceNodes) Count() int {
	return len(s.nodes)
}

func NewRegistry(baseDir string) *Registry {
	return &Registry{
		services: make(map[string]*ServiceNodes),
		baseDir:  baseDir,
	}
}

func (r *Registry) BaseDir() string {
	return r.baseDir
}

// GetNodes returns a copy of the current physical nodes for a service
func (r *Registry) GetNodes(serviceName string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	sn, ok := r.services[serviceName]
	if !ok {
		return nil
	}
	return slices.Clone(sn.nodes)
}

// GetState returns a snapshot of all services and their current nodes.
func (r *Registry) GetState() map[string][]string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	state := make(map[string][]string)
	for name, sn := range r.services {
		state[name] = slices.Clone(sn.nodes)
	}
	return state
}

// Scan returns (changed, error)
func (r *Registry) Scan() (bool, error) {
	scannedState := make(map[string][]string)
	var changes []NodesChangedState

	err := filepath.WalkDir(r.baseDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && strings.HasSuffix(d.Name(), ".sock") {
			dir := filepath.Dir(path)
			rel, err := filepath.Rel(r.baseDir, dir)
			if err != nil {
				return nil
			}

			serviceName := filepath.ToSlash(rel)
			if serviceName == "." {
				return nil
			}

			scannedState[serviceName] = append(scannedState[serviceName], path)
		}
		return nil
	})

	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	r.mu.RLock()
	currentServices := make(map[string]*ServiceNodes)
	maps.Copy(currentServices, r.services)
	r.mu.RUnlock()

	// 1. Check for updates and additions
	for name, nodes := range scannedState {
		oldService, exists := currentServices[name]
		slices.Sort(nodes)

		needsUpdate := !exists
		if exists {
			oldNodes := slices.Clone(oldService.nodes)
			slices.Sort(oldNodes)
			if !slices.Equal(nodes, oldNodes) {
				needsUpdate = true
			}
		}

		if needsUpdate {
			changes = append(changes, NodesChangedState{name, nodes})
		}
	}

	// 2. Check for deletions
	for name := range currentServices {
		if _, stillExists := scannedState[name]; !stillExists {
			changes = append(changes, NodesChangedState{name, nil})
		}
	}

	if len(changes) == 0 {
		return false, nil
	}

	r.mu.Lock()
	newInternalState := make(map[string]*ServiceNodes)
	for name, nodes := range scannedState {
		newInternalState[name] = &ServiceNodes{nodes: nodes}
	}
	r.services = newInternalState
	r.mu.Unlock()

	r.subMu.RLock()
	subs := slices.Clone(r.subscribers)
	r.subMu.RUnlock()

	for _, state := range changes {
		for _, sub := range subs {
			sub(state.ServiceName, state.Nodes)
		}
	}

	return true, nil
}

func (r *Registry) Subscribe(callback NodesChangedCallback) {
	r.subMu.Lock()
	defer r.subMu.Unlock()
	r.subscribers = append(r.subscribers, callback)
}

func (r *Registry) OnFailure(serviceName, failedNodePath string, shouldDeleteFile bool) {
	//nodes that don't require file deletion are temporarily kept in the registry.
	if !shouldDeleteFile {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	sn, ok := r.services[serviceName]
	if !ok {
		return
	}

	if shouldDeleteFile {
		os.Remove(failedNodePath)
	}

	for i, node := range sn.nodes {
		if node == failedNodePath {
			sn.nodes = slices.Delete(sn.nodes, i, i+1)
			break
		}
	}

	updatedNodes := slices.Clone(sn.nodes)

	if len(sn.nodes) == 0 {
		delete(r.services, serviceName)
	} else {
		updatedNodes = nil
	}

	r.subMu.RLock()
	subs := slices.Clone(r.subscribers)
	r.subMu.RUnlock()
	for _, sub := range subs {
		sub(serviceName, updatedNodes)
	}

}
