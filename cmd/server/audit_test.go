package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/auth"
	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
)

// TestAT029_AuditExport_SuppressionReconstruction verifies that:
// 1. An authorized auditor can query the /v1/audit endpoint.
// 2. Mutations to a suppression capture revision numbers, before_state, after_state, correlation_id, and reason.
// 3. The historical state sequence can be perfectly reconstructed.
func TestAT029_AuditExport_SuppressionReconstruction(t *testing.T) {
	f := newAuthzFixture(t)
	tenant := authzTenant

	// 1. Initial creation of a suppression via HTTP API with X-Correlation-ID
	suppCreateReq, _ := json.Marshal(map[string]any{
		"subject_id": "cus-audit-rec-1",
		"scope":      "global",
		"reason":     "initial customer opt-out",
		"active":     true,
	})
	req1 := httptest.NewRequest(http.MethodPost, "/v1/suppressions", bytes.NewReader(suppCreateReq))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Authorization", "Bearer "+f.token(tenant, "admin-user", auth.RolePrivacyAdmin))
	req1.Header.Set("X-Correlation-ID", "corr-audit-create-001")
	w1 := httptest.NewRecorder()
	f.handler.ServeHTTP(w1, req1)
	if w1.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on suppression creation, got %d: %s", w1.Code, w1.Body.String())
	}
	var createdSupp domain.Suppression
	if err := json.Unmarshal(w1.Body.Bytes(), &createdSupp); err != nil {
		t.Fatalf("failed to unmarshal created suppression: %v", err)
	}
	expectedResourceID := createdSupp.ID

	// 2. Update the suppression (deactivate) via HTTP API with a different correlation ID
	suppUpdateReq, _ := json.Marshal(map[string]any{
		"subject_id": "cus-audit-rec-1",
		"scope":      "global",
		"reason":     "customer rescinded opt-out via preference center",
		"active":     false,
	})
	req2 := httptest.NewRequest(http.MethodPost, "/v1/suppressions", bytes.NewReader(suppUpdateReq))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+f.token(tenant, "admin-user", auth.RolePrivacyAdmin))
	req2.Header.Set("X-Correlation-ID", "corr-audit-update-002")
	w2 := httptest.NewRecorder()
	f.handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on suppression update, got %d: %s", w2.Code, w2.Body.String())
	}

	// 3. Auditor queries GET /v1/audit?resource_type=suppression&resource_id=<expectedResourceID>
	exportURL := fmt.Sprintf("/v1/audit?resource_type=suppression&resource_id=%s", expectedResourceID)
	reqAudit := httptest.NewRequest(http.MethodGet, exportURL, nil)
	reqAudit.Header.Set("Authorization", "Bearer "+f.token(tenant, "auditor-1", auth.RoleAuditor))
	wAudit := httptest.NewRecorder()
	f.handler.ServeHTTP(wAudit, reqAudit)
	if wAudit.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on audit export query, got %d: %s", wAudit.Code, wAudit.Body.String())
	}

	var resp listResponse[domain.AuditEvent]
	if err := json.Unmarshal(wAudit.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode audit export response: %v", err)
	}

	if resp.Total != 2 || len(resp.Items) != 2 {
		t.Fatalf("expected 2 audit events for suppression, got total=%d, items=%d", resp.Total, len(resp.Items))
	}

	// Event 1: Creation
	ev1 := resp.Items[0]
	if ev1.Action != "consent.suppression.changed" {
		t.Errorf("event 1: expected action 'consent.suppression.changed', got %q", ev1.Action)
	}
	if ev1.Revision != 1 {
		t.Errorf("event 1: expected revision 1, got %d", ev1.Revision)
	}
	if ev1.ResourceType != "suppression" {
		t.Errorf("event 1: expected resource_type 'suppression', got %q", ev1.ResourceType)
	}
	if ev1.ResourceID != expectedResourceID {
		t.Errorf("event 1: expected resource_id %q, got %q", expectedResourceID, ev1.ResourceID)
	}
	if ev1.CorrelationID != "corr-audit-create-001" {
		t.Errorf("event 1: expected correlation_id 'corr-audit-create-001', got %q", ev1.CorrelationID)
	}
	if ev1.Reason != "initial customer opt-out" {
		t.Errorf("event 1: expected reason 'initial customer opt-out', got %q", ev1.Reason)
	}
	if len(ev1.BeforeState) != 0 {
		t.Errorf("event 1: expected before_state to be empty for create, got %+v", ev1.BeforeState)
	}
	if active, ok := ev1.AfterState["active"].(bool); !ok || !active {
		t.Errorf("event 1: expected after_state.active to be true, got %v", ev1.AfterState["active"])
	}
	if reason, ok := ev1.AfterState["reason"].(string); !ok || reason != "initial customer opt-out" {
		t.Errorf("event 1: expected after_state.reason 'initial customer opt-out', got %v", ev1.AfterState["reason"])
	}

	// Event 2: Update (Deactivation)
	ev2 := resp.Items[1]
	if ev2.Action != "consent.suppression.changed" {
		t.Errorf("event 2: expected action 'consent.suppression.changed', got %q", ev2.Action)
	}
	if ev2.Revision != 2 {
		t.Errorf("event 2: expected revision 2, got %d", ev2.Revision)
	}
	if ev2.ResourceType != "suppression" {
		t.Errorf("event 2: expected resource_type 'suppression', got %q", ev2.ResourceType)
	}
	if ev2.ResourceID != expectedResourceID {
		t.Errorf("event 2: expected resource_id %q, got %q", expectedResourceID, ev2.ResourceID)
	}
	if ev2.CorrelationID != "corr-audit-update-002" {
		t.Errorf("event 2: expected correlation_id 'corr-audit-update-002', got %q", ev2.CorrelationID)
	}
	if ev2.Reason != "customer rescinded opt-out via preference center" {
		t.Errorf("event 2: expected reason 'customer rescinded opt-out via preference center', got %q", ev2.Reason)
	}
	if beforeActive, ok := ev2.BeforeState["active"].(bool); !ok || !beforeActive {
		t.Errorf("event 2: expected before_state.active to be true, got %v", ev2.BeforeState["active"])
	}
	if afterActive, ok := ev2.AfterState["active"].(bool); !ok || afterActive {
		t.Errorf("event 2: expected after_state.active to be false, got %v", ev2.AfterState["active"])
	}
	if afterReason, ok := ev2.AfterState["reason"].(string); !ok || afterReason != "customer rescinded opt-out via preference center" {
		t.Errorf("event 2: expected after_state.reason 'customer rescinded opt-out via preference center', got %v", ev2.AfterState["reason"])
	}
}

// TestAT029_AuditExport_StrictAuthorization verifies that only administrative and auditor
// roles can access the audit export API, and standard self-service or reader roles receive 403 Forbidden.
func TestAT029_AuditExport_StrictAuthorization(t *testing.T) {
	f := newAuthzFixture(t)
	tenant := authzTenant

	unauthorizedRoles := []struct {
		name string
		role string
	}{
		{"SelfService", auth.RoleSelfService},
		{"ConsentReader", auth.RoleConsentReader},
		{"Service", auth.RoleService},
		{"ConsentWriter", auth.RoleConsentWriter},
	}

	for _, tc := range unauthorizedRoles {
		t.Run("Forbidden_"+tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/audit", nil)
			req.Header.Set("Authorization", "Bearer "+f.token(tenant, "unauth-user", tc.role))
			w := httptest.NewRecorder()
			f.handler.ServeHTTP(w, req)

			if w.Code != http.StatusForbidden {
				t.Fatalf("expected 403 Forbidden for role %s, got %d: %s", tc.role, w.Code, w.Body.String())
			}

			var errResp errorBody
			_ = json.Unmarshal(w.Body.Bytes(), &errResp)
			if errResp.Error != "role_not_authorized" {
				t.Errorf("expected error 'role_not_authorized', got %q", errResp.Error)
			}
		})
	}

	t.Run("Unauthorized_NoToken", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/audit", nil)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized without token, got %d: %s", w.Code, w.Body.String())
		}
	})

	authorizedRoles := []struct {
		name string
		role string
	}{
		{"Auditor", auth.RoleAuditor},
		{"PrivacyAdmin", auth.RolePrivacyAdmin},
		{"ConsentAdmin", auth.RoleConsentAdmin},
	}

	for _, tc := range authorizedRoles {
		t.Run("Authorized_"+tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/audit", nil)
			req.Header.Set("Authorization", "Bearer "+f.token(tenant, "auth-user", tc.role))
			w := httptest.NewRecorder()
			f.handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200 OK for authorized role %s, got %d: %s", tc.role, w.Code, w.Body.String())
			}
		})
	}
}

// TestAT029_AuditExport_TenantIsolation verifies that audit export is strictly scoped
// to the caller's tenant ID, preventing cross-tenant information leakage.
func TestAT029_AuditExport_TenantIsolation(t *testing.T) {
	f := newAuthzFixture(t)
	tenantA := "tenant-alpha"
	tenantB := "tenant-beta"

	// Register a purpose in Tenant A
	purposeReqA, _ := json.Marshal(map[string]any{
		"code":                         "alpha_purpose",
		"category":                     "marketing",
		"allowed_channels":             []string{"email"},
		"requires_affirmative_consent": true,
		"requires_double_opt_in":       false,
		"active":                       true,
	})
	reqA := httptest.NewRequest(http.MethodPost, "/v1/purposes", bytes.NewReader(purposeReqA))
	reqA.Header.Set("Content-Type", "application/json")
	reqA.Header.Set("Authorization", "Bearer "+f.token(tenantA, "admin-a", auth.RoleConsentAdmin))
	wA := httptest.NewRecorder()
	f.handler.ServeHTTP(wA, reqA)
	if wA.Code != http.StatusCreated {
		t.Fatalf("failed to create purpose in tenant A: %d %s", wA.Code, wA.Body.String())
	}

	// Register a purpose in Tenant B
	purposeReqB, _ := json.Marshal(map[string]any{
		"code":                         "beta_purpose",
		"category":                     "analytics",
		"allowed_channels":             []string{"web"},
		"requires_affirmative_consent": false,
		"requires_double_opt_in":       false,
		"active":                       true,
	})
	reqB := httptest.NewRequest(http.MethodPost, "/v1/purposes", bytes.NewReader(purposeReqB))
	reqB.Header.Set("Content-Type", "application/json")
	reqB.Header.Set("Authorization", "Bearer "+f.token(tenantB, "admin-b", auth.RoleConsentAdmin))
	wB := httptest.NewRecorder()
	f.handler.ServeHTTP(wB, reqB)
	if wB.Code != http.StatusCreated {
		t.Fatalf("failed to create purpose in tenant B: %d %s", wB.Code, wB.Body.String())
	}

	// Auditor from Tenant B queries Tenant A's purpose resource ID
	leakQuery := "/v1/audit?resource_type=purpose&resource_id=alpha_purpose"
	reqAuditB := httptest.NewRequest(http.MethodGet, leakQuery, nil)
	reqAuditB.Header.Set("Authorization", "Bearer "+f.token(tenantB, "auditor-b", auth.RoleAuditor))
	wAuditB := httptest.NewRecorder()
	f.handler.ServeHTTP(wAuditB, reqAuditB)

	if wAuditB.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from Tenant B query, got %d", wAuditB.Code)
	}

	var respB listResponse[domain.AuditEvent]
	if err := json.Unmarshal(wAuditB.Body.Bytes(), &respB); err != nil {
		t.Fatalf("failed to unmarshal Tenant B response: %v", err)
	}

	if respB.Total != 0 || len(respB.Items) != 0 {
		t.Fatalf("CROSS-TENANT LEAK: Tenant B received Tenant A's audit events: %+v", respB.Items)
	}

	// Auditor from Tenant A queries and finds the event
	reqAuditA := httptest.NewRequest(http.MethodGet, leakQuery, nil)
	reqAuditA.Header.Set("Authorization", "Bearer "+f.token(tenantA, "auditor-a", auth.RoleAuditor))
	wAuditA := httptest.NewRecorder()
	f.handler.ServeHTTP(wAuditA, reqAuditA)

	if wAuditA.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from Tenant A query, got %d", wAuditA.Code)
	}

	var respA listResponse[domain.AuditEvent]
	_ = json.Unmarshal(wAuditA.Body.Bytes(), &respA)
	if respA.Total != 1 || len(respA.Items) != 1 {
		t.Fatalf("expected Tenant A to find exactly 1 audit event, got %d", respA.Total)
	}
	if respA.Items[0].TenantID != tenantA {
		t.Errorf("expected tenant_id %q, got %q", tenantA, respA.Items[0].TenantID)
	}
}

// TestAT029_AuditExport_PaginationAndValidation tests pagination parsing and validation on /v1/audit.
func TestAT029_AuditExport_PaginationAndValidation(t *testing.T) {
	f := newAuthzFixture(t)
	tenant := authzTenant

	// Method not allowed
	reqPost := httptest.NewRequest(http.MethodPost, "/v1/audit", nil)
	reqPost.Header.Set("Authorization", "Bearer "+f.token(tenant, "auditor-1", auth.RoleAuditor))
	wPost := httptest.NewRecorder()
	f.handler.ServeHTTP(wPost, reqPost)
	if wPost.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 Method Not Allowed for POST /v1/audit, got %d", wPost.Code)
	}

	// Invalid limit
	reqBadLimit := httptest.NewRequest(http.MethodGet, "/v1/audit?limit=-5", nil)
	reqBadLimit.Header.Set("Authorization", "Bearer "+f.token(tenant, "auditor-1", auth.RoleAuditor))
	wBadLimit := httptest.NewRecorder()
	f.handler.ServeHTTP(wBadLimit, reqBadLimit)
	if wBadLimit.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for negative limit, got %d", wBadLimit.Code)
	}

	// Invalid offset
	reqBadOffset := httptest.NewRequest(http.MethodGet, "/v1/audit?offset=-1", nil)
	reqBadOffset.Header.Set("Authorization", "Bearer "+f.token(tenant, "auditor-1", auth.RoleAuditor))
	wBadOffset := httptest.NewRecorder()
	f.handler.ServeHTTP(wBadOffset, reqBadOffset)
	if wBadOffset.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for negative offset, got %d", wBadOffset.Code)
	}
}

// TestAT029_DatabaseAuditImmutability verifies that consent_audit_events is strictly append-only
// and that database-level trigger restrictions reject any UPDATE or DELETE mutations.
func TestAT029_DatabaseAuditImmutability(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL / DATABASE_URL is not configured; skipping direct PostgreSQL immutability trigger test")
	}

	ctx := context.Background()
	pg, err := store.NewPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to connect to PostgreSQL: %v", err)
	}
	defer pg.Close()

	pool := pg.Pool()
	tenantID := fmt.Sprintf("tenant-audit-immut-%d", time.Now().UnixNano())
	eventID := fmt.Sprintf("aud_test_%d", time.Now().UnixNano())

	// 1. Insert a sample audit event directly into consent_audit_events
	_, err = pool.Exec(ctx, `
		INSERT INTO consent_audit_events (
			id, tenant_id, actor_id, event_type, resource_type, resource_id,
			revision, before_state, after_state, correlation_id, reason, created_at
		) VALUES (
			$1, $2, 'actor-test', 'created', 'suppression', 'test-subj:global:*',
			1, '{}'::jsonb, '{"active":true}'::jsonb, 'corr-test', 'test reason', now()
		)
	`, eventID, tenantID)
	if err != nil {
		t.Fatalf("failed to insert audit event: %v", err)
	}

	// 2. Direct UPDATE attempt MUST FAIL due to trg_consent_audit_immutable
	_, updateErr := pool.Exec(ctx, "UPDATE consent_audit_events SET reason = 'tampered' WHERE id = $1", eventID)
	if updateErr == nil {
		t.Fatal("SECURITY VIOLATION: direct UPDATE on consent_audit_events succeeded; trigger failed to prevent mutation!")
	}
	if !containsImmutabilityError(updateErr.Error()) {
		t.Fatalf("expected immutability trigger error, got: %v", updateErr)
	}
	t.Logf("PASS: Direct UPDATE on consent_audit_events successfully blocked by immutability trigger: %v", updateErr)

	// 3. Direct DELETE attempt MUST FAIL due to trg_consent_audit_immutable
	_, deleteErr := pool.Exec(ctx, "DELETE FROM consent_audit_events WHERE id = $1", eventID)
	if deleteErr == nil {
		t.Fatal("SECURITY VIOLATION: direct DELETE on consent_audit_events succeeded; trigger failed to prevent deletion!")
	}
	if !containsImmutabilityError(deleteErr.Error()) {
		t.Fatalf("expected immutability trigger error, got: %v", deleteErr)
	}
	t.Logf("PASS: Direct DELETE on consent_audit_events successfully blocked by immutability trigger: %v", deleteErr)
}

func containsImmutabilityError(msg string) bool {
	return bytes.Contains([]byte(msg), []byte("append-only")) ||
		bytes.Contains([]byte(msg), []byte("prohibited")) ||
		bytes.Contains([]byte(msg), []byte("reject_consent_audit_mutation"))
}
