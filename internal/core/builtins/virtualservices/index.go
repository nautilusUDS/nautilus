package virtualservices

import (
	"fmt"
	"nautilus/internal/core/builtins"
	"nautilus/internal/core/metrics"
	"net"
	"net/http"
	"strings"
	"time"
)

// --- Internal Virtual Services ---

func Echo(args ...string) http.HandlerFunc {
	msg := "Nautilus Echo"
	if len(args) > 0 && args[0] != "" {
		msg = args[0]
	}
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "%s\n", msg)
		fmt.Fprintf(w, "Method: %s\n", r.Method)
		fmt.Fprintf(w, "Path: %s\n", r.URL.Path)
		fmt.Fprintf(w, "Host: %s\n", r.Host)
		fmt.Fprintf(w, "RemoteAddr: %s\n", r.RemoteAddr)
		fmt.Fprintf(w, "---\nHeaders:\n")
		for k, v := range r.Header {
			fmt.Fprintf(w, "%s: %v\n", k, v)
		}
	}
}

func OK(args ...string) http.HandlerFunc {
	msg := "OK"
	if len(args) > 0 && args[0] != "" {
		msg = args[0]
	}
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(msg))
	}
}

func ERR(args ...string) http.HandlerFunc {
	msg := "ERR"
	if len(args) > 0 && args[0] != "" {
		msg = args[0]
	}
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(msg))
	}
}

func Metrics(args ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		metrics.Global.WritePrometheus(w)
	}
}

func Discovery(state map[string][]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, "{\n")
		i := 0
		for svc, nodes := range state {
			fmt.Fprintf(w, "  \"%s\": [\n", svc)
			for j, node := range nodes {
				fmt.Fprintf(w, "    \"%s\"", node)
				if j < len(nodes)-1 {
					fmt.Fprintf(w, ",")
				}
				fmt.Fprintf(w, "\n")
			}
			fmt.Fprintf(w, "  ]")
			if i < len(state)-1 {
				fmt.Fprintf(w, ",")
			}
			fmt.Fprintf(w, "\n")
			i++
		}
		fmt.Fprintf(w, "}\n")
	}
}

func JSON(args ...string) http.HandlerFunc {
	data := "{}"
	if len(args) > 0 {
		data = args[0]
	}
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(data))
	}
}

func Ping(targetService string, nodes []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if len(nodes) == 0 {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, "{\"service\": \"%s\", \"status\": \"unreachable\", \"reason\": \"no nodes discovered\"}\n", targetService)
			return
		}

		// layer 4 connectivity check
		start := time.Now()
		conn, err := net.DialTimeout("unix", nodes[0], 1*time.Second)
		duration := time.Since(start)

		if err != nil {
			w.WriteHeader(http.StatusGatewayTimeout)
			fmt.Fprintf(w, "{\"service\": \"%s\", \"status\": \"down\", \"node\": \"%s\", \"error\": \"%s\"}\n", targetService, nodes[0], err)
			return
		}
		conn.Close()

		fmt.Fprintf(w, "{\"service\": \"%s\", \"status\": \"up\", \"node\": \"%s\", \"latency_ms\": %d}\n", targetService, nodes[0], duration.Milliseconds())
	}
}

// Registry maps virtual service names (with $) to their factories.
var Registry = map[string]builtins.Factory{
	"$echo":     Echo,
	"$ok":       OK,
	"$err":      ERR,
	"$health":   OK,
	"$metrics":  Metrics,
	"$services": nil, // Runtime resolution
	"$json":     JSON,
	"$ping":     nil, // Runtime resolution
}

// IsValid checks if a virtual service expression is valid.
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
