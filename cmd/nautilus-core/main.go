package main

import (
	"flag"
	"fmt"
	"log"
	"nautilus/internal/core/configwatcher"
	"nautilus/internal/core/proxy"
	"nautilus/internal/core/registry"
	"nautilus/internal/core/watcher"
	"os"
	"path/filepath"
	"strconv"

	"github.com/lucap9056/go-lifecycle/lifecycle"
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func main() {
	// 1. Define and parse flags
	configFlag := flag.String("config", getEnv("NAUTILUS_CONFIG", "nautilus.ntl"), "Path to config file (.ntl or Ntlfile)")
	ntlcFlag := flag.String("ntlc", getEnv("NAUTILUS_NTLC", "ntlc"), "Path to ntlc executable")
	servicesFlag := flag.String("services", getEnv("NAUTILUS_SERVICES_DIR", "/var/run/nautilus/services"), "Path to services directory")
	entrypointDirFlag := flag.String("entrypoint-dir", getEnv("NAUTILUS_ENTRYPOINT_DIR", "/var/run/nautilus/entrypoints"), "Path to entrypoint directory")
	entrypointCountFlag := flag.String("entrypoint-count", getEnv("NAUTILUS_ENTRYPOINT_COUNT", "1"), "")

	flag.Parse()

	configPath := *configFlag
	ntlcPath := *ntlcFlag
	servicesDir := *servicesFlag
	entrypointDir := *entrypointDirFlag
	entrypointCount, err := strconv.Atoi(*entrypointCountFlag)
	if err != nil {
		log.Fatalf("Invalid entrypoint count: %v", err)
	}

	if err := os.MkdirAll(entrypointDir, 0777); err != nil {
		log.Fatalf("Failed to create entrypoint directory: %v", err)
	}
	// Ensure permissions if directory already existed
	os.Chmod(entrypointDir, 0777)

	// Create services directory if it doesn't exist
	if err := os.MkdirAll(servicesDir, 0777); err != nil {
		log.Fatalf("Failed to create services directory: %v", err)
	}
	os.Chmod(servicesDir, 0777)

	lc := lifecycle.New()

	// 3. Initialize Registry and Watcher for dynamic node discovery
	reg, err := registry.NewRegistry(servicesDir)
	if err != nil {
		log.Fatalf("Failed to initialize registry: %v", err)
	}

	manager := proxy.NewManager(reg)

	// 2. Initialize Config Watcher (Handles load & hot-reload)
	cw, err := configwatcher.NewConfigWatcher(configPath, ntlcPath, manager)
	if err != nil {
		log.Fatalf("Failed to initialize config watcher: %v", err)
	}
	if err := cw.LoadInitial(); err != nil {
		log.Fatalf("Failed to load initial route table: %v", err)
	}

	w, err := watcher.NewWatcher(reg)
	if err != nil {
		log.Fatalf("Failed to create watcher: %v", err)
	}

	// 4. Register Cleanup Hook
	lc.OnExit(func() {
		w.Close()
		cw.Close()
	})

	// 5. Start All Watchers and Listener
	if err := cw.Start(); err != nil {
		log.Printf("Warning: Config watcher failed to start: %v", err)
	}

	if err := w.Start(); err != nil {
		log.Fatalf("Failed to start watcher: %v", err)
	}

	socketPaths := make([]string, entrypointCount)
	for i := range entrypointCount {
		socketName := fmt.Sprintf("nautilus-%d.sock", i)
		socketPaths[i] = filepath.Join(entrypointDir, socketName)
	}

	for _, socketPath := range socketPaths {
		if _, err := os.Stat(socketPath); err == nil {
			if err := os.Remove(socketPath); err != nil {
				log.Printf("Warning: Failed to remove existing socket: %v", err)
			}
		}

		go func(s string) {
			if err := manager.StartUDSListener(s); err != nil {
				log.Printf("Critical: Listener failed: %v", err)
				lc.Exit()
			}
		}(socketPath)

	}

	lc.OnExit(func() {
		log.Println("Nautilus Core shutting down...")
		for _, socketPath := range socketPaths {
			log.Printf("Removing socket: %s", socketPath)
			if _, err := os.Stat(socketPath); err == nil {
				os.Remove(socketPath)
			}
		}
	})

	lc.Wait()
}
