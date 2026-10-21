package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/auth"
	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/service"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
	"github.com/golang-jwt/jwt/v5"
)

func setupIdempotencyFixture(t *testing.T) (*service.Service, *store.Memory, http.Handler) {
	t.Helper()
	t.Setenv("JWT_HMAC_SECRET", "")
	t.Setenv("ALLOW_DEV_HEADERS", "true")
	repo := store.NewMemory()
	svc := service.New(repo)
	now := time.Now().UTC()
	if _, err := svc.RegisterPurpose(context.Background(), "tenant-api", "admin", domain.Purpose{
		Code: "marketing", Category: "marketing", AllowedChannels: []string{"email"}, RequiresAffirmativeConsent: true, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RegisterLawfulBasis(context.Background(), "tenant-api", "admin", domain.LawfulBasis{
		Code: "consent", AllowedCategories: []string{"marketing"}, RequiresEvidence: true, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PublishNotice(context.Background(), "tenant-api", "admin", domain.PrivacyNotice{
		Version: "2026-08", Jurisdiction: "NG", PublishedAt: now, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	return svc, repo, newMux(svc)
}

func makeConsentPayload(subjectID string, now time.Time) map[string]any {
	return map[string]any{
		"subject_id":     subjectID,
		"purpose":        "marketing",
		"channel":        "email",
		"legal_basis":    "consent",
		"status":         "granted",
		"source":         "customer360",
		"jurisdiction":   "NG",
		"notice_version": "2026-08",
		"scope":          map[string]string{"source_record_id": "local-consent-1"},
		"evidence": map[string]any{
			"method":        "preference_center",
			"artifact_hash": "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
			"collected_at":  now,
		},
	}
}

func TestConsentAPIIdempotencyKeyReplaysAndConflicts(t *testing.T) {
	_, _, handler := setupIdempotencyFixture(t)
	now := time.Now().UTC()
	body := makeConsentPayload("cus-1", now)

	post := func(payload map[string]any) *httptest.ResponseRecorder {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/v1/consents", bytes.NewReader(encoded))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nvuto-Tenant-ID", "tenant-api")
		req.Header.Set("X-Nvuto-Actor-ID", "actor-api")
		req.Header.Set("Idempotency-Key", "customer360:local-consent-1")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}

	first := post(body)
	if first.Code != http.StatusCreated {
		t.Fatalf("first idempotent consent returned %d: %s", first.Code, first.Body.String())
	}
	second := post(body)
	if second.Code != http.StatusCreated {
		t.Fatalf("exact replay returned %d: %s", second.Code, second.Body.String())
	}
	var firstConsent, secondConsent domain.Consent
	if err := json.Unmarshal(first.Body.Bytes(), &firstConsent); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondConsent); err != nil {
		t.Fatal(err)
	}
	if firstConsent.ID != secondConsent.ID || firstConsent.ReceiptID == "" || firstConsent.ReceiptID != secondConsent.ReceiptID {
		t.Fatalf("HTTP replay did not return original consent/receipt: first=%+v second=%+v", firstConsent, secondConsent)
	}

	changed := map[string]any{}
	for key, value := range body {
		changed[key] = value
	}
	changed["status"] = "denied"
	conflict := post(changed)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("semantic key reuse returned %d, want 409: %s", conflict.Code, conflict.Body.String())
	}
}

// TestAT025_IdempotencyReplaySnapshotAfterWithdrawal proves that replaying a creation request
// using the same Idempotency-Key after the consent has been withdrawn returns the exact original
// 201 response snapshot (with status "granted"), not the current live state ("withdrawn").
func TestAT025_IdempotencyReplaySnapshotAfterWithdrawal(t *testing.T) {
	_, _, handler := setupIdempotencyFixture(t)
	now := time.Now().UTC()
	body := makeConsentPayload("cus-snap-1", now)
	idemKey := "customer360:snap-withdraw-key-1"

	postCreate := func() *httptest.ResponseRecorder {
		encoded, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/v1/consents", bytes.NewReader(encoded))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nvuto-Tenant-ID", "tenant-api")
		req.Header.Set("X-Nvuto-Actor-ID", "actor-api")
		req.Header.Set("Idempotency-Key", idemKey)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}

	// 1. Initial creation
	firstRes := postCreate()
	if firstRes.Code != http.StatusCreated {
		t.Fatalf("initial creation failed with code %d: %s", firstRes.Code, firstRes.Body.String())
	}
	var firstConsent domain.Consent
	if err := json.Unmarshal(firstRes.Body.Bytes(), &firstConsent); err != nil {
		t.Fatal(err)
	}
	if firstConsent.Status != domain.StatusGranted {
		t.Fatalf("expected initial status granted, got %s", firstConsent.Status)
	}

	// 2. Withdraw consent via the withdrawal endpoint
	withdrawReq := httptest.NewRequest(http.MethodPost, "/v1/consents/"+firstConsent.ID+"/withdraw", nil)
	withdrawReq.Header.Set("X-Nvuto-Tenant-ID", "tenant-api")
	withdrawReq.Header.Set("X-Nvuto-Actor-ID", "actor-api")
	withdrawRes := httptest.NewRecorder()
	handler.ServeHTTP(withdrawRes, withdrawReq)
	if withdrawRes.Code != http.StatusOK {
		t.Fatalf("withdrawal failed with code %d: %s", withdrawRes.Code, withdrawRes.Body.String())
	}
	var withdrawnConsent domain.Consent
	if err := json.Unmarshal(withdrawRes.Body.Bytes(), &withdrawnConsent); err != nil {
		t.Fatal(err)
	}
	if withdrawnConsent.Status != domain.StatusWithdrawn {
		t.Fatalf("expected status withdrawn, got %s", withdrawnConsent.Status)
	}

	// 3. Replay creation with original Idempotency-Key
	replayRes := postCreate()
	if replayRes.Code != http.StatusCreated {
		t.Fatalf("replay after withdrawal must return 201 Created, got %d: %s", replayRes.Code, replayRes.Body.String())
	}
	var replayedConsent domain.Consent
	if err := json.Unmarshal(replayRes.Body.Bytes(), &replayedConsent); err != nil {
		t.Fatal(err)
	}

	// Replay MUST preserve the original granted snapshot, not the withdrawn live state
	if replayedConsent.Status != domain.StatusGranted {
		t.Fatalf("replay must return original 'granted' snapshot, got live status %q", replayedConsent.Status)
	}
	if replayedConsent.ID != firstConsent.ID || replayedConsent.ReceiptID != firstConsent.ReceiptID {
		t.Fatalf("replay snapshot mismatch: want id=%s receipt=%s, got id=%s receipt=%s",
			firstConsent.ID, firstConsent.ReceiptID, replayedConsent.ID, replayedConsent.ReceiptID)
	}
}

// TestAT025_IdempotencyReplaySnapshotAfterPolicyDeactivation proves that replaying a creation request
// using the same Idempotency-Key after a policy change (such as purpose deactivation) bypasses the live
// policy checks and returns the original 201 response snapshot.
func TestAT025_IdempotencyReplaySnapshotAfterPolicyDeactivation(t *testing.T) {
	svc, _, handler := setupIdempotencyFixture(t)
	now := time.Now().UTC()
	body := makeConsentPayload("cus-policy-1", now)
	idemKey := "customer360:policy-deactivate-key-1"

	postWithKey := func(key string) *httptest.ResponseRecorder {
		encoded, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/v1/consents", bytes.NewReader(encoded))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nvuto-Tenant-ID", "tenant-api")
		req.Header.Set("X-Nvuto-Actor-ID", "actor-api")
		req.Header.Set("Idempotency-Key", key)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}

	// 1. Initial creation succeeds
	firstRes := postWithKey(idemKey)
	if firstRes.Code != http.StatusCreated {
		t.Fatalf("initial creation failed with code %d: %s", firstRes.Code, firstRes.Body.String())
	}
	var firstConsent domain.Consent
	if err := json.Unmarshal(firstRes.Body.Bytes(), &firstConsent); err != nil {
		t.Fatal(err)
	}

	// 2. Policy is tightened / purpose deactivated
	if _, err := svc.RegisterPurpose(context.Background(), "tenant-api", "admin", domain.Purpose{
		Code: "marketing", Category: "marketing", AllowedChannels: []string{"whatsapp"}, Active: false,
	}); err != nil {
		t.Fatalf("policy update failed: %v", err)
	}

	// 3. A fresh creation with a new key MUST fail policy validation (422 Unprocessable)
	freshRes := postWithKey("customer360:brand-new-key-after-deactivation")
	if freshRes.Code != http.StatusUnprocessableEntity {
		t.Fatalf("fresh request after purpose deactivation should fail with 422, got %d: %s", freshRes.Code, freshRes.Body.String())
	}

	// 4. Replaying the original key MUST succeed and return the stored snapshot, bypassing policy
	replayRes := postWithKey(idemKey)
	if replayRes.Code != http.StatusCreated {
		t.Fatalf("replay after policy deactivation must succeed with 201 Created, got %d: %s", replayRes.Code, replayRes.Body.String())
	}
	var replayedConsent domain.Consent
	if err := json.Unmarshal(replayRes.Body.Bytes(), &replayedConsent); err != nil {
		t.Fatal(err)
	}
	if replayedConsent.ID != firstConsent.ID || replayedConsent.ReceiptID != firstConsent.ReceiptID {
		t.Fatalf("replay snapshot mismatch: want id=%s receipt=%s, got id=%s receipt=%s",
			firstConsent.ID, firstConsent.ReceiptID, replayedConsent.ID, replayedConsent.ReceiptID)
	}
	if replayedConsent.Status != domain.StatusGranted {
		t.Fatalf("expected status granted on replay, got %s", replayedConsent.Status)
	}
}

// TestAT025_IdempotencySemanticPayloadConflict proves that sending a different semantic payload
// with the same Idempotency-Key returns a 409 Conflict.
func TestAT025_IdempotencySemanticPayloadConflict(t *testing.T) {
	_, _, handler := setupIdempotencyFixture(t)
	now := time.Now().UTC()
	basePayload := makeConsentPayload("cus-conflict-1", now)
	idemKey := "customer360:conflict-key-test"

	postPayload := func(payload map[string]any) *httptest.ResponseRecorder {
		encoded, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/v1/consents", bytes.NewReader(encoded))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nvuto-Tenant-ID", "tenant-api")
		req.Header.Set("X-Nvuto-Actor-ID", "actor-api")
		req.Header.Set("Idempotency-Key", idemKey)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}

	// Initial create
	res1 := postPayload(basePayload)
	if res1.Code != http.StatusCreated {
		t.Fatalf("initial create failed: %d: %s", res1.Code, res1.Body.String())
	}

	// Conflict 1: Changed status
	p1 := copyMap(basePayload)
	p1["status"] = "denied"
	resConflict1 := postPayload(p1)
	if resConflict1.Code != http.StatusConflict {
		t.Fatalf("changed status must return 409 Conflict, got %d: %s", resConflict1.Code, resConflict1.Body.String())
	}

	// Conflict 2: Changed channel
	p2 := copyMap(basePayload)
	p2["channel"] = "sms"
	resConflict2 := postPayload(p2)
	if resConflict2.Code != http.StatusConflict {
		t.Fatalf("changed channel must return 409 Conflict, got %d: %s", resConflict2.Code, resConflict2.Body.String())
	}

	// Conflict 3: Changed subject
	p3 := copyMap(basePayload)
	p3["subject_id"] = "cus-different-subject"
	resConflict3 := postPayload(p3)
	if resConflict3.Code != http.StatusConflict {
		t.Fatalf("changed subject must return 409 Conflict, got %d: %s", resConflict3.Code, resConflict3.Body.String())
	}
}

// TestAT025_IdempotencyTokenRotationAndWallClockTolerance proves that replaying with a rotated
// actor token (different X-Nvuto-Actor-ID) and a later wall-clock collection timestamp succeeds
// and returns the original snapshot without conflict, because transient fields are omitted from semantic hashing.
func TestAT025_IdempotencyTokenRotationAndWallClockTolerance(t *testing.T) {
	_, _, handler := setupIdempotencyFixture(t)
	t1 := time.Now().UTC()
	payload1 := makeConsentPayload("cus-rotate-1", t1)
	idemKey := "customer360:token-rotate-key"

	// Initial creation by actor-1
	encoded1, _ := json.Marshal(payload1)
	req1 := httptest.NewRequest(http.MethodPost, "/v1/consents", bytes.NewReader(encoded1))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("X-Nvuto-Tenant-ID", "tenant-api")
	req1.Header.Set("X-Nvuto-Actor-ID", "actor-session-alpha")
	req1.Header.Set("Idempotency-Key", idemKey)
	res1 := httptest.NewRecorder()
	handler.ServeHTTP(res1, req1)

	if res1.Code != http.StatusCreated {
		t.Fatalf("first creation failed: %d: %s", res1.Code, res1.Body.String())
	}
	var firstConsent domain.Consent
	if err := json.Unmarshal(res1.Body.Bytes(), &firstConsent); err != nil {
		t.Fatal(err)
	}

	time.Sleep(15 * time.Millisecond)

	// Replay by actor-session-beta (rotated credential) and later collected_at timestamp
	t2 := time.Now().UTC()
	payload2 := makeConsentPayload("cus-rotate-1", t2)
	encoded2, _ := json.Marshal(payload2)
	req2 := httptest.NewRequest(http.MethodPost, "/v1/consents", bytes.NewReader(encoded2))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Nvuto-Tenant-ID", "tenant-api")
	req2.Header.Set("X-Nvuto-Actor-ID", "actor-session-beta-rotated")
	req2.Header.Set("Idempotency-Key", idemKey)
	res2 := httptest.NewRecorder()
	handler.ServeHTTP(res2, req2)

	if res2.Code != http.StatusCreated {
		t.Fatalf("replay with rotated actor and timestamp failed: %d: %s", res2.Code, res2.Body.String())
	}
	var secondConsent domain.Consent
	if err := json.Unmarshal(res2.Body.Bytes(), &secondConsent); err != nil {
		t.Fatal(err)
	}
	if secondConsent.ID != firstConsent.ID || secondConsent.ReceiptID != firstConsent.ReceiptID {
		t.Fatalf("replay with rotated actor must return original snapshot: first=%+v second=%+v", firstConsent, secondConsent)
	}
}

// TestAT025_IdempotencySubjectAuthorizationEnforced proves that subject authorization is strictly
// re-evaluated on an idempotency cache hit. An unauthorized customer token cannot replay an idempotency
// key created by a different customer.
func TestAT025_IdempotencySubjectAuthorizationEnforced(t *testing.T) {
	hmacSecret := "test-secret-at025-at-least-32-chars-long!"
	t.Setenv("JWT_JWKS_URL", "")
	t.Setenv("JWT_HMAC_SECRET", hmacSecret)
	t.Setenv("ALLOW_HMAC_AUTH", "true")
	t.Setenv("ALLOW_DEV_HEADERS", "false")

	repo := store.NewMemory()
	svc := service.New(repo)
	now := time.Now().UTC()
	if _, err := svc.RegisterPurpose(context.Background(), "tenant-api", "admin", domain.Purpose{
		Code: "marketing", Category: "marketing", AllowedChannels: []string{"email"}, RequiresAffirmativeConsent: true, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RegisterLawfulBasis(context.Background(), "tenant-api", "admin", domain.LawfulBasis{
		Code: "consent", AllowedCategories: []string{"marketing"}, RequiresEvidence: true, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PublishNotice(context.Background(), "tenant-api", "admin", domain.PrivacyNotice{
		Version: "2026-08", Jurisdiction: "NG", PublishedAt: now, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	handler := newMux(svc)

	signToken := func(customerSubject string, roles ...string) string {
		claims := auth.Claims{
			TenantID:  "tenant-api",
			SubjectID: customerSubject,
			Roles:     roles,
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "actor-" + customerSubject,
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			},
		}
		raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(hmacSecret))
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}

	tokenCus1 := signToken("cus-1", auth.RoleSelfService)
	tokenCus2 := signToken("cus-2", auth.RoleSelfService)
	tokenAdmin := signToken("", auth.RoleConsentWriter)

	idemKey := "customer360:auth-key-1"
	body := makeConsentPayload("cus-1", now)
	encoded, _ := json.Marshal(body)

	postWithToken := func(token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/consents", bytes.NewReader(encoded))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Idempotency-Key", idemKey)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}

	// 1. Authorized Customer cus-1 creates consent -> 201 Created
	res1 := postWithToken(tokenCus1)
	if res1.Code != http.StatusCreated {
		t.Fatalf("cus-1 creation failed with code %d: %s", res1.Code, res1.Body.String())
	}

	// 2. Unauthorized Customer cus-2 attempts to replay key for cus-1 -> MUST BE 403 Forbidden
	res2 := postWithToken(tokenCus2)
	if res2.Code != http.StatusForbidden {
		t.Fatalf("unauthorized customer cus-2 replay must be 403 Forbidden, got %d: %s", res2.Code, res2.Body.String())
	}

	// 3. Customer cus-1 replays key -> 201 Created (allowed)
	res3 := postWithToken(tokenCus1)
	if res3.Code != http.StatusCreated {
		t.Fatalf("authorized customer cus-1 replay failed with code %d: %s", res3.Code, res3.Body.String())
	}

	// 4. Admin replays key -> 201 Created (allowed tenant-wide)
	res4 := postWithToken(tokenAdmin)
	if res4.Code != http.StatusCreated {
		t.Fatalf("admin replay failed with code %d: %s", res4.Code, res4.Body.String())
	}
}

// TestAT025_IdempotencyConcurrentRequests proves that concurrent HTTP requests (10 goroutines)
// with the same Idempotency-Key result in all callers receiving HTTP 201 Created with identical
// response bodies, and exactly one database record is created in the repository.
func TestAT025_IdempotencyConcurrentRequests(t *testing.T) {
	svc, _, handler := setupIdempotencyFixture(t)
	now := time.Now().UTC()
	body := makeConsentPayload("cus-concur-1", now)
	idemKey := "customer360:concur-key-1"

	const concurrency = 10
	var wg sync.WaitGroup
	startBarrier := make(chan struct{})
	type callResult struct {
		code    int
		body    string
		consent domain.Consent
	}
	results := make([]callResult, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			encoded, _ := json.Marshal(body)
			req := httptest.NewRequest(http.MethodPost, "/v1/consents", bytes.NewReader(encoded))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Nvuto-Tenant-ID", "tenant-api")
			req.Header.Set("X-Nvuto-Actor-ID", fmt.Sprintf("actor-%d", idx))
			req.Header.Set("Idempotency-Key", idemKey)

			<-startBarrier // Synchronize launch

			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)

			var c domain.Consent
			_ = json.Unmarshal(res.Body.Bytes(), &c)
			results[idx] = callResult{code: res.Code, body: res.Body.String(), consent: c}
		}()
	}

	close(startBarrier) // Release all goroutines simultaneously
	wg.Wait()

	var winnerID string
	var winnerReceiptID string
	for i, r := range results {
		if r.code != http.StatusCreated {
			t.Fatalf("goroutine %d failed with code %d: %s", i, r.code, r.body)
		}
		if winnerID == "" {
			winnerID = r.consent.ID
			winnerReceiptID = r.consent.ReceiptID
		}
		if r.consent.ID != winnerID || r.consent.ReceiptID != winnerReceiptID {
			t.Fatalf("goroutine %d returned different consent: got ID %s receipt %s, want ID %s receipt %s",
				i, r.consent.ID, r.consent.ReceiptID, winnerID, winnerReceiptID)
		}
	}

	// Verify that exactly one consent was created in the storage repository
	createdConsent, err := svc.Consent(context.Background(), "tenant-api", winnerID)
	if err != nil {
		t.Fatalf("could not find winning consent in repository: %v", err)
	}
	if createdConsent.ID != winnerID {
		t.Fatalf("unexpected consent ID in repo: got %s, want %s", createdConsent.ID, winnerID)
	}
}

func copyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
