package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/auth"
	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/service"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
	"github.com/golang-jwt/jwt/v5"
)

const (
	authzSecret = "authz-test-secret-long-enough-for-hs256"
	authzTenant = "tenant-authz"
)

type authzFixture struct {
	t        *testing.T
	repo     *store.Memory
	svc      *service.Service
	handler  http.Handler
	evidence domain.Evidence
}

func newAuthzFixture(t *testing.T) *authzFixture {
	t.Helper()
	t.Setenv("JWT_JWKS_URL", "")
	t.Setenv("JWT_HMAC_SECRET", authzSecret)
	t.Setenv("ALLOW_HMAC_AUTH", "true")
	t.Setenv("ALLOW_DEV_HEADERS", "false")
	repo := store.NewMemory()
	svc := service.New(repo)
	publishedAt := time.Now().UTC().Add(-time.Minute)
	for _, tenant := range []string{authzTenant, "tenant-other"} {
		if _, err := svc.RegisterPurpose(context.Background(), tenant, "admin", domain.Purpose{Code: "marketing", Category: "marketing", AllowedChannels: []string{"email"}, RequiresAffirmativeConsent: true, Active: true}); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.RegisterPurpose(context.Background(), tenant, "admin", domain.Purpose{Code: "double_marketing", Category: "marketing", AllowedChannels: []string{"email"}, RequiresAffirmativeConsent: true, RequiresDoubleOptIn: true, Active: true}); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.RegisterLawfulBasis(context.Background(), tenant, "admin", domain.LawfulBasis{Code: "consent", AllowedCategories: []string{"marketing"}, RequiresEvidence: true, Active: true}); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.PublishNotice(context.Background(), tenant, "admin", domain.PrivacyNotice{Version: "2026-08", Jurisdiction: "NG", PublishedAt: publishedAt, Active: true}); err != nil {
			t.Fatal(err)
		}
	}
	return &authzFixture{
		t:        t,
		repo:     repo,
		svc:      svc,
		handler:  newMux(svc),
		evidence: domain.Evidence{Method: "preference_center", ArtifactHash: "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", CollectedAt: time.Now().UTC()},
	}
}

func (f *authzFixture) token(tenant, customer string, roles ...string) string {
	f.t.Helper()
	claims := auth.Claims{
		TenantID:  tenant,
		SubjectID: customer,
		Roles:     roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "idp-user-" + customer + "-" + strings.Join(roles, "+"),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
	}
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(authzSecret))
	if err != nil {
		f.t.Fatal(err)
	}
	return raw
}

func (f *authzFixture) do(token, method, path string, body any) *httptest.ResponseRecorder {
	f.t.Helper()
	var encoded bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&encoded).Encode(body); err != nil {
			f.t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &encoded)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, req)
	return response
}

func (f *authzFixture) consentBody(subject string) map[string]any {
	return map[string]any{
		"subject_id": subject, "purpose": "marketing", "channel": "email", "legal_basis": "consent", "status": "granted",
		"source": "preference_center", "jurisdiction": "NG", "notice_version": "2026-08", "evidence": f.evidence,
	}
}

func (f *authzFixture) doiBody(subject string) map[string]any {
	return map[string]any{
		"subject_id": subject, "purpose": "double_marketing", "channel": "email", "source": "signup_form",
		"jurisdiction": "NG", "notice_version": "2026-08", "evidence": f.evidence, "expires_at": time.Now().UTC().Add(time.Hour),
	}
}

// mutationCounts captures the persisted side effects a rejected request must not change.
func (f *authzFixture) mutationCounts() (audit, outbox int) {
	return len(f.repo.Audit()), len(f.repo.Outbox())
}

func (f *authzFixture) assertNoMutation(label string, audit, outbox int) {
	f.t.Helper()
	if a, o := f.mutationCounts(); a != audit || o != outbox {
		f.t.Fatalf("%s: rejected request mutated state (audit %d->%d, outbox %d->%d)", label, audit, a, outbox, o)
	}
}

// T-030: a read-only tenant-wide role must not widen a self_service writer.
func TestMixedRoleSelfServiceCannotWriteOtherSubjects(t *testing.T) {
	f := newAuthzFixture(t)
	victimConsent, err := f.svc.RecordGoverned(context.Background(), authzTenant, "admin", "cus-victim", "marketing", "email", "consent", "granted", "admin", nil, nil, "NG", "2026-08", f.evidence)
	if err != nil {
		t.Fatal(err)
	}

	mixedRoleSets := [][]string{
		{"self_service", "service"},
		{"self_service", "consent_reader"},
		{"self_service", "service", "consent_reader"},
	}
	for _, roles := range mixedRoleSets {
		t.Run(strings.Join(roles, "+"), func(t *testing.T) {
			token := f.token(authzTenant, "cus-attacker", roles...)
			writes := []struct {
				name   string
				method string
				path   string
				body   any
			}{
				{"record consent", http.MethodPost, "/v1/consents", f.consentBody("cus-victim")},
				{"withdraw consent", http.MethodPost, "/v1/consents/" + victimConsent.ID + "/withdraw", nil},
				{"set suppression", http.MethodPost, "/v1/suppressions", map[string]any{"subject_id": "cus-victim", "scope": "global", "reason": "opt_out", "active": true}},
				{"start double opt-in", http.MethodPost, "/v1/double-opt-in", f.doiBody("cus-victim")},
			}
			for _, w := range writes {
				audit, outbox := f.mutationCounts()
				response := f.do(token, w.method, w.path, w.body)
				if response.Code != http.StatusForbidden {
					t.Fatalf("%s for another subject returned %d, want 403: %s", w.name, response.Code, response.Body.String())
				}
				f.assertNoMutation(w.name, audit, outbox)
			}
			stored, err := f.repo.Get(context.Background(), authzTenant, victimConsent.ID)
			if err != nil || stored.Status != domain.StatusGranted {
				t.Fatalf("victim consent altered: %+v err=%v", stored, err)
			}

			// Own-subject writes remain allowed for the self_service portion.
			if response := f.do(token, http.MethodPost, "/v1/consents", f.consentBody("cus-attacker")); response.Code != http.StatusCreated {
				t.Fatalf("own-subject write returned %d: %s", response.Code, response.Body.String())
			}
			// The read role still legitimately grants tenant-wide reads.
			if response := f.do(token, http.MethodPost, "/v1/evaluate", map[string]any{"subject_id": "cus-victim", "purpose": "marketing", "channel": "email"}); response.Code != http.StatusOK {
				t.Fatalf("tenant-wide read returned %d: %s", response.Code, response.Body.String())
			}
		})
	}

	// Explicitly granted tenant-wide writer authority still works.
	writer := f.token(authzTenant, "cus-attacker", "self_service", "consent_writer")
	if response := f.do(writer, http.MethodPost, "/v1/consents", f.consentBody("cus-victim")); response.Code != http.StatusCreated {
		t.Fatalf("explicit consent_writer returned %d: %s", response.Code, response.Body.String())
	}
}

func TestSubjectScopePermissionMatrix(t *testing.T) {
	cases := []struct {
		roles     []string
		customer  string
		act       action
		subject   string
		permitted bool
	}{
		// Single roles, write.
		{[]string{"privacy_admin"}, "", actionWrite, "cus-b", true},
		{[]string{"consent_admin"}, "", actionWrite, "cus-b", true},
		{[]string{"consent_writer"}, "", actionWrite, "cus-b", true},
		{[]string{"developer"}, "", actionWrite, "cus-b", true},
		{[]string{"consent_reader"}, "", actionWrite, "cus-b", false},
		{[]string{"service"}, "", actionWrite, "cus-b", false},
		{[]string{"self_service"}, "cus-a", actionWrite, "cus-a", true},
		{[]string{"self_service"}, "cus-a", actionWrite, "cus-b", false},
		{[]string{"self_service"}, "", actionWrite, "", false},
		// Single roles, read.
		{[]string{"consent_reader"}, "", actionRead, "cus-b", true},
		{[]string{"service"}, "", actionRead, "cus-b", true},
		{[]string{"self_service"}, "cus-a", actionRead, "cus-b", false},
		{[]string{"self_service"}, "cus-a", actionRead, "cus-a", true},
		// Mixed roles (T-030).
		{[]string{"self_service", "service"}, "cus-a", actionWrite, "cus-b", false},
		{[]string{"self_service", "consent_reader"}, "cus-a", actionWrite, "cus-b", false},
		{[]string{"self_service", "service"}, "cus-a", actionWrite, "cus-a", true},
		{[]string{"self_service", "service"}, "cus-a", actionRead, "cus-b", true},
		{[]string{"self_service", "consent_writer"}, "cus-a", actionWrite, "cus-b", true},
		// No roles.
		{nil, "cus-a", actionRead, "cus-a", false},
	}
	for _, tc := range cases {
		p := auth.Principal{TenantID: authzTenant, ActorID: "actor", SubjectID: tc.customer, Roles: tc.roles}
		if got := subjectAuthorized(p, tc.act, tc.subject); got != tc.permitted {
			t.Errorf("roles=%v customer=%q act=%d subject=%q: got %v, want %v", tc.roles, tc.customer, tc.act, tc.subject, got, tc.permitted)
		}
	}
}

// T-002: DOI confirmation must enforce the self-service customer boundary.
func TestDoubleOptInConfirmationEnforcesCustomerBoundary(t *testing.T) {
	f := newAuthzFixture(t)
	challenge, err := f.svc.StartDoubleOptIn(context.Background(), authzTenant, "admin", "cus-a", "double_marketing", "email", "signup_form", "NG", "2026-08", f.evidence, time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"token": challenge.ConfirmationToken}

	assertStillPending := func(label string) {
		t.Helper()
		stored, err := f.repo.Get(context.Background(), authzTenant, challenge.Consent.ID)
		if err != nil || stored.Status != domain.StatusPendingConfirmation || stored.ReceiptID != "" || stored.ConfirmationTokenHash == "" {
			t.Fatalf("%s: challenge mutated: %+v err=%v", label, stored, err)
		}
	}

	rejected := []struct {
		name  string
		token string
		want  int
	}{
		{"customer B self_service", f.token(authzTenant, "cus-b", "self_service"), http.StatusNotFound},
		{"customer B self_service+service", f.token(authzTenant, "cus-b", "self_service", "service"), http.StatusNotFound},
		{"customer B self_service+consent_reader", f.token(authzTenant, "cus-b", "self_service", "consent_reader"), http.StatusNotFound},
		{"read-only service role", f.token(authzTenant, "", "service"), http.StatusForbidden},
		{"other tenant admin", f.token("tenant-other", "", "consent_admin"), http.StatusNotFound},
		{"self_service missing customer claim", f.token(authzTenant, "", "self_service"), http.StatusUnauthorized},
	}
	for _, tc := range rejected {
		audit, outbox := f.mutationCounts()
		response := f.do(tc.token, http.MethodPost, "/v1/double-opt-in/confirm", body)
		if response.Code != tc.want {
			t.Fatalf("%s: confirm returned %d, want %d: %s", tc.name, response.Code, tc.want, response.Body.String())
		}
		f.assertNoMutation(tc.name, audit, outbox)
		assertStillPending(tc.name)
	}

	// A cross-subject attempt is indistinguishable from an unknown token.
	unknown := f.do(f.token(authzTenant, "cus-b", "self_service"), http.MethodPost, "/v1/double-opt-in/confirm", map[string]any{"token": "deadbeef"})
	crossSubject := f.do(f.token(authzTenant, "cus-b", "self_service"), http.MethodPost, "/v1/double-opt-in/confirm", body)
	var uBody, csBody struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(unknown.Body.Bytes(), &uBody)
	_ = json.Unmarshal(crossSubject.Body.Bytes(), &csBody)
	if unknown.Code != crossSubject.Code || uBody != csBody {
		t.Fatalf("cross-subject response distinguishable from unknown token: %d %+v vs %d %+v", crossSubject.Code, csBody, unknown.Code, uBody)
	}

	// Customer A confirms their own challenge.
	response := f.do(f.token(authzTenant, "cus-a", "self_service"), http.MethodPost, "/v1/double-opt-in/confirm", body)
	if response.Code != http.StatusOK {
		t.Fatalf("own-subject confirm returned %d: %s", response.Code, response.Body.String())
	}
	var confirmed domain.Consent
	if err := json.Unmarshal(response.Body.Bytes(), &confirmed); err != nil {
		t.Fatal(err)
	}
	if confirmed.Status != domain.StatusGranted || confirmed.SubjectID != "cus-a" || confirmed.ReceiptID == "" {
		t.Fatalf("own-subject confirm produced %+v", confirmed)
	}
}

func TestDoubleOptInConfirmationByTenantWideWriter(t *testing.T) {
	for _, role := range []string{"consent_writer", "privacy_admin", "consent_admin", "developer"} {
		t.Run(role, func(t *testing.T) {
			f := newAuthzFixture(t)
			if role == "developer" {
				t.Setenv("ALLOW_DEV_HEADERS", "true")
			}
			challenge, err := f.svc.StartDoubleOptIn(context.Background(), authzTenant, "admin", "cus-a", "double_marketing", "email", "signup_form", "NG", "2026-08", f.evidence, time.Now().UTC().Add(time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			response := f.do(f.token(authzTenant, "", role), http.MethodPost, "/v1/double-opt-in/confirm", map[string]any{"token": challenge.ConfirmationToken})
			if response.Code != http.StatusOK {
				t.Fatalf("tenant-wide role %s confirm returned %d: %s", role, response.Code, response.Body.String())
			}
			stored, err := f.repo.Get(context.Background(), authzTenant, challenge.Consent.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.Status != domain.StatusGranted || stored.ReceiptID == "" {
				t.Fatalf("tenant-wide role %s failed to grant consent: %+v", role, stored)
			}
		})
	}
}

func TestServiceConfirmDoubleOptInScopeFailsClosed(t *testing.T) {
	f := newAuthzFixture(t)
	challenge, err := f.svc.StartDoubleOptIn(context.Background(), authzTenant, "admin", "cus-a", "double_marketing", "email", "signup_form", "NG", "2026-08", f.evidence, time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	for name, scope := range map[string]service.SubjectScope{
		"zero value":        {},
		"empty own subject": service.OwnSubject(""),
		"other subject":     service.OwnSubject("cus-b"),
	} {
		audit, outbox := f.mutationCounts()
		if _, err := f.svc.ConfirmDoubleOptIn(context.Background(), authzTenant, "actor", scope, challenge.ConfirmationToken); err != store.ErrNotFound {
			t.Fatalf("%s scope: got %v, want ErrNotFound", name, err)
		}
		f.assertNoMutation(name, audit, outbox)
	}
	if _, err := f.svc.ConfirmDoubleOptIn(context.Background(), authzTenant, "actor", service.OwnSubject("cus-a"), challenge.ConfirmationToken); err != nil {
		t.Fatalf("own subject scope rejected: %v", err)
	}
}

// T-030: Explicitly reject the developer role in production environments.
func TestDeveloperRoleExplicitlyRejectedInProduction(t *testing.T) {
	f := newAuthzFixture(t)
	t.Setenv("ALLOW_DEV_HEADERS", "false")
	devToken := f.token(authzTenant, "cus-dev", "developer")

	// Write endpoint rejection
	audit, outbox := f.mutationCounts()
	respWrite := f.do(devToken, http.MethodPost, "/v1/consents", f.consentBody("cus-dev"))
	if respWrite.Code != http.StatusUnauthorized && respWrite.Code != http.StatusForbidden {
		t.Fatalf("developer write in production returned %d, want 401 or 403: %s", respWrite.Code, respWrite.Body.String())
	}
	f.assertNoMutation("developer write rejection", audit, outbox)

	// Read endpoint rejection
	respRead := f.do(devToken, http.MethodPost, "/v1/evaluate", map[string]any{"subject_id": "cus-dev", "purpose": "marketing", "channel": "email"})
	if respRead.Code != http.StatusUnauthorized && respRead.Code != http.StatusForbidden {
		t.Fatalf("developer read in production returned %d, want 401 or 403: %s", respRead.Code, respRead.Body.String())
	}

	// But in development mode (ALLOW_DEV_HEADERS=true), developer role is accepted.
	t.Setenv("ALLOW_DEV_HEADERS", "true")
	respDevWrite := f.do(devToken, http.MethodPost, "/v1/consents", f.consentBody("cus-dev"))
	if respDevWrite.Code != http.StatusCreated {
		t.Fatalf("developer write with ALLOW_DEV_HEADERS=true returned %d: %s", respDevWrite.Code, respDevWrite.Body.String())
	}
}

// T-030: Exhaustive role-matrix tests checking all 7 roles against read and write boundaries.
func TestExhaustiveRoleMatrixAll7Roles(t *testing.T) {
	allRoles := []struct {
		name        string
		role        string
		canWriteOwn bool
		canWriteAll bool
		canReadOwn  bool
		canReadAll  bool
	}{
		{"privacy_admin", "privacy_admin", true, true, true, true},
		{"consent_admin", "consent_admin", true, true, true, true},
		{"consent_writer", "consent_writer", true, true, true, true},
		{"consent_reader", "consent_reader", false, false, true, true},
		{"service", "service", false, false, true, true},
		{"self_service", "self_service", true, false, true, false},
		{"developer", "developer", false, false, false, false}, // in production
	}

	for _, tc := range allRoles {
		t.Run(tc.name, func(t *testing.T) {
			f := newAuthzFixture(t)
			t.Setenv("ALLOW_DEV_HEADERS", "false")
			customer := "cus-" + tc.name
			token := f.token(authzTenant, customer, tc.role)

			// 1. Write own subject
			audit, outbox := f.mutationCounts()
			respOwnWrite := f.do(token, http.MethodPost, "/v1/consents", f.consentBody(customer))
			if tc.canWriteOwn {
				if respOwnWrite.Code != http.StatusCreated {
					t.Fatalf("%s write own subject returned %d, want 201: %s", tc.name, respOwnWrite.Code, respOwnWrite.Body.String())
				}
			} else {
				if respOwnWrite.Code != http.StatusForbidden && respOwnWrite.Code != http.StatusUnauthorized {
					t.Fatalf("%s write own subject returned %d, want 401 or 403: %s", tc.name, respOwnWrite.Code, respOwnWrite.Body.String())
				}
				f.assertNoMutation(tc.name+" write own subject", audit, outbox)
			}

			// 2. Write other subject
			audit, outbox = f.mutationCounts()
			respOtherWrite := f.do(token, http.MethodPost, "/v1/consents", f.consentBody("cus-other"))
			if tc.canWriteAll {
				if respOtherWrite.Code != http.StatusCreated {
					t.Fatalf("%s write other subject returned %d, want 201: %s", tc.name, respOtherWrite.Code, respOtherWrite.Body.String())
				}
			} else {
				if respOtherWrite.Code != http.StatusForbidden && respOtherWrite.Code != http.StatusUnauthorized {
					t.Fatalf("%s write other subject returned %d, want 401 or 403: %s", tc.name, respOtherWrite.Code, respOtherWrite.Body.String())
				}
				f.assertNoMutation(tc.name+" write other subject", audit, outbox)
			}

			// 3. Read own subject
			respOwnRead := f.do(token, http.MethodPost, "/v1/evaluate", map[string]any{"subject_id": customer, "purpose": "marketing", "channel": "email"})
			if tc.canReadOwn {
				if respOwnRead.Code != http.StatusOK {
					t.Fatalf("%s read own subject returned %d, want 200: %s", tc.name, respOwnRead.Code, respOwnRead.Body.String())
				}
			} else {
				if respOwnRead.Code != http.StatusForbidden && respOwnRead.Code != http.StatusUnauthorized {
					t.Fatalf("%s read own subject returned %d, want 401 or 403: %s", tc.name, respOwnRead.Code, respOwnRead.Body.String())
				}
			}

			// 4. Read other subject
			respOtherRead := f.do(token, http.MethodPost, "/v1/evaluate", map[string]any{"subject_id": "cus-other", "purpose": "marketing", "channel": "email"})
			if tc.canReadAll {
				if respOtherRead.Code != http.StatusOK {
					t.Fatalf("%s read other subject returned %d, want 200: %s", tc.name, respOtherRead.Code, respOtherRead.Body.String())
				}
			} else {
				if respOtherRead.Code != http.StatusForbidden && respOtherRead.Code != http.StatusUnauthorized {
					t.Fatalf("%s read other subject returned %d, want 401 or 403: %s", tc.name, respOtherRead.Code, respOtherRead.Body.String())
				}
			}
		})
	}
}

// T-030: Protected administrative suppressions cannot be revoked by self_service callers.
func TestProtectedAdministrativeSuppressionCannotBeRevokedBySelfService(t *testing.T) {
	f := newAuthzFixture(t)
	target := "cus-protected-target"

	adminToken := f.token(authzTenant, "admin-user", "privacy_admin")
	selfToken := f.token(authzTenant, target, "self_service")
	mixedToken := f.token(authzTenant, target, "self_service", "consent_reader")

	// 1. Administrative user creates an administrative global suppression.
	respAdminGlobal := f.do(adminToken, http.MethodPost, "/v1/suppressions", map[string]any{
		"subject_id": target,
		"scope":      "global",
		"reason":     "legal_compliance_hold",
		"active":     true,
	})
	if respAdminGlobal.Code != http.StatusCreated {
		t.Fatalf("admin create global suppression failed: %d %s", respAdminGlobal.Code, respAdminGlobal.Body.String())
	}

	// Also admin creates a channel suppression.
	respAdminChannel := f.do(adminToken, http.MethodPost, "/v1/suppressions", map[string]any{
		"subject_id": target,
		"scope":      "channel",
		"channel":    "email",
		"reason":     "regulatory_channel_block",
		"active":     true,
	})
	if respAdminChannel.Code != http.StatusCreated {
		t.Fatalf("admin create channel suppression failed: %d %s", respAdminChannel.Code, respAdminChannel.Body.String())
	}

	// 2. Self_service caller attempts to clear / revoke the global administrative suppression.
	auditBefore, outboxBefore := f.mutationCounts()
	respRevokeGlobal := f.do(selfToken, http.MethodPost, "/v1/suppressions", map[string]any{
		"subject_id": target,
		"scope":      "global",
		"reason":     "trying_to_unsuppress",
		"active":     false,
	})
	if respRevokeGlobal.Code != http.StatusForbidden {
		t.Fatalf("self_service revoke admin global suppression returned %d, want 403: %s", respRevokeGlobal.Code, respRevokeGlobal.Body.String())
	}
	var errBody struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(respRevokeGlobal.Body.Bytes(), &errBody)
	if errBody.Error != "suppression_protected" {
		t.Fatalf("expected error code suppression_protected, got %q", errBody.Error)
	}
	f.assertNoMutation("self_service clear admin global suppression", auditBefore, outboxBefore)

	// 3. Mixed-role caller ["self_service", "consent_reader"] also cannot revoke the channel suppression.
	auditBefore, outboxBefore = f.mutationCounts()
	respRevokeChannel := f.do(mixedToken, http.MethodPost, "/v1/suppressions", map[string]any{
		"subject_id": target,
		"scope":      "channel",
		"channel":    "email",
		"reason":     "trying_to_unsuppress_channel",
		"active":     false,
	})
	if respRevokeChannel.Code != http.StatusForbidden {
		t.Fatalf("mixed self_service revoke admin channel suppression returned %d, want 403: %s", respRevokeChannel.Code, respRevokeChannel.Body.String())
	}
	_ = json.Unmarshal(respRevokeChannel.Body.Bytes(), &errBody)
	if errBody.Error != "suppression_protected" {
		t.Fatalf("expected error code suppression_protected, got %q", errBody.Error)
	}
	f.assertNoMutation("mixed self_service clear admin channel suppression", auditBefore, outboxBefore)

	// 4. Verify suppression is STILL ACTIVE in storage.
	stored, err := f.svc.Evaluate(context.Background(), authzTenant, target, "marketing", "email", time.Time{})
	if err != nil || stored.Allowed {
		t.Fatalf("administrative suppression must remain active: %+v", stored)
	}

	// 5. Self-service caller creates their own purpose_channel suppression.
	respSelfCreate := f.do(selfToken, http.MethodPost, "/v1/suppressions", map[string]any{
		"subject_id": target,
		"scope":      "purpose_channel",
		"purpose":    "marketing",
		"channel":    "email",
		"reason":     "customer_preference",
		"active":     true,
	})
	if respSelfCreate.Code != http.StatusCreated {
		t.Fatalf("self_service create own suppression returned %d: %s", respSelfCreate.Code, respSelfCreate.Body.String())
	}

	// 6. Self-service caller CAN revoke their own purpose_channel suppression.
	respSelfRevoke := f.do(selfToken, http.MethodPost, "/v1/suppressions", map[string]any{
		"subject_id": target,
		"scope":      "purpose_channel",
		"purpose":    "marketing",
		"channel":    "email",
		"reason":     "customer_re_opt_in",
		"active":     false,
	})
	if respSelfRevoke.Code != http.StatusCreated {
		t.Fatalf("self_service revoke own suppression returned %d: %s", respSelfRevoke.Code, respSelfRevoke.Body.String())
	}

	// 7. Administrative caller CAN revoke the protected administrative suppression.
	respAdminRevoke := f.do(adminToken, http.MethodPost, "/v1/suppressions", map[string]any{
		"subject_id": target,
		"scope":      "global",
		"reason":     "admin_lifted_hold",
		"active":     false,
	})
	if respAdminRevoke.Code != http.StatusCreated {
		t.Fatalf("admin revoke global suppression returned %d: %s", respAdminRevoke.Code, respAdminRevoke.Body.String())
	}
}
