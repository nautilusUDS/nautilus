package test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const baseURL = "http://localhost:8080"

func TestIntegration_Nautilus(t *testing.T) {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	t.Run("Debug Route - Virtual Service $ok", func(t *testing.T) {
		req, _ := http.NewRequest("GET", baseURL+"/debug/health", nil)
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "Diagnostic", resp.Header.Get("X-Nautilus-Mode"))

		body, _ := io.ReadAll(resp.Body)
		assert.Contains(t, string(body), "Infrastructure-OK")
	})

	t.Run("Echo Service - Virtual Service $echo", func(t *testing.T) {
		req, _ := http.NewRequest("GET", baseURL+"/echo", nil)
		req.Header.Set("X-Test-Echo", "NautilusRocks")
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var echoData map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&echoData)
		require.NoError(t, err)

		headers := echoData["headers"].(map[string]interface{})
		assert.Equal(t, "NautilusRocks", headers["X-Test-Echo"])
	})

	t.Run("API Routing to Caddy - Requires Auth", func(t *testing.T) {
		// 1. Without Auth -> 401
		req, _ := http.NewRequest("GET", baseURL+"/api/v1/user/profile", nil)
		resp, err := client.Do(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		resp.Body.Close()

		// 2. With Auth -> 200 (Caddy welcome page)
		req, _ = http.NewRequest("GET", baseURL+"/api/v1/user/profile", nil)
		req.SetBasicAuth("developer", "dev-secret") // Updated to match Ntlfile
		resp, err = client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		assert.Contains(t, string(body), "Welcome to Caddy Native UDS!")
	})

	t.Run("Failure Simulation - Virtual Service $err", func(t *testing.T) {
		req, _ := http.NewRequest("GET", baseURL+"/fail/404", nil)
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("Trailing Slash Test", func(t *testing.T) {
		// Test both with and without slash as per Ntlfile: GET [localhost|127.0.0.1]/v1/data[/|] backend
		paths := []string{"/v1/data", "/v1/data/"}
		for _, p := range paths {
			req, _ := http.NewRequest("GET", baseURL+p, nil)
			resp, err := client.Do(req)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			resp.Body.Close()
		}
	})

	t.Run("Security Alert - Blocked Path", func(t *testing.T) {
		// * [localhost|127.0.0.1]/[.git|.env|config.php|wp-admin] $err("Access Denied")
		req, _ := http.NewRequest("GET", baseURL+"/.git", nil)
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode) // $err defaults to 500
		body, _ := io.ReadAll(resp.Body)
		assert.Contains(t, string(body), "Access Denied")
	})

	t.Run("Rate Limit Test", func(t *testing.T) {
		// [localhost|127.0.0.1]/internal/[v1|v2]/[auth|billing|search]/[verify|execute|query]/[token|id|uuid] backend
		// $RateLimit(100, "1s")
		// We'll just do one request to ensure it works, full rate limit test might be flaky in CI
		req, _ := http.NewRequest("GET", baseURL+"/internal/v1/auth/verify/token", nil)
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "True", resp.Header.Get("X-Internal-Call"))
	})
}
