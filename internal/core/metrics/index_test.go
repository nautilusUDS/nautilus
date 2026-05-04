package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetrics(t *testing.T) {
	// Test legacy methods
	Global.IncRequests()
	Global.IncErrors()
	Global.IncUpdates()
	Global.AddActive(5)
	Global.AddActive(-2)

	// Test new metrics
	Global.RequestDuration.WithLabelValues("GET", "/test").Observe(0.1)
	Global.HTTPRequestsTotal.WithLabelValues("service-a", "200", "GET").Inc()

	// Verify Prometheus output
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	Global.WritePrometheus(w, req)

	body := w.Body.String()

	expectedMetrics := []string{
		"nautilus_requests_total 1",
		"nautilus_errors_total 1",
		"nautilus_config_updates_total 1",
		"nautilus_active_requests 3",
		"nautilus_uptime_seconds",
		"nautilus_request_duration_seconds_bucket",
		"nautilus_http_requests_total{method=\"GET\",service=\"service-a\",status=\"200\"} 1",
	}

	for _, em := range expectedMetrics {
		if !strings.Contains(body, em) {
			t.Errorf("Expected metric %q not found in output", em)
		}
	}
}
