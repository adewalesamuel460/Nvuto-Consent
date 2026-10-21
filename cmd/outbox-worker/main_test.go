package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
	"github.com/nats-io/nats.go"
)

func TestEnvelopeIsVersionedCloudEventOnStableSubject(t *testing.T) {
	item := event{ID: "evt-1", SchemaVersion: 1, Source: "nvuto/consent", TenantID: "tenant-a", EventType: "consent.granted", AggregateType: "consent", AggregateID: "con-1", Payload: json.RawMessage(`{"subject_id":"cus-1"}`), OccurredAt: time.Now().UTC()}
	if got := subject("nvuto.consent", item); got != "nvuto.consent.granted" {
		t.Fatalf("unexpected subject %q", got)
	}
	body, err := envelope(item)
	if err != nil {
		t.Fatal(err)
	}
	var output cloudEvent
	if err := json.Unmarshal(body, &output); err != nil {
		t.Fatal(err)
	}
	if output.SpecVersion != "1.0" || output.SchemaVersion != 1 || output.TenantID != "tenant-a" || output.Subject != "consent/con-1" {
		t.Fatalf("incomplete event envelope: %+v", output)
	}
}

func TestVerifyStreamConfig(t *testing.T) {
	const stream = "NVUTO_CONSENT"
	const expectedSubject = "nvuto.consent.>"
	const replicas = 3

	baseConfig := func() nats.StreamConfig {
		return nats.StreamConfig{
			Name:       stream,
			Subjects:   []string{expectedSubject},
			Storage:    nats.FileStorage,
			Retention:  nats.LimitsPolicy,
			Duplicates: jetStreamDuplicateWindow,
			Replicas:   replicas,
		}
	}

	t.Run("matching configuration passes", func(t *testing.T) {
		cfg := baseConfig()
		if err := verifyStreamConfig(cfg, stream, expectedSubject, replicas); err != nil {
			t.Fatalf("expected nil for matching config, got: %v", err)
		}
	})

	t.Run("subject mismatch fails", func(t *testing.T) {
		cfg := baseConfig()
		cfg.Subjects = []string{"other.subject.>"}
		if err := verifyStreamConfig(cfg, stream, expectedSubject, replicas); err == nil {
			t.Fatal("expected error for drifted subjects, got nil")
		}
	})

	t.Run("extra subject fails", func(t *testing.T) {
		cfg := baseConfig()
		cfg.Subjects = []string{expectedSubject, "other.subject.>"}
		if err := verifyStreamConfig(cfg, stream, expectedSubject, replicas); err == nil {
			t.Fatal("expected error for extra subjects, got nil")
		}
	})

	t.Run("storage type mismatch fails", func(t *testing.T) {
		cfg := baseConfig()
		cfg.Storage = nats.MemoryStorage
		if err := verifyStreamConfig(cfg, stream, expectedSubject, replicas); err == nil {
			t.Fatal("expected error for memory storage drift, got nil")
		}
	})

	t.Run("replica count mismatch fails", func(t *testing.T) {
		cfg := baseConfig()
		cfg.Replicas = 1
		if err := verifyStreamConfig(cfg, stream, expectedSubject, replicas); err == nil {
			t.Fatal("expected error for replica count drift, got nil")
		}
	})

	t.Run("deduplication window mismatch fails", func(t *testing.T) {
		cfg := baseConfig()
		cfg.Duplicates = 5 * time.Minute
		if err := verifyStreamConfig(cfg, stream, expectedSubject, replicas); err == nil {
			t.Fatal("expected error for deduplication window drift, got nil")
		}
	})
}

func TestWorkerStructuredJSONLoggingCriticalEvents(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))

	repo := store.NewMemory()
	// Insert an outbox event
	if err := repo.AppendOutbox(context.Background(), domain.OutboxEvent{
		ID:            "evt-obs-1",
		TenantID:      "tenant-obs",
		EventType:     "consent.granted",
		AggregateType: "consent",
		AggregateID:   "con-obs-1",
		Payload:       map[string]any{"secret_field": "must-not-be-logged"},
		OccurredAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	published := make([]string, 0)
	w := newWorker(repo, func(_ context.Context, item event) error {
		published = append(published, item.ID)
		return nil
	}, "worker-obs-test")
	w.logger = logger

	// Run one cycle
	ctx := context.Background()
	w.cycle(ctx, ctx)

	if len(published) != 1 || published[0] != "evt-obs-1" {
		t.Fatalf("expected evt-obs-1 to be published, got: %v", published)
	}

	rawLogs := strings.TrimSpace(logBuf.String())
	if rawLogs == "" {
		t.Fatal("expected at least one log line from worker")
	}
	lines := strings.Split(rawLogs, "\n")
	foundClaim := false
	foundPublish := false
	foundBatch := false

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("log line is not valid JSON (%v): %s", err, line)
		}
		msg, _ := entry["msg"].(string)
		switch msg {
		case "claim_succeeded":
			foundClaim = true
			if entry["worker_id"] != "worker-obs-test" {
				t.Fatalf("unexpected worker_id in claim log: %v", entry)
			}
			if entry["claimed_count"] != float64(1) {
				t.Fatalf("expected claimed_count=1, got: %v", entry["claimed_count"])
			}
		case "publish_succeeded":
			foundPublish = true
			if entry["event_id"] != "evt-obs-1" || entry["tenant_id"] != "tenant-obs" {
				t.Fatalf("unexpected attributes in publish log: %v", entry)
			}
		case "batch_completed":
			foundBatch = true
			if entry["processed_count"] != float64(1) {
				t.Fatalf("expected processed_count=1, got: %v", entry["processed_count"])
			}
		}

		// Security boundary: raw claim tokens and payloads must NEVER be logged (T-021)
		if strings.Contains(line, "must-not-be-logged") {
			t.Fatalf("payload secret leaked to log: %s", line)
		}
		if strings.Contains(line, "token") && !strings.Contains(line, "complete_succeeded") {
			// check that no raw claim token string appears
			if strings.Contains(line, "claim_token") {
				t.Fatalf("raw claim_token leaked in log: %s", line)
			}
		}
	}

	if !foundClaim || !foundPublish || !foundBatch {
		t.Fatalf("missing required structured log events: claim=%v, publish=%v, batch=%v. Log buffer: %s",
			foundClaim, foundPublish, foundBatch, logBuf.String())
	}
}

func TestWorkerHealthEndpointsMethodRestrictions(t *testing.T) {
	h := &workerHealthHandler{}
	mux := newWorkerHealthMux(h)

	methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete, "CONNECT"}
	endpoints := []string{"/healthz", "/readyz"}

	for _, endpoint := range endpoints {
		for _, method := range methods {
			t.Run(endpoint+"_"+method, func(t *testing.T) {
				req := httptest.NewRequest(method, endpoint, nil)
				resp := httptest.NewRecorder()
				mux.ServeHTTP(resp, req)

				if resp.Code != http.StatusMethodNotAllowed {
					t.Fatalf("expected 405 Method Not Allowed for %s %s, got %d", method, endpoint, resp.Code)
				}
				if allow := resp.Header().Get("Allow"); allow != http.MethodGet {
					t.Fatalf("expected Allow: %s, got %s", http.MethodGet, allow)
				}
			})
		}
	}
}

func TestWorkerHealthLivenessIndependentOfDependencies(t *testing.T) {
	// Worker liveness should return 200 even when dependencies fail readiness
	h := &workerHealthHandler{
		customReady: func(ctx context.Context) error {
			return errors.New("dependencies down")
		},
	}
	mux := newWorkerHealthMux(h)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /healthz, got %d", resp.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Fatalf("expected status: ok, got %v", body["status"])
	}
}

type mockWorkerRepo struct {
	store.Repository
	pingErr         error
	verifySchemaErr error
}

func (m *mockWorkerRepo) Ping(ctx context.Context) error {
	return m.pingErr
}

func (m *mockWorkerRepo) VerifySchema(ctx context.Context) error {
	return m.verifySchemaErr
}

func TestWorkerReadyzReflectsDependencies(t *testing.T) {
	t.Run("fails when database ping fails", func(t *testing.T) {
		h := &workerHealthHandler{
			repo: &mockWorkerRepo{
				Repository: store.NewMemory(),
				pingErr:    errors.New("db connection refused"),
			},
		}
		mux := newWorkerHealthMux(h)

		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		resp := httptest.NewRecorder()
		mux.ServeHTTP(resp, req)

		if resp.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503 for failed ping, got %d", resp.Code)
		}
		var body map[string]string
		if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body["status"] != "not_ready" {
			t.Fatalf("expected status: not_ready, got %v", body["status"])
		}
	})

	t.Run("fails when schema verification fails", func(t *testing.T) {
		h := &workerHealthHandler{
			repo: &mockWorkerRepo{
				Repository:      store.NewMemory(),
				verifySchemaErr: errors.New("missing outbox table"),
			},
		}
		mux := newWorkerHealthMux(h)

		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		resp := httptest.NewRecorder()
		mux.ServeHTTP(resp, req)

		if resp.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503 for failed schema, got %d", resp.Code)
		}
	})

	t.Run("succeeds when ready", func(t *testing.T) {
		h := &workerHealthHandler{
			customReady: func(ctx context.Context) error {
				return nil
			},
		}
		mux := newWorkerHealthMux(h)

		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		resp := httptest.NewRecorder()
		mux.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for ready worker, got %d", resp.Code)
		}
		var body map[string]string
		if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body["status"] != "ready" {
			t.Fatalf("expected status: ready, got %v", body["status"])
		}
	})
}
