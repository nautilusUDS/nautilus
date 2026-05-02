package forwarder_test

import (
	"context"
	"io"
	"nautilus/internal/core/builtins/builtinsmware"
	"nautilus/internal/core/forwarder"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForwarder_Forward(t *testing.T) {
	// 1. Setup a mock UDS Server
	tmpDir, err := os.MkdirTemp("", "nautilus-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	socketPath := filepath.Join(tmpDir, "test.sock")

	l, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	defer l.Close()

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "example.com", r.Header.Get("X-Forwarded-Host"))
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Hello from UDS"))
		}),
	}
	go server.Serve(l)
	defer server.Shutdown(context.Background())

	// 2. Use Forwarder to send request to UDS
	f := forwarder.NewForwarder(nil)

	req := httptest.NewRequest("GET", "http://example.com/api/test", nil)
	w := httptest.NewRecorder()

	f.Forward(w, req, "test-service", socketPath)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "Hello from UDS", string(body))
}

func TestForwarder_ForwardMiddleware(t *testing.T) {
	// 1. Setup a mock UDS Middleware Server
	tmpDir, err := os.MkdirTemp("", "nautilus-mw-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	socketPath := filepath.Join(tmpDir, "mw.sock")

	l, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	defer l.Close()

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Auth") == "valid" {
				w.Header().Set("X-User-ID", "123")
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusUnauthorized)
			}
		}),
	}
	go server.Serve(l)
	defer server.Shutdown(context.Background())

	f := forwarder.NewForwarder(nil)

	t.Run("Authorized", func(t *testing.T) {
		req := httptest.NewRequest("GET", "http://example.com/", nil)
		req.Header.Set("X-Auth", "valid")
		w := builtinsmware.NewResponseWriter()

		ok := f.ForwardMiddleware(w, req, "/", socketPath)
		assert.True(t, ok)
		assert.Equal(t, "123", req.Header.Get("X-User-ID"))
	})

	t.Run("Unauthorized", func(t *testing.T) {
		req := httptest.NewRequest("GET", "http://example.com/", nil)
		req.Header.Set("X-Auth", "invalid")
		w := builtinsmware.NewResponseWriter()

		ok := f.ForwardMiddleware(w, req, "/", socketPath)
		assert.False(t, ok)
		assert.Equal(t, http.StatusUnauthorized, w.GetCode())
	})
}
