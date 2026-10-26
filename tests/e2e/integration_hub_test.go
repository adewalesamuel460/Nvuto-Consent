package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
)

// -----------------------------------------------------------------------------
// INT-08: Integration Hub & CloudEvents 1.0 Test Harness
// -----------------------------------------------------------------------------

type integrationHubCluster struct {
	apiURL        string
	databaseURL   string
	natsURL       string
	streamName    string
	subjectPrefix string
	nc            *nats.Conn
	js            nats.JetStreamContext
	dbPool        *pgxpool.Pool
	serverCmd     *exec.Cmd
	workerCmd     *exec.Cmd
	serverPort    int
	repoRoot      string
}

func setupIntegrationHubCluster(t *testing.T) *integrationHubCluster {
	t.Helper()

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		dbURL = "postgres://yugabyte:yugabyte@127.0.0.1:5433/consent?sslmode=disable"
	}

	natsURL := os.Getenv("TEST_NATS_URL")
	if natsURL == "" {
		natsURL = os.Getenv("NATS_URL")
	}
	if natsURL == "" {
		natsURL = "nats://127.0.0.1:4222"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		if strings.EqualFold(os.Getenv("NVUTO_REQUIRE_INTEGRATION"), "true") {
			t.Fatalf("database connection failed: %v", err)
		}
		t.Skipf("database connection failed (%v); skipping integration hub e2e tests", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		if strings.EqualFold(os.Getenv("NVUTO_REQUIRE_INTEGRATION"), "true") {
			t.Fatalf("database ping failed: %v", err)
		}
		t.Skipf("database ping failed (%v); skipping integration hub e2e tests", err)
	}

	nc, err := nats.Connect(natsURL, nats.Timeout(3*time.Second))
	if err != nil {
		pool.Close()
		if strings.EqualFold(os.Getenv("NVUTO_REQUIRE_INTEGRATION"), "true") {
			t.Fatalf("NATS connection failed: %v", err)
		}
		t.Skipf("NATS connection failed (%v); skipping integration hub e2e tests", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		pool.Close()
		t.Fatalf("JetStream context failed: %v", err)
	}

	repoRoot := findRepoRoot(t)
	serverBin, workerBin := ensureBinaries(t, repoRoot)
	serverPort := getFreePort(t)
	workerHealthPort := getFreePort(t)

	apiURL := fmt.Sprintf("http://127.0.0.1:%d", serverPort)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	streamName := "NVUTO_HUB_" + suffix
	subjectPrefix := "nvuto.consent.hub." + suffix

	serverCmd := exec.Command(serverBin)
	serverCmd.Dir = repoRoot
	serverCmd.Env = append(os.Environ(),
		fmt.Sprintf("PORT=%d", serverPort),
		"DATABASE_URL="+dbURL,
		"ALLOW_DEV_HEADERS=true",
		"ENVIRONMENT=test",
		"REQUEST_TIMEOUT=5s",
	)
	serverCmd.Stdout = os.Stdout
	serverCmd.Stderr = os.Stderr
	if err := serverCmd.Start(); err != nil {
		nc.Close()
		pool.Close()
		t.Fatalf("failed to start server process: %v", err)
	}

	workerCmd := exec.Command(workerBin)
	workerCmd.Dir = repoRoot
	workerCmd.Env = append(os.Environ(),
		fmt.Sprintf("HEALTH_PORT=%d", workerHealthPort),
		"DATABASE_URL="+dbURL,
		"NATS_URL="+natsURL,
		"ALLOW_DEV_HEADERS=true",
		"ALLOW_INSECURE_NATS=true",
		"ENVIRONMENT=test",
		"NATS_STREAM="+streamName,
		"NATS_SUBJECT_PREFIX="+subjectPrefix,
		"NATS_REPLICAS=1",
		"OUTBOX_POLL_INTERVAL=50ms",
	)
	workerCmd.Stdout = os.Stdout
	workerCmd.Stderr = os.Stderr
	if err := workerCmd.Start(); err != nil {
		_ = serverCmd.Process.Kill()
		nc.Close()
		pool.Close()
		t.Fatalf("failed to start worker process: %v", err)
	}

	waitForReady(t, fmt.Sprintf("http://127.0.0.1:%d/readyz", serverPort), 10*time.Second)
	waitForReady(t, fmt.Sprintf("http://127.0.0.1:%d/readyz", workerHealthPort), 10*time.Second)

	cluster := &integrationHubCluster{
		apiURL:        apiURL,
		databaseURL:   dbURL,
		natsURL:       natsURL,
		streamName:    streamName,
		subjectPrefix: subjectPrefix,
		nc:            nc,
		js:            js,
		dbPool:        pool,
		serverCmd:     serverCmd,
		workerCmd:     workerCmd,
		serverPort:    serverPort,
		repoRoot:      repoRoot,
	}

	t.Cleanup(func() {
		if cluster.workerCmd != nil && cluster.workerCmd.Process != nil {
			_ = cluster.workerCmd.Process.Kill()
		}
		if cluster.serverCmd != nil && cluster.serverCmd.Process != nil {
			_ = cluster.serverCmd.Process.Kill()
		}
		_ = js.DeleteStream(streamName)
		nc.Close()
		pool.Close()
	})

	return cluster
}

func setupHubTenantPolicy(t *testing.T, client *http.Client, apiURL, tenantID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)

	// Purpose
	active := true
	affirm := true
	doi := false
	resp, body := doHTTP(t, client, http.MethodPost, apiURL+"/v1/purposes", tenantID, map[string]any{
		"code":                         "marketing",
		"category":                     "marketing",
		"allowed_channels":             []string{"email", "sms", "webhook"},
		"requires_affirmative_consent": &affirm,
		"requires_double_opt_in":       &doi,
		"active":                       &active,
	})
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("register purpose failed (%d): %s", resp.StatusCode, string(body))
	}

	// Lawful Basis
	reqEvidence := true
	resp, body = doHTTP(t, client, http.MethodPost, apiURL+"/v1/lawful-bases", tenantID, map[string]any{
		"code":               "consent",
		"allowed_categories": []string{"marketing"},
		"requires_evidence":  &reqEvidence,
		"active":             &active,
	})
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("register lawful basis failed (%d): %s", resp.StatusCode, string(body))
	}

	// Privacy Notice
	resp, body = doHTTP(t, client, http.MethodPost, apiURL+"/v1/notices", tenantID, map[string]any{
		"version":      "2026-hub-v1",
		"jurisdiction": "GLOBAL",
		"published_at": now,
		"active":       &active,
		"content":      "Standard Hub Integration Policy Notice",
	})
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("publish notice failed (%d): %s", resp.StatusCode, string(body))
	}
}

// -----------------------------------------------------------------------------
// CloudEvents 1.0 Specification Compliant Structs
// -----------------------------------------------------------------------------

type CloudEvent10 struct {
	SpecVersion       string          `json:"specversion"`
	ID                string          `json:"id"`
	Source            string          `json:"source"`
	Type              string          `json:"type"`
	Subject           string          `json:"subject"`
	Time              time.Time       `json:"time"`
	DataContentType   string          `json:"datacontenttype"`
	SchemaVersion     int             `json:"nvutoschemaversion"`
	TenantID          string          `json:"nvutotenantid"`
	AggregateType     string          `json:"nvutoaggregatetype,omitempty"`
	AggregateRevision int64           `json:"nvutoaggregaterevision,omitempty"`
	Data              json.RawMessage `json:"data"`
}

type MockWebhookReceiver struct {
	server       *httptest.Server
	URL          string
	mu           sync.Mutex
	events       []CloudEvent10
	requestCount int32
}

func newMockWebhookReceiver() *MockWebhookReceiver {
	receiver := &MockWebhookReceiver{
		events: make([]CloudEvent10, 0),
	}
	receiver.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&receiver.requestCount, 1)
		var ev CloudEvent10
		if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		receiver.mu.Lock()
		receiver.events = append(receiver.events, ev)
		receiver.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	receiver.URL = receiver.server.URL
	return receiver
}

func (m *MockWebhookReceiver) Close() {
	if m.server != nil {
		m.server.Close()
	}
}

func (m *MockWebhookReceiver) GetEvents() []CloudEvent10 {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := make([]CloudEvent10, len(m.events))
	copy(res, m.events)
	return res
}

func (m *MockWebhookReceiver) Count() int {
	return int(atomic.LoadInt32(&m.requestCount))
}

// SimulatedIntegrationHub routes CloudEvents to tenant-specific destinations
type SimulatedIntegrationHub struct {
	apiURL                  string
	routes                  map[string]string // tenantID -> webhookURL
	mu                      sync.Mutex
	revisions               map[string]int64 // key: tenantID:aggregateKey -> highest revision
	dispatchedCount         int
	duplicateDiscardCount   int
	crossTenantLeakageCount int
	httpClient              *http.Client
}

func newSimulatedIntegrationHub(apiURL string, routes map[string]string) *SimulatedIntegrationHub {
	return &SimulatedIntegrationHub{
		apiURL:     apiURL,
		routes:     routes,
		revisions:  make(map[string]int64),
		httpClient: &http.Client{Timeout: 3 * time.Second},
	}
}

// HandleEvent handles a NATS JetStream message according to CloudEvents 1.0,
// tenant routing isolation, monotonic revision deduplication (T-010), and expiry reconciliation.
func (h *SimulatedIntegrationHub) HandleEvent(msg *nats.Msg) error {
	_ = msg.Ack()

	var ev CloudEvent10
	if err := json.Unmarshal(msg.Data, &ev); err != nil {
		return err
	}

	// Fallback to headers if extension attributes are extracted from transport
	if ev.TenantID == "" {
		ev.TenantID = msg.Header.Get("X-Nvuto-Tenant-ID")
	}
	if ev.AggregateRevision <= 0 {
		if rawRev := msg.Header.Get("X-Nvuto-Aggregate-Revision"); rawRev != "" {
			ev.AggregateRevision, _ = strconv.ParseInt(rawRev, 10, 64)
		}
	}

	// 1. Validate CloudEvents 1.0 Spec Compliance
	if ev.SpecVersion != "1.0" {
		return fmt.Errorf("invalid specversion: %q (want '1.0')", ev.SpecVersion)
	}
	if ev.ID == "" || ev.Source == "" || ev.Type == "" {
		return fmt.Errorf("missing required CloudEvents 1.0 core attributes (id=%q, source=%q, type=%q)", ev.ID, ev.Source, ev.Type)
	}

	// 2. Replay & Deduplication Check (Beyond Broker Dedup Window, T-010 / D-13)
	// Keyed on tenantID:subject
	aggKey := fmt.Sprintf("%s:%s", ev.TenantID, ev.Subject)

	h.mu.Lock()
	highestRev, found := h.revisions[aggKey]
	if found && ev.AggregateRevision <= highestRev {
		// Event is stale or duplicate beyond broker dedup window -> safely discard without downstream dispatch
		h.duplicateDiscardCount++
		h.mu.Unlock()
		return nil
	}
	h.revisions[aggKey] = ev.AggregateRevision
	h.mu.Unlock()

	// 3. Expiry Reconciliation
	// If the payload contains an expires_at that has already passed, reconcile live status via evaluate
	var payloadData struct {
		SubjectID string     `json:"subject_id"`
		Purpose   string     `json:"purpose"`
		Channel   string     `json:"channel"`
		ExpiresAt *time.Time `json:"expires_at"`
		Status    string     `json:"status"`
	}
	if err := json.Unmarshal(ev.Data, &payloadData); err == nil && payloadData.ExpiresAt != nil {
		if time.Now().UTC().After(*payloadData.ExpiresAt) {
			// Query live evaluate endpoint to reconcile true state
			evalReqBody, _ := json.Marshal(map[string]any{
				"subject_id": payloadData.SubjectID,
				"purpose":    payloadData.Purpose,
				"channel":    payloadData.Channel,
			})
			req, err := http.NewRequest(http.MethodPost, h.apiURL+"/v1/consents/evaluate", bytes.NewReader(evalReqBody))
			if err == nil {
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Nvuto-Tenant-ID", ev.TenantID)
				req.Header.Set("X-Nvuto-Actor-ID", "integration-hub-reconciler")
				resp, err := h.httpClient.Do(req)
				if err == nil {
					defer resp.Body.Close()
					var decision struct {
						Allowed bool   `json:"allowed"`
						Reason  string `json:"reason"`
					}
					if err := json.NewDecoder(resp.Body).Decode(&decision); err == nil && !decision.Allowed {
						// Reconciled: Update payload status to expired/withdrawn before downstream sync
						payloadData.Status = "expired"
						updatedData, _ := json.Marshal(payloadData)
						ev.Data = updatedData
					}
				}
			}
		}
	}

	// 4. Tenant Routing & Isolation
	targetWebhookURL, ok := h.routes[ev.TenantID]
	if !ok {
		// Unrecognized tenant or no route configured -> quarantine
		return nil
	}

	// Dispatch to designated tenant webhook
	body, _ := json.Marshal(ev)
	req, err := http.NewRequest(http.MethodPost, targetWebhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/cloudevents+json")
	req.Header.Set("X-Nvuto-Tenant-ID", ev.TenantID)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		h.mu.Lock()
		h.dispatchedCount++
		h.mu.Unlock()
	}

	return nil
}

// TestIntegrationHub verifies INT-08 end-to-end integration:
// 1. CloudEvents 1.0 Schema Compliance & Routing Extensions:
//   - Validates specversion="1.0", id, source, type, subject, datacontenttype.
//   - Validates routing extension attribute nvutotenantid and aggregate revision nvutoaggregaterevision.
//
// 2. Tenant Isolation & Exclusive Webhook Routing:
//   - Directs Tenant Alpha events exclusively to Alpha Webhook, Beta to Beta Webhook.
//   - Proves 0 cross-tenant event leakage.
//
// 3. Replay & Reconciliation Beyond Dedup Window (T-010 / D-13):
//   - Proves duplicate outbox events arriving beyond the broker's 10-minute dedup window
//     are safely dropped without firing duplicate downstream webhook payloads.
//
// 4. Expiry Reconciliation:
//   - Proves racing or delayed deliveries for expired consents are reconciled against
//     current state rather than propagating stale granted payloads downstream.
func TestIntegrationHub(t *testing.T) {
	cluster := setupIntegrationHubCluster(t)
	client := &http.Client{Timeout: 5 * time.Second}

	// Valid evidence hash
	validHash := "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	now := time.Now().UTC()
	evidence := map[string]any{
		"method":        "integration_hub_intake",
		"artifact_hash": validHash,
		"collected_at":  now.Format(time.RFC3339),
	}

	// =========================================================================
	// Subtest 1: CloudEvents 1.0 Schema Compliance & Routing Extensions
	// =========================================================================
	t.Run("CloudEvents10_SchemaCompliance_AndRoutingExtensions", func(t *testing.T) {
		tenantID := fmt.Sprintf("tenant-hub-schema-%d", time.Now().UnixNano())
		setupHubTenantPolicy(t, client, cluster.apiURL, tenantID)

		receivedMsgChan := make(chan *nats.Msg, 10)
		sub, err := cluster.nc.Subscribe(cluster.subjectPrefix+".>", func(msg *nats.Msg) {
			receivedMsgChan <- msg
		})
		if err != nil {
			t.Fatalf("failed to subscribe to NATS: %v", err)
		}
		defer sub.Unsubscribe()

		// Trigger mutation: Record a consent grant
		subjectID := fmt.Sprintf("sub-schema-%d", time.Now().UnixNano())
		resp, body := doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantID, map[string]any{
			"subject_id":     subjectID,
			"purpose":        "marketing",
			"channel":        "email",
			"legal_basis":    "consent",
			"status":         "granted",
			"source":         "crm_hub",
			"jurisdiction":   "GLOBAL",
			"notice_version": "2026-hub-v1",
			"evidence":       evidence,
		})
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			t.Fatalf("failed to record consent (%d): %s", resp.StatusCode, string(body))
		}

		// Wait for message from NATS JetStream matching consent.granted
		var ev CloudEvent10
		deadline := time.Now().Add(10 * time.Second)
		foundGranted := false
		for time.Now().Before(deadline) && !foundGranted {
			select {
			case msg := <-receivedMsgChan:
				var candidate CloudEvent10
				if err := json.Unmarshal(msg.Data, &candidate); err == nil {
					// Verify CloudEvents 1.0 schema on every event received
					if candidate.SpecVersion != "1.0" {
						t.Fatalf("expected specversion '1.0', got %q", candidate.SpecVersion)
					}
					if candidate.ID == "" || candidate.Source != "nvuto/consent" {
						t.Fatalf("invalid CloudEvents core attributes: id=%q, source=%q", candidate.ID, candidate.Source)
					}
					if candidate.Type == "consent.granted" && candidate.TenantID == tenantID {
						ev = candidate
						foundGranted = true
						// Verify transport headers
						if msg.Header.Get("Content-Type") != "application/cloudevents+json" {
							t.Fatalf("expected header Content-Type 'application/cloudevents+json', got %q", msg.Header.Get("Content-Type"))
						}
						if msg.Header.Get("X-Nvuto-Tenant-ID") != tenantID {
							t.Fatalf("expected header X-Nvuto-Tenant-ID %q, got %q", tenantID, msg.Header.Get("X-Nvuto-Tenant-ID"))
						}
						if msg.Header.Get("Nats-Msg-Id") == "" {
							t.Fatalf("expected non-empty Nats-Msg-Id header for deduplication")
						}
					}
				}
			case <-time.After(50 * time.Millisecond):
			}
		}
		if !foundGranted {
			t.Fatalf("timed out waiting for consent.granted CloudEvent from NATS JetStream")
		}

		// Verify CloudEvents 1.0 specification core attributes
		if ev.SpecVersion != "1.0" {
			t.Fatalf("expected specversion '1.0', got %q", ev.SpecVersion)
		}
		if ev.ID == "" {
			t.Fatalf("expected non-empty id attribute")
		}
		if ev.Source != "nvuto/consent" {
			t.Fatalf("expected source 'nvuto/consent', got %q", ev.Source)
		}
		if !strings.HasPrefix(ev.Type, "consent.") {
			t.Fatalf("expected type with 'consent.' prefix, got %q", ev.Type)
		}
		if ev.DataContentType != "application/json" {
			t.Fatalf("expected datacontenttype 'application/json', got %q", ev.DataContentType)
		}
		if ev.Time.IsZero() {
			t.Fatalf("expected valid non-zero event timestamp")
		}
		if !strings.HasPrefix(ev.Subject, "consent") || !strings.Contains(ev.Subject, "/") {
			t.Fatalf("expected subject starting with 'consent' and containing '/', got %q", ev.Subject)
		}

		// Verify Extension Attributes
		if ev.TenantID != tenantID {
			t.Fatalf("expected routing extension nvutotenantid %q, got %q", tenantID, ev.TenantID)
		}
		if ev.AggregateRevision < 1 {
			t.Fatalf("expected nvutoaggregaterevision >= 1, got %d", ev.AggregateRevision)
		}

		t.Logf("PASS: CloudEvents 1.0 specification compliance confirmed (specversion=%q, id=%q, tenant=%q, revision=%d)", ev.SpecVersion, ev.ID, ev.TenantID, ev.AggregateRevision)
	})

	// =========================================================================
	// Subtest 2: Tenant Isolation & Exclusive Webhook Routing
	// =========================================================================
	t.Run("TenantIsolation_ExclusiveWebhookRouting", func(t *testing.T) {
		tenantAlpha := fmt.Sprintf("tenant-hub-alpha-%d", time.Now().UnixNano())
		tenantBeta := fmt.Sprintf("tenant-hub-beta-%d", time.Now().UnixNano())

		setupHubTenantPolicy(t, client, cluster.apiURL, tenantAlpha)
		setupHubTenantPolicy(t, client, cluster.apiURL, tenantBeta)

		evidenceAlpha := map[string]any{
			"method":        "integration_hub_intake",
			"artifact_hash": validHash,
			"collected_at":  time.Now().UTC().Format(time.RFC3339),
		}
		evidenceBeta := map[string]any{
			"method":        "integration_hub_intake",
			"artifact_hash": validHash,
			"collected_at":  time.Now().UTC().Format(time.RFC3339),
		}

		alphaReceiver := newMockWebhookReceiver()
		defer alphaReceiver.Close()

		betaReceiver := newMockWebhookReceiver()
		defer betaReceiver.Close()

		// Configure Integration Hub routes
		routes := map[string]string{
			tenantAlpha: alphaReceiver.URL,
			tenantBeta:  betaReceiver.URL,
		}
		hub := newSimulatedIntegrationHub(cluster.apiURL, routes)

		// Subscribe Hub to NATS
		sub, err := cluster.nc.Subscribe(cluster.subjectPrefix+".>", func(msg *nats.Msg) {
			_ = hub.HandleEvent(msg)
		})
		if err != nil {
			t.Fatalf("failed to subscribe hub to NATS: %v", err)
		}
		defer sub.Unsubscribe()

		// Mutate 5 consents for Alpha
		const eventCount = 5
		for i := 0; i < eventCount; i++ {
			subj := fmt.Sprintf("sub-alpha-%d-%d", time.Now().UnixNano(), i)
			resp, body := doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantAlpha, map[string]any{
				"subject_id":     subj,
				"purpose":        "marketing",
				"channel":        "email",
				"legal_basis":    "consent",
				"status":         "granted",
				"source":         "hub_alpha",
				"jurisdiction":   "GLOBAL",
				"notice_version": "2026-hub-v1",
				"evidence":       evidenceAlpha,
			})
			if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
				t.Fatalf("failed to create alpha consent %d (%d): %s", i, resp.StatusCode, string(body))
			}
		}

		// Mutate 5 consents for Beta
		for i := 0; i < eventCount; i++ {
			subj := fmt.Sprintf("sub-beta-%d-%d", time.Now().UnixNano(), i)
			resp, body := doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantBeta, map[string]any{
				"subject_id":     subj,
				"purpose":        "marketing",
				"channel":        "sms",
				"legal_basis":    "consent",
				"status":         "granted",
				"source":         "hub_beta",
				"jurisdiction":   "GLOBAL",
				"notice_version": "2026-hub-v1",
				"evidence":       evidenceBeta,
			})
			if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
				t.Fatalf("failed to create beta consent %d (%d): %s", i, resp.StatusCode, string(body))
			}
		}

		// Wait for both webhook receivers to collect 5 consent.granted events each
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			alphaCount := 0
			for _, ev := range alphaReceiver.GetEvents() {
				if ev.Type == "consent.granted" {
					alphaCount++
				}
			}
			betaCount := 0
			for _, ev := range betaReceiver.GetEvents() {
				if ev.Type == "consent.granted" {
					betaCount++
				}
			}
			if alphaCount >= eventCount && betaCount >= eventCount {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}

		alphaEvents := alphaReceiver.GetEvents()
		betaEvents := betaReceiver.GetEvents()

		var alphaGranted, betaGranted []CloudEvent10
		for _, ev := range alphaEvents {
			if ev.Type == "consent.granted" {
				alphaGranted = append(alphaGranted, ev)
			}
		}
		for _, ev := range betaEvents {
			if ev.Type == "consent.granted" {
				betaGranted = append(betaGranted, ev)
			}
		}

		if len(alphaGranted) != eventCount {
			t.Fatalf("expected Alpha webhook to receive %d granted events, got %d (total events: %d)", eventCount, len(alphaGranted), len(alphaEvents))
		}
		if len(betaGranted) != eventCount {
			t.Fatalf("expected Beta webhook to receive %d granted events, got %d (total events: %d)", eventCount, len(betaGranted), len(betaEvents))
		}

		// Verify Strict Isolation: Zero cross-tenant leakage!
		for _, ev := range alphaEvents {
			if ev.TenantID != tenantAlpha {
				t.Fatalf("CRITICAL SECURITY VIOLATION: Cross-tenant leakage detected! Alpha received event for tenant %q", ev.TenantID)
			}
		}
		for _, ev := range betaEvents {
			if ev.TenantID != tenantBeta {
				t.Fatalf("CRITICAL SECURITY VIOLATION: Cross-tenant leakage detected! Beta received event for tenant %q", ev.TenantID)
			}
		}

		t.Logf("PASS: Tenant isolation confirmed: Alpha received %d Alpha events (0 Beta), Beta received %d Beta events (0 Alpha). Zero cross-tenant leakage!", len(alphaEvents), len(betaEvents))
	})

	// =========================================================================
	// Subtest 3: Replay & Reconciliation Beyond Dedup Window (T-010 / D-13)
	// =========================================================================
	t.Run("ReplayAndReconciliation_BeyondDedupWindow_T010", func(t *testing.T) {
		tenantID := fmt.Sprintf("tenant-hub-dedup-%d", time.Now().UnixNano())
		setupHubTenantPolicy(t, client, cluster.apiURL, tenantID)

		receiver := newMockWebhookReceiver()
		defer receiver.Close()

		routes := map[string]string{
			tenantID: receiver.URL,
		}
		hub := newSimulatedIntegrationHub(cluster.apiURL, routes)

		// Create consent subject
		subjectID := fmt.Sprintf("sub-dedup-%d", time.Now().UnixNano())
		aggregateID := fmt.Sprintf("con-dedup-%d", time.Now().UnixNano())

		// Step A: Process initial consent.granted event (Revision 1)
		payloadRev1, _ := json.Marshal(map[string]any{
			"id":         aggregateID,
			"tenant_id":  tenantID,
			"subject_id": subjectID,
			"purpose":    "marketing",
			"channel":    "email",
			"status":     "granted",
		})
		msgRev1 := &nats.Msg{
			Subject: cluster.subjectPrefix + ".granted",
			Header: nats.Header{
				"Content-Type":               []string{"application/cloudevents+json"},
				"X-Nvuto-Tenant-ID":          []string{tenantID},
				"X-Nvuto-Aggregate-Revision": []string{"1"},
				"Nats-Msg-Id":                []string{"msg-init-001"},
			},
		}
		envRev1, _ := json.Marshal(CloudEvent10{
			SpecVersion:       "1.0",
			ID:                "evt-init-001",
			Source:            "nvuto/consent",
			Type:              "consent.granted",
			Subject:           "consent/" + aggregateID,
			Time:              time.Now().UTC(),
			DataContentType:   "application/json",
			TenantID:          tenantID,
			AggregateRevision: 1,
			Data:              payloadRev1,
		})
		msgRev1.Data = envRev1

		if err := hub.HandleEvent(msgRev1); err != nil {
			t.Fatalf("failed to handle initial Revision 1 message: %v", err)
		}
		if receiver.Count() != 1 {
			t.Fatalf("expected webhook receiver count 1, got %d", receiver.Count())
		}
		t.Log("Step A Confirmed: Revision 1 event dispatched to webhook (count=1)")

		// Step B: Process consent.withdrawn event (Revision 2)
		payloadRev2, _ := json.Marshal(map[string]any{
			"id":         aggregateID,
			"tenant_id":  tenantID,
			"subject_id": subjectID,
			"purpose":    "marketing",
			"channel":    "email",
			"status":     "withdrawn",
		})
		msgRev2 := &nats.Msg{
			Subject: cluster.subjectPrefix + ".withdrawn",
			Header: nats.Header{
				"Content-Type":               []string{"application/cloudevents+json"},
				"X-Nvuto-Tenant-ID":          []string{tenantID},
				"X-Nvuto-Aggregate-Revision": []string{"2"},
				"Nats-Msg-Id":                []string{"msg-init-002"},
			},
		}
		envRev2, _ := json.Marshal(CloudEvent10{
			SpecVersion:       "1.0",
			ID:                "evt-init-002",
			Source:            "nvuto/consent",
			Type:              "consent.withdrawn",
			Subject:           "consent/" + aggregateID,
			Time:              time.Now().UTC(),
			DataContentType:   "application/json",
			TenantID:          tenantID,
			AggregateRevision: 2,
			Data:              payloadRev2,
		})
		msgRev2.Data = envRev2

		if err := hub.HandleEvent(msgRev2); err != nil {
			t.Fatalf("failed to handle Revision 2 message: %v", err)
		}
		if receiver.Count() != 2 {
			t.Fatalf("expected webhook receiver count 2, got %d", receiver.Count())
		}
		t.Log("Step B Confirmed: Revision 2 event dispatched to webhook (count=2)")

		// Step C: Inject duplicate Revision 1 event with NEW synthetic Msg-Id
		// Simulating NATS re-delivering message AFTER the 10-minute broker dedup window expires
		t.Log("Step C: Injecting replayed Revision 1 message with new Msg-Id (simulating post-10-min dedup window)...")
		msgReplayRev1 := &nats.Msg{
			Subject: cluster.subjectPrefix + ".granted",
			Header: nats.Header{
				"Content-Type":               []string{"application/cloudevents+json"},
				"X-Nvuto-Tenant-ID":          []string{tenantID},
				"X-Nvuto-Aggregate-Revision": []string{"1"},
				"Nats-Msg-Id":                []string{"msg-replayed-after-dedup-expired-003"}, // New message ID!
			},
		}
		envReplayRev1, _ := json.Marshal(CloudEvent10{
			SpecVersion:       "1.0",
			ID:                "evt-replayed-003",
			Source:            "nvuto/consent",
			Type:              "consent.granted",
			Subject:           "consent/" + aggregateID,
			Time:              time.Now().UTC().Add(-15 * time.Minute),
			DataContentType:   "application/json",
			TenantID:          tenantID,
			AggregateRevision: 1, // Stale Revision 1!
			Data:              payloadRev1,
		})
		msgReplayRev1.Data = envReplayRev1

		if err := hub.HandleEvent(msgReplayRev1); err != nil {
			t.Fatalf("failed to handle replayed Revision 1: %v", err)
		}

		// Step D: Inject duplicate Revision 2 event with NEW Msg-Id
		t.Log("Step D: Injecting replayed duplicate Revision 2 message with new Msg-Id...")
		msgReplayRev2 := &nats.Msg{
			Subject: cluster.subjectPrefix + ".withdrawn",
			Header: nats.Header{
				"Content-Type":               []string{"application/cloudevents+json"},
				"X-Nvuto-Tenant-ID":          []string{tenantID},
				"X-Nvuto-Aggregate-Revision": []string{"2"},
				"Nats-Msg-Id":                []string{"msg-duplicate-after-dedup-expired-004"}, // New message ID!
			},
		}
		envReplayRev2, _ := json.Marshal(CloudEvent10{
			SpecVersion:       "1.0",
			ID:                "evt-replayed-004",
			Source:            "nvuto/consent",
			Type:              "consent.withdrawn",
			Subject:           "consent/" + aggregateID,
			Time:              time.Now().UTC().Add(-12 * time.Minute),
			DataContentType:   "application/json",
			TenantID:          tenantID,
			AggregateRevision: 2, // Duplicate Revision 2!
			Data:              payloadRev2,
		})
		msgReplayRev2.Data = envReplayRev2

		if err := hub.HandleEvent(msgReplayRev2); err != nil {
			t.Fatalf("failed to handle replayed Revision 2: %v", err)
		}

		// Assertion: Webhook invocation count MUST REMAIN AT 2!
		if receiver.Count() != 2 {
			t.Fatalf("CRITICAL RECONCILIATION FAILURE: Webhook received duplicate events beyond dedup window! Expected count 2, got %d", receiver.Count())
		}
		if hub.duplicateDiscardCount != 2 {
			t.Fatalf("expected duplicateDiscardCount to be 2, got %d", hub.duplicateDiscardCount)
		}

		t.Logf("PASS: Integration Hub successfully relied on AggregateRevision to discard duplicate events beyond 10-min broker dedup window (discards=%d, webhook count remains %d)", hub.duplicateDiscardCount, receiver.Count())
	})

	// =========================================================================
	// Subtest 4: Expiry Reconciliation Racing with Delayed Delivery
	// =========================================================================
	t.Run("ExpiryReconciliation_RacingWithDelayedDelivery", func(t *testing.T) {
		tenantID := fmt.Sprintf("tenant-hub-exp-%d", time.Now().UnixNano())
		setupHubTenantPolicy(t, client, cluster.apiURL, tenantID)

		receiver := newMockWebhookReceiver()
		defer receiver.Close()

		routes := map[string]string{
			tenantID: receiver.URL,
		}
		hub := newSimulatedIntegrationHub(cluster.apiURL, routes)

		subjectID := fmt.Sprintf("sub-hub-exp-%d", time.Now().UnixNano())
		aggregateID := fmt.Sprintf("con-hub-exp-%d", time.Now().UnixNano())

		// Fresh evidence collected after notice publication
		sub4Now := time.Now().UTC()
		evidenceSub4 := map[string]any{
			"method":        "integration_hub_intake",
			"artifact_hash": validHash,
			"collected_at":  sub4Now.Format(time.RFC3339),
		}

		// Create consent with future expiry, then backdate in DB so evaluate returns expired
		future := sub4Now.Add(1 * time.Hour).Format(time.RFC3339)
		resp, body := doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantID, map[string]any{
			"subject_id":     subjectID,
			"purpose":        "marketing",
			"channel":        "email",
			"legal_basis":    "consent",
			"status":         "granted",
			"source":         "hub_exp",
			"jurisdiction":   "GLOBAL",
			"notice_version": "2026-hub-v1",
			"expires_at":     future,
			"evidence":       evidenceSub4,
		})
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			t.Fatalf("failed to record consent (%d): %s", resp.StatusCode, string(body))
		}
		var createdConsent struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(body, &createdConsent)

		// Backdate in DB so that evaluate returns expired (allowed=false, reason=expired)
		_, err := cluster.dbPool.Exec(context.Background(), `
			UPDATE consents SET created_at = NOW() - INTERVAL '2 hours', expires_at = NOW() - INTERVAL '1 hour' WHERE id = $1
		`, createdConsent.ID)
		if err != nil {
			t.Fatalf("failed to backdate expires_at: %v", err)
		}

		// Simulate a delayed event delivery that still claims status="granted", but with past expires_at
		pastExpires := now.Add(-1 * time.Hour)
		delayedPayload, _ := json.Marshal(map[string]any{
			"id":         createdConsent.ID,
			"tenant_id":  tenantID,
			"subject_id": subjectID,
			"purpose":    "marketing",
			"channel":    "email",
			"status":     "granted", // Stale granted claim!
			"expires_at": pastExpires,
		})

		msgDelayed := &nats.Msg{
			Subject: cluster.subjectPrefix + ".granted",
			Header: nats.Header{
				"Content-Type":               []string{"application/cloudevents+json"},
				"X-Nvuto-Tenant-ID":          []string{tenantID},
				"X-Nvuto-Aggregate-Revision": []string{"1"},
				"Nats-Msg-Id":                []string{"msg-delayed-001"},
			},
		}
		envDelayed, _ := json.Marshal(CloudEvent10{
			SpecVersion:       "1.0",
			ID:                "evt-delayed-001",
			Source:            "nvuto/consent",
			Type:              "consent.granted",
			Subject:           "consent/" + aggregateID,
			Time:              pastExpires,
			DataContentType:   "application/json",
			TenantID:          tenantID,
			AggregateRevision: 1,
			Data:              delayedPayload,
		})
		msgDelayed.Data = envDelayed

		// Hub processes event and performs live expiry reconciliation
		if err := hub.HandleEvent(msgDelayed); err != nil {
			t.Fatalf("failed to handle delayed message: %v", err)
		}

		// Verify that the webhook received the reconciled status
		events := receiver.GetEvents()
		if len(events) != 1 {
			t.Fatalf("expected 1 event dispatched to webhook, got %d", len(events))
		}

		var dispatchedData struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(events[0].Data, &dispatchedData); err != nil {
			t.Fatalf("failed to decode dispatched event data: %v", err)
		}

		if dispatchedData.Status != "expired" {
			t.Fatalf("RECONCILIATION FAILURE: Webhook received stale status %q instead of reconciled 'expired'", dispatchedData.Status)
		}

		t.Logf("PASS: Expiry reconciliation verified: delayed event with stale 'granted' status was reconciled against canonical evaluation engine and dispatched downstream as 'expired'")
	})
}
