package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// -----------------------------------------------------------------------------
// INT-07: AI Model Training & Inference Ingestion Test Harness
// -----------------------------------------------------------------------------

type aiCluster struct {
	apiURL     string
	dbURL      string
	dbPool     *pgxpool.Pool
	serverCmd  *exec.Cmd
	serverPort int
	repoRoot   string
}

func setupAICluster(t *testing.T) *aiCluster {
	t.Helper()

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		dbURL = "postgres://yugabyte:yugabyte@127.0.0.1:5433/consent?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		if strings.EqualFold(os.Getenv("NVUTO_REQUIRE_INTEGRATION"), "true") {
			t.Fatalf("database connection failed: %v", err)
		}
		t.Skipf("database connection failed (%v); skipping AI e2e tests", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		if strings.EqualFold(os.Getenv("NVUTO_REQUIRE_INTEGRATION"), "true") {
			t.Fatalf("database ping failed: %v", err)
		}
		t.Skipf("database ping failed (%v); skipping AI e2e tests", err)
	}

	repoRoot := findRepoRoot(t)
	serverBin, _ := ensureBinaries(t, repoRoot)
	serverPort := getFreePort(t)
	apiURL := fmt.Sprintf("http://127.0.0.1:%d", serverPort)

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
		pool.Close()
		t.Fatalf("failed to start server process: %v", err)
	}

	waitForReady(t, fmt.Sprintf("http://127.0.0.1:%d/readyz", serverPort), 10*time.Second)

	cluster := &aiCluster{
		apiURL:     apiURL,
		dbURL:      dbURL,
		dbPool:     pool,
		serverCmd:  serverCmd,
		serverPort: serverPort,
		repoRoot:   repoRoot,
	}

	t.Cleanup(func() {
		if cluster.serverCmd != nil && cluster.serverCmd.Process != nil {
			_ = cluster.serverCmd.Process.Kill()
		}
		pool.Close()
	})

	return cluster
}

func setupAITenantPolicy(t *testing.T, client *http.Client, apiURL, tenantID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)

	// 1. Lawful Bases: "consent", "contract", "legitimate_interest"
	lawfulBases := []struct {
		Code             string
		Categories       []string
		RequiresEvidence bool
	}{
		{Code: "consent", Categories: []string{"ai_training", "analytics"}, RequiresEvidence: true},
		{Code: "contract", Categories: []string{"ai_training", "service"}, RequiresEvidence: false},
		{Code: "legitimate_interest", Categories: []string{"ai_training", "analytics"}, RequiresEvidence: false},
	}
	for _, lb := range lawfulBases {
		active := true
		reqEvidence := lb.RequiresEvidence
		resp, body := doHTTP(t, client, http.MethodPost, apiURL+"/v1/lawful-bases", tenantID, map[string]any{
			"code":               lb.Code,
			"allowed_categories": lb.Categories,
			"requires_evidence":  &reqEvidence,
			"active":             &active,
		})
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			t.Fatalf("register lawful basis %s failed (%d): %s", lb.Code, resp.StatusCode, string(body))
		}
	}

	// 2. Purposes:
	// - "ai_model_training": strictly requires affirmative consent (NDPA / GDPR Art. 9)
	// - "ai_general_analytics": does not require affirmative consent (allows legitimate_interest/contract)
	purposes := []struct {
		Code                string
		Category            string
		AllowedChannels     []string
		AffirmativeConsent  bool
		RequiresDoubleOptIn bool
	}{
		{
			Code:                "ai_model_training",
			Category:            "ai_training",
			AllowedChannels:     []string{"training_dataset", "rag_inference", "model_fine_tuning"},
			AffirmativeConsent:  true,
			RequiresDoubleOptIn: false,
		},
		{
			Code:                "ai_general_analytics",
			Category:            "ai_training",
			AllowedChannels:     []string{"training_dataset", "rag_inference"},
			AffirmativeConsent:  false,
			RequiresDoubleOptIn: false,
		},
	}
	for _, p := range purposes {
		affirm := p.AffirmativeConsent
		doi := p.RequiresDoubleOptIn
		active := true
		resp, body := doHTTP(t, client, http.MethodPost, apiURL+"/v1/purposes", tenantID, map[string]any{
			"code":                         p.Code,
			"category":                     p.Category,
			"allowed_channels":             p.AllowedChannels,
			"requires_affirmative_consent": &affirm,
			"requires_double_opt_in":       &doi,
			"active":                       &active,
		})
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			t.Fatalf("register purpose %s failed (%d): %s", p.Code, resp.StatusCode, string(body))
		}
	}

	// 3. Privacy Notice: "2026-ai-v1"
	active := true
	resp, body := doHTTP(t, client, http.MethodPost, apiURL+"/v1/notices", tenantID, map[string]any{
		"version":      "2026-ai-v1",
		"jurisdiction": "GLOBAL",
		"published_at": now,
		"active":       &active,
		"content":      "Standard AI model training and automated inference policy notice",
	})
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("publish notice failed (%d): %s", resp.StatusCode, string(body))
	}
}

// SimulatedAIPipeline simulates an LLM training / RAG inference pipeline
type SimulatedAIPipeline struct {
	apiURL     string
	tenantID   string
	httpClient *http.Client
}

func (ai *SimulatedAIPipeline) EvaluateSingle(ctx context.Context, subjectID, purpose, channel string) (bool, string, error) {
	reqBody, _ := json.Marshal(map[string]any{
		"subject_id": subjectID,
		"purpose":    purpose,
		"channel":    channel,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ai.apiURL+"/v1/consents/evaluate", bytes.NewReader(reqBody))
	if err != nil {
		return false, "client_error_fail_closed", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nvuto-Tenant-ID", ai.tenantID)
	req.Header.Set("X-Nvuto-Actor-ID", "ai-ingestion-worker-01")

	resp, err := ai.httpClient.Do(req)
	if err != nil {
		// Strict fail-closed on network error/timeout
		return false, "service_unavailable_fail_closed", nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return false, "service_unavailable_fail_closed", nil
	}

	var res struct {
		Allowed bool   `json:"allowed"`
		Reason  string `json:"reason"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return false, "decode_error_fail_closed", nil
	}

	return res.Allowed, res.Reason, nil
}

func (ai *SimulatedAIPipeline) EvaluateBatch(ctx context.Context, requests []map[string]string) ([]bool, error) {
	evalRequests := make([]map[string]any, len(requests))
	for i, r := range requests {
		evalRequests[i] = map[string]any{
			"subject_id": r["subject_id"],
			"purpose":    r["purpose"],
			"channel":    r["channel"],
		}
	}
	reqBody, _ := json.Marshal(map[string]any{
		"requests": evalRequests,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ai.apiURL+"/v1/consents/evaluate-batch", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nvuto-Tenant-ID", ai.tenantID)
	req.Header.Set("X-Nvuto-Actor-ID", "ai-batch-worker-01")

	resp, err := ai.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected batch evaluate status: %d", resp.StatusCode)
	}

	var res struct {
		Decisions []struct {
			Allowed bool   `json:"allowed"`
			Reason  string `json:"reason"`
		} `json:"decisions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	allowedList := make([]bool, len(res.Decisions))
	for i, d := range res.Decisions {
		allowedList[i] = d.Allowed
	}
	return allowedList, nil
}

// TestAIIntegration executes INT-07 verification:
// 1. Affirmative Consent Enforcement for AI Processing (GDPR Art. 9, NDPA):
//   - Rejects passive legal bases (contract, legitimate_interest) when purpose requires affirmative consent.
//
// 2. Affirmative Legal Basis (consent) Accepted:
//   - Allows ingestion only when active affirmative consent exists.
//
// 3. Negative Boundary Conditions:
//   - Blocks ingestion on missing consent, explicit denial, expiration, and suppression.
//
// 4. Batch Dataset Ingestion Filtering:
//   - Evaluates 100 candidate interaction records simultaneously, admitting only affirmative consent.
//
// 5. Strict Fail-Closed Safe-Deny on API Outage:
//   - Safe-denies and halts ingestion when Consent API is unreachable.
func TestAIIntegration(t *testing.T) {
	cluster := setupAICluster(t)
	client := &http.Client{Timeout: 5 * time.Second}
	tenantID := fmt.Sprintf("tenant-ai-%d", time.Now().UnixNano())

	setupAITenantPolicy(t, client, cluster.apiURL, tenantID)

	pipeline := &SimulatedAIPipeline{
		apiURL:     cluster.apiURL,
		tenantID:   tenantID,
		httpClient: client,
	}

	validArtifactHash := "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	// =========================================================================
	// Subtest 1: Passive Legal Basis Rejection for Affirmative AI Purpose
	// =========================================================================
	t.Run("AffirmativeConsent_Required_RejectsPassiveLegalBasis", func(t *testing.T) {
		// Case 1A: Attempting to record a grant relying on passive "contract" for ai_model_training
		// is rejected at the API policy boundary with 422 Unprocessable Entity
		subjectContract := fmt.Sprintf("sub-ai-contract-%d", time.Now().UnixNano())
		resp, body := doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantID, map[string]any{
			"subject_id":     subjectContract,
			"purpose":        "ai_model_training",
			"channel":        "training_dataset",
			"legal_basis":    "contract",
			"status":         "granted",
			"source":         "terms_of_service",
			"jurisdiction":   "GLOBAL",
			"notice_version": "2026-ai-v1",
		})
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 UnprocessableEntity on passive contract grant for affirmative purpose, got %d: %s", resp.StatusCode, string(body))
		}
		t.Logf("PASS: API policy boundary rejected passive contract grant with 422 Unprocessable Entity")

		// Case 1B: If an existing grant with legal_basis="contract" exists in DB (e.g. legacy or policy transition),
		// Evaluate strictly returns allowed=false, reason="policy_requirement_unmet"
		// Seed a grant row with legal_basis="contract" for ai_model_training directly in DB
		consentID := fmt.Sprintf("con-passive-%d", time.Now().UnixNano())
		now := time.Now().UTC()
		_, err := cluster.dbPool.Exec(context.Background(), `
			INSERT INTO consents (id, tenant_id, subject_id, purpose, channel, legal_basis, status, source, jurisdiction, notice_version, version, preference_generation, granted_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 1, 1, $11, $11, $11)
		`, consentID, tenantID, subjectContract, "ai_model_training", "training_dataset", "contract", "granted", "seeded_passive", "GLOBAL", "2026-ai-v1", now)
		if err != nil {
			t.Fatalf("failed to seed passive contract grant: %v", err)
		}

		allowed, reason, err := pipeline.EvaluateSingle(context.Background(), subjectContract, "ai_model_training", "training_dataset")
		if err != nil {
			t.Fatalf("evaluation failed: %v", err)
		}
		if allowed {
			t.Fatalf("CRITICAL SECURITY VIOLATION: Passive legal basis 'contract' was allowed during evaluation for purpose requiring affirmative consent!")
		}
		if reason != "policy_requirement_unmet" {
			t.Fatalf("expected reason 'policy_requirement_unmet', got %q", reason)
		}
		t.Logf("PASS: Evaluation engine strictly rejects passive 'contract' basis (allowed=false, reason=%q)", reason)

		// Case 1C: Attempting to record a grant relying on passive "legitimate_interest"
		subjectLegit := fmt.Sprintf("sub-ai-legit-%d", time.Now().UnixNano())
		resp, body = doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantID, map[string]any{
			"subject_id":     subjectLegit,
			"purpose":        "ai_model_training",
			"channel":        "training_dataset",
			"legal_basis":    "legitimate_interest",
			"status":         "granted",
			"source":         "data_policy",
			"jurisdiction":   "GLOBAL",
			"notice_version": "2026-ai-v1",
		})
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 UnprocessableEntity on passive legitimate_interest grant, got %d: %s", resp.StatusCode, string(body))
		}

		// Seed passive legitimate_interest in DB and verify evaluate rejection
		consentIDLegit := fmt.Sprintf("con-passive-legit-%d", time.Now().UnixNano())
		_, err = cluster.dbPool.Exec(context.Background(), `
			INSERT INTO consents (id, tenant_id, subject_id, purpose, channel, legal_basis, status, source, jurisdiction, notice_version, version, preference_generation, granted_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 1, 1, $11, $11, $11)
		`, consentIDLegit, tenantID, subjectLegit, "ai_model_training", "training_dataset", "legitimate_interest", "granted", "seeded_passive", "GLOBAL", "2026-ai-v1", now)
		if err != nil {
			t.Fatalf("failed to seed passive legitimate_interest grant: %v", err)
		}

		allowed, reason, err = pipeline.EvaluateSingle(context.Background(), subjectLegit, "ai_model_training", "training_dataset")
		if err != nil {
			t.Fatalf("evaluation failed: %v", err)
		}
		if allowed {
			t.Fatalf("CRITICAL SECURITY VIOLATION: Passive legal basis 'legitimate_interest' was allowed for purpose requiring affirmative consent!")
		}
		if reason != "policy_requirement_unmet" {
			t.Fatalf("expected reason 'policy_requirement_unmet', got %q", reason)
		}
		t.Logf("PASS: Evaluation engine strictly rejects passive 'legitimate_interest' basis (allowed=false, reason=%q)", reason)
	})

	// =========================================================================
	// Subtest 2: Affirmative Legal Basis (consent) Accepted
	// =========================================================================
	t.Run("AffirmativeConsent_Required_AcceptsAffirmativeConsent", func(t *testing.T) {
		subjectAffirm := fmt.Sprintf("sub-ai-affirm-%d", time.Now().UnixNano())
		now := time.Now().UTC()
		evidence := map[string]any{
			"method":        "explicit_opt_in_modal",
			"artifact_hash": validArtifactHash,
			"collected_at":  now.Format(time.RFC3339),
		}

		resp, body := doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantID, map[string]any{
			"subject_id":     subjectAffirm,
			"purpose":        "ai_model_training",
			"channel":        "training_dataset",
			"legal_basis":    "consent",
			"status":         "granted",
			"source":         "user_settings_optin",
			"jurisdiction":   "GLOBAL",
			"notice_version": "2026-ai-v1",
			"evidence":       evidence,
		})
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			t.Fatalf("failed to record affirmative consent (%d): %s", resp.StatusCode, string(body))
		}

		allowed, reason, err := pipeline.EvaluateSingle(context.Background(), subjectAffirm, "ai_model_training", "training_dataset")
		if err != nil {
			t.Fatalf("evaluation failed: %v", err)
		}
		if !allowed || (reason != "granted" && reason != "active_grant") {
			t.Fatalf("expected affirmative consent to be allowed with reason 'granted', got allowed=%v reason=%q", allowed, reason)
		}

		t.Logf("PASS: Affirmative consent with evidence strictly permits AI ingestion (allowed=%v, reason=%q)", allowed, reason)
	})

	// =========================================================================
	// Subtest 3: Negative Boundary Conditions Strictly Block AI Pipeline
	// =========================================================================
	t.Run("NegativeConditions_BlockAIPipeline", func(t *testing.T) {
		// 3A: Missing consent (unregistered user)
		unregisteredSubject := fmt.Sprintf("sub-ai-none-%d", time.Now().UnixNano())
		allowed, reason, err := pipeline.EvaluateSingle(context.Background(), unregisteredSubject, "ai_model_training", "training_dataset")
		if err != nil || allowed || reason != "no_consent" {
			t.Fatalf("expected no_consent for unregistered subject, got allowed=%v reason=%q (err: %v)", allowed, reason, err)
		}
		t.Log("PASS: Missing consent strictly blocks AI ingestion (reason=no_consent)")

		// 3B: Explicit denial (status: "denied")
		deniedSubject := fmt.Sprintf("sub-ai-denied-%d", time.Now().UnixNano())
		resp, body := doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantID, map[string]any{
			"subject_id":     deniedSubject,
			"purpose":        "ai_model_training",
			"channel":        "training_dataset",
			"legal_basis":    "consent",
			"status":         "denied",
			"source":         "user_opt_out",
			"jurisdiction":   "GLOBAL",
			"notice_version": "2026-ai-v1",
		})
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			t.Fatalf("failed to record denied consent (%d): %s", resp.StatusCode, string(body))
		}

		allowed, reason, err = pipeline.EvaluateSingle(context.Background(), deniedSubject, "ai_model_training", "training_dataset")
		if err != nil || allowed || reason != "denied" {
			t.Fatalf("expected denied for opt-out subject, got allowed=%v reason=%q (err: %v)", allowed, reason, err)
		}
		t.Log("PASS: Explicit denial strictly blocks AI ingestion (reason=denied)")

		// 3C: Expired consent (expires_at in the past)
		expiredSubject := fmt.Sprintf("sub-ai-expired-%d", time.Now().UnixNano())
		now := time.Now().UTC()
		future := now.Add(1 * time.Hour).Format(time.RFC3339)
		evidence := map[string]any{
			"method":        "explicit_opt_in_modal",
			"artifact_hash": validArtifactHash,
			"collected_at":  now.Format(time.RFC3339),
		}
		resp, body = doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantID, map[string]any{
			"subject_id":     expiredSubject,
			"purpose":        "ai_model_training",
			"channel":        "training_dataset",
			"legal_basis":    "consent",
			"status":         "granted",
			"source":         "user_settings_optin",
			"jurisdiction":   "GLOBAL",
			"notice_version": "2026-ai-v1",
			"expires_at":     future,
			"evidence":       evidence,
		})
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			t.Fatalf("failed to record consent for expiry test (%d): %s", resp.StatusCode, string(body))
		}

		// Backdate created_at to 2 hours ago and expires_at to 1 hour ago (satisfying expires_at > created_at)
		_, err = cluster.dbPool.Exec(context.Background(), `
			UPDATE consents SET created_at = NOW() - INTERVAL '2 hours', expires_at = NOW() - INTERVAL '1 hour' WHERE tenant_id = $1 AND subject_id = $2
		`, tenantID, expiredSubject)
		if err != nil {
			t.Fatalf("failed to expire consent in DB: %v", err)
		}

		allowed, reason, err = pipeline.EvaluateSingle(context.Background(), expiredSubject, "ai_model_training", "training_dataset")
		if err != nil || allowed || reason != "expired" {
			t.Fatalf("expected expired for past consent, got allowed=%v reason=%q (err: %v)", allowed, reason, err)
		}
		t.Log("PASS: Expired consent strictly blocks AI ingestion (reason=expired)")

		// 3D: Suppressed consent (active grant suppressed by global suppression)
		suppressedSubject := fmt.Sprintf("sub-ai-suppressed-%d", time.Now().UnixNano())
		resp, body = doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantID, map[string]any{
			"subject_id":     suppressedSubject,
			"purpose":        "ai_model_training",
			"channel":        "training_dataset",
			"legal_basis":    "consent",
			"status":         "granted",
			"source":         "user_settings_optin",
			"jurisdiction":   "GLOBAL",
			"notice_version": "2026-ai-v1",
			"evidence":       evidence,
		})
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			t.Fatalf("failed to record consent for suppression test (%d): %s", resp.StatusCode, string(body))
		}

		// Apply global suppression
		active := true
		resp, body = doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/suppressions", tenantID, map[string]any{
			"subject_id": suppressedSubject,
			"scope":      "global",
			"reason":     "legal_hold_ai_exclusion",
			"active":     &active,
		})
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			t.Fatalf("failed to apply suppression (%d): %s", resp.StatusCode, string(body))
		}

		allowed, reason, err = pipeline.EvaluateSingle(context.Background(), suppressedSubject, "ai_model_training", "training_dataset")
		if err != nil || allowed || reason != "global_suppression" {
			t.Fatalf("expected global_suppression for suppressed subject, got allowed=%v reason=%q (err: %v)", allowed, reason, err)
		}
		t.Log("PASS: Global suppression strictly blocks AI ingestion (reason=global_suppression)")
	})

	// =========================================================================
	// Subtest 4: Large-Scale Batch AI Dataset Ingestion (100 Candidates)
	// =========================================================================
	t.Run("BatchDatasetIngestion_FiltersDisallowedSubjects", func(t *testing.T) {
		const totalCandidates = 100
		requests := make([]map[string]string, totalCandidates)

		now := time.Now().UTC()
		evidence := map[string]any{
			"method":        "explicit_opt_in_modal",
			"artifact_hash": validArtifactHash,
			"collected_at":  now.Format(time.RFC3339),
		}

		// Composition:
		// 00..29: Affirmative Consent (30 subjects) -> ALLOWED
		// 30..49: Passive Contract (20 subjects)     -> DENIED (policy_requirement_unmet)
		// 50..69: Missing Consent (20 subjects)      -> DENIED (no_consent)
		// 70..79: Explicit Denial (10 subjects)      -> DENIED (denied)
		// 80..89: Expired Consent (10 subjects)      -> DENIED (expired)
		// 90..99: Suppressed Consent (10 subjects)   -> DENIED (global_suppression)

		for i := 0; i < totalCandidates; i++ {
			subj := fmt.Sprintf("sub-batch-ai-%d-%03d", time.Now().UnixNano(), i)
			requests[i] = map[string]string{
				"subject_id": subj,
				"purpose":    "ai_model_training",
				"channel":    "training_dataset",
			}

			switch {
			case i < 30:
				// Affirmative consent
				resp, body := doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantID, map[string]any{
					"subject_id":     subj,
					"purpose":        "ai_model_training",
					"channel":        "training_dataset",
					"legal_basis":    "consent",
					"status":         "granted",
					"source":         "opt_in_flow",
					"jurisdiction":   "GLOBAL",
					"notice_version": "2026-ai-v1",
					"evidence":       evidence,
				})
				if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
					t.Fatalf("failed to record affirmative consent for batch %d (%d): %s", i, resp.StatusCode, string(body))
				}
			case i < 50:
				// Passive contract: seed in DB to simulate legacy/passive record
				cid := fmt.Sprintf("con-batch-passive-%d-%d", time.Now().UnixNano(), i)
				_, err := cluster.dbPool.Exec(context.Background(), `
					INSERT INTO consents (id, tenant_id, subject_id, purpose, channel, legal_basis, status, source, jurisdiction, notice_version, version, preference_generation, granted_at, created_at, updated_at)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 1, 1, $11, $11, $11)
				`, cid, tenantID, subj, "ai_model_training", "training_dataset", "contract", "granted", "contract_flow", "GLOBAL", "2026-ai-v1", now)
				if err != nil {
					t.Fatalf("failed to seed passive contract for batch %d: %v", i, err)
				}
			case i < 70:
				// Missing consent (do nothing)
			case i < 80:
				// Explicit denial
				doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantID, map[string]any{
					"subject_id":     subj,
					"purpose":        "ai_model_training",
					"channel":        "training_dataset",
					"legal_basis":    "consent",
					"status":         "denied",
					"source":         "opt_out_flow",
					"jurisdiction":   "GLOBAL",
					"notice_version": "2026-ai-v1",
				})
			case i < 90:
				// Expired consent: create with future expiry, then backdate
				future := now.Add(1 * time.Hour).Format(time.RFC3339)
				resp, body := doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantID, map[string]any{
					"subject_id":     subj,
					"purpose":        "ai_model_training",
					"channel":        "training_dataset",
					"legal_basis":    "consent",
					"status":         "granted",
					"source":         "opt_in_flow",
					"jurisdiction":   "GLOBAL",
					"notice_version": "2026-ai-v1",
					"expires_at":     future,
					"evidence":       evidence,
				})
				if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
					t.Fatalf("failed to record consent for batch expiry %d (%d): %s", i, resp.StatusCode, string(body))
				}
				var created struct {
					ID string `json:"id"`
				}
				_ = json.Unmarshal(body, &created)
				_, err := cluster.dbPool.Exec(context.Background(), "UPDATE consents SET created_at = NOW() - INTERVAL '2 hours', expires_at = NOW() - INTERVAL '1 hour' WHERE id = $1", created.ID)
				if err != nil {
					t.Fatalf("failed to backdate expires_at for batch %d: %v", i, err)
				}
			default:
				// Suppressed consent
				doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/consents", tenantID, map[string]any{
					"subject_id":     subj,
					"purpose":        "ai_model_training",
					"channel":        "training_dataset",
					"legal_basis":    "consent",
					"status":         "granted",
					"source":         "opt_in_flow",
					"jurisdiction":   "GLOBAL",
					"notice_version": "2026-ai-v1",
					"evidence":       evidence,
				})
				active := true
				doHTTP(t, client, http.MethodPost, cluster.apiURL+"/v1/suppressions", tenantID, map[string]any{
					"subject_id": subj,
					"scope":      "global",
					"reason":     "batch_ai_suppression",
					"active":     &active,
				})
			}
		}

		// Execute Batch Evaluation via POST /v1/consents/evaluate-batch
		allowedDecisions, err := pipeline.EvaluateBatch(context.Background(), requests)
		if err != nil {
			t.Fatalf("batch evaluate failed: %v", err)
		}
		if len(allowedDecisions) != totalCandidates {
			t.Fatalf("expected %d decisions, got %d", totalCandidates, len(allowedDecisions))
		}

		var totalAllowed, totalDenied int
		for i, allowed := range allowedDecisions {
			if allowed {
				totalAllowed++
				if i >= 30 {
					t.Fatalf("Security failure: subject index %d was allowed but expected to be denied!", i)
				}
			} else {
				totalDenied++
				if i < 30 {
					t.Fatalf("Unexpected rejection: subject index %d was denied but had affirmative consent!", i)
				}
			}
		}

		if totalAllowed != 30 || totalDenied != 70 {
			t.Fatalf("Expected exactly 30 allowed and 70 denied; got allowed=%d, denied=%d", totalAllowed, totalDenied)
		}

		t.Logf("PASS: AI batch dataset ingestion strictly filtered: 100 candidate records evaluated -> 30 affirmative consents admitted, 70 non-affirmative/passive/denied/expired/suppressed records purged")
	})

	// =========================================================================
	// Subtest 5: Strict Fail-Closed Safe-Deny on API Outage
	// =========================================================================
	t.Run("StrictFailClosed_SafeDeny_OnAPIOutage", func(t *testing.T) {
		// Use dead/unreachable port to simulate offline service
		offlinePipeline := &SimulatedAIPipeline{
			apiURL:     "http://127.0.0.1:59999", // Unreachable port
			tenantID:   tenantID,
			httpClient: &http.Client{Timeout: 50 * time.Millisecond},
		}

		allowed, reason, err := offlinePipeline.EvaluateSingle(context.Background(), "sub-outage-test", "ai_model_training", "training_dataset")
		if err != nil {
			t.Fatalf("unexpected hard failure on fail-closed evaluation: %v", err)
		}
		if allowed {
			t.Fatalf("CRITICAL SECURITY FAILURE: AI pipeline allowed communication during an API outage!")
		}
		if reason != "service_unavailable_fail_closed" {
			t.Fatalf("expected reason 'service_unavailable_fail_closed', got %q", reason)
		}

		t.Logf("PASS: Strict Fail-Closed verified: AI pipeline refused to ingest data when Consent API was unreachable (allowed=%v, reason=%q)", allowed, reason)
	})
}
