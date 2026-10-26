package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/auth"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
)

// -----------------------------------------------------------------------------
// Cluster Test Harness for Agent-Assisted Products: Service (INT-02),
// Contact Center (INT-03), and CRM (INT-05)
// -----------------------------------------------------------------------------

type agentAssistedCluster struct {
	apiURL        string
	databaseURL   string
	natsURL       string
	jwtSecret     string
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

func setupAgentAssistedCluster(t *testing.T) *agentAssistedCluster {
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
		t.Skipf("database connection failed (%v); skipping agent-assisted e2e tests", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		if strings.EqualFold(os.Getenv("NVUTO_REQUIRE_INTEGRATION"), "true") {
			t.Fatalf("database ping failed: %v", err)
		}
		t.Skipf("database ping failed (%v); skipping agent-assisted e2e tests", err)
	}

	// Verify NATS connectivity
	nc, err := nats.Connect(natsURL, nats.Timeout(3*time.Second))
	if err != nil {
		pool.Close()
		if strings.EqualFold(os.Getenv("NVUTO_REQUIRE_INTEGRATION"), "true") {
			t.Fatalf("NATS connection failed: %v", err)
		}
		t.Skipf("NATS connection failed (%v); skipping agent-assisted e2e tests", err)
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
	jwtSecret := fmt.Sprintf("agent-assisted-test-secret-%d", time.Now().UnixNano())

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	streamName := "NVUTO_AGENT_" + suffix
	subjectPrefix := "nvuto.agent." + suffix

	serverCmd := exec.Command(serverBin)
	serverCmd.Dir = repoRoot
	serverCmd.Env = append(os.Environ(),
		fmt.Sprintf("PORT=%d", serverPort),
		"DATABASE_URL="+dbURL,
		"ALLOW_DEV_HEADERS=true",
		"ALLOW_HMAC_AUTH=true",
		"JWT_HMAC_SECRET="+jwtSecret,
		"ALLOW_INSECURE_NATS=true",
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

	cluster := &agentAssistedCluster{
		apiURL:        apiURL,
		databaseURL:   dbURL,
		natsURL:       natsURL,
		jwtSecret:     jwtSecret,
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

func setupAgentTenantPrerequisites(t *testing.T, client *http.Client, apiURL, jwtSecret, tenantID string) {
	t.Helper()
	adminToken, err := auth.SignAuthentikToken(jwtSecret, tenantID, "admin-agent", "", []string{
		auth.RolePrivacyAdmin,
		auth.RoleConsentAdmin,
		auth.RoleConsentWriter,
	}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("failed to sign admin token: %v", err)
	}

	// 1. Create Lawful Bases: "consent" covering marketing, service, and support
	lawfulBases := []struct {
		Code       string
		Categories []string
	}{
		{Code: "consent", Categories: []string{"marketing", "service", "support"}},
		{Code: "contract", Categories: []string{"service", "support"}},
	}
	for _, lb := range lawfulBases {
		active := true
		reqEvidence := false
		resp, body := doAuthorizedReq(t, client, http.MethodPost, apiURL+"/v1/lawful-bases", adminToken, map[string]any{
			"code":               lb.Code,
			"allowed_categories": lb.Categories,
			"requires_evidence":  &reqEvidence,
			"active":             &active,
		})
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			t.Fatalf("failed to create lawful basis %s: %d %s", lb.Code, resp.StatusCode, string(body))
		}
	}

	// 2. Create Purposes: marketing, service_notifications, support_outreach
	purposes := []struct {
		Code     string
		Category string
		Channels []string
		Affirm   bool
		DOI      bool
	}{
		{Code: "marketing", Category: "marketing", Channels: []string{"email", "sms", "whatsapp"}, Affirm: true, DOI: false},
		{Code: "service_notifications", Category: "service", Channels: []string{"email", "sms", "whatsapp"}, Affirm: false, DOI: false},
		{Code: "support_outreach", Category: "support", Channels: []string{"call", "email", "sms"}, Affirm: false, DOI: false},
	}
	for _, p := range purposes {
		active := true
		affirm := p.Affirm
		doi := p.DOI
		resp, body := doAuthorizedReq(t, client, http.MethodPost, apiURL+"/v1/purposes", adminToken, map[string]any{
			"code":                         p.Code,
			"category":                     p.Category,
			"allowed_channels":             p.Channels,
			"requires_affirmative_consent": &affirm,
			"requires_double_opt_in":       &doi,
			"active":                       &active,
		})
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			t.Fatalf("failed to create purpose %s: %d %s", p.Code, resp.StatusCode, string(body))
		}
	}

	// 3. Publish Privacy Notice
	activeNotice := true
	resp, body := doAuthorizedReq(t, client, http.MethodPost, apiURL+"/v1/notices", adminToken, map[string]any{
		"jurisdiction": "US-CA",
		"version":      "2026-10",
		"published_at": time.Now().Add(-1 * time.Hour).UTC(),
		"active":       &activeNotice,
		"content":      "Agent-assisted customer privacy policy notice",
	})
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("failed to publish notice: %d %s", resp.StatusCode, string(body))
	}
}

// -----------------------------------------------------------------------------
// TestAgentAssistedIntegration
// E2E Verification Suite for INT-02 (Service), INT-03 (Contact Center), and INT-05 (CRM)
// -----------------------------------------------------------------------------

func TestAgentAssistedIntegration(t *testing.T) {
	cluster := setupAgentAssistedCluster(t)
	client := &http.Client{Timeout: 5 * time.Second}
	tenantID := fmt.Sprintf("tenant-agent-%d", time.Now().UnixNano())

	// Configure prerequisite policy registries (purposes, lawful bases, notices)
	setupAgentTenantPrerequisites(t, client, cluster.apiURL, cluster.jwtSecret, tenantID)

	// Subtest 1: Role-Scoped Access Verification (RBAC) across 4 distinct roles
	t.Run("RBAC_RoleScopedAccess", func(t *testing.T) {
		customerSubject := "cust-rbac-001"

		// 1. Generate JWTs for four distinct roles
		tokenReader, err := auth.SignAuthentikToken(cluster.jwtSecret, tenantID, "agent-tier1-reader", "", []string{auth.RoleConsentReader}, time.Now().Add(time.Hour))
		if err != nil {
			t.Fatalf("failed to sign consent_reader token: %v", err)
		}
		tokenWriter, err := auth.SignAuthentikToken(cluster.jwtSecret, tenantID, "agent-crm-writer", "", []string{auth.RoleConsentWriter}, time.Now().Add(time.Hour))
		if err != nil {
			t.Fatalf("failed to sign consent_writer token: %v", err)
		}
		tokenService, err := auth.SignAuthentikToken(cluster.jwtSecret, tenantID, "service-dispatcher", "", []string{auth.RoleService}, time.Now().Add(time.Hour))
		if err != nil {
			t.Fatalf("failed to sign service token: %v", err)
		}
		tokenAdmin, err := auth.SignAuthentikToken(cluster.jwtSecret, tenantID, "dpo-admin", "", []string{auth.RolePrivacyAdmin}, time.Now().Add(time.Hour))
		if err != nil {
			t.Fatalf("failed to sign privacy_admin token: %v", err)
		}

		// -----------------------------------------------------------------
		// Assertion 1: consent_reader (tier-1 support agent) can evaluate and
		// fetch states, but is strictly DENIED (403 Forbidden) on grant & withdraw.
		// -----------------------------------------------------------------

		// 1a. consent_reader CAN evaluate
		resp, body := doAuthorizedReq(t, client, http.MethodPost, cluster.apiURL+"/v1/consents/evaluate", tokenReader, map[string]any{
			"subject_id": customerSubject,
			"purpose":    "marketing",
			"channel":    "email",
		})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("consent_reader evaluate expected 200 OK, got %d: %s", resp.StatusCode, string(body))
		}

		// 1b. consent_reader CAN fetch consent list/state
		resp, body = doAuthorizedReq(t, client, http.MethodGet, cluster.apiURL+"/v1/consents?subject_id="+customerSubject, tokenReader, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("consent_reader fetch consents expected 200 OK, got %d: %s", resp.StatusCode, string(body))
		}

		// 1c. consent_reader CAN fetch suppression list/state
		resp, body = doAuthorizedReq(t, client, http.MethodGet, cluster.apiURL+"/v1/suppressions?subject_id="+customerSubject, tokenReader, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("consent_reader fetch suppressions expected 200 OK, got %d: %s", resp.StatusCode, string(body))
		}

		// 1d. consent_reader is EXPLICITLY DENIED (403 Forbidden) when attempting to GRANT consent
		resp, body = doAuthorizedReq(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tokenReader, map[string]any{
			"subject_id":     customerSubject,
			"purpose":        "marketing",
			"channel":        "email",
			"legal_basis":    "consent",
			"status":         "granted",
			"source":         "agent_support",
			"jurisdiction":   "US-CA",
			"notice_version": "2026-10",
		})
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("consent_reader grant expected 403 Forbidden, got %d: %s", resp.StatusCode, string(body))
		}
		var errResp map[string]any
		_ = json.Unmarshal(body, &errResp)
		if errResp["error"] != "role_not_authorized" && errResp["code"] != "role_not_authorized" {
			t.Logf("consent_reader grant error response: %s", string(body))
		}

		// 1e. consent_reader is EXPLICITLY DENIED (403 Forbidden) when attempting to WITHDRAW consent
		resp, body = doAuthorizedReq(t, client, http.MethodPost, cluster.apiURL+"/v1/consents/con-dummy-id/withdraw", tokenReader, nil)
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("consent_reader withdraw expected 403 Forbidden, got %d: %s", resp.StatusCode, string(body))
		}

		// 1f. consent_reader is EXPLICITLY DENIED (403 Forbidden) when attempting to set suppressions
		resp, body = doAuthorizedReq(t, client, http.MethodPost, cluster.apiURL+"/v1/suppressions", tokenReader, map[string]any{
			"subject_id": customerSubject,
			"scope":      "channel",
			"channel":    "email",
			"reason":     "reader_optout_attempt",
		})
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("consent_reader set suppression expected 403 Forbidden, got %d: %s", resp.StatusCode, string(body))
		}

		// -----------------------------------------------------------------
		// Assertion 2: service role (automated outbound service) can evaluate
		// and read, but is strictly DENIED (403 Forbidden) on mutations.
		// -----------------------------------------------------------------
		resp, body = doAuthorizedReq(t, client, http.MethodPost, cluster.apiURL+"/v1/consents/evaluate", tokenService, map[string]any{
			"subject_id": customerSubject,
			"purpose":    "marketing",
			"channel":    "email",
		})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("service evaluate expected 200 OK, got %d: %s", resp.StatusCode, string(body))
		}

		resp, body = doAuthorizedReq(t, client, http.MethodPost, cluster.apiURL+"/v1/consents/evaluate-batch", tokenService, map[string]any{
			"requests": []map[string]any{
				{"subject_id": customerSubject, "purpose": "marketing", "channel": "email"},
			},
		})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("service batch evaluate expected 200 OK, got %d: %s", resp.StatusCode, string(body))
		}

		resp, body = doAuthorizedReq(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tokenService, map[string]any{
			"subject_id":     customerSubject,
			"purpose":        "marketing",
			"channel":        "email",
			"legal_basis":    "consent",
			"status":         "granted",
			"source":         "service_automated",
			"jurisdiction":   "US-CA",
			"notice_version": "2026-10",
		})
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("service grant expected 403 Forbidden, got %d: %s", resp.StatusCode, string(body))
		}

		// -----------------------------------------------------------------
		// Assertion 3: consent_writer (CRM user) CAN grant and withdraw on
		// behalf of customers, but cannot mutate administrative policy.
		// -----------------------------------------------------------------
		// 3a. Grant on behalf of customer
		resp, body = doAuthorizedReq(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tokenWriter, map[string]any{
			"subject_id":     customerSubject,
			"purpose":        "marketing",
			"channel":        "email",
			"legal_basis":    "consent",
			"status":         "granted",
			"source":         "crm_agent_assisted",
			"jurisdiction":   "US-CA",
			"notice_version": "2026-10",
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("consent_writer grant expected 201 Created, got %d: %s", resp.StatusCode, string(body))
		}
		var createdConsent map[string]any
		if err := json.Unmarshal(body, &createdConsent); err != nil {
			t.Fatalf("failed to decode created consent: %v", err)
		}
		consentID := createdConsent["id"].(string)

		// 3b. Withdraw on behalf of customer
		resp, body = doAuthorizedReq(t, client, http.MethodPost, fmt.Sprintf("%s/v1/consents/%s/withdraw", cluster.apiURL, consentID), tokenWriter, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("consent_writer withdraw expected 200 OK, got %d: %s", resp.StatusCode, string(body))
		}

		// 3c. consent_writer CANNOT alter admin policy (403 Forbidden)
		activeBool := true
		affirmBool := true
		doiBool := false
		resp, body = doAuthorizedReq(t, client, http.MethodPost, cluster.apiURL+"/v1/purposes", tokenWriter, map[string]any{
			"code":                         "admin_only_purpose",
			"category":                     "marketing",
			"allowed_channels":             []string{"email"},
			"requires_affirmative_consent": &affirmBool,
			"requires_double_opt_in":       &doiBool,
			"active":                       &activeBool,
		})
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("consent_writer policy alteration expected 403 Forbidden, got %d: %s", resp.StatusCode, string(body))
		}

		// -----------------------------------------------------------------
		// Assertion 4: privacy_admin has full administrative write authority
		// -----------------------------------------------------------------
		resp, body = doAuthorizedReq(t, client, http.MethodPost, cluster.apiURL+"/v1/purposes", tokenAdmin, map[string]any{
			"code":                         "admin_verified_purpose",
			"category":                     "marketing",
			"allowed_channels":             []string{"email"},
			"requires_affirmative_consent": &affirmBool,
			"requires_double_opt_in":       &doiBool,
			"active":                       &activeBool,
		})
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			t.Fatalf("privacy_admin policy alteration expected 201/200, got %d: %s", resp.StatusCode, string(body))
		}

		t.Log("PASS: RBAC role-scoped verification confirmed for service, consent_writer, consent_reader, and privacy_admin")
	})

	// Subtest 2: Agent-Assisted Audit Traceability (CRM / Contact Center internal accountability)
	t.Run("AgentAssisted_AuditTraceability", func(t *testing.T) {
		ctx := context.Background()
		customerSubject := fmt.Sprintf("cust-trace-%d", time.Now().UnixNano())
		agentActorID := "crm-agent-sarah-442"

		// CRM Agent JWT token with actor_id bound to the agent
		tokenAgent, err := auth.SignAuthentikToken(cluster.jwtSecret, tenantID, agentActorID, "", []string{auth.RoleConsentWriter}, time.Now().Add(time.Hour))
		if err != nil {
			t.Fatalf("failed to sign agent token: %v", err)
		}

		// 1. Agent grants consent on behalf of the customer via POST /v1/consents
		// Explicitly passing agentActorID in the payload while targeting customerSubject
		grantPayload := map[string]any{
			"actor_id":       agentActorID,
			"subject_id":     customerSubject,
			"purpose":        "marketing",
			"channel":        "email",
			"legal_basis":    "consent",
			"status":         "granted",
			"source":         "crm_call_center",
			"jurisdiction":   "US-CA",
			"notice_version": "2026-10",
		}
		resp, body := doAuthorizedReq(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tokenAgent, grantPayload)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("agent-assisted grant expected 201 Created, got %d: %s", resp.StatusCode, string(body))
		}
		var granted map[string]any
		if err := json.Unmarshal(body, &granted); err != nil {
			t.Fatalf("failed to unmarshal granted consent: %v", err)
		}
		consentID := granted["id"].(string)
		receiptID := granted["receipt_id"].(string)

		// 2. Query consent_audit_events directly from PostgreSQL
		// Verify agentActorID was recorded as actor_id while consent targets customer
		var auditActorID, auditAction, auditResourceID string
		err = cluster.dbPool.QueryRow(ctx, `
			SELECT actor_id, action, consent_id
			FROM consent_audit_events
			WHERE tenant_id = $1 AND consent_id = $2 AND action = 'consent.granted'
		`, tenantID, consentID).Scan(&auditActorID, &auditAction, &auditResourceID)
		if err != nil {
			t.Fatalf("failed to query consent_audit_events for grant: %v", err)
		}
		if auditActorID != agentActorID {
			t.Fatalf("audit event actor_id mismatch: expected %q (agent), got %q", agentActorID, auditActorID)
		}
		if auditResourceID != consentID {
			t.Fatalf("audit event consent_id mismatch: expected %q, got %q", consentID, auditResourceID)
		}

		// 3. Query consent_receipts directly from PostgreSQL
		// Verify agent's ID is recorded in actor_id and customer's ID is recorded in subject_id
		var receiptActorID, receiptSubjectID string
		err = cluster.dbPool.QueryRow(ctx, `
			SELECT actor_id, subject_id
			FROM consent_receipts
			WHERE tenant_id = $1 AND id = $2
		`, tenantID, receiptID).Scan(&receiptActorID, &receiptSubjectID)
		if err != nil {
			t.Fatalf("failed to query consent_receipts for grant: %v", err)
		}
		if receiptActorID != agentActorID {
			t.Fatalf("receipt actor_id mismatch: expected agent %q, got %q", agentActorID, receiptActorID)
		}
		if receiptSubjectID != customerSubject {
			t.Fatalf("receipt subject_id mismatch: expected customer %q, got %q", customerSubject, receiptSubjectID)
		}

		// 4. Agent executes a withdrawal on behalf of the customer via POST /v1/consents/{id}/withdraw
		withdrawPayload := map[string]any{
			"actor_id": agentActorID,
		}
		resp, body = doAuthorizedReq(t, client, http.MethodPost, fmt.Sprintf("%s/v1/consents/%s/withdraw", cluster.apiURL, consentID), tokenAgent, withdrawPayload)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("agent-assisted withdraw expected 200 OK, got %d: %s", resp.StatusCode, string(body))
		}

		// 5. Query consent_audit_events for the withdrawal event
		var withdrawAuditActorID, withdrawAction string
		err = cluster.dbPool.QueryRow(ctx, `
			SELECT actor_id, action
			FROM consent_audit_events
			WHERE tenant_id = $1 AND consent_id = $2 AND action = 'consent.withdrawn'
		`, tenantID, consentID).Scan(&withdrawAuditActorID, &withdrawAction)
		if err != nil {
			t.Fatalf("failed to query consent_audit_events for withdraw: %v", err)
		}
		if withdrawAuditActorID != agentActorID {
			t.Fatalf("withdraw audit actor_id mismatch: expected agent %q, got %q", agentActorID, withdrawAuditActorID)
		}

		// 6. Query consents table: verify customer's canonical state is updated to withdrawn
		var currentStatus, currentSubjectID string
		err = cluster.dbPool.QueryRow(ctx, `
			SELECT status, subject_id
			FROM consents
			WHERE tenant_id = $1 AND id = $2
		`, tenantID, consentID).Scan(&currentStatus, &currentSubjectID)
		if err != nil {
			t.Fatalf("failed to query consents table: %v", err)
		}
		if currentStatus != "withdrawn" {
			t.Fatalf("expected consent status withdrawn, got %q", currentStatus)
		}
		if currentSubjectID != customerSubject {
			t.Fatalf("expected customer subject_id %q, got %q", customerSubject, currentSubjectID)
		}

		t.Logf("PASS: Agent-assisted audit traceability verified — agent %q recorded across audit & receipts while updating customer %q", agentActorID, customerSubject)
	})

	// Subtest 3: Suppression Precedence Hierarchy (Global > Channel > Purpose+Channel > Active Grant)
	t.Run("SuppressionPrecedence_Hierarchy", func(t *testing.T) {
		customerSubject := fmt.Sprintf("cust-prec-%d", time.Now().UnixNano())

		tokenAdmin, err := auth.SignAuthentikToken(cluster.jwtSecret, tenantID, "admin-sup-precedence", "", []string{auth.RolePrivacyAdmin, auth.RoleConsentWriter}, time.Now().Add(time.Hour))
		if err != nil {
			t.Fatalf("failed to sign admin token: %v", err)
		}

		evaluate := func(purpose, channel string) (bool, string) {
			resp, body := doAuthorizedReq(t, client, http.MethodPost, cluster.apiURL+"/v1/consents/evaluate", tokenAdmin, map[string]any{
				"subject_id": customerSubject,
				"purpose":    purpose,
				"channel":    channel,
			})
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("evaluate %s/%s failed with status %d: %s", purpose, channel, resp.StatusCode, string(body))
			}
			var dec map[string]any
			if err := json.Unmarshal(body, &dec); err != nil {
				t.Fatalf("failed to unmarshal decision: %v", err)
			}
			return dec["allowed"].(bool), dec["reason"].(string)
		}

		// Step 0: Create active grants for purpose "marketing" on channels "email" and "sms"
		for _, ch := range []string{"email", "sms"} {
			resp, body := doAuthorizedReq(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tokenAdmin, map[string]any{
				"subject_id":     customerSubject,
				"purpose":        "marketing",
				"channel":        ch,
				"legal_basis":    "consent",
				"status":         "granted",
				"source":         "precedence_test",
				"jurisdiction":   "US-CA",
				"notice_version": "2026-10",
			})
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("grant %s failed: %d %s", ch, resp.StatusCode, string(body))
			}
		}

		// Assert both channels start as Allowed (Active Grant)
		allowedEmail, reasonEmail := evaluate("marketing", "email")
		allowedSMS, reasonSMS := evaluate("marketing", "sms")
		if !allowedEmail || (reasonEmail != "granted" && reasonEmail != "active_grant") {
			t.Fatalf("expected email granted/active_grant, got allowed=%v reason=%q", allowedEmail, reasonEmail)
		}
		if !allowedSMS || (reasonSMS != "granted" && reasonSMS != "active_grant") {
			t.Fatalf("expected sms granted/active_grant, got allowed=%v reason=%q", allowedSMS, reasonSMS)
		}

		// Step 1: Add Purpose+Channel Suppression on (marketing, sms)
		// Hierarchy Level: Purpose+Channel Suppression > Active Grant
		resp, body := doAuthorizedReq(t, client, http.MethodPost, cluster.apiURL+"/v1/suppressions", tokenAdmin, map[string]any{
			"subject_id": customerSubject,
			"scope":      "purpose_channel",
			"purpose":    "marketing",
			"channel":    "sms",
			"reason":     "purpose_channel_optout",
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create purpose_channel suppression failed: %d %s", resp.StatusCode, string(body))
		}

		allowedSMS, reasonSMS = evaluate("marketing", "sms")
		allowedEmail, reasonEmail = evaluate("marketing", "email")
		if allowedSMS || reasonSMS != "purpose_channel_suppression" {
			t.Fatalf("expected sms purpose_channel_suppression, got allowed=%v reason=%q", allowedSMS, reasonSMS)
		}
		if !allowedEmail || (reasonEmail != "granted" && reasonEmail != "active_grant") {
			t.Fatalf("expected email granted/active_grant, got allowed=%v reason=%q", allowedEmail, reasonEmail)
		}

		// Step 2: Add Channel Suppression on "sms"
		// Hierarchy Level: Channel Suppression > Purpose+Channel Suppression
		resp, body = doAuthorizedReq(t, client, http.MethodPost, cluster.apiURL+"/v1/suppressions", tokenAdmin, map[string]any{
			"subject_id": customerSubject,
			"scope":      "channel",
			"purpose":    "",
			"channel":    "sms",
			"reason":     "channel_wide_stop",
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create channel suppression failed: %d %s", resp.StatusCode, string(body))
		}

		// Assert SMS evaluation now yields "channel_suppression" (overriding purpose_channel_suppression)
		allowedSMS, reasonSMS = evaluate("marketing", "sms")
		allowedEmail, reasonEmail = evaluate("marketing", "email")
		if allowedSMS || reasonSMS != "channel_suppression" {
			t.Fatalf("expected sms channel_suppression (precedence over purpose_channel), got allowed=%v reason=%q", allowedSMS, reasonSMS)
		}
		// Email still allowed under active grant
		if !allowedEmail || (reasonEmail != "granted" && reasonEmail != "active_grant") {
			t.Fatalf("expected email still active grant, got allowed=%v reason=%q", allowedEmail, reasonEmail)
		}

		// Step 3: Add Global Suppression
		// Hierarchy Level: Global Suppression > Channel Suppression > Active Grant
		resp, body = doAuthorizedReq(t, client, http.MethodPost, cluster.apiURL+"/v1/suppressions", tokenAdmin, map[string]any{
			"subject_id": customerSubject,
			"scope":      "global",
			"purpose":    "",
			"channel":    "",
			"reason":     "global_do_not_contact",
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create global suppression failed: %d %s", resp.StatusCode, string(body))
		}

		// Assert BOTH SMS and Email now evaluate to "global_suppression"
		allowedSMS, reasonSMS = evaluate("marketing", "sms")
		allowedEmail, reasonEmail = evaluate("marketing", "email")
		if allowedSMS || reasonSMS != "global_suppression" {
			t.Fatalf("expected sms global_suppression (precedence over channel), got allowed=%v reason=%q", allowedSMS, reasonSMS)
		}
		if allowedEmail || reasonEmail != "global_suppression" {
			t.Fatalf("expected email global_suppression (precedence over active grant), got allowed=%v reason=%q", allowedEmail, reasonEmail)
		}

		t.Log("PASS: Suppression precedence hierarchy verified — Global > Channel > Purpose+Channel > Active Grant")
	})

	// Subtest 4: Protected Administrative Suppressions & Override Resistance (T-030)
	t.Run("ProtectedAdministrativeSuppressions", func(t *testing.T) {
		ctx := context.Background()
		customerSubject := fmt.Sprintf("cust-legal-%d", time.Now().UnixNano())

		// privacy_admin token
		tokenAdmin, err := auth.SignAuthentikToken(cluster.jwtSecret, tenantID, "dpo-compliance", "", []string{auth.RolePrivacyAdmin, auth.RoleConsentWriter}, time.Now().Add(time.Hour))
		if err != nil {
			t.Fatalf("failed to sign admin token: %v", err)
		}

		// Standard consent_writer token (CRM agent)
		tokenWriter, err := auth.SignAuthentikToken(cluster.jwtSecret, tenantID, "crm-agent-dave", "", []string{auth.RoleConsentWriter}, time.Now().Add(time.Hour))
		if err != nil {
			t.Fatalf("failed to sign consent_writer token: %v", err)
		}

		// 1. privacy_admin places an administrative global legal suppression (legal_hold)
		activeTrue := true
		resp, body := doAuthorizedReq(t, client, http.MethodPost, cluster.apiURL+"/v1/suppressions", tokenAdmin, map[string]any{
			"subject_id": customerSubject,
			"scope":      "global",
			"purpose":    "",
			"channel":    "",
			"reason":     "legal_hold",
			"active":     &activeTrue,
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("privacy_admin legal_hold suppression creation failed: %d %s", resp.StatusCode, string(body))
		}

		// Verify suppression is recorded in database with created_by_role = "privacy_admin"
		var supID, createdByRole string
		var supActive bool
		err = cluster.dbPool.QueryRow(ctx, `
			SELECT id, created_by_role, active
			FROM consent_suppressions
			WHERE tenant_id = $1 AND subject_id = $2 AND scope = 'global'
		`, tenantID, customerSubject).Scan(&supID, &createdByRole, &supActive)
		if err != nil {
			t.Fatalf("failed to query consent_suppressions: %v", err)
		}
		if !supActive || createdByRole != "privacy_admin" {
			t.Fatalf("expected active administrative suppression created_by_role=privacy_admin, got active=%v role=%q", supActive, createdByRole)
		}

		// 2. Standard consent_writer attempts to DELETE / REVOKE this administrative suppression
		// (Sending active: false to clear the suppression)
		activeFalse := false
		resp, body = doAuthorizedReq(t, client, http.MethodPost, cluster.apiURL+"/v1/suppressions", tokenWriter, map[string]any{
			"subject_id": customerSubject,
			"scope":      "global",
			"purpose":    "",
			"channel":    "",
			"reason":     "crm_agent_attempted_clear",
			"active":     &activeFalse,
		})
		// MUST receive 403 Forbidden (ErrSuppressionProtected)
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("consent_writer attempting to clear administrative suppression expected 403 Forbidden, got %d: %s", resp.StatusCode, string(body))
		}

		// Verify in DB that the suppression remains STRICTLY active and unchanged
		err = cluster.dbPool.QueryRow(ctx, `
			SELECT active, created_by_role
			FROM consent_suppressions
			WHERE tenant_id = $1 AND subject_id = $2 AND scope = 'global'
		`, tenantID, customerSubject).Scan(&supActive, &createdByRole)
		if err != nil {
			t.Fatalf("failed to query consent_suppressions after attack: %v", err)
		}
		if !supActive || createdByRole != "privacy_admin" {
			t.Fatalf("CRITICAL: Administrative suppression was improperly cleared or altered! active=%v role=%q", supActive, createdByRole)
		}

		// 3. Standard consent_writer attempts to GRANT a new consent for the subject under legal hold
		resp, body = doAuthorizedReq(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tokenWriter, map[string]any{
			"subject_id":     customerSubject,
			"purpose":        "marketing",
			"channel":        "email",
			"legal_basis":    "consent",
			"status":         "granted",
			"source":         "crm_override_attempt",
			"jurisdiction":   "US-CA",
			"notice_version": "2026-10",
		})
		// MUST receive 409 Conflict (or 403 Forbidden)
		if resp.StatusCode != http.StatusConflict && resp.StatusCode != http.StatusForbidden {
			t.Fatalf("consent_writer grant under active legal_hold expected 409 Conflict or 403 Forbidden, got %d: %s", resp.StatusCode, string(body))
		}

		// 4. privacy_admin (authorized administrator) can successfully clear/revoke the legal hold
		resp, body = doAuthorizedReq(t, client, http.MethodPost, cluster.apiURL+"/v1/suppressions", tokenAdmin, map[string]any{
			"subject_id": customerSubject,
			"scope":      "global",
			"purpose":    "",
			"channel":    "",
			"reason":     "legal_hold_released",
			"active":     &activeFalse,
		})
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			t.Fatalf("privacy_admin release legal_hold expected 201/200, got %d: %s", resp.StatusCode, string(body))
		}

		// Verify suppression in DB is now inactive
		err = cluster.dbPool.QueryRow(ctx, `
			SELECT active
			FROM consent_suppressions
			WHERE tenant_id = $1 AND subject_id = $2 AND scope = 'global'
		`, tenantID, customerSubject).Scan(&supActive)
		if err != nil {
			t.Fatalf("failed to query consent_suppressions after admin release: %v", err)
		}
		if supActive {
			t.Fatalf("expected suppression to be inactive after admin release, but active=%v", supActive)
		}

		// 5. With legal hold released, standard consent_writer CAN now grant consent successfully
		resp, body = doAuthorizedReq(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tokenWriter, map[string]any{
			"subject_id":     customerSubject,
			"purpose":        "marketing",
			"channel":        "email",
			"legal_basis":    "consent",
			"status":         "granted",
			"source":         "crm_legitimate_grant",
			"jurisdiction":   "US-CA",
			"notice_version": "2026-10",
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("consent_writer grant after legal hold release expected 201 Created, got %d: %s", resp.StatusCode, string(body))
		}

		t.Log("PASS: Protected administrative suppressions strictly enforced — non-admin writers cannot clear legal holds or grant conflicting consents")
	})
}
