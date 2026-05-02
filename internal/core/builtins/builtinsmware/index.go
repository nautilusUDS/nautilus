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

		if after, ok := strings.CutPrefix(r.URL.Path, prefix); ok {
			r.URL.Path = after

			if r.URL.RawPath != "" {
				r.URL.RawPath = strings.TrimPrefix(r.URL.RawPath, url.PathEscape(prefix))
			}
			r.RequestURI = r.URL.RequestURI()
		}
	}
}

func RewritePath(args ...string) http.HandlerFunc {
	old, new := parseTwoArgs(args)
	return func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.ReplaceAll(r.URL.Path, old, new)

		if r.URL.RawPath != "" {
			r.URL.RawPath = strings.ReplaceAll(r.URL.RawPath, old, new)
		}
		r.RequestURI = r.URL.RequestURI()
	}
}

func SetQuery(args ...string) http.HandlerFunc {
	key, val := parseTwoArgs(args)
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		q.Set(key, val)
		r.URL.RawQuery = q.Encode()
		r.RequestURI = r.URL.RequestURI()
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

// --- Debugging & Utilities ---

func Log(args ...string) http.HandlerFunc {
	prefix, _ := parseTwoArgs(args)
	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("[%s] %s %s from %s\n", prefix, r.Method, r.URL.Path, r.RemoteAddr)
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
