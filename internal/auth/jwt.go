package auth

import (
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ExtractAuthentikSubject extracts the canonical Customer Graph subject ID from Authentik JWT claims.
// It prioritizes:
// 1. Explicit `customer_subject_id` claim.
// 2. Custom `subject_id` claim.
// 3. Standard `sub` claim IF it is formatted as a canonical Customer Graph ID (e.g. `cgraph:<tenant>:<type>:<uuid>`).
func ExtractAuthentikSubject(claims *Claims) string {
	if claims == nil {
		return ""
	}
	if s := strings.TrimSpace(claims.SubjectID); s != "" {
		return s
	}
	if s := strings.TrimSpace(claims.CustomSubjectID); s != "" {
		return s
	}
	trimmedSub := strings.TrimSpace(claims.Subject)
	if strings.HasPrefix(trimmedSub, "cgraph:") {
		return trimmedSub
	}
	return ""
}

// ExtractRoles combines the `roles` and `groups` claims emitted by Authentik.
func ExtractRoles(claims *Claims) []string {
	if claims == nil {
		return nil
	}
	var roles []string
	seen := make(map[string]struct{})
	for _, r := range claims.Roles {
		r = strings.TrimSpace(r)
		if r != "" {
			if _, ok := seen[r]; !ok {
				seen[r] = struct{}{}
				roles = append(roles, r)
			}
		}
	}
	for _, g := range claims.Groups {
		g = strings.TrimSpace(g)
		if g != "" {
			if _, ok := seen[g]; !ok {
				seen[g] = struct{}{}
				roles = append(roles, g)
			}
		}
	}
	return roles
}

// SignAuthentikToken generates a signed HMAC-SHA256 JWT simulating an Authentik IdP session
// for the Customer Portal.
func SignAuthentikToken(secret, tenantID, actorID, customerSubjectID string, roles []string, expiresAt time.Time) (string, error) {
	if secret == "" {
		return "", errors.New("signing secret cannot be empty")
	}
	if tenantID == "" {
		return "", errors.New("tenant_id is required")
	}
	if actorID == "" {
		actorID = "authentik-session-user"
	}
	claims := Claims{
		TenantID:  tenantID,
		SubjectID: customerSubjectID,
		Roles:     roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   actorID,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// SignAuthentikTokenWithClaims signs arbitrary Claims with HMAC-SHA256 for testing various
// Authentik claim mappings and edge cases.
func SignAuthentikTokenWithClaims(secret string, claims Claims) (string, error) {
	if secret == "" {
		return "", errors.New("signing secret cannot be empty")
	}
	if claims.TenantID == "" {
		return "", errors.New("tenant_id is required")
	}
	if claims.RegisteredClaims.ExpiresAt == nil {
		claims.RegisteredClaims.ExpiresAt = jwt.NewNumericDate(time.Now().UTC().Add(time.Hour))
	}
	if claims.RegisteredClaims.IssuedAt == nil {
		claims.RegisteredClaims.IssuedAt = jwt.NewNumericDate(time.Now().UTC())
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}
