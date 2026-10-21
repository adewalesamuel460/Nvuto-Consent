package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
)

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "/"},
		{"/healthz", "/healthz"},
		{"/readyz", "/readyz"},
		{"/metrics", "/metrics"},
		{"/v1/consents/con-123", "/v1/consents/:id"},
		{"/v1/consents/con-123/withdraw", "/v1/consents/:id"},
		{"/v1/receipts/rec-456", "/v1/receipts/:id"},
		{"/v1/purposes", "/v1/purposes"},
		{"/v1/purposes/", "/v1/purposes"},
		{"/v1/evaluate", "/v1/evaluate"},
		{"/v1/evaluate:batch", "/v1/evaluate:batch"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := NormalizePath(tc.input)
			if got != tc.expected {
				t.Fatalf("NormalizePath(%q) = %q, expected %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestTelemetryMetricsRecordingAndPrivacyConstraints(t *testing.T) {
	m := NewMetrics()

	// 1. HTTP Request recording
	m.RecordHTTPRequest("/v1/consents/con-abc", http.MethodPost, http.StatusOK, "tenant-1", 15*time.Millisecond)

	// 2. Evaluation decisions
	m.RecordEvaluationDecision("tenant-1", "marketing", "email", true, "opt_in_granted")
	m.RecordEvaluationDecision("tenant-1", "telemetry", "sms", false, "opt_out")

	// 3. Auth rejections
	m.RecordAuthFailure("tenant-1", "unauthenticated")
	m.RecordAuthFailure("tenant-1", "role_forbidden")
	m.RecordAuthFailure("tenant-1", "subject_forbidden")

	// 4. JWKS refresh
	m.RecordJWKSRefresh("success")
	m.RecordJWKSRefresh("throttled")

	// 5. DB pool saturation
	m.UpdateDBPool(5, 10, 15, 25)

	// 6. Outbox worker metrics
	m.RecordOutboxPublishAttempt("tenant-1", "consent.granted")
	m.RecordOutboxPublishSuccess("tenant-1", "consent.granted")
	m.RecordOutboxPublishFailure("tenant-1", "consent.granted", "broker_timeout")
	m.SetOutboxLag("tenant-1", 42.5)
	m.RecordOutboxQuarantined("tenant-1", "consent.granted")
	m.SetOutboxPendingCount("tenant-1", 3)

	// Verify all metrics in registry adhere to privacy constraints:
	// No metric label should ever be named "subject_id", "token", or "secret"
	metricFamilies, err := m.Registry.Gather()
	if err != nil {
		t.Fatalf("Gather failed: %v", err)
	}

	if len(metricFamilies) == 0 {
		t.Fatal("expected registered metric families")
	}

	for _, mf := range metricFamilies {
		for _, metric := range mf.Metric {
			for _, label := range metric.Label {
				name := strings.ToLower(label.GetName())
				if name == "subject_id" || name == "subject" || strings.Contains(name, "secret") || strings.Contains(name, "token") {
					t.Fatalf("Privacy violation! Prohibited label name found in metric %s: %s", mf.GetName(), label.GetName())
				}
				val := label.GetValue()
				if strings.Contains(val, "Bearer ") || strings.Contains(val, "secret") {
					t.Fatalf("Privacy violation! Prohibited value found in metric %s label %s: %s", mf.GetName(), label.GetName(), val)
				}
			}
		}
	}
}

func TestHTTPMiddlewareIntegration(t *testing.T) {
	m := NewMetrics()

	handler := m.HTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"accepted"}`))
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/consents/con-test-999", nil)
	req.Header.Set("X-Tenant-ID", "tenant-test")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status 202 Accepted, got %d", rr.Code)
	}

	metricFamilies, err := m.Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}

	foundReqTotal := false
	for _, mf := range metricFamilies {
		if mf.GetName() == "nvuto_consent_http_requests_total" {
			foundReqTotal = true
			if len(mf.Metric) != 1 {
				t.Fatalf("expected 1 metric entry, got %d", len(mf.Metric))
			}
			labels := map[string]string{}
			for _, l := range mf.Metric[0].Label {
				labels[l.GetName()] = l.GetValue()
			}
			if labels["endpoint"] != "/v1/consents/:id" {
				t.Fatalf("expected normalized endpoint /v1/consents/:id, got %q", labels["endpoint"])
			}
			if labels["status"] != "202" {
				t.Fatalf("expected status 202, got %q", labels["status"])
			}
			if labels["tenant_id"] != "tenant-test" {
				t.Fatalf("expected tenant_id tenant-test, got %q", labels["tenant_id"])
			}
		}
	}

	if !foundReqTotal {
		t.Fatal("nvuto_consent_http_requests_total metric family was not found in registry")
	}
}

func TestTenantContextHelpers(t *testing.T) {
	ctx := context.Background()
	if got := TenantFromContext(ctx); got != "" {
		t.Fatalf("expected empty string for fresh context, got %q", got)
	}

	ctxWithTenant := WithTenantID(ctx, "tenant-xyz")
	if got := TenantFromContext(ctxWithTenant); got != "tenant-xyz" {
		t.Fatalf("expected tenant-xyz, got %q", got)
	}
}

func TestMetricsHandlerServesOpenMetrics(t *testing.T) {
	m := NewMetrics()
	m.RecordJWKSRefresh("success")

	srv := m.Handler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /metrics, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "nvuto_consent_auth_jwks_refresh_total") {
		t.Fatalf("expected metrics output to include jwks refresh metric, got: %s", body)
	}
}

// Compile check for prometheus metric model
var _ = (*dto.MetricFamily)(nil)
