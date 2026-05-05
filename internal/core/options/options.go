package options

import (
	"flag"
	"os"
	"regexp"
	"strconv"
)

type Options struct {
	ConfigPath      string
	NtlcPath        string
	ServicesDir     string
	EntrypointDir   string
	EntrypointCount int
	LogLevel        string
	Token           string
	ForceClean      bool
}

func Load() *Options {
	configPtr := EnvString("config", "NAUTILUS_CONFIG", "nautilus.ntl", "Path to config file (.ntl or Ntlfile)")
	ntlcPtr := EnvString("ntlc", "NAUTILUS_NTLC", "ntlc", "Path to ntlc executable")
	servicesDirPtr := EnvString("services", "NAUTILUS_SERVICES_DIR", "/var/run/nautilus/services", "Path to services directory")
	entrypointDirPtr := EnvString("entrypoint-dir", "NAUTILUS_ENTRYPOINT_DIR", "/var/run/nautilus/entrypoints", "Path to entrypoint directory")
	entrypointCountPtr := EnvString("entrypoint-count", "NAUTILUS_ENTRYPOINT_COUNT", "1", "Number of entrypoint instances to spawn")
	logLevelPtr := EnvString("log-level", "NAUTILUS_LOG_LEVEL", "info", "Log level (debug, info, warn, error, dpanic, panic, fatal)")
	tokenPtr := EnvString("token", "NAUTILUS_TOKEN", "", "")
	forceCleanPtr := EnvString("force-clean", "NAUTILUS_FORCE_CLEAN", "false", "Force clean")

	flag.Parse()

	entrypointCount, err := strconv.Atoi(*entrypointCountPtr)
	if err != nil {
		entrypointCount = 1
	}
	reg := regexp.MustCompile(`[^a-zA-Z0-9._-]`)

	return &Options{
		ConfigPath:      *configPtr,
		NtlcPath:        *ntlcPtr,
		ServicesDir:     *servicesDirPtr,
		EntrypointDir:   *entrypointDirPtr,
		EntrypointCount: entrypointCount,
		LogLevel:        *logLevelPtr,
		Token:           reg.ReplaceAllString(*tokenPtr, "_"),
		ForceClean:      (*forceCleanPtr) == "true",
	}
}

func EnvString(name, envKey, fallback, usage string) *string {
	return flag.String(name, getEnv(envKey, fallback), usage)
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
