package watcher

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"nautilus/internal/core/registry"

	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	registry *registry.Registry

	dirtyServices map[string]struct{}
	dirtyMu       sync.Mutex

	eventSignal chan struct{}
	cancel      context.CancelFunc
	fsWatcher   *fsnotify.Watcher
}

func NewWatcher(r *registry.Registry) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	watcher := &Watcher{
		registry:      r,
		dirtyServices: make(map[string]struct{}),
		eventSignal:   make(chan struct{}, 1),
		fsWatcher:     fw,
	}

	return watcher, nil
}

func (w *Watcher) addRecursive(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			log.Printf("Watching directory: %s", path)
			return w.fsWatcher.Add(path)
		}
		return nil
	})
}

func (w *Watcher) Start() error {
	// Initial Scan
	if err := w.registry.Scan(""); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel

	go w.listenEvents(ctx)
	go w.runWorkerLoop(ctx)

	root := w.registry.BaseDir()
	if err := w.addRecursive(root); err != nil {
		return err
	}

	return nil
}

func (w *Watcher) listenEvents(ctx context.Context) {
	baseDir := w.registry.BaseDir()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-w.fsWatcher.Events:
			if !ok {
				return
			}

			if event.Has(fsnotify.Create) {
				info, err := os.Stat(event.Name)
				if err == nil && info.IsDir() {
					w.addRecursive(event.Name)
				}
			}
			switch event.Op {
			case fsnotify.Create, fsnotify.Write, fsnotify.Remove, fsnotify.Rename:
				relPath, err := filepath.Rel(baseDir, event.Name)
				if err != nil {
					log.Printf("[ERR] failed to get relative path: %v", err)
					continue
				}

				serviceName := filepath.Dir(relPath)
				if serviceName != "" && serviceName != "." {
					w.dirtyMu.Lock()
					w.dirtyServices[serviceName] = struct{}{}
					w.dirtyMu.Unlock()

					select {
					case w.eventSignal <- struct{}{}:
					default:
					}
				}
			}

		case err, ok := <-w.fsWatcher.Errors:
			if !ok {
				return
			}
			log.Println("fsnotify error:", err)
		}
	}
}

func (w *Watcher) runWorkerLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	var tickerChan <-chan time.Time

	for {
		select {
		case <-ctx.Done():
			if ticker != nil {
				ticker.Stop()
			}
			return
		case <-w.eventSignal:
			w.dirtyMu.Lock()
			toScan := make(map[string]struct{})
			for svc := range w.dirtyServices {
				toScan[svc] = struct{}{}
			}
			w.dirtyServices = make(map[string]struct{})
			w.dirtyMu.Unlock()

			for svcName := range toScan {
				w.registry.Scan(svcName)
				log.Printf("Targeted scan completed for: %s", svcName)
			}
		case <-tickerChan:
			if err := w.registry.Scan(""); err != nil {
				log.Println("[ERR] error scanning registry", err)
			}
		}
	}
}

func (w *Watcher) Close() error {
	if w.cancel != nil {
		w.cancel()
	}
	if w.fsWatcher != nil {
		w.fsWatcher.Close()
	}
	return nil
}
