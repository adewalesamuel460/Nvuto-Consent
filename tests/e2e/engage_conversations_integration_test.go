package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
)

// -----------------------------------------------------------------------------
// Cluster Test Harness for Engage (INT-01) and Conversations (INT-06)
// -----------------------------------------------------------------------------

type engageConversationsCluster struct {
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

func setupEngageConversationsCluster(t *testing.T) *engageConversationsCluster {
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

	// Verify NATS connectivity
	nc, err := nats.Connect(natsURL, nats.Timeout(3*time.Second))
	if err != nil {
		pool.Close()
		if strings.EqualFold(os.Getenv("NVUTO_REQUIRE_INTEGRATION"), "true") {
			t.Fatalf("NATS connection failed: %v", err)
		}
		t.Skipf("NATS connection failed (%v); skipping e2e tests", err)
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
	streamName := "NVUTO_ENGCONV_" + suffix
	subjectPrefix := "nvuto.consent.engconv." + suffix

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
		"OUTBOX_POLL_INTERVAL=100ms",
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

	cluster := &engageConversationsCluster{
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

func setupEngagePolicy(t *testing.T, client *http.Client, apiURL, tenantID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)

	// Purpose with email, sms, and whatsapp channels
	resp, body := doHTTP(t, client, http.MethodPost, apiURL+"/v1/purposes", tenantID, map[string]any{
		"code":                         "marketing",
		"category":                     "marketing",
		"allowed_channels":             []string{"email", "sms", "whatsapp", "voice"},
		"requires_affirmative_consent": true,
		"requires_double_opt_in":       false,
		"active":                       true,
	})
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("register purpose failed (%d): %s", resp.StatusCode, string(body))
	}

	// Lawful Basis
	resp, body = doHTTP(t, client, http.MethodPost, apiURL+"/v1/lawful-bases", tenantID, map[string]any{
		"code":               "consent",
		"allowed_categories": []string{"marketing"},
		"requires_evidence":  true,
		"active":             true,
	})
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("register lawful basis failed (%d): %s", resp.StatusCode, string(body))
	}

	// Privacy Notice
	resp, body = doHTTP(t, client, http.MethodPost, apiURL+"/v1/notices", tenantID, map[string]any{
		"version":      "2026-10",
		"jurisdiction": "NG",
		"published_at": now,
		"active":       true,
		"content":      "Standard privacy notice for Engage & Conversations E2E test",
	})
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("publish notice failed (%d): %s", resp.StatusCode, string(body))
	}
}

// -----------------------------------------------------------------------------
// INT-06: Conversations Simulated Real-Time Messaging Consumer
// -----------------------------------------------------------------------------

type OutboundMessage struct {
	SubjectID string
	Purpose   string
	Channel   string
	Content   string
	SentAt    time.Time
}

type DroppedMessage struct {
	SubjectID string
	Purpose   string
	Channel   string
	Content   string
	Reason    string
	DroppedAt time.Time
}

type ConversationsConsumer struct {
	apiURL          string
	httpClient      *http.Client
	injectedOutage  bool
	injectedTimeout bool

	mu              sync.Mutex
	sentMessages    []OutboundMessage
	droppedMessages []DroppedMessage
}

func newConversationsConsumer(apiURL string) *ConversationsConsumer {
	return &ConversationsConsumer{
		apiURL:     apiURL,
		httpClient: &http.Client{Timeout: 3 * time.Second},
	}
}

// CanSend simulates the real-time pre-send check calling POST /v1/consents/evaluate.
// Under ANY outage, timeout, or 503/500, it enforces strict Fail-Closed (Safe-Deny).
func (c *ConversationsConsumer) CanSend(ctx context.Context, tenantID, subjectID, purpose, channel string) (bool, string, error) {
	if c.injectedOutage {
		return false, "service_unavailable_fail_closed", nil
	}

	evalCtx := ctx
	if c.injectedTimeout {
		var cancel context.CancelFunc
		evalCtx, cancel = context.WithTimeout(ctx, 1*time.Millisecond)
		defer cancel()
		time.Sleep(5 * time.Millisecond)
	}

	reqBody, _ := json.Marshal(map[string]any{
		"subject_id": subjectID,
		"purpose":    purpose,
		"channel":    channel,
	})

	req, err := http.NewRequestWithContext(evalCtx, http.MethodPost, c.apiURL+"/v1/consents/evaluate", bytes.NewReader(reqBody))
	if err != nil {
		return false, "service_unavailable_fail_closed", nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nvuto-Tenant-ID", tenantID)
	req.Header.Set("X-Nvuto-Actor-ID", "conversations-service")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Strict Fail-Closed: connection error, timeout, server down
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

	return decision.Allowed, decision.Reason, nil
}

// AttemptSend simulates the outbound send boundary for SMS or WhatsApp.
// If CanSend is false, the message is immediately dropped and recorded in droppedMessages.
func (c *ConversationsConsumer) AttemptSend(ctx context.Context, tenantID, subjectID, purpose, channel, content string) (bool, string, error) {
	allowed, reason, err := c.CanSend(ctx, tenantID, subjectID, purpose, channel)
	c.mu.Lock()
	defer c.mu.Unlock()

	if !allowed || err != nil {
		c.droppedMessages = append(c.droppedMessages, DroppedMessage{
			SubjectID: subjectID,
			Purpose:   purpose,
			Channel:   channel,
			Content:   content,
			Reason:    reason,
			DroppedAt: time.Now().UTC(),
		})
		return false, reason, err
	}

	c.sentMessages = append(c.sentMessages, OutboundMessage{
		SubjectID: subjectID,
		Purpose:   purpose,
		Channel:   channel,
		Content:   content,
		SentAt:    time.Now().UTC(),
	})
	return true, reason, nil
}

// -----------------------------------------------------------------------------
// INT-01: Engage Marketing Automation Platform Batch Consumer
// -----------------------------------------------------------------------------

type CampaignBlastResult struct {
	TotalAudience     int
	SentRecipients    []string
	DroppedRecipients map[string]string // subjectID -> reason
	AbortedDueToError bool
}

type EngageConsumer struct {
	apiURL          string
	httpClient      *http.Client
	injectedOutage  bool
	injectedTimeout bool
}

func newEngageConsumer(apiURL string) *EngageConsumer {
	return &EngageConsumer{
		apiURL:     apiURL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// FilterAudience performs pre-send batch evaluation calling POST /v1/consents/evaluate-batch.
// It accepts 100+ subjects simultaneously and enforces strict Fail-Closed on failure.
func (e *EngageConsumer) FilterAudience(ctx context.Context, tenantID, purpose, channel string, subjects []string) ([]string, map[string]string, error) {
	if e.injectedOutage {
		dropped := make(map[string]string, len(subjects))
		for _, s := range subjects {
			dropped[s] = "service_unavailable_fail_closed"
		}
		return nil, dropped, errors.New("service_unavailable_fail_closed")
	}

	evalCtx := ctx
	if e.injectedTimeout {
		var cancel context.CancelFunc
		evalCtx, cancel = context.WithTimeout(ctx, 1*time.Millisecond)
		defer cancel()
		time.Sleep(5 * time.Millisecond)
	}

	reqs := make([]map[string]any, len(subjects))
	for i, s := range subjects {
		reqs[i] = map[string]any{
			"subject_id": s,
			"purpose":    purpose,
			"channel":    channel,
		}
	}

	reqBody, _ := json.Marshal(map[string]any{"requests": reqs})
	httpReq, err := http.NewRequestWithContext(evalCtx, http.MethodPost, e.apiURL+"/v1/consents/evaluate-batch", bytes.NewReader(reqBody))
	if err != nil {
		dropped := make(map[string]string, len(subjects))
		for _, s := range subjects {
			dropped[s] = "service_unavailable_fail_closed"
		}
		return nil, dropped, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Nvuto-Tenant-ID", tenantID)
	httpReq.Header.Set("X-Nvuto-Actor-ID", "engage-marketing-engine")

	resp, err := e.httpClient.Do(httpReq)
	if err != nil {
		// Strict Fail-Closed on network failure, timeout, or unreachable service
		dropped := make(map[string]string, len(subjects))
		for _, s := range subjects {
			dropped[s] = "service_unavailable_fail_closed"
		}
		return nil, dropped, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		dropped := make(map[string]string, len(subjects))
		for _, s := range subjects {
			dropped[s] = "service_unavailable_fail_closed"
		}
		return nil, dropped, fmt.Errorf("server error status %d", resp.StatusCode)
	}

	var batchResp struct {
		Decisions []struct {
			Allowed   bool   `json:"allowed"`
			Reason    string `json:"reason"`
			ReceiptID string `json:"receipt_id"`
		} `json:"decisions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&batchResp); err != nil {
		dropped := make(map[string]string, len(subjects))
		for _, s := range subjects {
			dropped[s] = "service_unavailable_fail_closed"
		}
		return nil, dropped, err
	}

	var allowed []string
	dropped := make(map[string]string)
	for i, dec := range batchResp.Decisions {
		subj := subjects[i]
		if dec.Allowed {
			allowed = append(allowed, subj)
		} else {
			dropped[subj] = dec.Reason
		}
	}
	return allowed, dropped, nil
}

// ExecuteCampaignBlast simulates the audience generation and outbound blast boundary.
func (e *EngageConsumer) ExecuteCampaignBlast(ctx context.Context, tenantID, campaignID, purpose, channel string, audience []string) (CampaignBlastResult, error) {
	allowed, dropped, err := e.FilterAudience(ctx, tenantID, purpose, channel, audience)
	if err != nil {
		return CampaignBlastResult{
			TotalAudience:     len(audience),
			SentRecipients:    nil,
			DroppedRecipients: dropped,
			AbortedDueToError: true,
		}, err
	}

	sent := make([]string, len(allowed))
	copy(sent, allowed)

	return CampaignBlastResult{
		TotalAudience:     len(audience),
		SentRecipients:    sent,
		DroppedRecipients: dropped,
		AbortedDueToError: false,
	}, nil
}

// -----------------------------------------------------------------------------
// T-010: Monotonic Revision Subscriber & Local Replica (Engage / Conversations)
// -----------------------------------------------------------------------------

type MonotonicReplicaSubscriber struct {
	mu              sync.RWMutex
	consents        map[string]localConsent     // key: tenant/subject/purpose/channel
	suppressions    map[string]localSuppression // key: tenant/subject/scope/purpose/channel
	staleDiscards   int
	processedEvents int
	lastReceivedAt  time.Time
}

func newMonotonicReplicaSubscriber() *MonotonicReplicaSubscriber {
	return &MonotonicReplicaSubscriber{
		consents:     make(map[string]localConsent),
		suppressions: make(map[string]localSuppression),
	}
}

func (s *MonotonicReplicaSubscriber) HandleMsg(msg *nats.Msg) {
	_ = msg.Ack()

	var env cloudEventEnvelope
	if err := json.Unmarshal(msg.Data, &env); err != nil {
		return
	}

	rev := env.AggregateRevision
	if rev <= 0 {
		if rawHdr := msg.Header.Get("X-Nvuto-Aggregate-Revision"); rawHdr != "" {
			rev, _ = strconv.ParseInt(rawHdr, 10, 64)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastReceivedAt = time.Now().UTC()

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
		existing, found := s.consents[key]

		// Rule 1: Monotonic Revision Check (T-010 Out-of-Order Discard Invariant)
		if found && rev <= existing.Revision {
			s.staleDiscards++
			return // Strictly discard stale or duplicate event
		}

		expiresAt := time.Time{}
		if data.ExpiresAt != nil {
			expiresAt = *data.ExpiresAt
		}

		s.consents[key] = localConsent{
			Status:    data.Status,
			Revision:  rev,
			ExpiresAt: expiresAt,
		}
		s.processedEvents++

	case "consent.suppression.changed", "consent.suppression.created":
		var data suppressionEventData
		if err := json.Unmarshal(env.Data, &data); err != nil {
			return
		}
		if rev <= 0 {
			rev = data.Version
		}

		key := fmt.Sprintf("%s/%s/%s/%s/%s", env.TenantID, data.SubjectID, data.Scope, data.Purpose, data.Channel)
		existing, found := s.suppressions[key]
		if found && rev <= existing.Revision {
			s.staleDiscards++
			return // Discard stale suppression update
		}

		s.suppressions[key] = localSuppression{
			Active:   data.Active,
			Revision: rev,
		}
		s.processedEvents++
	}
}

func (s *MonotonicReplicaSubscriber) EvaluateLocal(tenantID, subjectID, purpose, channel string) (bool, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 1. Check active suppressions
	globalKey := fmt.Sprintf("%s/%s/global//", tenantID, subjectID)
	if sup, ok := s.suppressions[globalKey]; ok && sup.Active {
		return false, "suppressed"
	}
	channelKey := fmt.Sprintf("%s/%s/channel//%s", tenantID, subjectID, channel)
	if sup, ok := s.suppressions[channelKey]; ok && sup.Active {
		return false, "suppressed"
	}
	specificKey := fmt.Sprintf("%s/%s/specific/%s/%s", tenantID, subjectID, purpose, channel)
	if sup, ok := s.suppressions[specificKey]; ok && sup.Active {
		return false, "suppressed"
	}

	// 2. Check local consent projection
	key := fmt.Sprintf("%s/%s/%s/%s", tenantID, subjectID, purpose, channel)
	consent, ok := s.consents[key]
	if !ok {
		return false, "no_consent"
	}
	if consent.Status != "granted" {
		return false, consent.Status
	}
	if !consent.ExpiresAt.IsZero() && time.Now().UTC().After(consent.ExpiresAt) {
		return false, "expired"
	}

	return true, "granted"
}

// -----------------------------------------------------------------------------
// Master E2E Integration Suite: Engage & Conversations
// -----------------------------------------------------------------------------

func TestEngageConversations_Integration(t *testing.T) {
	cluster := setupEngageConversationsCluster(t)
	tenantID := fmt.Sprintf("tenant-e2e-engconv-%d", time.Now().UnixNano())
	client := &http.Client{Timeout: 5 * time.Second}

	// Bootstrap tenant policy
	setupEngagePolicy(t, client, cluster.apiURL, tenantID)

	// =========================================================================
	// Subtest 1: Conversations (INT-06) Single-Send Strict Fail-Closed
	// =========================================================================
	t.Run("Conversations_INT06_SingleSend_FailClosed", func(t *testing.T) {
		conv := newConversationsConsumer(cluster.apiURL)
		ctx := context.Background()

		// Condition 1: No consent record exists -> message dropped
		t.Run("Condition_1_NoConsentExists_RefusesSend", func(t *testing.T) {
			subjectNoConsent := fmt.Sprintf("cgraph:%s:individual:conv_noconsent_%d", tenantID, time.Now().UnixNano())
			sent, reason, err := conv.AttemptSend(ctx, tenantID, subjectNoConsent, "marketing", "sms", "Promotion SMS")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sent {
				t.Fatalf("VIOLATION: Conversations dispatched SMS when no consent record exists")
			}
			if reason != "no_consent" {
				t.Fatalf("expected reason 'no_consent', got %q", reason)
			}
			t.Logf("Confirmed: SMS dropped with reason=%s for subject with no consent record", reason)
		})

		// Condition 2: Consent is denied or withdrawn -> message dropped
		t.Run("Condition_2_ConsentDeniedOrWithdrawn_RefusesSend", func(t *testing.T) {
			// Part 2a: Explicitly denied consent
			subjectDenied := fmt.Sprintf("cgraph:%s:individual:conv_denied_%d", tenantID, time.Now().UnixNano())
			now := time.Now().UTC()
			resp, body := doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantID, map[string]any{
				"subject_id":     subjectDenied,
				"purpose":        "marketing",
				"channel":        "whatsapp",
				"legal_basis":    "consent",
				"status":         "denied",
				"source":         "conversations_inbound_stop",
				"jurisdiction":   "NG",
				"notice_version": "2026-10",
				"evidence": map[string]any{
					"method":        "sms_reply",
					"artifact_hash": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
					"collected_at":  now.Format(time.RFC3339),
				},
			})
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("failed to record denied consent (%d): %s", resp.StatusCode, string(body))
			}

			sent, reason, err := conv.AttemptSend(ctx, tenantID, subjectDenied, "marketing", "whatsapp", "Hello WhatsApp")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sent {
				t.Fatalf("VIOLATION: Conversations dispatched WhatsApp message when consent is denied")
			}
			if reason != "denied" {
				t.Fatalf("expected reason 'denied', got %q", reason)
			}
			t.Logf("Confirmed: WhatsApp message dropped with reason=%s for denied subject", reason)

			// Part 2b: Granted then withdrawn consent
			subjectWithdrawn := fmt.Sprintf("cgraph:%s:individual:conv_withdrawn_%d", tenantID, time.Now().UnixNano())
			resp, body = doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantID, map[string]any{
				"subject_id":     subjectWithdrawn,
				"purpose":        "marketing",
				"channel":        "sms",
				"legal_basis":    "consent",
				"status":         "granted",
				"source":         "web_opt_in",
				"jurisdiction":   "NG",
				"notice_version": "2026-10",
				"evidence": map[string]any{
					"method":        "web_click",
					"artifact_hash": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
					"collected_at":  now.Format(time.RFC3339),
				},
			})
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("failed to record granted consent (%d): %s", resp.StatusCode, string(body))
			}
			var created struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(body, &created)

			// Withdraw consent
			resp, body = doHTTP(t, client, http.MethodPost, fmt.Sprintf("%s/v1/consents/%s/withdraw", cluster.apiURL, created.ID), tenantID, nil)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("failed to withdraw consent (%d): %s", resp.StatusCode, string(body))
			}

			sent, reason, err = conv.AttemptSend(ctx, tenantID, subjectWithdrawn, "marketing", "sms", "SMS after withdrawal")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sent {
				t.Fatalf("VIOLATION: Conversations dispatched SMS when consent is withdrawn")
			}
			if reason != "withdrawn" {
				t.Fatalf("expected reason 'withdrawn', got %q", reason)
			}
			t.Logf("Confirmed: SMS dropped with reason=%s for withdrawn subject", reason)
		})

		// Condition 3: Consent has expired -> message dropped
		t.Run("Condition_3_ConsentExpired_RefusesSend", func(t *testing.T) {
			subjectExpired := fmt.Sprintf("cgraph:%s:individual:conv_expired_%d", tenantID, time.Now().UnixNano())
			now := time.Now().UTC()
			resp, body := doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantID, map[string]any{
				"subject_id":     subjectExpired,
				"purpose":        "marketing",
				"channel":        "whatsapp",
				"legal_basis":    "consent",
				"status":         "granted",
				"source":         "web_opt_in",
				"jurisdiction":   "NG",
				"notice_version": "2026-10",
				"evidence": map[string]any{
					"method":        "web_click",
					"artifact_hash": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
					"collected_at":  now.Format(time.RFC3339),
				},
			})
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("failed to record consent (%d): %s", resp.StatusCode, string(body))
			}
			var created struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(body, &created)

			// Backdate expires_at and created_at in the database to satisfy chk_consents_temporal_expiry (expires_at > created_at)
			_, err := cluster.dbPool.Exec(ctx, "UPDATE consents SET created_at = NOW() - INTERVAL '2 hours', expires_at = NOW() - INTERVAL '1 hour' WHERE id = $1", created.ID)
			if err != nil {
				t.Fatalf("failed to backdate expires_at: %v", err)
			}

			sent, reason, err := conv.AttemptSend(ctx, tenantID, subjectExpired, "marketing", "whatsapp", "WhatsApp after expiry")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sent {
				t.Fatalf("VIOLATION: Conversations dispatched WhatsApp message when consent has expired")
			}
			if reason != "expired" {
				t.Fatalf("expected reason 'expired', got %q", reason)
			}
			t.Logf("Confirmed: WhatsApp message dropped with reason=%s for expired consent", reason)
		})

		// Condition 4: Subject exists in global or channel-specific suppression list -> message dropped
		t.Run("Condition_4_SubjectSuppressed_RefusesSend", func(t *testing.T) {
			subjectSuppressed := fmt.Sprintf("cgraph:%s:individual:conv_suppressed_%d", tenantID, time.Now().UnixNano())
			now := time.Now().UTC()

			// Grant consent first
			resp, body := doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantID, map[string]any{
				"subject_id":     subjectSuppressed,
				"purpose":        "marketing",
				"channel":        "sms",
				"legal_basis":    "consent",
				"status":         "granted",
				"source":         "web_opt_in",
				"jurisdiction":   "NG",
				"notice_version": "2026-10",
				"evidence": map[string]any{
					"method":        "web_click",
					"artifact_hash": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
					"collected_at":  now.Format(time.RFC3339),
				},
			})
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("failed to record consent (%d): %s", resp.StatusCode, string(body))
			}

			// Add channel-specific suppression (purpose must be empty for channel scope)
			resp, body = doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/suppressions", tenantID, map[string]any{
				"subject_id": subjectSuppressed,
				"scope":      "channel",
				"purpose":    "",
				"channel":    "sms",
				"reason":     "dnc_telecom_registry",
				"active":     true,
			})
			if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
				t.Fatalf("failed to add channel suppression (%d): %s", resp.StatusCode, string(body))
			}

			sent, reason, err := conv.AttemptSend(ctx, tenantID, subjectSuppressed, "marketing", "sms", "SMS to suppressed user")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sent {
				t.Fatalf("VIOLATION: Conversations dispatched SMS when subject is suppressed")
			}
			if reason != "suppressed" && !strings.HasSuffix(reason, "_suppression") {
				t.Fatalf("expected reason 'suppressed' or '*_suppression', got %q", reason)
			}
			t.Logf("Confirmed: SMS dropped with reason=%s for channel-suppressed subject", reason)
		})

		// Condition 5: API unavailable / 503 / timeout -> fail-closed, refuses send
		t.Run("Condition_5_APIOutageOrTimeout_StrictFailClosed", func(t *testing.T) {
			subjectOutage := fmt.Sprintf("cgraph:%s:individual:conv_outage_%d", tenantID, time.Now().UnixNano())

			// 5a. Injected client timeout (1ms)
			convTimeout := newConversationsConsumer(cluster.apiURL)
			convTimeout.injectedTimeout = true

			sent, reason, _ := convTimeout.AttemptSend(ctx, tenantID, subjectOutage, "marketing", "sms", "Timeout send")
			if sent {
				t.Fatalf("VIOLATION: Conversations defaulted to allow during timeout")
			}
			if reason != "service_unavailable_fail_closed" {
				t.Fatalf("expected reason 'service_unavailable_fail_closed', got %q", reason)
			}
			t.Logf("Confirmed: Timeout resulted in fail-closed drop (reason=%s)", reason)

			// 5b. Injected outage flag
			convOutage := newConversationsConsumer(cluster.apiURL)
			convOutage.injectedOutage = true

			sent, reason, _ = convOutage.AttemptSend(ctx, tenantID, subjectOutage, "marketing", "sms", "Outage send")
			if sent {
				t.Fatalf("VIOLATION: Conversations defaulted to allow during injected outage")
			}
			if reason != "service_unavailable_fail_closed" {
				t.Fatalf("expected reason 'service_unavailable_fail_closed', got %q", reason)
			}
			t.Logf("Confirmed: Injected outage resulted in fail-closed drop (reason=%s)", reason)

			// 5c. Deliberate unreachable endpoint (closed port simulating crashed API server)
			convDeadServer := newConversationsConsumer("http://127.0.0.1:59999")
			sent, reason, _ = convDeadServer.AttemptSend(ctx, tenantID, subjectOutage, "marketing", "sms", "Offline server send")
			if sent {
				t.Fatalf("VIOLATION: Conversations defaulted to allow when API server is stopped/unreachable")
			}
			if reason != "service_unavailable_fail_closed" {
				t.Fatalf("expected reason 'service_unavailable_fail_closed', got %q", reason)
			}
			t.Logf("Confirmed: Dead server resulted in fail-closed drop (reason=%s)", reason)
		})

		// Positive Control: Valid consent allows send
		t.Run("PositiveControl_ActiveConsent_AllowsSend", func(t *testing.T) {
			subjectActive := fmt.Sprintf("cgraph:%s:individual:conv_active_%d", tenantID, time.Now().UnixNano())
			now := time.Now().UTC()
			resp, body := doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantID, map[string]any{
				"subject_id":     subjectActive,
				"purpose":        "marketing",
				"channel":        "sms",
				"legal_basis":    "consent",
				"status":         "granted",
				"source":         "web_opt_in",
				"jurisdiction":   "NG",
				"notice_version": "2026-10",
				"evidence": map[string]any{
					"method":        "web_click",
					"artifact_hash": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
					"collected_at":  now.Format(time.RFC3339),
				},
			})
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("failed to record consent (%d): %s", resp.StatusCode, string(body))
			}

			sent, reason, err := conv.AttemptSend(ctx, tenantID, subjectActive, "marketing", "sms", "Hello active user!")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !sent {
				t.Fatalf("expected active consent to allow send, got reason=%s", reason)
			}
			if reason != "granted" {
				t.Fatalf("expected reason 'granted', got %q", reason)
			}
			t.Logf("Confirmed: SMS dispatched successfully with reason=%s", reason)
		})
	})

	// =========================================================================
	// Subtest 2: Engage (INT-01) 100+ Subjects Batch Evaluation & Fail-Closed
	// =========================================================================
	t.Run("Engage_INT01_BatchBlast_FailClosed", func(t *testing.T) {
		engage := newEngageConsumer(cluster.apiURL)
		ctx := context.Background()

		// Generate an audience of 120 subjects (> 100 entries):
		// - 30 Active granted subjects (0..29)
		// - 20 No consent subjects (30..49)
		// - 20 Denied subjects (50..69)
		// - 20 Withdrawn subjects (70..89)
		// - 15 Expired subjects (90..104)
		// - 15 Suppressed subjects (105..119)
		const totalAudience = 120
		audience := make([]string, totalAudience)
		for i := 0; i < totalAudience; i++ {
			audience[i] = fmt.Sprintf("cgraph:%s:individual:engage_aud_%03d_%d", tenantID, i, time.Now().UnixNano())
		}

		now := time.Now().UTC()
		evidence := map[string]any{
			"method":        "marketing_portal",
			"artifact_hash": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			"collected_at":  now.Format(time.RFC3339),
		}

		// 1. Seed Active Granted (0..29)
		t.Log("Seeding 30 active granted subjects...")
		for i := 0; i < 30; i++ {
			resp, body := doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantID, map[string]any{
				"subject_id":     audience[i],
				"purpose":        "marketing",
				"channel":        "email",
				"legal_basis":    "consent",
				"status":         "granted",
				"source":         "campaign_optin",
				"jurisdiction":   "NG",
				"notice_version": "2026-10",
				"evidence":       evidence,
			})
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("failed to seed granted subject %d: %s", i, string(body))
			}
		}

		// 2. No consent records for 30..49 (remain unseeded)

		// 3. Seed Denied Subjects (50..69)
		t.Log("Seeding 20 explicitly denied subjects...")
		for i := 50; i < 70; i++ {
			resp, body := doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantID, map[string]any{
				"subject_id":     audience[i],
				"purpose":        "marketing",
				"channel":        "email",
				"legal_basis":    "consent",
				"status":         "denied",
				"source":         "campaign_optin",
				"jurisdiction":   "NG",
				"notice_version": "2026-10",
				"evidence":       evidence,
			})
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("failed to seed denied subject %d: %s", i, string(body))
			}
		}

		// 4. Seed Withdrawn Subjects (70..89)
		t.Log("Seeding 20 granted then withdrawn subjects...")
		for i := 70; i < 90; i++ {
			resp, body := doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantID, map[string]any{
				"subject_id":     audience[i],
				"purpose":        "marketing",
				"channel":        "email",
				"legal_basis":    "consent",
				"status":         "granted",
				"source":         "campaign_optin",
				"jurisdiction":   "NG",
				"notice_version": "2026-10",
				"evidence":       evidence,
			})
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("failed to seed consent for subject %d: %s", i, string(body))
			}
			var c struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(body, &c)
			// Withdraw
			resp, body = doHTTP(t, client, http.MethodPost, fmt.Sprintf("%s/v1/consents/%s/withdraw", cluster.apiURL, c.ID), tenantID, nil)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("failed to withdraw consent for subject %d: %s", i, string(body))
			}
		}

		// 5. Seed Expired Subjects (90..104)
		t.Log("Seeding 15 expired subjects...")
		for i := 90; i < 105; i++ {
			resp, body := doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantID, map[string]any{
				"subject_id":     audience[i],
				"purpose":        "marketing",
				"channel":        "email",
				"legal_basis":    "consent",
				"status":         "granted",
				"source":         "campaign_optin",
				"jurisdiction":   "NG",
				"notice_version": "2026-10",
				"evidence":       evidence,
			})
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("failed to seed consent for subject %d: %s", i, string(body))
			}
			var c struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(body, &c)
			_, err := cluster.dbPool.Exec(ctx, "UPDATE consents SET created_at = NOW() - INTERVAL '3 hours', expires_at = NOW() - INTERVAL '2 hours' WHERE id = $1", c.ID)
			if err != nil {
				t.Fatalf("failed to backdate expires_at for subject %d: %v", i, err)
			}
		}

		// 6. Seed Suppressed Subjects (105..119)
		t.Log("Seeding 15 suppressed subjects...")
		for i := 105; i < 120; i++ {
			// Grant consent first
			resp, body := doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantID, map[string]any{
				"subject_id":     audience[i],
				"purpose":        "marketing",
				"channel":        "email",
				"legal_basis":    "consent",
				"status":         "granted",
				"source":         "campaign_optin",
				"jurisdiction":   "NG",
				"notice_version": "2026-10",
				"evidence":       evidence,
			})
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("failed to seed consent for subject %d: %s", i, string(body))
			}
			// Add suppression (purpose must be empty for channel scope)
			resp, body = doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/suppressions", tenantID, map[string]any{
				"subject_id": audience[i],
				"scope":      "channel",
				"purpose":    "",
				"channel":    "email",
				"reason":     "global_opt_out",
				"active":     true,
			})
			if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
				t.Fatalf("failed to add suppression for subject %d: %s", i, string(body))
			}
		}

		// Execute Batch Evaluation via POST /v1/consents/evaluate-batch for all 120 subjects simultaneously
		t.Log("Engage calling POST /v1/consents/evaluate-batch for 120 subjects simultaneously...")
		blastResult, err := engage.ExecuteCampaignBlast(ctx, tenantID, "blast-q4-holiday", "marketing", "email", audience)
		if err != nil {
			t.Fatalf("ExecuteCampaignBlast failed: %v", err)
		}

		if blastResult.TotalAudience != 120 {
			t.Fatalf("expected total audience 120, got %d", blastResult.TotalAudience)
		}
		if len(blastResult.SentRecipients) != 30 {
			t.Fatalf("VIOLATION: expected exactly 30 sent recipients, got %d", len(blastResult.SentRecipients))
		}
		if len(blastResult.DroppedRecipients) != 90 {
			t.Fatalf("VIOLATION: expected exactly 90 dropped recipients, got %d", len(blastResult.DroppedRecipients))
		}

		// Verify 30 sent recipients match audience 0..29
		for i := 0; i < 30; i++ {
			expectedSubject := audience[i]
			found := false
			for _, s := range blastResult.SentRecipients {
				if s == expectedSubject {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("expected subject %s to be in sent recipients, but was dropped", expectedSubject)
			}
		}

		// Verify dropped reasons for 30..49 (no_consent)
		for i := 30; i < 50; i++ {
			subj := audience[i]
			reason, ok := blastResult.DroppedRecipients[subj]
			if !ok || reason != "no_consent" {
				t.Errorf("subject %s (no consent): expected drop reason 'no_consent', got %q", subj, reason)
			}
		}

		// Verify dropped reasons for 50..69 (denied)
		for i := 50; i < 70; i++ {
			subj := audience[i]
			reason, ok := blastResult.DroppedRecipients[subj]
			if !ok || reason != "denied" {
				t.Errorf("subject %s (denied): expected drop reason 'denied', got %q", subj, reason)
			}
		}

		// Verify dropped reasons for 70..89 (withdrawn)
		for i := 70; i < 90; i++ {
			subj := audience[i]
			reason, ok := blastResult.DroppedRecipients[subj]
			if !ok || reason != "withdrawn" {
				t.Errorf("subject %s (withdrawn): expected drop reason 'withdrawn', got %q", subj, reason)
			}
		}

		// Verify dropped reasons for 90..104 (expired)
		for i := 90; i < 105; i++ {
			subj := audience[i]
			reason, ok := blastResult.DroppedRecipients[subj]
			if !ok || reason != "expired" {
				t.Errorf("subject %s (expired): expected drop reason 'expired', got %q", subj, reason)
			}
		}

		// Verify dropped reasons for 105..119 (suppressed)
		for i := 105; i < 120; i++ {
			subj := audience[i]
			reason, ok := blastResult.DroppedRecipients[subj]
			if !ok || (reason != "suppressed" && !strings.HasSuffix(reason, "_suppression")) {
				t.Errorf("subject %s (suppressed): expected drop reason 'suppressed' or '*_suppression', got %q", subj, reason)
			}
		}
		t.Logf("Confirmed: 120-recipient batch successfully filtered with zero illegal sends (30 sent, 90 dropped)")

		// Verify Engage Fail-Closed on Outage / Timeout
		t.Run("Engage_Outage_StrictFailClosed_AbortsBlast", func(t *testing.T) {
			engageOutage := newEngageConsumer(cluster.apiURL)
			engageOutage.injectedOutage = true

			outageResult, err := engageOutage.ExecuteCampaignBlast(ctx, tenantID, "blast-outage-test", "marketing", "email", audience)
			if err == nil {
				t.Fatal("expected error during campaign blast with API outage, got nil")
			}
			if !outageResult.AbortedDueToError {
				t.Fatal("expected campaign blast to be marked AbortedDueToError")
			}
			if len(outageResult.SentRecipients) != 0 {
				t.Fatalf("VIOLATION: Engage sent %d messages during API outage! Must be 0.", len(outageResult.SentRecipients))
			}
			if len(outageResult.DroppedRecipients) != 120 {
				t.Fatalf("expected all 120 recipients to be dropped during outage, got %d", len(outageResult.DroppedRecipients))
			}
			for _, s := range audience {
				if r := outageResult.DroppedRecipients[s]; r != "service_unavailable_fail_closed" {
					t.Fatalf("expected drop reason 'service_unavailable_fail_closed', got %q for %s", r, s)
				}
			}
			t.Log("Confirmed: API outage caused Engage to abort blast and safely drop 100% of audience")
		})
	})

	// =========================================================================
	// Subtest 3: T-010 JetStream Consumption & Monotonic Revisions
	// =========================================================================
	t.Run("JetStream_T010_MonotonicRevisions_OutOfOrderRejection", func(t *testing.T) {
		subscriber := newMonotonicReplicaSubscriber()

		// Subscribe to JetStream stream events
		sub, err := cluster.nc.Subscribe(cluster.subjectPrefix+".>", subscriber.HandleMsg)
		if err != nil {
			t.Fatalf("failed to subscribe to NATS: %v", err)
		}
		defer sub.Unsubscribe()

		subjectOOO := fmt.Sprintf("cgraph:%s:individual:ooo_user_%d", tenantID, time.Now().UnixNano())
		now := time.Now().UTC()
		evidence := map[string]any{
			"method":        "web_click",
			"artifact_hash": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			"collected_at":  now.Format(time.RFC3339),
		}

		// Step 1: Record initial consent.granted (Revision 1)
		t.Log("Step 1: Recording consent.granted (Revision 1)...")
		resp, body := doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantID, map[string]any{
			"subject_id":     subjectOOO,
			"purpose":        "marketing",
			"channel":        "sms",
			"legal_basis":    "consent",
			"status":         "granted",
			"source":         "web_optin",
			"jurisdiction":   "NG",
			"notice_version": "2026-10",
			"evidence":       evidence,
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("failed to record consent (%d): %s", resp.StatusCode, string(body))
		}
		var createdConsent struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(body, &createdConsent)

		// Wait for Revision 1 event to arrive at local replica
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			allowed, reason := subscriber.EvaluateLocal(tenantID, subjectOOO, "marketing", "sms")
			if allowed && reason == "granted" {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		allowed, reason := subscriber.EvaluateLocal(tenantID, subjectOOO, "marketing", "sms")
		if !allowed || reason != "granted" {
			t.Fatalf("subscriber failed to converge on Revision 1 (granted); allowed=%v reason=%s", allowed, reason)
		}
		t.Log("Step 1 Confirmed: Subscriber evaluated subject as granted at Revision 1")

		// Step 2: Record consent.withdrawn (Revision 2)
		t.Log("Step 2: Recording consent.withdrawn (Revision 2)...")
		resp, body = doHTTP(t, client, http.MethodPost, fmt.Sprintf("%s/v1/consents/%s/withdraw", cluster.apiURL, createdConsent.ID), tenantID, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("failed to withdraw consent (%d): %s", resp.StatusCode, string(body))
		}

		// Wait for Revision 2 event to arrive at local replica
		deadline = time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			allowed, reason = subscriber.EvaluateLocal(tenantID, subjectOOO, "marketing", "sms")
			if !allowed && reason == "withdrawn" {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		allowed, reason = subscriber.EvaluateLocal(tenantID, subjectOOO, "marketing", "sms")
		if allowed || reason != "withdrawn" {
			t.Fatalf("subscriber failed to converge on Revision 2 (withdrawn); allowed=%v reason=%s", allowed, reason)
		}
		t.Log("Step 2 Confirmed: Subscriber evaluated subject as withdrawn at Revision 2")

		// Step 3: Maliciously replay or inject delayed consent.granted event with Revision 1
		t.Log("Step 3: Injecting delayed/replayed consent.granted event (Revision 1)...")
		stalePayload, _ := json.Marshal(map[string]any{
			"id":                    createdConsent.ID,
			"tenant_id":             tenantID,
			"subject_id":            subjectOOO,
			"purpose":               "marketing",
			"channel":               "sms",
			"status":                "granted",
			"preference_generation": 1,
			"aggregate_revision":    1,
		})

		staleEnv := cloudEventEnvelope{
			SpecVersion:       "1.0",
			ID:                fmt.Sprintf("delayed-replay-%d", time.Now().UnixNano()),
			Source:            "nvuto/consent",
			Type:              "consent.granted",
			Subject:           "consent/" + createdConsent.ID,
			Time:              time.Now().UTC().Add(-10 * time.Minute),
			TenantID:          tenantID,
			AggregateType:     "consent",
			AggregateRevision: 1, // Stale Revision 1!
			Data:              stalePayload,
		}
		staleBody, _ := json.Marshal(staleEnv)

		staleMsg := nats.NewMsg(cluster.subjectPrefix + ".granted")
		staleMsg.Header.Set("X-Nvuto-Aggregate-Revision", "1")
		staleMsg.Header.Set("X-Nvuto-Tenant-ID", tenantID)
		staleMsg.Data = staleBody

		if err := cluster.nc.PublishMsg(staleMsg); err != nil {
			t.Fatalf("failed to publish stale event: %v", err)
		}

		// Wait briefly for subscriber message handler to process
		time.Sleep(200 * time.Millisecond)

		// Assert that the subscriber's reconciliation logic strictly discarded the stale Revision 1 event
		subscriber.mu.RLock()
		staleCount := subscriber.staleDiscards
		localKey := fmt.Sprintf("%s/%s/%s/%s", tenantID, subjectOOO, "marketing", "sms")
		currentConsent := subscriber.consents[localKey]
		subscriber.mu.RUnlock()

		if staleCount == 0 {
			t.Fatalf("VIOLATION: Subscriber failed to increment staleDiscards counter for out-of-order event")
		}
		if currentConsent.Revision != 2 {
			t.Fatalf("VIOLATION: Local replica revision was overwritten by stale event! Revision=%d, want 2", currentConsent.Revision)
		}
		if currentConsent.Status != "withdrawn" {
			t.Fatalf("VIOLATION: Local replica status was reverted to %q by stale Revision 1 event!", currentConsent.Status)
		}

		// Local replica must continue to evaluate subject as withdrawn
		allowedAfterReplay, reasonAfterReplay := subscriber.EvaluateLocal(tenantID, subjectOOO, "marketing", "sms")
		if allowedAfterReplay || reasonAfterReplay != "withdrawn" {
			t.Fatalf("CRITICAL SECURITY VIOLATION: Subject evaluated as allowed=%v (reason=%s) after replaying stale grant! Must remain withdrawn.", allowedAfterReplay, reasonAfterReplay)
		}

		t.Logf("T-010 Verification Success: Out-of-order Revision 1 correctly discarded (discards=%d). Local state remains strictly withdrawn (Revision 2).", staleCount)
	})

	// =========================================================================
	// Subtest 4: Suppression Propagation Latency SLO (< 2.0s)
	// =========================================================================
	t.Run("JetStream_Suppression_Propagation_SLO", func(t *testing.T) {
		subscriber := newMonotonicReplicaSubscriber()

		sub, err := cluster.nc.Subscribe(cluster.subjectPrefix+".>", subscriber.HandleMsg)
		if err != nil {
			t.Fatalf("failed to subscribe to NATS: %v", err)
		}
		defer sub.Unsubscribe()

		subjectSLO := fmt.Sprintf("cgraph:%s:individual:slo_user_%d", tenantID, time.Now().UnixNano())
		now := time.Now().UTC()
		evidence := map[string]any{
			"method":        "web_click",
			"artifact_hash": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			"collected_at":  now.Format(time.RFC3339),
		}

		// Initial grant
		resp, body := doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantID, map[string]any{
			"subject_id":     subjectSLO,
			"purpose":        "marketing",
			"channel":        "sms",
			"legal_basis":    "consent",
			"status":         "granted",
			"source":         "web_optin",
			"jurisdiction":   "NG",
			"notice_version": "2026-10",
			"evidence":       evidence,
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("failed to record consent: %s", string(body))
		}

		// Wait until subscriber sees initial grant
		grantDeadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(grantDeadline) {
			if ok, _ := subscriber.EvaluateLocal(tenantID, subjectSLO, "marketing", "sms"); ok {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if ok, _ := subscriber.EvaluateLocal(tenantID, subjectSLO, "marketing", "sms"); !ok {
			t.Fatal("subscriber failed to hydrate initial grant")
		}

		// Record suppression and measure propagation latency (purpose must be empty for channel scope)
		startTime := time.Now()
		resp, body = doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/suppressions", tenantID, map[string]any{
			"subject_id": subjectSLO,
			"scope":      "channel",
			"purpose":    "",
			"channel":    "sms",
			"reason":     "urgent_regulatory_dnc",
			"active":     true,
		})
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			t.Fatalf("failed to create suppression: %s", string(body))
		}

		// Poll local replica until suppression is recognized
		sloDeadline := time.Now().Add(4 * time.Second)
		var latency time.Duration
		suppressionRecognized := false

		for time.Now().Before(sloDeadline) {
			allowed, reason := subscriber.EvaluateLocal(tenantID, subjectSLO, "marketing", "sms")
			if !allowed && reason == "suppressed" {
				latency = time.Since(startTime)
				suppressionRecognized = true
				break
			}
			time.Sleep(20 * time.Millisecond)
		}

		if !suppressionRecognized {
			t.Fatal("VIOLATION: Suppression event was not recognized by subscriber within timeout")
		}

		const sloBudget = 2 * time.Second
		if latency >= sloBudget {
			t.Fatalf("SLO BREACH: Suppression propagation took %v, exceeding 2.0s budget", latency)
		}

		t.Logf("Suppression SLO Verified: Event propagated through NATS JetStream and evaluated in %v (well below %v SLO budget)", latency, sloBudget)
	})
}
