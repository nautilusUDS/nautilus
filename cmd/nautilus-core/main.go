package main

import (
	"flag"
	"fmt"
	"nautilus/internal/core/configwatcher"
	"nautilus/internal/core/logs"
	"nautilus/internal/core/proxy"
	"nautilus/internal/core/registry"
	"nautilus/internal/core/watcher"
	"os"
	"path/filepath"
	"strconv"

	"github.com/lucap9056/go-lifecycle/lifecycle"
	"go.uber.org/zap"
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
	entrypointCountFlag := flag.String("entrypoint-count", getEnv("NAUTILUS_ENTRYPOINT_COUNT", "1"), "Number of entrypoint instances to spawn")
	logLevelFlag := flag.String("log-level", getEnv("NAUTILUS_LOG_LEVEL", "info"), "Log level (debug, info, warn, error, dpanic, panic, fatal)")

	flag.Parse()

	logs.InitLogger(*logLevelFlag)
	defer logs.Sync()

	configPath := *configFlag
	ntlcPath := *ntlcFlag
	servicesDir := *servicesFlag
	entrypointDir := *entrypointDirFlag
	entrypointCount, err := strconv.Atoi(*entrypointCountFlag)
	if err != nil {
		logs.Out.Error("Invalid entrypoint count", zap.Error(err))
		entrypointCount = 1
	}

	// Create entrypoint directory if it doesn't exist
	if err := os.MkdirAll(entrypointDir, 0755); err != nil {
		logs.Out.Error("Failed to create entrypoint directory", zap.Error(err))
		return
	}
	// Ensure permissions if directory already existed
	os.Chmod(entrypointDir, 0755)

	// Create services directory if it doesn't exist
	// 01777: Sticky bit ensures only the owner can delete their own socket
	if err := os.MkdirAll(servicesDir, 01777); err != nil {
		logs.Out.Error("Failed to create services directory", zap.Error(err))
		return
	}
	os.Chmod(servicesDir, 01777)

	lc := lifecycle.New()

	// 2. Initialize Registry and Watcher for dynamic node discovery
	reg, err := registry.NewRegistry(servicesDir)
	if err != nil {
		logs.Out.Error("Failed to initialize registry", zap.Error(err))
		return
	}

	manager := proxy.NewManager(reg)

	// 3. Initialize Config Watcher (Handles load & hot-reload)
	cw, err := configwatcher.NewConfigWatcher(configPath, ntlcPath, manager)
	if err != nil {
		logs.Out.Error("Failed to initialize config watcher", zap.Error(err))
		return
	}
	if err := cw.LoadInitial(); err != nil {
		logs.Out.Error("Failed to load initial route table", zap.Error(err))
		return
	}

	w, err := watcher.NewWatcher(reg)
	if err != nil {
		logs.Out.Error("Failed to initialize watcher", zap.Error(err))
		return
	}

	// 4. Register Cleanup Hook
	lc.OnExit(func() {
		w.Close()
		cw.Close()
	})

	// 5. Start All Watchers and Listener
	if err := cw.Start(); err != nil {
		logs.Out.Error("Failed to start config watcher", zap.Error(err))
		return
	}

	if err := w.Start(); err != nil {
		logs.Out.Error("Failed to start watcher", zap.Error(err))
		return
	}

	socketPaths := make([]string, entrypointCount)
	for i := range entrypointCount {
		socketName := fmt.Sprintf("nautilus-%d.sock", i)
		socketPaths[i] = filepath.Join(entrypointDir, socketName)
	}

	for _, socketPath := range socketPaths {
		if _, err := os.Stat(socketPath); err == nil {
			if err := os.Remove(socketPath); err != nil {
				logs.Out.Error("Failed to remove existing socket", zap.Error(err))
			}
		}

		go func(s string) {
			if err := manager.StartUDSListener(s); err != nil {
				logs.Out.Error("Failed to start listener", zap.Error(err))
				lc.Exit()
			}
		}(socketPath)

	}

	lc.OnExit(func() {
		logs.Out.Info("Shutting down Nautilus Core...")
		for _, socketPath := range socketPaths {
			logs.Out.Info("Removing socket", zap.String("socketPath", socketPath))
			if _, err := os.Stat(socketPath); err == nil {
				os.Remove(socketPath)
			}
		}
	})

	lc.Wait()
}
