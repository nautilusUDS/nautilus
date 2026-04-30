package builtinsmware

import (
	"fmt"
	"log"
	"nautilus/internal/core/builtins"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// --- Header Operations ---

func SetHeader(args ...string) http.HandlerFunc {
	key, val := parseTwoArgs(args)
	return func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set(key, val)
	}
}

func DelHeader(args ...string) http.HandlerFunc {
	key, _ := parseTwoArgs(args)
	return func(w http.ResponseWriter, r *http.Request) {
		r.Header.Del(key)
	}
}

func SetHost(args ...string) http.HandlerFunc {
	host, _ := parseTwoArgs(args)
	return func(w http.ResponseWriter, r *http.Request) {
		r.Host = host
	}
}

// --- Path & Query Operations ---

func PathTrimPrefix(args ...string) http.HandlerFunc {
	prefix, _ := parseTwoArgs(args)
	return func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, prefix)
		if r.URL.RawPath != "" {
			r.URL.RawPath = strings.TrimPrefix(r.URL.RawPath, prefix)
		}
	}
}

func RewritePath(args ...string) http.HandlerFunc {
	old, new := parseTwoArgs(args)
	return func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.ReplaceAll(r.URL.Path, old, new)
	}
}

func SetQuery(args ...string) http.HandlerFunc {
	key, val := parseTwoArgs(args)
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		q.Set(key, val)
		r.URL.RawQuery = q.Encode()
	}
}

// --- Security & Auth ---

func BasicAuth(args ...string) http.HandlerFunc {
	user, pass := parseTwoArgs(args)
	return func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != user || p != pass {
			w.Header().Set("WWW-Authenticate", `Basic realm="Nautilus Protected"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		}
	}
}

func IPAllow(args ...string) http.HandlerFunc {
	if len(args) != 1 {
		log.Printf("IPAllow error: expected 1 argument")
		return func(w http.ResponseWriter, r *http.Request) {}
	}
	_, ipNet, err := net.ParseCIDR(args[0])
	if err != nil {
		log.Printf("IPAllow error: invalid CIDR %s", args)
		return func(w http.ResponseWriter, r *http.Request) {}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		ipStr, _, _ := net.SplitHostPort(r.RemoteAddr)
		ip := net.ParseIP(ipStr)
		if !ipNet.Contains(ip) {
			http.Error(w, "Forbidden: IP not allowed", http.StatusForbidden)
		}
	}
}

// --- Security & Limits ---

func MaxBodySize(args ...string) http.HandlerFunc {
	sizeStr, _ := parseTwoArgs(args)
	size, err := strconv.ParseInt(sizeStr, 10, 64)
	if err != nil {
		log.Printf("MaxBodySize error: invalid size %s", args)
		return func(w http.ResponseWriter, r *http.Request) {}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, size)
	}
}

// --- Debugging & Utilities ---

func Log(args ...string) http.HandlerFunc {
	prefix, _ := parseTwoArgs(args)
	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("[%s] %s %s from %s\n", prefix, r.Method, r.URL.Path, r.RemoteAddr)
	}
}

// --- Redirect ---

func Redirect(args ...string) http.HandlerFunc {
	if len(args) < 2 {
		return func(w http.ResponseWriter, r *http.Request) {}
	}
	code, _ := strconv.Atoi(args[0])
	_, err := url.Parse(args[1])
	if err != nil {
		return func(w http.ResponseWriter, r *http.Request) {}
	}
	target := args[1]
	return func(w http.ResponseWriter, r *http.Request) {
		newURL, _ := url.Parse(target)
		newURL.RawQuery = r.URL.RawQuery
		target = newURL.String()

		http.Redirect(w, r, target, code)
	}
}

// --- RateLimit ---

// --- Rate Limiting ---

func RateLimit(args ...string) http.HandlerFunc {
	if len(args) < 2 {
		log.Printf("RateLimit error: expected (count, seconds)")
		return func(w http.ResponseWriter, r *http.Request) {}
	}

	limit, _ := strconv.Atoi(args[0])
	seconds, _ := strconv.Atoi(args[1])
	duration := time.Duration(seconds) * time.Second

	var mu sync.Mutex
	clients := make(map[string][]time.Time)

	return func(w http.ResponseWriter, r *http.Request) {
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)

		mu.Lock()
		defer mu.Unlock()

		now := time.Now()
		var valid []time.Time
		for _, t := range clients[ip] {
			if now.Sub(t) < duration {
				valid = append(valid, t)
			}
		}

		if len(valid) >= limit {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		clients[ip] = append(valid, now)
	}
}

// --- Registry ---

var Registry = map[string]builtins.Factory{
	"$SetHeader":      SetHeader,
	"$DelHeader":      DelHeader,
	"$SetHost":        SetHost,
	"$PathTrimPrefix": PathTrimPrefix,
	"$RewritePath":    RewritePath,
	"$SetQuery":       SetQuery,
	"$BasicAuth":      BasicAuth,
	"$IPAllow":        IPAllow,
	"$MaxBodySize":    MaxBodySize,
	"$Log":            Log,
	"$Redirect":       Redirect,
	"$RateLimit":      RateLimit,
}

// IsValid checks if an expression looks like a builtin and if it exists in the registry.
func IsValid(expr string) (bool, string) {
	if !strings.HasPrefix(expr, "$") {
		return false, ""
	}

	funcName := expr
	start := strings.Index(expr, "(")
	if start != -1 {
		funcName = expr[:start]
	}

	_, ok := Registry[funcName]
	if !ok {
		return false, funcName
	}

	if start != -1 {
		end := strings.LastIndex(expr, ")")
		if end == -1 || end < start {
			return false, ""
		}
	}

	return true, ""
}

func parseTwoArgs(args []string) (string, string) {
	switch len(args) {
	case 1:
		return args[0], ""
	case 2:
		return args[0], args[1]
	default:
		return "", ""
	}
}
