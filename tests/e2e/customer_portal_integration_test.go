package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/auth"
	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
)

type portalE2ECluster struct {
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
}

func setupPortalE2ECluster(t *testing.T) *portalE2ECluster {
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
		t.Skipf("database connection failed (%v); skipping portal e2e tests", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		if strings.EqualFold(os.Getenv("NVUTO_REQUIRE_INTEGRATION"), "true") {
			t.Fatalf("database ping failed: %v", err)
		}
		t.Skipf("database ping failed (%v); skipping portal e2e tests", err)
	}

	// Verify NATS connectivity
	nc, err := nats.Connect(natsURL, nats.Timeout(3*time.Second))
	if err != nil {
		pool.Close()
		if strings.EqualFold(os.Getenv("NVUTO_REQUIRE_INTEGRATION"), "true") {
			t.Fatalf("NATS connection failed: %v", err)
		}
		t.Skipf("NATS connection failed (%v); skipping portal e2e tests", err)
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
	jwtSecret := fmt.Sprintf("authentik-test-secret-%d", time.Now().UnixNano())

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	streamName := "NVUTO_PORTAL_" + suffix
	subjectPrefix := "nvuto.portal." + suffix

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

	cluster := &portalE2ECluster{
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

func doAuthorizedReq(t *testing.T, client *http.Client, method, url, bearerToken string, body any) (*http.Response, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("failed to marshal request body: %v", err)
		}
		r = bytes.NewReader(encoded)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		t.Fatalf("failed to create http request: %v", err)
	}
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("http request %s %s failed: %v", method, url, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	return resp, respBytes
}

func setupPortalTenantPrerequisites(t *testing.T, client *http.Client, apiURL, jwtSecret, tenantID string) {
	t.Helper()
	adminToken, err := auth.SignAuthentikToken(jwtSecret, tenantID, "portal-admin", "", []string{
		auth.RolePrivacyAdmin,
		auth.RoleConsentAdmin,
		auth.RoleConsentWriter,
	}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("failed to sign admin token: %v", err)
	}

	// 1. Create Lawful Bases
	lawfulBases := []struct {
		Code       string
		Categories []string
	}{
		{Code: "consent", Categories: []string{"marketing", "analytics"}},
		{Code: "legitimate_interest", Categories: []string{"analytics"}},
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

	// 2. Create Purposes
	purposes := []struct {
		Code     string
		Category string
		Channels []string
		Affirm   bool
		DOI      bool
	}{
		{Code: "marketing", Category: "marketing", Channels: []string{"email", "sms"}, Affirm: true, DOI: false},
		{Code: "analytics", Category: "analytics", Channels: []string{"web", "email"}, Affirm: false, DOI: false},
		{Code: "newsletter", Category: "marketing", Channels: []string{"email"}, Affirm: true, DOI: true},
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
		"published_at": time.Now().UTC(),
		"active":       &activeNotice,
		"content":      "Standard privacy notice for E2E verification",
	})
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("failed to publish notice: %d %s", resp.StatusCode, string(body))
	}
}

// TestCustomerPortal_SelfServiceLifecycle verifies the end-to-end integration for INT-04:
// 1. Authentik JWT subject binding mapping to canonical subject_id.
// 2. Initial state hydration via read projection (T-024).
// 3. Sequential user clicks: Grant consent -> Deny purpose -> Initiate DOI -> Confirm DOI -> Withdraw consent.
// 4. Persistence reload confirming exact persisted state and decisions.
func TestCustomerPortal_SelfServiceLifecycle(t *testing.T) {
	cluster := setupPortalE2ECluster(t)
	client := &http.Client{Timeout: 5 * time.Second}
	tenantID := fmt.Sprintf("tenant-portal-%d", time.Now().UnixNano()%100000)

	setupPortalTenantPrerequisites(t, client, cluster.apiURL, cluster.jwtSecret, tenantID)

	// Canonical Customer Graph Subject ID for Alice
	aliceSubject := fmt.Sprintf("cgraph:%s:customer:alice-%d", tenantID, time.Now().UnixNano()%100000)
	aliceActor := "authentik-user-alice-session"

	aliceToken, err := auth.SignAuthentikToken(cluster.jwtSecret, tenantID, aliceActor, aliceSubject, []string{auth.RoleSelfService}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("failed to sign Alice's Authentik token: %v", err)
	}

	// -------------------------------------------------------------------------
	// Step 1: Hydration (Initial Load)
	// -------------------------------------------------------------------------
	t.Log("Step 1: Hydrating initial preferences for Alice from read projection...")
	resp, body := doAuthorizedReq(t, client, http.MethodGet, fmt.Sprintf("%s/v1/consents?subject_id=%s", cluster.apiURL, aliceSubject), aliceToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("hydration failed: expected 200 OK, got %d: %s", resp.StatusCode, string(body))
	}

	var initialList struct {
		Items []domain.Consent `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(body, &initialList); err != nil {
		t.Fatalf("failed to parse hydration response: %v", err)
	}
	if initialList.Total != 0 || len(initialList.Items) != 0 {
		t.Fatalf("expected initial preference state to have 0 items, got total=%d len=%d", initialList.Total, len(initialList.Items))
	}

	// Verify pre-send evaluate returns no_consent
	evalResp, evalBody := doAuthorizedReq(t, client, http.MethodPost, cluster.apiURL+"/v1/evaluate", aliceToken, map[string]any{
		"subject_id": aliceSubject,
		"purpose":    "marketing",
		"channel":    "email",
	})
	if evalResp.StatusCode != http.StatusOK {
		t.Fatalf("evaluate failed: expected 200 OK, got %d: %s", evalResp.StatusCode, string(evalBody))
	}
	var evalDecision struct {
		Allowed bool   `json:"allowed"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal(evalBody, &evalDecision); err != nil {
		t.Fatalf("failed to parse evaluate response: %v", err)
	}
	if evalDecision.Allowed || evalDecision.Reason != "no_consent" {
		t.Fatalf("expected initial evaluate allowed=false reason=no_consent, got allowed=%v reason=%s", evalDecision.Allowed, evalDecision.Reason)
	}

	// -------------------------------------------------------------------------
	// Step 2: Mutation 1 - Grant Consent (Marketing)
	// -------------------------------------------------------------------------
	t.Log("Step 2: Alice clicks 'Grant' for Marketing...")
	grantResp, grantBody := doAuthorizedReq(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", aliceToken, map[string]any{
		"subject_id":     aliceSubject,
		"purpose":        "marketing",
		"channel":        "email",
		"legal_basis":    "consent",
		"status":         "granted",
		"source":         "customer_portal",
		"jurisdiction":   "US-CA",
		"notice_version": "2026-10",
	})
	if grantResp.StatusCode != http.StatusCreated {
		t.Fatalf("grant consent failed: expected 201 Created, got %d: %s", grantResp.StatusCode, string(grantBody))
	}
	var marketingConsent domain.Consent
	if err := json.Unmarshal(grantBody, &marketingConsent); err != nil {
		t.Fatalf("failed to unmarshal marketing consent: %v", err)
	}
	if marketingConsent.Status != domain.StatusGranted {
		t.Fatalf("expected marketing status=%s, got %s", domain.StatusGranted, marketingConsent.Status)
	}
	marketingConsentID := marketingConsent.ID

	var mktGen int64
	if err := cluster.dbPool.QueryRow(context.Background(), "SELECT preference_generation FROM consents WHERE id = $1", marketingConsentID).Scan(&mktGen); err != nil {
		t.Fatalf("failed to query marketing consent preference_generation: %v", err)
	}
	if mktGen != 1 {
		t.Fatalf("expected db preference_generation=1, got %d", mktGen)
	}

	// -------------------------------------------------------------------------
	// Step 3: Mutation 2 - Deny Different Purpose (Analytics)
	// -------------------------------------------------------------------------
	t.Log("Step 3: Alice clicks 'Deny' for Analytics...")
	denyResp, denyBody := doAuthorizedReq(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", aliceToken, map[string]any{
		"subject_id":     aliceSubject,
		"purpose":        "analytics",
		"channel":        "web",
		"legal_basis":    "consent",
		"status":         "denied",
		"source":         "customer_portal",
		"jurisdiction":   "US-CA",
		"notice_version": "2026-10",
	})
	if denyResp.StatusCode != http.StatusCreated {
		t.Fatalf("deny consent failed: expected 201 Created, got %d: %s", denyResp.StatusCode, string(denyBody))
	}
	var analyticsConsent domain.Consent
	if err := json.Unmarshal(denyBody, &analyticsConsent); err != nil {
		t.Fatalf("failed to unmarshal analytics consent: %v", err)
	}
	if analyticsConsent.Status != domain.StatusDenied {
		t.Fatalf("expected analytics status=%s, got %s", domain.StatusDenied, analyticsConsent.Status)
	}

	// -------------------------------------------------------------------------
	// Step 4: Mutation 3 - Initiate Double-Opt-In (Newsletter)
	// -------------------------------------------------------------------------
	t.Log("Step 4: Alice initiates Double-Opt-In for Newsletter...")
	doiResp, doiBody := doAuthorizedReq(t, client, http.MethodPost, cluster.apiURL+"/v1/double-opt-in", aliceToken, map[string]any{
		"subject_id":     aliceSubject,
		"purpose":        "newsletter",
		"channel":        "email",
		"source":         "customer_portal",
		"jurisdiction":   "US-CA",
		"notice_version": "2026-10",
		"expires_at":     time.Now().UTC().Add(24 * time.Hour),
	})
	if doiResp.StatusCode != http.StatusCreated {
		t.Fatalf("start DOI failed: expected 201 Created, got %d: %s", doiResp.StatusCode, string(doiBody))
	}
	var doiChallengeResp domain.DoubleOptInResponse
	if err := json.Unmarshal(doiBody, &doiChallengeResp); err != nil {
		t.Fatalf("failed to unmarshal DOI response: %v", err)
	}
	if doiChallengeResp.Status != domain.StatusPendingConfirmation {
		t.Fatalf("expected DOI status=%s, got %s", domain.StatusPendingConfirmation, doiChallengeResp.Status)
	}
	if doiChallengeResp.ChallengeID == "" {
		t.Fatal("expected non-empty challenge_id in DOI response")
	}

	// Verify token is omitted in HTTP response (T-003 / security requirement)
	if strings.Contains(string(doiBody), "confirmation_token") {
		t.Fatalf("security violation: raw confirmation_token exposed in HTTP response: %s", string(doiBody))
	}

	// -------------------------------------------------------------------------
	// Step 5: Mutation 4 - Confirm Double-Opt-In
	// -------------------------------------------------------------------------
	t.Log("Step 5: Retrieving DOI token from database outbox and confirming DOI...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var outboxPayloadBytes []byte
	queryErr := cluster.dbPool.QueryRow(ctx,
		"SELECT payload FROM consent_outbox_events WHERE tenant_id = $1 AND event_type = 'consent.challenge.created' AND aggregate_id = $2 ORDER BY occurred_at DESC LIMIT 1",
		tenantID, doiChallengeResp.ChallengeID,
	).Scan(&outboxPayloadBytes)
	if queryErr != nil {
		t.Fatalf("failed to query outbox for DOI challenge: %v", queryErr)
	}

	var outboxPayload struct {
		Token string `json:"confirmation_token"`
	}
	if err := json.Unmarshal(outboxPayloadBytes, &outboxPayload); err != nil || outboxPayload.Token == "" {
		t.Fatalf("failed to extract confirmation token from outbox payload: %v, payload: %s", err, string(outboxPayloadBytes))
	}

	confirmResp, confirmBody := doAuthorizedReq(t, client, http.MethodPost, cluster.apiURL+"/v1/double-opt-in/confirm", aliceToken, map[string]any{
		"token": outboxPayload.Token,
	})
	if confirmResp.StatusCode != http.StatusOK {
		t.Fatalf("confirm DOI failed: expected 200 OK, got %d: %s", confirmResp.StatusCode, string(confirmBody))
	}
	var confirmedConsent domain.Consent
	if err := json.Unmarshal(confirmBody, &confirmedConsent); err != nil {
		t.Fatalf("failed to unmarshal confirmed consent: %v", err)
	}
	if confirmedConsent.Status != domain.StatusGranted {
		t.Fatalf("expected confirmed status=%s, got %s", domain.StatusGranted, confirmedConsent.Status)
	}
	newsletterReceiptID := confirmedConsent.ReceiptID

	// -------------------------------------------------------------------------
	// Step 6: Mutation 5 - Withdraw Previously Granted Consent (Marketing)
	// -------------------------------------------------------------------------
	t.Log("Step 6: Alice withdraws previously granted Marketing consent...")
	withdrawResp, withdrawBody := doAuthorizedReq(t, client, http.MethodPost, fmt.Sprintf("%s/v1/consents/%s/withdraw", cluster.apiURL, marketingConsentID), aliceToken, nil)
	if withdrawResp.StatusCode != http.StatusOK {
		t.Fatalf("withdraw consent failed: expected 200 OK, got %d: %s", withdrawResp.StatusCode, string(withdrawBody))
	}
	var withdrawnConsent domain.Consent
	if err := json.Unmarshal(withdrawBody, &withdrawnConsent); err != nil {
		t.Fatalf("failed to unmarshal withdrawn consent: %v", err)
	}
	if withdrawnConsent.Status != domain.StatusWithdrawn {
		t.Fatalf("expected withdrawn status=%s, got %s", domain.StatusWithdrawn, withdrawnConsent.Status)
	}

	var wdnGen int64
	if err := cluster.dbPool.QueryRow(context.Background(), "SELECT preference_generation FROM consents WHERE id = $1", marketingConsentID).Scan(&wdnGen); err != nil {
		t.Fatalf("failed to query withdrawn consent preference_generation: %v", err)
	}
	if wdnGen != 2 {
		t.Fatalf("expected db withdrawn preference_generation=2, got %d", wdnGen)
	}

	// -------------------------------------------------------------------------
	// Step 7: Persistence Reload (Frontend Reload Verification)
	// -------------------------------------------------------------------------
	t.Log("Step 7: Simulating page refresh - Reloading preferences from read projection...")
	reloadResp, reloadBody := doAuthorizedReq(t, client, http.MethodGet, fmt.Sprintf("%s/v1/consents?subject_id=%s", cluster.apiURL, aliceSubject), aliceToken, nil)
	if reloadResp.StatusCode != http.StatusOK {
		t.Fatalf("reload failed: expected 200 OK, got %d: %s", reloadResp.StatusCode, string(reloadBody))
	}

	var reloadedList struct {
		Items []domain.Consent `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(reloadBody, &reloadedList); err != nil {
		t.Fatalf("failed to parse reloaded response: %v", err)
	}

	if reloadedList.Total != 3 || len(reloadedList.Items) != 3 {
		t.Fatalf("expected exactly 3 persisted preferences on reload, got total=%d len=%d", reloadedList.Total, len(reloadedList.Items))
	}

	stateByPurpose := make(map[string]domain.Consent)
	for _, item := range reloadedList.Items {
		stateByPurpose[item.Purpose] = item
	}

	// 1. Marketing must be withdrawn
	mkt, ok := stateByPurpose["marketing"]
	if !ok || mkt.Status != domain.StatusWithdrawn {
		t.Fatalf("marketing consent mismatch: ok=%v status=%s", ok, mkt.Status)
	}

	// 2. Analytics must be denied
	ana, ok := stateByPurpose["analytics"]
	if !ok || ana.Status != domain.StatusDenied {
		t.Fatalf("analytics consent mismatch: ok=%v status=%s", ok, ana.Status)
	}

	// 3. Newsletter must be granted
	nws, ok := stateByPurpose["newsletter"]
	if !ok || nws.Status != domain.StatusGranted {
		t.Fatalf("newsletter consent mismatch: ok=%v status=%s", ok, nws.Status)
	}

	// Verify evaluation decisions on reload
	evaluations := []struct {
		purpose string
		channel string
		allowed bool
		reason  string
	}{
		{"marketing", "email", false, "withdrawn"},
		{"analytics", "web", false, "denied"},
		{"newsletter", "email", true, "granted"},
	}

	for _, tc := range evaluations {
		evR, evB := doAuthorizedReq(t, client, http.MethodPost, cluster.apiURL+"/v1/evaluate", aliceToken, map[string]any{
			"subject_id": aliceSubject,
			"purpose":    tc.purpose,
			"channel":    tc.channel,
		})
		if evR.StatusCode != http.StatusOK {
			t.Fatalf("evaluate for %s failed: %d %s", tc.purpose, evR.StatusCode, string(evB))
		}
		var d struct {
			Allowed bool   `json:"allowed"`
			Reason  string `json:"reason"`
		}
		if err := json.Unmarshal(evB, &d); err != nil {
			t.Fatalf("failed to parse evaluate decision: %v", err)
		}
		if d.Allowed != tc.allowed || d.Reason != tc.reason {
			t.Fatalf("evaluate mismatch for %s: expected allowed=%v reason=%s, got allowed=%v reason=%s", tc.purpose, tc.allowed, tc.reason, d.Allowed, d.Reason)
		}
	}

	// Verify Alice can inspect her own receipt
	rcptResp, rcptBody := doAuthorizedReq(t, client, http.MethodGet, fmt.Sprintf("%s/v1/receipts/%s", cluster.apiURL, newsletterReceiptID), aliceToken, nil)
	if rcptResp.StatusCode != http.StatusOK {
		t.Fatalf("failed to fetch Alice's receipt: %d %s", rcptResp.StatusCode, string(rcptBody))
	}
	var rcpt domain.ConsentReceipt
	if err := json.Unmarshal(rcptBody, &rcpt); err != nil {
		t.Fatalf("failed to unmarshal receipt: %v", err)
	}
	if rcpt.SubjectID != aliceSubject {
		t.Fatalf("receipt subject mismatch: expected %s, got %s", aliceSubject, rcpt.SubjectID)
	}

	t.Log("SUCCESS: Customer Portal self-service lifecycle verified completely.")
}

// TestCustomerPortal_CrossSubjectIsolation strictly asserts the boundary limits:
// 1. Authentik JWT bound to Subject_A attempts POST /v1/consents for Subject_B -> 403 Forbidden.
// 2. Authentik JWT bound to Subject_A attempts GET /v1/consents for Subject_B -> 403 Forbidden.
// 3. Authentik JWT bound to Subject_A attempts POST /v1/double-opt-in for Subject_B -> 403 Forbidden.
// 4. Authentik JWT bound to Subject_A attempts POST /v1/consents/{bobID}/withdraw -> 403 Forbidden.
// 5. Authentik JWT bound to Subject_A attempts POST /v1/evaluate for Subject_B -> 403 Forbidden.
// 6. Authentik JWT bound to Subject_A attempts GET /v1/receipts/{bobID} -> 403 Forbidden.
// Crucial: Queries the real database to assert EXACTLY ZERO records were written to
// consents, consent_audit_events, or consent_outbox_events from Subject_A's attacks.
func TestCustomerPortal_CrossSubjectIsolation(t *testing.T) {
	cluster := setupPortalE2ECluster(t)
	client := &http.Client{Timeout: 5 * time.Second}
	tenantID := fmt.Sprintf("tenant-portal-iso-%d", time.Now().UnixNano()%100000)

	setupPortalTenantPrerequisites(t, client, cluster.apiURL, cluster.jwtSecret, tenantID)

	subjectAlice := fmt.Sprintf("cgraph:%s:customer:alice-%d", tenantID, time.Now().UnixNano()%100000)
	subjectBob := fmt.Sprintf("cgraph:%s:customer:bob-%d", tenantID, time.Now().UnixNano()%100000)

	tokenAlice, err := auth.SignAuthentikToken(cluster.jwtSecret, tenantID, "session-alice", subjectAlice, []string{auth.RoleSelfService}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("failed to sign Alice token: %v", err)
	}
	tokenBob, err := auth.SignAuthentikToken(cluster.jwtSecret, tenantID, "session-bob", subjectBob, []string{auth.RoleSelfService}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("failed to sign Bob token: %v", err)
	}

	// Pre-create a legitimate consent for Bob using Bob's own token
	bobResp, bobBody := doAuthorizedReq(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tokenBob, map[string]any{
		"subject_id":     subjectBob,
		"purpose":        "marketing",
		"channel":        "email",
		"legal_basis":    "consent",
		"status":         "granted",
		"source":         "customer_portal",
		"jurisdiction":   "US-CA",
		"notice_version": "2026-10",
	})
	if bobResp.StatusCode != http.StatusCreated {
		t.Fatalf("setup Bob consent failed: %d %s", bobResp.StatusCode, string(bobBody))
	}
	var bobConsent domain.Consent
	if err := json.Unmarshal(bobBody, &bobConsent); err != nil {
		t.Fatalf("failed to parse Bob consent: %v", err)
	}
	bobConsentID := bobConsent.ID

	// -------------------------------------------------------------------------
	// Attack 1: Alice attempts to WRITE consent for Bob (POST /v1/consents)
	// -------------------------------------------------------------------------
	t.Log("Attack 1: Alice attempts to write consent for Bob...")
	att1Resp, att1Body := doAuthorizedReq(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tokenAlice, map[string]any{
		"subject_id":     subjectBob,
		"purpose":        "analytics",
		"channel":        "web",
		"legal_basis":    "consent",
		"status":         "granted",
		"source":         "customer_portal",
		"jurisdiction":   "US-CA",
		"notice_version": "2026-10",
	})
	if att1Resp.StatusCode != http.StatusForbidden {
		t.Fatalf("SECURITY BREACH: expected 403 Forbidden for cross-subject write, got %d: %s", att1Resp.StatusCode, string(att1Body))
	}

	// -------------------------------------------------------------------------
	// Attack 2: Alice attempts to READ Bob's consents (GET /v1/consents)
	// -------------------------------------------------------------------------
	t.Log("Attack 2: Alice attempts to read Bob's preferences...")
	att2Resp, att2Body := doAuthorizedReq(t, client, http.MethodGet, fmt.Sprintf("%s/v1/consents?subject_id=%s", cluster.apiURL, subjectBob), tokenAlice, nil)
	if att2Resp.StatusCode != http.StatusForbidden {
		t.Fatalf("SECURITY BREACH: expected 403 Forbidden for cross-subject read, got %d: %s", att2Resp.StatusCode, string(att2Body))
	}

	// -------------------------------------------------------------------------
	// Attack 3: Alice attempts to initiate DOI on Bob's behalf (POST /v1/double-opt-in)
	// -------------------------------------------------------------------------
	t.Log("Attack 3: Alice attempts to initiate DOI for Bob...")
	att3Resp, att3Body := doAuthorizedReq(t, client, http.MethodPost, cluster.apiURL+"/v1/double-opt-in", tokenAlice, map[string]any{
		"subject_id":     subjectBob,
		"purpose":        "newsletter",
		"channel":        "email",
		"source":         "customer_portal",
		"jurisdiction":   "US-CA",
		"notice_version": "2026-10",
		"expires_at":     time.Now().UTC().Add(24 * time.Hour),
	})
	if att3Resp.StatusCode != http.StatusForbidden {
		t.Fatalf("SECURITY BREACH: expected 403 Forbidden for cross-subject DOI initiation, got %d: %s", att3Resp.StatusCode, string(att3Body))
	}

	// -------------------------------------------------------------------------
	// Attack 4: Alice attempts to WITHDRAW Bob's consent (POST /v1/consents/{id}/withdraw)
	// -------------------------------------------------------------------------
	t.Log("Attack 4: Alice attempts to withdraw Bob's marketing consent...")
	att4Resp, att4Body := doAuthorizedReq(t, client, http.MethodPost, fmt.Sprintf("%s/v1/consents/%s/withdraw", cluster.apiURL, bobConsentID), tokenAlice, nil)
	if att4Resp.StatusCode != http.StatusForbidden {
		t.Fatalf("SECURITY BREACH: expected 403 Forbidden for cross-subject withdrawal, got %d: %s", att4Resp.StatusCode, string(att4Body))
	}

	// -------------------------------------------------------------------------
	// Attack 5: Alice attempts to EVALUATE Bob's consent (POST /v1/evaluate)
	// -------------------------------------------------------------------------
	t.Log("Attack 5: Alice attempts to evaluate Bob's consent...")
	att5Resp, att5Body := doAuthorizedReq(t, client, http.MethodPost, cluster.apiURL+"/v1/evaluate", tokenAlice, map[string]any{
		"subject_id": subjectBob,
		"purpose":    "marketing",
		"channel":    "email",
	})
	if att5Resp.StatusCode != http.StatusForbidden {
		t.Fatalf("SECURITY BREACH: expected 403 Forbidden for cross-subject evaluate, got %d: %s", att5Resp.StatusCode, string(att5Body))
	}

	// -------------------------------------------------------------------------
	// Attack 6: Alice attempts to READ Bob's receipt (GET /v1/receipts/{id})
	// -------------------------------------------------------------------------
	t.Log("Attack 6: Alice attempts to read Bob's receipt...")
	bobReceiptID := bobConsent.ReceiptID
	if bobReceiptID == "" {
		t.Fatal("expected non-empty receipt_id on Bob's consent")
	}
	att6Resp, att6Body := doAuthorizedReq(t, client, http.MethodGet, fmt.Sprintf("%s/v1/receipts/%s", cluster.apiURL, bobReceiptID), tokenAlice, nil)
	if att6Resp.StatusCode != http.StatusForbidden {
		t.Fatalf("SECURITY BREACH: expected 403 Forbidden for cross-subject receipt read, got %d: %s", att6Resp.StatusCode, string(att6Body))
	}

	// -------------------------------------------------------------------------
	// CRUCIAL DATABASE ZERO-PERSISTENCE ASSERTION (Fail-Closed Verification)
	// -------------------------------------------------------------------------
	t.Log("Verifying zero persistence in PostgreSQL for all rejected attacks...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Exactly ZERO records written to `consents` for analytics (the purpose Alice tried to write for Bob)
	var consentCount int
	err = cluster.dbPool.QueryRow(ctx,
		"SELECT COUNT(*) FROM consents WHERE tenant_id = $1 AND subject_id = $2 AND purpose = 'analytics'",
		tenantID, subjectBob,
	).Scan(&consentCount)
	if err != nil {
		t.Fatalf("database query error on consents: %v", err)
	}
	if consentCount != 0 {
		t.Fatalf("CRITICAL SECURITY VIOLATION: expected 0 records in consents for rejected cross-subject write, found %d", consentCount)
	}

	// 2. Exactly ZERO audit events written under Alice's actor_id
	var auditCount int
	err = cluster.dbPool.QueryRow(ctx,
		"SELECT COUNT(*) FROM consent_audit_events WHERE tenant_id = $1 AND actor_id = 'session-alice'",
		tenantID,
	).Scan(&auditCount)
	if err != nil {
		t.Fatalf("database query error on consent_audit_events: %v", err)
	}
	if auditCount != 0 {
		t.Fatalf("CRITICAL SECURITY VIOLATION: expected 0 records in consent_audit_events from rejected attacker actor, found %d", auditCount)
	}

	// 3. Exactly ZERO outbox events written for the rejected analytics write or rejected DOI
	var outboxCount int
	err = cluster.dbPool.QueryRow(ctx,
		"SELECT COUNT(*) FROM consent_outbox_events WHERE tenant_id = $1 AND (payload->>'subject_id' = $2 AND payload->>'purpose' = 'analytics')",
		tenantID, subjectBob,
	).Scan(&outboxCount)
	if err != nil {
		t.Fatalf("database query error on consent_outbox_events: %v", err)
	}
	if outboxCount != 0 {
		t.Fatalf("CRITICAL SECURITY VIOLATION: expected 0 records in consent_outbox_events for rejected cross-subject attack, found %d", outboxCount)
	}

	// 4. Bob's existing marketing consent in the database remains untouched (still granted, generation 1)
	var bobStatus string
	var bobGen int64
	err = cluster.dbPool.QueryRow(ctx,
		"SELECT status, preference_generation FROM consents WHERE id = $1",
		bobConsentID,
	).Scan(&bobStatus, &bobGen)
	if err != nil {
		t.Fatalf("database query error on Bob's consent: %v", err)
	}
	if bobStatus != "granted" || bobGen != 1 {
		t.Fatalf("Bob's consent was altered by rejected attack: status=%s gen=%d", bobStatus, bobGen)
	}

	t.Log("SUCCESS: Strict cross-subject isolation verified. System failed closed with zero persistence.")
}

// TestCustomerPortal_AuthentikClaimMappings verifies claim extraction variants:
// 1. customer_subject_id claim takes precedence.
// 2. custom subject_id claim is extracted if customer_subject_id is absent.
// 3. canonical cgraph:... in sub is extracted.
// 4. plain actor username in sub without customer_subject_id is rejected for self_service.
func TestCustomerPortal_AuthentikClaimMappings(t *testing.T) {
	cluster := setupPortalE2ECluster(t)
	client := &http.Client{Timeout: 5 * time.Second}
	tenantID := fmt.Sprintf("tenant-claims-%d", time.Now().UnixNano()%100000)

	setupPortalTenantPrerequisites(t, client, cluster.apiURL, cluster.jwtSecret, tenantID)

	canonicalSubject := fmt.Sprintf("cgraph:%s:customer:charlie-%d", tenantID, time.Now().UnixNano()%100000)

	// Case A: Token with custom `customer_subject_id` claim
	tokenA, err := auth.SignAuthentikToken(cluster.jwtSecret, tenantID, "user-session-1", canonicalSubject, []string{auth.RoleSelfService}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	respA, bodyA := doAuthorizedReq(t, client, http.MethodGet, fmt.Sprintf("%s/v1/consents?subject_id=%s", cluster.apiURL, canonicalSubject), tokenA, nil)
	if respA.StatusCode != http.StatusOK {
		t.Fatalf("Case A failed: expected 200 OK, got %d: %s", respA.StatusCode, string(bodyA))
	}

	// Case B: Token with custom `subject_id` claim (using SignAuthentikTokenWithClaims)
	claimsB := auth.Claims{
		TenantID:        tenantID,
		CustomSubjectID: canonicalSubject,
		Roles:           []string{auth.RoleSelfService},
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-session-2",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tokenB, err := auth.SignAuthentikTokenWithClaims(cluster.jwtSecret, claimsB)
	if err != nil {
		t.Fatal(err)
	}
	respB, bodyB := doAuthorizedReq(t, client, http.MethodGet, fmt.Sprintf("%s/v1/consents?subject_id=%s", cluster.apiURL, canonicalSubject), tokenB, nil)
	if respB.StatusCode != http.StatusOK {
		t.Fatalf("Case B failed: expected 200 OK, got %d: %s", respB.StatusCode, string(bodyB))
	}

	// Case C: Token with canonical cgraph:... in standard `sub` claim
	claimsC := auth.Claims{
		TenantID: tenantID,
		Roles:    []string{auth.RoleSelfService},
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   canonicalSubject,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tokenC, err := auth.SignAuthentikTokenWithClaims(cluster.jwtSecret, claimsC)
	if err != nil {
		t.Fatal(err)
	}
	respC, bodyC := doAuthorizedReq(t, client, http.MethodGet, fmt.Sprintf("%s/v1/consents?subject_id=%s", cluster.apiURL, canonicalSubject), tokenC, nil)
	if respC.StatusCode != http.StatusOK {
		t.Fatalf("Case C failed: expected 200 OK, got %d: %s", respC.StatusCode, string(bodyC))
	}

	// Case D: Negative test - Token with plain actor username in sub and NO customer_subject_id
	claimsD := auth.Claims{
		TenantID: tenantID,
		Roles:    []string{auth.RoleSelfService},
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "plain_authentik_username",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tokenD, err := auth.SignAuthentikTokenWithClaims(cluster.jwtSecret, claimsD)
	if err != nil {
		t.Fatal(err)
	}
	respD, _ := doAuthorizedReq(t, client, http.MethodGet, fmt.Sprintf("%s/v1/consents?subject_id=%s", cluster.apiURL, canonicalSubject), tokenD, nil)
	// Must fail because self_service token without canonical subject is invalid
	if respD.StatusCode == http.StatusOK {
		t.Fatalf("Case D security failure: plain username token without customer_subject_id was accepted for self_service")
	}

	t.Log("SUCCESS: Authentik claim mapping variants verified.")
}
