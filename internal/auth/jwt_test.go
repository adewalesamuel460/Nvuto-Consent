package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestExtractAuthentikSubject(t *testing.T) {
	cases := []struct {
		name     string
		claims   Claims
		expected string
	}{
		{
			name: "custom customer_subject_id takes precedence",
			claims: Claims{
				SubjectID:       "cgraph:tenant-a:individual:usr_101",
				CustomSubjectID: "usr_fallback",
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: "authentik_actor_uuid",
				},
			},
			expected: "cgraph:tenant-a:individual:usr_101",
		},
		{
			name: "custom subject_id claim used when customer_subject_id is empty",
			claims: Claims{
				CustomSubjectID: "cgraph:tenant-a:individual:usr_102",
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: "authentik_actor_uuid",
				},
			},
			expected: "cgraph:tenant-a:individual:usr_102",
		},
		{
			name: "standard sub claim accepted when formatted as canonical Customer Graph ID",
			claims: Claims{
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: "cgraph:tenant-a:individual:usr_103",
				},
			},
			expected: "cgraph:tenant-a:individual:usr_103",
		},
		{
			name: "plain actor sub is not treated as customer subject",
			claims: Claims{
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: "admin_user_42",
				},
			},
			expected: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := ExtractAuthentikSubject(&tc.claims)
			if actual != tc.expected {
				t.Errorf("ExtractAuthentikSubject() = %q, want %q", actual, tc.expected)
			}
		})
	}
}

func TestAuthentikPortalTokenSubjectBinding(t *testing.T) {
	const secret = "portal-test-authentik-secret-32b!"
	t.Setenv("JWT_HMAC_SECRET", secret)
	t.Setenv("ALLOW_HMAC_AUTH", "true")
	t.Setenv("ALLOW_DEV_HEADERS", "false")

	tenantID := "tenant-portal-01"
	customerID := "cgraph:tenant-portal-01:individual:customer_4892"

	// Sign token with self_service role
	tokenStr, err := SignAuthentikToken(secret, tenantID, "auth0|60d21b4667d0d", customerID, []string{RoleSelfService}, time.Now().Add(15*time.Minute))
	if err != nil {
		t.Fatalf("failed to sign Authentik portal token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/consents?subject_id="+customerID, nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)

	principal, err := FromRequest(req)
	if err != nil {
		t.Fatalf("FromRequest failed: %v", err)
	}

	if principal.TenantID != tenantID {
		t.Errorf("expected tenant %q, got %q", tenantID, principal.TenantID)
	}
	if principal.SubjectID != customerID {
		t.Errorf("expected customer subject %q, got %q", customerID, principal.SubjectID)
	}
	if !HasAnyRole(principal, RoleSelfService) {
		t.Errorf("expected self_service role, got %v", principal.Roles)
	}
}
