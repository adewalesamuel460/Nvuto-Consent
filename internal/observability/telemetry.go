package observability

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics encapsulates all Prometheus metric collectors for API, outbox worker, and database pool.
type Metrics struct {
	Registry *prometheus.Registry

	// API Metrics
	HTTPRequestsTotal          *prometheus.CounterVec
	HTTPRequestDurationSeconds *prometheus.HistogramVec
	EvaluationDecisionsTotal   *prometheus.CounterVec
	AuthFailuresTotal          *prometheus.CounterVec
	JWKSRefreshTotal           *prometheus.CounterVec

	// Database Metrics
	DBPoolConnections *prometheus.GaugeVec

	// Outbox Worker Metrics
	OutboxPublishAttemptsTotal   *prometheus.CounterVec
	OutboxPublishSuccessTotal    *prometheus.CounterVec
	OutboxPublishFailuresTotal   *prometheus.CounterVec
	OutboxLagSeconds             *prometheus.GaugeVec
	OutboxQuarantinedEventsTotal *prometheus.CounterVec
	OutboxPendingEventsCount     *prometheus.GaugeVec
}

// NewMetrics initializes and registers all platform observability metrics.
// All label sets are strictly bounded and avoid high-cardinality or sensitive data.
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		Registry: reg,

		HTTPRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "nvuto_consent",
				Subsystem: "http",
				Name:      "requests_total",
				Help:      "Total count of HTTP requests handled by the consent API.",
			},
			[]string{"endpoint", "method", "status", "tenant_id"},
		),

		HTTPRequestDurationSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "nvuto_consent",
				Subsystem: "http",
				Name:      "request_duration_seconds",
				Help:      "Histogram of HTTP request latencies across endpoints.",
				Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			},
			[]string{"endpoint", "method", "status"},
		),

		EvaluationDecisionsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "nvuto_consent",
				Subsystem: "decision",
				Name:      "evaluations_total",
				Help:      "Total count of consent policy evaluations partitioned by decision and reason.",
			},
			[]string{"tenant_id", "purpose", "channel", "allowed", "reason"},
		),

		AuthFailuresTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "nvuto_consent",
				Subsystem: "auth",
				Name:      "failures_total",
				Help:      "Total count of authentication and authorization rejections.",
			},
			[]string{"tenant_id", "reason"},
		),

		JWKSRefreshTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "nvuto_consent",
				Subsystem: "auth",
				Name:      "jwks_refresh_total",
				Help:      "Total number of OIDC JWKS key set fetches performed.",
			},
			[]string{"status"},
		),

		DBPoolConnections: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "nvuto_consent",
				Subsystem: "db",
				Name:      "pool_connections",
				Help:      "Number of connections in the YSQL/PostgreSQL connection pool by state.",
			},
			[]string{"state"},
		),

		OutboxPublishAttemptsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "nvuto_consent",
				Subsystem: "outbox",
				Name:      "publish_attempts_total",
				Help:      "Total number of outbox publishing attempts to NATS JetStream.",
			},
			[]string{"tenant_id", "event_type"},
		),

		OutboxPublishSuccessTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "nvuto_consent",
				Subsystem: "outbox",
				Name:      "publish_success_total",
				Help:      "Total number of acknowledged outbox event publications.",
			},
			[]string{"tenant_id", "event_type"},
		),

		OutboxPublishFailuresTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "nvuto_consent",
				Subsystem: "outbox",
				Name:      "publish_failures_total",
				Help:      "Total number of outbox publishing failures.",
			},
			[]string{"tenant_id", "event_type", "reason"},
		),

		OutboxLagSeconds: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "nvuto_consent",
				Subsystem: "outbox",
				Name:      "lag_seconds",
				Help:      "Age in seconds of the oldest pending, non-quarantined outbox event.",
			},
			[]string{"tenant_id"},
		),

		OutboxQuarantinedEventsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "nvuto_consent",
				Subsystem: "outbox",
				Name:      "quarantined_events_total",
				Help:      "Total number of poison outbox events routed to quarantine (DLQ) after exceeding retry limits.",
			},
			[]string{"tenant_id", "event_type"},
		),

		OutboxPendingEventsCount: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "nvuto_consent",
				Subsystem: "outbox",
				Name:      "pending_events_count",
				Help:      "Current count of active pending events in the transactional outbox queue.",
			},
			[]string{"tenant_id"},
		),
	}

	reg.MustRegister(
		m.HTTPRequestsTotal,
		m.HTTPRequestDurationSeconds,
		m.EvaluationDecisionsTotal,
		m.AuthFailuresTotal,
		m.JWKSRefreshTotal,
		m.DBPoolConnections,
		m.OutboxPublishAttemptsTotal,
		m.OutboxPublishSuccessTotal,
		m.OutboxPublishFailuresTotal,
		m.OutboxLagSeconds,
		m.OutboxQuarantinedEventsTotal,
		m.OutboxPendingEventsCount,
	)

	return m
}

// DefaultMetrics is the singleton metrics registry for the application.
var DefaultMetrics = NewMetrics()

// Handler returns an HTTP handler serving Prometheus scrape metrics.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{EnableOpenMetrics: true})
}

// Handler returns the HTTP scrape handler for DefaultMetrics.
func Handler() http.Handler {
	return DefaultMetrics.Handler()
}

// RecordHTTPRequest records an HTTP request duration, status, and count.
func (m *Metrics) RecordHTTPRequest(endpoint, method string, status int, tenantID string, duration time.Duration) {
	if tenantID == "" {
		tenantID = "none"
	}
	statusStr := strconv.Itoa(status)
	normEndpoint := NormalizePath(endpoint)
	m.HTTPRequestsTotal.WithLabelValues(normEndpoint, method, statusStr, tenantID).Inc()
	m.HTTPRequestDurationSeconds.WithLabelValues(normEndpoint, method, statusStr).Observe(duration.Seconds())
}

// RecordEvaluationDecision records a consent evaluation outcome without sensitive identifiers.
func (m *Metrics) RecordEvaluationDecision(tenantID, purpose, channel string, allowed bool, reason string) {
	if tenantID == "" {
		tenantID = "none"
	}
	if purpose == "" {
		purpose = "none"
	}
	if channel == "" {
		channel = "none"
	}
	allowedStr := "false"
	if allowed {
		allowedStr = "true"
	}
	if reason == "" {
		reason = "unknown"
	}
	m.EvaluationDecisionsTotal.WithLabelValues(tenantID, purpose, channel, allowedStr, reason).Inc()
}

// RecordAuthFailure increments authentication or authorization failures with bounded reason.
func (m *Metrics) RecordAuthFailure(tenantID, reason string) {
	if tenantID == "" {
		tenantID = "none"
	}
	if reason == "" {
		reason = "unauthorized"
	}
	m.AuthFailuresTotal.WithLabelValues(tenantID, reason).Inc()
}

// RecordJWKSRefresh records an OIDC JWKS retrieval status.
func (m *Metrics) RecordJWKSRefresh(status string) {
	if status == "" {
		status = "unknown"
	}
	m.JWKSRefreshTotal.WithLabelValues(status).Inc()
}

// UpdateDBPool updates database connection pool utilization gauges.
func (m *Metrics) UpdateDBPool(acquired, idle, total, maxConns int32) {
	m.DBPoolConnections.WithLabelValues("acquired").Set(float64(acquired))
	m.DBPoolConnections.WithLabelValues("idle").Set(float64(idle))
	m.DBPoolConnections.WithLabelValues("total").Set(float64(total))
	m.DBPoolConnections.WithLabelValues("max").Set(float64(maxConns))
}

// RecordOutboxPublishAttempt records a broker publish attempt.
func (m *Metrics) RecordOutboxPublishAttempt(tenantID, eventType string) {
	if tenantID == "" {
		tenantID = "none"
	}
	if eventType == "" {
		eventType = "unknown"
	}
	m.OutboxPublishAttemptsTotal.WithLabelValues(tenantID, eventType).Inc()
}

// RecordOutboxPublishSuccess records a successfully acknowledged publish.
func (m *Metrics) RecordOutboxPublishSuccess(tenantID, eventType string) {
	if tenantID == "" {
		tenantID = "none"
	}
	if eventType == "" {
		eventType = "unknown"
	}
	m.OutboxPublishSuccessTotal.WithLabelValues(tenantID, eventType).Inc()
}

// RecordOutboxPublishFailure records a failed publish attempt.
func (m *Metrics) RecordOutboxPublishFailure(tenantID, eventType, reason string) {
	if tenantID == "" {
		tenantID = "none"
	}
	if eventType == "" {
		eventType = "unknown"
	}
	boundedReason := reason
	if len(boundedReason) > 32 {
		boundedReason = boundedReason[:32]
	}
	m.OutboxPublishFailuresTotal.WithLabelValues(tenantID, eventType, boundedReason).Inc()
}

// SetOutboxLag sets the current outbox propagation lag gauge in seconds.
func (m *Metrics) SetOutboxLag(tenantID string, seconds float64) {
	if tenantID == "" {
		tenantID = "global"
	}
	m.OutboxLagSeconds.WithLabelValues(tenantID).Set(seconds)
}

// RecordOutboxQuarantined records an outbox event that exceeded retry limits and was moved to DLQ.
func (m *Metrics) RecordOutboxQuarantined(tenantID, eventType string) {
	if tenantID == "" {
		tenantID = "none"
	}
	if eventType == "" {
		eventType = "unknown"
	}
	m.OutboxQuarantinedEventsTotal.WithLabelValues(tenantID, eventType).Inc()
}

// SetOutboxPendingCount updates the gauge of pending outbox events.
func (m *Metrics) SetOutboxPendingCount(tenantID string, count float64) {
	if tenantID == "" {
		tenantID = "global"
	}
	m.OutboxPendingEventsCount.WithLabelValues(tenantID).Set(count)
}

// NormalizePath collapses parameterized route path parameters to prevent metric label explosions.
func NormalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	switch {
	case strings.HasPrefix(path, "/v1/consents/"):
		return "/v1/consents/:id"
	case strings.HasPrefix(path, "/v1/receipts/"):
		return "/v1/receipts/:id"
	case path == "/healthz" || path == "/readyz" || path == "/metrics":
		return path
	}
	// Strip trailing slashes
	if len(path) > 1 && strings.HasSuffix(path, "/") {
		path = strings.TrimSuffix(path, "/")
	}
	return path
}

type statusResponseWriter struct {
	http.ResponseWriter
	statusCode  int
	wroteHeader bool
}

func (w *statusResponseWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.statusCode = code
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.statusCode = http.StatusOK
		w.wroteHeader = true
	}
	return w.ResponseWriter.Write(b)
}

// HTTPMiddleware wraps an http.Handler with automatic Prometheus latency and request metrics.
func (m *Metrics) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		srw := &statusResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(srw, r)

		duration := time.Since(start)
		tenantID := r.Header.Get("X-Tenant-ID")
		m.RecordHTTPRequest(r.URL.Path, r.Method, srw.statusCode, tenantID, duration)
	})
}

// ContextKey is used for passing tenant ID safely through request contexts.
type ContextKey string

const TenantContextKey ContextKey = "nvuto_tenant_id"

// WithTenantID binds tenant ID to context for observability.
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, TenantContextKey, tenantID)
}

// TenantFromContext extracts tenant ID from context, or empty string.
func TenantFromContext(ctx context.Context) string {
	if val, ok := ctx.Value(TenantContextKey).(string); ok {
		return val
	}
	return ""
}
