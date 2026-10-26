package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
)

// CloudEvent standard envelope matching outbox worker output.
type cloudEventEnvelope struct {
	SpecVersion       string          `json:"specversion"`
	ID                string          `json:"id"`
	Source            string          `json:"source"`
	Type              string          `json:"type"`
	Subject           string          `json:"subject"`
	Time              time.Time       `json:"time"`
	TenantID          string          `json:"nvutotenantid"`
	AggregateType     string          `json:"nvutoaggregatetype"`
	AggregateRevision int64           `json:"nvutoaggregaterevision"`
	Data              json.RawMessage `json:"data"`
}

type consentEventData struct {
	ID                   string     `json:"id"`
	TenantID             string     `json:"tenant_id"`
	SubjectID            string     `json:"subject_id"`
	Purpose              string     `json:"purpose"`
	Channel              string     `json:"channel"`
	Status               string     `json:"status"`
	ExpiresAt            *time.Time `json:"expires_at,omitempty"`
	PreferenceGeneration int64      `json:"preference_generation"`
	AggregateRevision    int64      `json:"aggregate_revision"`
}

type suppressionEventData struct {
	ID        string `json:"id"`
	TenantID  string `json:"tenant_id"`
	SubjectID string `json:"subject_id"`
	Scope     string `json:"scope"`
	Purpose   string `json:"purpose"`
	Channel   string `json:"channel"`
	Active    bool   `json:"active"`
	Version   int64  `json:"version"`
}

// Local projection cached by downstream consumers.
type localConsent struct {
	Status    string
	Revision  int64
	ExpiresAt time.Time
}

type localSuppression struct {
	Active   bool
	Revision int64
}

// PreSendEvaluationMode determines how the consumer performs pre-send checks.
type PreSendEvaluationMode int

const (
	ModeSynchronousAPI PreSendEvaluationMode = iota
	ModeLocalReplica
)

// SimulatedConsumer represents a downstream product system (e.g. Nvuto CRM, Contact Center)
// that consumes NATS JetStream events and enforces pre-send consent evaluation.
type SimulatedConsumer struct {
	Name            string
	apiURL          string
	httpClient      *http.Client
	injectedOutage  bool
	injectedTimeout bool

	mu           sync.RWMutex
	consents     map[string]localConsent     // key: tenant/subject/purpose/channel
	suppressions map[string]localSuppression // key: tenant/subject/scope/purpose/channel
	staleEvents  int
}

func newSimulatedConsumer(name, apiURL string) *SimulatedConsumer {
	return &SimulatedConsumer{
		Name:         name,
		apiURL:       apiURL,
		httpClient:   &http.Client{Timeout: 3 * time.Second},
		consents:     make(map[string]localConsent),
		suppressions: make(map[string]localSuppression),
	}
}

// HandleEvent applies CloudEvents according to the reconciliation contract (T-010 / ARC-005):
// 1. Monotonic revision check (Rule 1 & Rule 2): if R_event <= R_local, discard immediately.
// 2. Safe-deny / idempotent state updates.
func (c *SimulatedConsumer) HandleEvent(msg *nats.Msg) {
	_ = msg.Ack()

	var env cloudEventEnvelope
	if err := json.Unmarshal(msg.Data, &env); err != nil {
		return
	}

	// Read revision from envelope or transport header
	rev := env.AggregateRevision
	if rev <= 0 {
		if rawHdr := msg.Header.Get("X-Nvuto-Aggregate-Revision"); rawHdr != "" {
			rev, _ = strconv.ParseInt(rawHdr, 10, 64)
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	switch env.Type {
	case "consent.granted", "consent.denied", "consent.withdrawn":
		var data consentEventData
		if err := json.Unmarshal(env.Data, &data); err != nil {
			return
		}
		if rev <= 0 {
			rev = data.AggregateRevision
			if rev <= 0 {
				rev = data.PreferenceGeneration
			}
		}

		key := fmt.Sprintf("%s/%s/%s/%s", env.TenantID, data.SubjectID, data.Purpose, data.Channel)
		existing, found := c.consents[key]

		// Rule 1: Monotonic Revision Check (The Out-of-Order Discard Invariant)
		if found && rev <= existing.Revision {
			c.staleEvents++
			return // Discard stale or duplicate event
		}

		expiresAt := time.Time{}
		if data.ExpiresAt != nil {
			expiresAt = *data.ExpiresAt
		}

		c.consents[key] = localConsent{
			Status:    data.Status,
			Revision:  rev,
			ExpiresAt: expiresAt,
		}

	case "consent.suppression.changed":
		var data suppressionEventData
		if err := json.Unmarshal(env.Data, &data); err != nil {
			return
		}
		if rev <= 0 {
			rev = data.Version
		}

		key := fmt.Sprintf("%s/%s/%s/%s/%s", env.TenantID, data.SubjectID, data.Scope, data.Purpose, data.Channel)
		existing, found := c.suppressions[key]
		if found && rev <= existing.Revision {
			c.staleEvents++
			return
		}

		c.suppressions[key] = localSuppression{
			Active:   data.Active,
			Revision: rev,
		}
	}
}

// CanCommunicate performs the mandatory Pre-Send Consent Check.
// Under ANY failure, timeout, or uncertainty, it strictly enforces the Fail-Closed (Safe-Deny) rule.
func (c *SimulatedConsumer) CanCommunicate(ctx context.Context, tenantID, subjectID, purpose, channel string, mode PreSendEvaluationMode) (bool, string, error) {
	if c.injectedOutage {
		// Rule 5: Safe-Deny on Failure or Uncertainty
		return false, "service_unavailable_fail_closed", nil
	}

	if mode == ModeSynchronousAPI {
		return c.evaluateViaAPI(ctx, tenantID, subjectID, purpose, channel)
	}

	return c.evaluateViaLocalReplica(tenantID, subjectID, purpose, channel)
}

func (c *SimulatedConsumer) evaluateViaAPI(ctx context.Context, tenantID, subjectID, purpose, channel string) (bool, string, error) {
	if c.injectedTimeout {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 1*time.Millisecond)
		defer cancel()
		time.Sleep(5 * time.Millisecond)
	}

	reqBody, _ := json.Marshal(map[string]any{
		"subject_id": subjectID,
		"purpose":    purpose,
		"channel":    channel,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+"/v1/evaluate", bytes.NewReader(reqBody))
	if err != nil {
		return false, "service_unavailable_fail_closed", nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nvuto-Tenant-ID", tenantID)
	req.Header.Set("X-Nvuto-Actor-ID", "consumer-"+c.Name)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Network timeout, connection failure, DNS error -> strictly Fail-Closed
		return false, "service_unavailable_fail_closed", nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return false, "service_unavailable_fail_closed", nil
	}

	var decision struct {
		Allowed   bool   `json:"allowed"`
		Reason    string `json:"reason"`
		ReceiptID string `json:"receipt_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decision); err != nil {
		return false, "service_unavailable_fail_closed", nil
	}

	if !decision.Allowed {
		return false, decision.Reason, nil
	}
	return true, "allowed", nil
}

func (c *SimulatedConsumer) evaluateViaLocalReplica(tenantID, subjectID, purpose, channel string) (bool, string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// 1. Check active suppressions
	globalKey := fmt.Sprintf("%s/%s/global//", tenantID, subjectID)
	if sup, ok := c.suppressions[globalKey]; ok && sup.Active {
		return false, "suppressed", nil
	}
	channelKey := fmt.Sprintf("%s/%s/channel//%s", tenantID, subjectID, channel)
	if sup, ok := c.suppressions[channelKey]; ok && sup.Active {
		return false, "suppressed", nil
	}
	specificKey := fmt.Sprintf("%s/%s/specific/%s/%s", tenantID, subjectID, purpose, channel)
	if sup, ok := c.suppressions[specificKey]; ok && sup.Active {
		return false, "suppressed", nil
	}

	// 2. Check local consent projection
	key := fmt.Sprintf("%s/%s/%s/%s", tenantID, subjectID, purpose, channel)
	consent, ok := c.consents[key]
	if !ok {
		return false, "not_found", nil
	}

	if consent.Status != "granted" {
		return false, consent.Status, nil
	}

	// 3. Rule 4: Passive wall-clock expiry check (Zero Expiry Events)
	if !consent.ExpiresAt.IsZero() && time.Now().UTC().After(consent.ExpiresAt) {
		return false, "expired", nil
	}

	return true, "allowed", nil
}

func (c *SimulatedConsumer) HasConsent(tenantID, subjectID, purpose, channel string) (localConsent, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	key := fmt.Sprintf("%s/%s/%s/%s", tenantID, subjectID, purpose, channel)
	val, ok := c.consents[key]
	return val, ok
}

// e2eCluster holds handles to the test topology.
type e2eCluster struct {
	apiURL        string
	databaseURL   string
	natsURL       string
	streamName    string
	subjectPrefix string
	nc            *nats.Conn
	js            nats.JetStreamContext
	serverCmd     *exec.Cmd
	workerCmd     *exec.Cmd
}

var (
	buildLock        sync.Mutex
	serverBinaryPath string
	workerBinaryPath string
)

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repository root containing go.mod")
		}
		dir = parent
	}
}

func ensureBinaries(t *testing.T, repoRoot string) (string, string) {
	t.Helper()
	buildLock.Lock()
	defer buildLock.Unlock()

	if serverBinaryPath != "" && workerBinaryPath != "" {
		return serverBinaryPath, workerBinaryPath
	}

	binDir := filepath.Join(repoRoot, "build-tmp")
	_ = os.MkdirAll(binDir, 0755)

	serverName := "server"
	workerName := "worker"
	if runtime.GOOS == "windows" {
		serverName += ".exe"
		workerName += ".exe"
	}
	sBin := filepath.Join(binDir, serverName)
	wBin := filepath.Join(binDir, workerName)

	buildEnv := append(os.Environ(), "GOTMPDIR="+binDir)

	cmdServer := exec.Command("go", "build", "-o", sBin, "./cmd/server")
	cmdServer.Dir = repoRoot
	cmdServer.Env = buildEnv
	if out, err := cmdServer.CombinedOutput(); err != nil {
		t.Fatalf("failed to compile server binary: %v\n%s", err, string(out))
	}

	cmdWorker := exec.Command("go", "build", "-o", wBin, "./cmd/outbox-worker")
	cmdWorker.Dir = repoRoot
	cmdWorker.Env = buildEnv
	if out, err := cmdWorker.CombinedOutput(); err != nil {
		t.Fatalf("failed to compile outbox-worker binary: %v\n%s", err, string(out))
	}

	serverBinaryPath = sBin
	workerBinaryPath = wBin
	return sBin, wBin
}

func getFreePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to acquire free TCP port: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func waitForReady(t *testing.T, url string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 500 * time.Millisecond}
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			return
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("service at %s failed to become ready within %v", url, timeout)
}

func setupE2ECluster(t *testing.T) *e2eCluster {
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

	// Verify database connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		if strings.EqualFold(os.Getenv("NVUTO_REQUIRE_INTEGRATION"), "true") {
			t.Fatalf("database connection failed: %v", err)
		}
		t.Skipf("database connection failed (%v); skipping e2e tests", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		if strings.EqualFold(os.Getenv("NVUTO_REQUIRE_INTEGRATION"), "true") {
			t.Fatalf("database ping failed: %v", err)
		}
		t.Skipf("database ping failed (%v); skipping e2e tests", err)
	}
	pool.Close()

	// Verify NATS connectivity
	nc, err := nats.Connect(natsURL, nats.Timeout(3*time.Second))
	if err != nil {
		if strings.EqualFold(os.Getenv("NVUTO_REQUIRE_INTEGRATION"), "true") {
			t.Fatalf("NATS connection failed: %v", err)
		}
		t.Skipf("NATS connection failed (%v); skipping e2e tests", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		t.Fatalf("JetStream context failed: %v", err)
	}

	repoRoot := findRepoRoot(t)
	apiURL := strings.TrimSpace(os.Getenv("CONSENT_API_URL"))

	var serverCmd, workerCmd *exec.Cmd
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	streamName := "NVUTO_E2E_" + suffix
	subjectPrefix := "nvuto.e2e." + suffix

	if apiURL == "" {
		serverBin, workerBin := ensureBinaries(t, repoRoot)
		serverPort := getFreePort(t)
		workerHealthPort := getFreePort(t)

		apiURL = fmt.Sprintf("http://127.0.0.1:%d", serverPort)

		serverCmd = exec.Command(serverBin)
		serverCmd.Dir = repoRoot
		serverCmd.Env = append(os.Environ(),
			fmt.Sprintf("PORT=%d", serverPort),
			"DATABASE_URL="+dbURL,
			"ALLOW_DEV_HEADERS=true",
			"ALLOW_INSECURE_NATS=true",
			"ENVIRONMENT=test",
			"REQUEST_TIMEOUT=5s",
		)
		serverCmd.Stdout = os.Stdout
		serverCmd.Stderr = os.Stderr
		if err := serverCmd.Start(); err != nil {
			nc.Close()
			t.Fatalf("failed to start server process: %v", err)
		}

		workerCmd = exec.Command(workerBin)
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
			"OUTBOX_POLL_INTERVAL=200ms",
		)
		workerCmd.Stdout = os.Stdout
		workerCmd.Stderr = os.Stderr
		if err := workerCmd.Start(); err != nil {
			_ = serverCmd.Process.Kill()
			nc.Close()
			t.Fatalf("failed to start worker process: %v", err)
		}

		waitForReady(t, fmt.Sprintf("http://127.0.0.1:%d/readyz", serverPort), 10*time.Second)
		waitForReady(t, fmt.Sprintf("http://127.0.0.1:%d/readyz", workerHealthPort), 10*time.Second)
	}

	cluster := &e2eCluster{
		apiURL:        apiURL,
		databaseURL:   dbURL,
		natsURL:       natsURL,
		streamName:    streamName,
		subjectPrefix: subjectPrefix,
		nc:            nc,
		js:            js,
		serverCmd:     serverCmd,
		workerCmd:     workerCmd,
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
	})

	return cluster
}

func doHTTP(t *testing.T, client *http.Client, method, url, tenantID string, body any) (*http.Response, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = bytes.NewReader(encoded)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Nvuto-Tenant-ID", tenantID)
	req.Header.Set("X-Nvuto-Actor-ID", "e2e-test-admin")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s request failed: %v", method, url, err)
	}
	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, respBytes
}

func setupTenantPolicy(t *testing.T, client *http.Client, apiURL, tenantID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)

	// Register Purpose
	resp, body := doHTTP(t, client, http.MethodPost, apiURL+"/v1/purposes", tenantID, map[string]any{
		"code":                         "marketing",
		"category":                     "marketing",
		"allowed_channels":             []string{"email", "sms", "voice"},
		"requires_affirmative_consent": true,
		"requires_double_opt_in":       false,
		"active":                       true,
	})
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("register purpose failed (%d): %s", resp.StatusCode, string(body))
	}

	// Register Lawful Basis
	resp, body = doHTTP(t, client, http.MethodPost, apiURL+"/v1/lawful-bases", tenantID, map[string]any{
		"code":               "consent",
		"allowed_categories": []string{"marketing"},
		"requires_evidence":  true,
		"active":             true,
	})
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("register lawful basis failed (%d): %s", resp.StatusCode, string(body))
	}

	// Publish Notice
	resp, body = doHTTP(t, client, http.MethodPost, apiURL+"/v1/notices", tenantID, map[string]any{
		"version":      "2026-10",
		"jurisdiction": "NG",
		"published_at": now,
		"active":       true,
		"content":      "Standard privacy notice for E2E verification",
	})
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("publish notice failed (%d): %s", resp.StatusCode, string(body))
	}
}

// TestE2E_ConsumerIntegration_FullWorkflow tests the canonical positive integration path:
// 1. Policy setup & consent record via HTTP API.
// 2. Transactional outbox write & worker publish to NATS JetStream.
// 3. Consumer subscription event reception & local projection update.
// 4. Pre-send consent check (both API evaluate and local replica) returning allowed=true.
func TestE2E_ConsumerIntegration_FullWorkflow(t *testing.T) {
	cluster := setupE2ECluster(t)
	tenantID := fmt.Sprintf("tenant-e2e-crm-%d", time.Now().UnixNano())
	client := &http.Client{Timeout: 5 * time.Second}

	setupTenantPolicy(t, client, cluster.apiURL, tenantID)

	// Instantiate simulated Nvuto CRM Consumer client
	consumer := newSimulatedConsumer("nvuto-crm", cluster.apiURL)

	// Subscribe to NATS events
	sub, err := cluster.nc.Subscribe(cluster.subjectPrefix+".>", consumer.HandleEvent)
	if err != nil {
		t.Fatalf("failed to subscribe consumer to NATS: %v", err)
	}
	defer sub.Unsubscribe()

	// Canonical Customer Graph ID for Nvuto CRM contact
	subjectID := "cgraph:" + tenantID + ":individual:crm_contact_90210"

	// Create consent record via API
	now := time.Now().UTC()
	recordReq := map[string]any{
		"subject_id":     subjectID,
		"purpose":        "marketing",
		"channel":        "email",
		"legal_basis":    "consent",
		"status":         "granted",
		"source":         "crm_opt_in_form",
		"jurisdiction":   "NG",
		"notice_version": "2026-10",
		"evidence": map[string]any{
			"method":        "web_form",
			"artifact_hash": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			"collected_at":  now.Format(time.RFC3339),
		},
	}

	resp, body := doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantID, recordReq)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("record consent failed (%d): %s", resp.StatusCode, string(body))
	}

	var createdConsent struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &createdConsent)
	if createdConsent.ID == "" {
		t.Fatal("expected non-empty consent ID from API")
	}

	// Wait for event propagation to consumer local replica (SLA < 2.0s)
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if c, ok := consumer.HasConsent(tenantID, subjectID, "marketing", "email"); ok && c.Status == "granted" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	c, ok := consumer.HasConsent(tenantID, subjectID, "marketing", "email")
	if !ok || c.Status != "granted" {
		t.Fatalf("consumer failed to receive granted event from NATS within timeout; ok=%v, status=%s", ok, c.Status)
	}

	// Execute Pre-Send Consent Checks
	// Option A: Direct Synchronous API check
	allowedAPI, reasonAPI, err := consumer.CanCommunicate(context.Background(), tenantID, subjectID, "marketing", "email", ModeSynchronousAPI)
	if err != nil || !allowedAPI {
		t.Fatalf("pre-send API check rejected valid consent: allowed=%v, reason=%s, err=%v", allowedAPI, reasonAPI, err)
	}

	// Option B: Local Replica evaluation
	allowedReplica, reasonReplica, err := consumer.CanCommunicate(context.Background(), tenantID, subjectID, "marketing", "email", ModeLocalReplica)
	if err != nil || !allowedReplica {
		t.Fatalf("pre-send local replica check rejected valid consent: allowed=%v, reason=%s, err=%v", allowedReplica, reasonReplica, err)
	}
}

// TestE2E_ConsumerIntegration_NegativePaths verifies that all 6 negative conditions
// actively block consumer communications:
// 1. No consent record exists.
// 2. Explicit customer denial (opt-out).
// 3. Customer consent withdrawal.
// 4. Active suppression.
// 5. Passive wall-clock expiry.
// 6. Service unavailability / timeout (Fail-Closed).
func TestE2E_ConsumerIntegration_NegativePaths(t *testing.T) {
	cluster := setupE2ECluster(t)
	tenantID := fmt.Sprintf("tenant-e2e-neg-%d", time.Now().UnixNano())
	client := &http.Client{Timeout: 5 * time.Second}

	setupTenantPolicy(t, client, cluster.apiURL, tenantID)

	consumer := newSimulatedConsumer("nvuto-contact-center", cluster.apiURL)
	sub, err := cluster.nc.Subscribe(cluster.subjectPrefix+".>", consumer.HandleEvent)
	if err != nil {
		t.Fatalf("failed to subscribe consumer to NATS: %v", err)
	}
	defer sub.Unsubscribe()

	now := time.Now().UTC()

	t.Run("NegativePath_1_NoConsent", func(t *testing.T) {
		unknownSubject := "cgraph:" + tenantID + ":individual:unconsented_lead_999"

		// Pre-send check must block communication
		allowedAPI, reasonAPI, _ := consumer.CanCommunicate(context.Background(), tenantID, unknownSubject, "marketing", "sms", ModeSynchronousAPI)
		if allowedAPI {
			t.Errorf("expected no-consent subject to be blocked in API check, got allowed=true (reason=%s)", reasonAPI)
		}

		allowedReplica, reasonReplica, _ := consumer.CanCommunicate(context.Background(), tenantID, unknownSubject, "marketing", "sms", ModeLocalReplica)
		if allowedReplica {
			t.Errorf("expected no-consent subject to be blocked in replica check, got allowed=true (reason=%s)", reasonReplica)
		}
	})

	t.Run("NegativePath_2_ExplicitDenial", func(t *testing.T) {
		deniedSubject := "cgraph:" + tenantID + ":individual:denied_customer_101"

		recordReq := map[string]any{
			"subject_id":     deniedSubject,
			"purpose":        "marketing",
			"channel":        "voice",
			"legal_basis":    "consent",
			"status":         "denied",
			"source":         "ivr_opt_out",
			"jurisdiction":   "NG",
			"notice_version": "2026-10",
			"evidence": map[string]any{
				"method":        "ivr_key_press",
				"artifact_hash": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
				"collected_at":  now.Format(time.RFC3339),
			},
		}

		resp, body := doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantID, recordReq)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("record denial failed (%d): %s", resp.StatusCode, string(body))
		}

		// Await event delivery
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			if c, ok := consumer.HasConsent(tenantID, deniedSubject, "marketing", "voice"); ok && c.Status == "denied" {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}

		allowedAPI, reasonAPI, _ := consumer.CanCommunicate(context.Background(), tenantID, deniedSubject, "marketing", "voice", ModeSynchronousAPI)
		if allowedAPI {
			t.Errorf("expected explicit denial to block API check, got allowed=true (reason=%s)", reasonAPI)
		}

		allowedReplica, reasonReplica, _ := consumer.CanCommunicate(context.Background(), tenantID, deniedSubject, "marketing", "voice", ModeLocalReplica)
		if allowedReplica {
			t.Errorf("expected explicit denial to block replica check, got allowed=true (reason=%s)", reasonReplica)
		}
	})

	t.Run("NegativePath_3_Withdrawal", func(t *testing.T) {
		withdrawnSubject := "cgraph:" + tenantID + ":individual:withdrawn_customer_202"

		// 1. Initial Grant
		recordReq := map[string]any{
			"subject_id":     withdrawnSubject,
			"purpose":        "marketing",
			"channel":        "email",
			"legal_basis":    "consent",
			"status":         "granted",
			"source":         "portal",
			"jurisdiction":   "NG",
			"notice_version": "2026-10",
			"evidence": map[string]any{
				"method":        "web_click",
				"artifact_hash": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
				"collected_at":  now.Format(time.RFC3339),
			},
		}

		resp, body := doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantID, recordReq)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("initial grant failed (%d): %s", resp.StatusCode, string(body))
		}
		var grant struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(body, &grant)

		// Await grant in consumer
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			if c, ok := consumer.HasConsent(tenantID, withdrawnSubject, "marketing", "email"); ok && c.Status == "granted" {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}

		// 2. Withdraw Consent
		resp, body = doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents/"+grant.ID+"/withdraw", tenantID, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("withdrawal failed (%d): %s", resp.StatusCode, string(body))
		}

		// Await withdrawal event in consumer
		deadline = time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			if c, ok := consumer.HasConsent(tenantID, withdrawnSubject, "marketing", "email"); ok && c.Status == "withdrawn" {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}

		c, _ := consumer.HasConsent(tenantID, withdrawnSubject, "marketing", "email")
		if c.Status != "withdrawn" {
			t.Fatalf("expected status=withdrawn in replica, got %s", c.Status)
		}

		// Pre-send check must block communication
		allowedAPI, reasonAPI, _ := consumer.CanCommunicate(context.Background(), tenantID, withdrawnSubject, "marketing", "email", ModeSynchronousAPI)
		if allowedAPI {
			t.Errorf("expected withdrawn consent to block API check, got allowed=true (reason=%s)", reasonAPI)
		}

		allowedReplica, reasonReplica, _ := consumer.CanCommunicate(context.Background(), tenantID, withdrawnSubject, "marketing", "email", ModeLocalReplica)
		if allowedReplica {
			t.Errorf("expected withdrawn consent to block replica check, got allowed=true (reason=%s)", reasonReplica)
		}
	})

	t.Run("NegativePath_4_ActiveSuppression", func(t *testing.T) {
		suppressedSubject := "cgraph:" + tenantID + ":individual:suppressed_customer_303"

		// 1. Initial Grant
		recordReq := map[string]any{
			"subject_id":     suppressedSubject,
			"purpose":        "marketing",
			"channel":        "sms",
			"legal_basis":    "consent",
			"status":         "granted",
			"source":         "customer_care",
			"jurisdiction":   "NG",
			"notice_version": "2026-10",
			"evidence": map[string]any{
				"method":        "agent_assisted",
				"artifact_hash": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
				"collected_at":  now.Format(time.RFC3339),
			},
		}

		resp, body := doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantID, recordReq)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("initial grant failed (%d): %s", resp.StatusCode, string(body))
		}

		// 2. Set Global Suppression
		supReq := map[string]any{
			"subject_id": suppressedSubject,
			"scope":      "global",
			"reason":     "Customer requested global contact suppression",
			"active":     true,
		}
		resp, body = doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/suppressions", tenantID, supReq)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create suppression failed (%d): %s", resp.StatusCode, string(body))
		}

		// Await suppression event in consumer
		deadline := time.Now().Add(4 * time.Second)
		supKey := fmt.Sprintf("%s/%s/global//", tenantID, suppressedSubject)
		for time.Now().Before(deadline) {
			consumer.mu.RLock()
			s, ok := consumer.suppressions[supKey]
			consumer.mu.RUnlock()
			if ok && s.Active {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}

		// Pre-send check must block communication despite granted consent
		allowedAPI, reasonAPI, _ := consumer.CanCommunicate(context.Background(), tenantID, suppressedSubject, "marketing", "sms", ModeSynchronousAPI)
		if allowedAPI {
			t.Errorf("expected active suppression to block API check, got allowed=true (reason=%s)", reasonAPI)
		}

		allowedReplica, reasonReplica, _ := consumer.CanCommunicate(context.Background(), tenantID, suppressedSubject, "marketing", "sms", ModeLocalReplica)
		if allowedReplica {
			t.Errorf("expected active suppression to block replica check, got allowed=true (reason=%s)", reasonReplica)
		}
	})

	t.Run("NegativePath_5_WallClockExpiry", func(t *testing.T) {
		expiredSubject := "cgraph:" + tenantID + ":individual:expired_customer_404"

		// Grant consent that expires 500ms in the future
		shortExpiry := time.Now().UTC().Add(500 * time.Millisecond)
		recordReq := map[string]any{
			"subject_id":     expiredSubject,
			"purpose":        "marketing",
			"channel":        "email",
			"legal_basis":    "consent",
			"status":         "granted",
			"source":         "temporary_campaign",
			"jurisdiction":   "NG",
			"notice_version": "2026-10",
			"expires_at":     shortExpiry.Format(time.RFC3339Nano),
			"evidence": map[string]any{
				"method":        "promo_qr",
				"artifact_hash": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
				"collected_at":  now.Format(time.RFC3339),
			},
		}

		resp, body := doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantID, recordReq)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create expiring consent failed (%d): %s", resp.StatusCode, string(body))
		}

		// Await event in replica
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			if _, ok := consumer.HasConsent(tenantID, expiredSubject, "marketing", "email"); ok {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}

		// Wait past wall-clock expiry time
		time.Sleep(800 * time.Millisecond)

		// Note: Zero expiry events are emitted by design (Rule 4).
		// Both the API evaluate endpoint and local replica must passively evaluate wall-clock expiry.
		allowedAPI, reasonAPI, _ := consumer.CanCommunicate(context.Background(), tenantID, expiredSubject, "marketing", "email", ModeSynchronousAPI)
		if allowedAPI {
			t.Errorf("expected expired consent to block API check, got allowed=true (reason=%s)", reasonAPI)
		}

		allowedReplica, reasonReplica, _ := consumer.CanCommunicate(context.Background(), tenantID, expiredSubject, "marketing", "email", ModeLocalReplica)
		if allowedReplica {
			t.Errorf("expected expired consent to block replica check, got allowed=true (reason=%s)", reasonReplica)
		}
	})

	t.Run("NegativePath_6_ServiceUnavailability_FailClosed", func(t *testing.T) {
		validSubject := "cgraph:" + tenantID + ":individual:valid_customer_505"

		// Grant valid consent
		recordReq := map[string]any{
			"subject_id":     validSubject,
			"purpose":        "marketing",
			"channel":        "email",
			"legal_basis":    "consent",
			"status":         "granted",
			"source":         "portal",
			"jurisdiction":   "NG",
			"notice_version": "2026-10",
			"evidence": map[string]any{
				"method":        "web_click",
				"artifact_hash": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
				"collected_at":  now.Format(time.RFC3339),
			},
		}
		resp, body := doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantID, recordReq)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("grant failed (%d): %s", resp.StatusCode, string(body))
		}

		// Inject simulated service unavailability (Rule 5: Fail-Closed)
		consumer.injectedOutage = true
		defer func() { consumer.injectedOutage = false }()

		allowed, reason, err := consumer.CanCommunicate(context.Background(), tenantID, validSubject, "marketing", "email", ModeSynchronousAPI)
		if allowed {
			t.Errorf("FAIL-CLOSED VIOLATION: communication was permitted during service outage! allowed=%v", allowed)
		}
		if reason != "service_unavailable_fail_closed" {
			t.Errorf("expected fail-closed reason 'service_unavailable_fail_closed', got %s", reason)
		}

		// Inject HTTP timeout
		consumer.injectedOutage = false
		consumer.injectedTimeout = true
		defer func() { consumer.injectedTimeout = false }()

		allowedTimeout, reasonTimeout, _ := consumer.CanCommunicate(context.Background(), tenantID, validSubject, "marketing", "email", ModeSynchronousAPI)
		if allowedTimeout {
			t.Errorf("FAIL-CLOSED VIOLATION: communication was permitted during client timeout! allowed=%v", allowedTimeout)
		}
		if reasonTimeout != "service_unavailable_fail_closed" {
			t.Errorf("expected timeout fail-closed reason 'service_unavailable_fail_closed', got %s", reasonTimeout)
		}
		_ = err
	})
}

// TestE2E_ConsumerIntegration_PropagationTiming proves that a consent withdrawal
// propagates end-to-end (API -> PostgreSQL -> outbox worker -> NATS JetStream -> consumer replica)
// and actively blocks communication within the approved propagation budget (< 2.0 seconds).
func TestE2E_ConsumerIntegration_PropagationTiming(t *testing.T) {
	cluster := setupE2ECluster(t)
	tenantID := fmt.Sprintf("tenant-e2e-timing-%d", time.Now().UnixNano())
	client := &http.Client{Timeout: 5 * time.Second}

	setupTenantPolicy(t, client, cluster.apiURL, tenantID)

	consumer := newSimulatedConsumer("nvuto-portal", cluster.apiURL)
	sub, err := cluster.nc.Subscribe(cluster.subjectPrefix+".>", consumer.HandleEvent)
	if err != nil {
		t.Fatalf("failed to subscribe consumer to NATS: %v", err)
	}
	defer sub.Unsubscribe()

	subjectID := "cgraph:" + tenantID + ":individual:timing_customer_777"
	now := time.Now().UTC()

	// Initial grant
	recordReq := map[string]any{
		"subject_id":     subjectID,
		"purpose":        "marketing",
		"channel":        "email",
		"legal_basis":    "consent",
		"status":         "granted",
		"source":         "timing_bench",
		"jurisdiction":   "NG",
		"notice_version": "2026-10",
		"evidence": map[string]any{
			"method":        "bench",
			"artifact_hash": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			"collected_at":  now.Format(time.RFC3339),
		},
	}

	resp, body := doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantID, recordReq)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("grant failed (%d): %s", resp.StatusCode, string(body))
	}
	var grant struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &grant)

	// Wait until consumer replica reflects the grant
	grantDeadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(grantDeadline) {
		if c, ok := consumer.HasConsent(tenantID, subjectID, "marketing", "email"); ok && c.Status == "granted" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Verify communication is currently allowed
	allowed, _, _ := consumer.CanCommunicate(context.Background(), tenantID, subjectID, "marketing", "email", ModeLocalReplica)
	if !allowed {
		t.Fatal("expected communication to be allowed prior to withdrawal timing test")
	}

	// --- START PROPAGATION TIMING BENCHMARK ---
	t0 := time.Now()

	// Execute withdrawal via HTTP API
	resp, body = doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents/"+grant.ID+"/withdraw", tenantID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("withdrawal failed (%d): %s", resp.StatusCode, string(body))
	}

	// Poll consumer replica until communication is actively blocked
	const maxBudget = 2 * time.Second
	pollDeadline := time.Now().Add(maxBudget)
	var propagated bool
	var elapsed time.Duration

	for time.Now().Before(pollDeadline) {
		blocked, _, _ := consumer.CanCommunicate(context.Background(), tenantID, subjectID, "marketing", "email", ModeLocalReplica)
		if !blocked {
			// Successfully blocked!
			elapsed = time.Since(t0)
			propagated = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if !propagated {
		t.Fatalf("PROPAGATION SLA VIOLATION: withdrawal failed to block consumer within approved budget of %v", maxBudget)
	}

	t.Logf("Measured End-to-End Consent Withdrawal Propagation Latency: %v (< %v budget)", elapsed, maxBudget)
	if elapsed >= maxBudget {
		t.Fatalf("measured latency %v exceeded approved propagation budget %v", elapsed, maxBudget)
	}
}

// TestE2E_ConsumerIntegration_ReverseDeliveryTrap proves Rule 1 & Rule 2:
// When events arrive out of order (e.g. withdrawal with revision 2 arrives BEFORE grant with revision 1),
// the consumer discards the delayed grant and remains in withdrawn state (Safe-Deny preserved).
func TestE2E_ConsumerIntegration_ReverseDeliveryTrap(t *testing.T) {
	consumer := newSimulatedConsumer("nvuto-crm-ordering", "http://127.0.0.1:9999")
	tenantID := "tenant-trap-001"
	subjectID := "cgraph:trap:usr_99"

	// Simulate Event 2 arriving first: consent.withdrawn with Revision = 2
	withdrawnMsg := &nats.Msg{
		Header: nats.Header{
			"X-Nvuto-Tenant-ID":          []string{tenantID},
			"X-Nvuto-Aggregate-Revision": []string{"2"},
			"X-Nvuto-Aggregate-Type":     []string{"consent.preference"},
		},
	}
	withdrawnEvent := cloudEventEnvelope{
		SpecVersion:       "1.0",
		Type:              "consent.withdrawn",
		TenantID:          tenantID,
		AggregateRevision: 2,
		Data: []byte(fmt.Sprintf(`{
			"tenant_id": "%s",
			"subject_id": "%s",
			"purpose": "marketing",
			"channel": "email",
			"status": "withdrawn",
			"preference_generation": 2,
			"aggregate_revision": 2
		}`, tenantID, subjectID)),
	}
	withdrawnMsg.Data, _ = json.Marshal(withdrawnEvent)

	consumer.HandleEvent(withdrawnMsg)

	// Verify state is withdrawn
	c, ok := consumer.HasConsent(tenantID, subjectID, "marketing", "email")
	if !ok || c.Status != "withdrawn" || c.Revision != 2 {
		t.Fatalf("unexpected state after event 2: ok=%v, status=%s, rev=%d", ok, c.Status, c.Revision)
	}

	// Now simulate Event 1 arriving LATER (the Reverse-Delivery Trap): consent.granted with Revision = 1
	delayedGrantedMsg := &nats.Msg{
		Header: nats.Header{
			"X-Nvuto-Tenant-ID":          []string{tenantID},
			"X-Nvuto-Aggregate-Revision": []string{"1"},
			"X-Nvuto-Aggregate-Type":     []string{"consent.preference"},
		},
	}
	delayedGrantedEvent := cloudEventEnvelope{
		SpecVersion:       "1.0",
		Type:              "consent.granted",
		TenantID:          tenantID,
		AggregateRevision: 1,
		Data: []byte(fmt.Sprintf(`{
			"tenant_id": "%s",
			"subject_id": "%s",
			"purpose": "marketing",
			"channel": "email",
			"status": "granted",
			"preference_generation": 1,
			"aggregate_revision": 1
		}`, tenantID, subjectID)),
	}
	delayedGrantedMsg.Data, _ = json.Marshal(delayedGrantedEvent)

	consumer.HandleEvent(delayedGrantedMsg)

	// Assert delayed event was discarded: status MUST remain "withdrawn"
	c, ok = consumer.HasConsent(tenantID, subjectID, "marketing", "email")
	if !ok || c.Status != "withdrawn" || c.Revision != 2 {
		t.Fatalf("REVERSE DELIVERY TRAP DETECTED: delayed grant resurrected permission! status=%s, rev=%d", c.Status, c.Revision)
	}

	if consumer.staleEvents != 1 {
		t.Fatalf("expected staleEvents count 1, got %d", consumer.staleEvents)
	}

	// Verify pre-send check remains blocked
	allowed, reason, _ := consumer.CanCommunicate(context.Background(), tenantID, subjectID, "marketing", "email", ModeLocalReplica)
	if allowed {
		t.Errorf("expected communication to be blocked after reverse-delivery trap test, got allowed=true (reason=%s)", reason)
	}
}
