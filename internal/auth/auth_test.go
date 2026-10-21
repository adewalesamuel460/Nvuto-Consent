package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func signedToken(t *testing.T, method jwt.SigningMethod, secret string, claims Claims) string {
	t.Helper()
	raw, err := jwt.NewWithClaims(method, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestAuthenticationFailsClosedAndDevHeadersRequireExplicitOptIn(t *testing.T) {
	t.Setenv("JWT_HMAC_SECRET", "")
	t.Setenv("ALLOW_DEV_HEADERS", "false")
	request := httptest.NewRequest("GET", "/", nil)
	request.Header.Set("X-Nvuto-Tenant-ID", "tenant-a")
	request.Header.Set("X-Nvuto-Actor-ID", "actor-a")
	if _, err := FromRequest(request); err == nil {
		t.Fatal("unconfigured authentication must fail closed")
	}
	t.Setenv("ALLOW_DEV_HEADERS", "true")
	principal, err := FromRequest(request)
	if err != nil || principal.TenantID != "tenant-a" || principal.ActorID != "actor-a" || !HasAnyRole(principal, "developer") {
		t.Fatalf("explicit dev mode rejected: principal=%+v err=%v", principal, err)
	}
}

func TestJWTValidationRejectsExpiredWrongAlgorithmAndHeaderFallback(t *testing.T) {
	const secret = "test-secret-at-least-long-enough"
	t.Setenv("JWT_HMAC_SECRET", secret)
	t.Setenv("ALLOW_HMAC_AUTH", "true")
	t.Setenv("ALLOW_DEV_HEADERS", "true")
	now := time.Now()
	valid := signedToken(t, jwt.SigningMethodHS256, secret, Claims{TenantID: "tenant-a", Roles: []string{"consent_admin"}, RegisteredClaims: jwt.RegisteredClaims{Subject: "actor-a", ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute))}})
	request := httptest.NewRequest("GET", "/", nil)
	request.Header.Set("Authorization", "Bearer "+valid)
	principal, err := FromRequest(request)
	if err != nil || principal.TenantID != "tenant-a" || !HasAnyRole(principal, "consent_admin") {
		t.Fatalf("valid JWT rejected: principal=%+v err=%v", principal, err)
	}

	expired := signedToken(t, jwt.SigningMethodHS256, secret, Claims{TenantID: "tenant-a", RegisteredClaims: jwt.RegisteredClaims{Subject: "actor-a", ExpiresAt: jwt.NewNumericDate(now.Add(-time.Minute))}})
	request.Header.Set("Authorization", "Bearer "+expired)
	if _, err := FromRequest(request); err == nil {
		t.Fatal("expired JWT must be rejected")
	}

	wrongAlgorithm := signedToken(t, jwt.SigningMethodHS384, secret, Claims{TenantID: "tenant-a", RegisteredClaims: jwt.RegisteredClaims{Subject: "actor-a", ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute))}})
	request.Header.Set("Authorization", "Bearer "+wrongAlgorithm)
	if _, err := FromRequest(request); err == nil {
		t.Fatal("non-HS256 JWT must be rejected")
	}

	request.Header.Del("Authorization")
	request.Header.Set("X-Nvuto-Tenant-ID", "tenant-a")
	request.Header.Set("X-Nvuto-Actor-ID", "actor-a")
	if _, err := FromRequest(request); err == nil {
		t.Fatal("development headers must not bypass configured JWT authentication")
	}
}

func TestAuthentikCompatibleJWKSValidation(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keyID := "authentik-key-1"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": keyID,
			"kty": "RSA",
			"use": "sig",
			"n":   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(privateKey.PublicKey.E)).Bytes()),
		}}})
	}))
	defer server.Close()
	t.Setenv("ALLOW_DEV_HEADERS", "true")
	t.Setenv("JWT_JWKS_URL", server.URL)
	t.Setenv("JWT_ISSUER", "https://auth.nvuto.example/application/o/customer-services/")
	t.Setenv("JWT_AUDIENCE", "nvuto-customer-services")
	t.Setenv("JWT_HMAC_SECRET", "must-not-be-used-when-jwks-is-configured")
	claims := Claims{
		TenantID:  "tenant-a",
		SubjectID: "cus-1",
		Roles:     []string{"self_service"},
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "authentik-user",
			Issuer:    "https://auth.nvuto.example/application/o/customer-services/",
			Audience:  jwt.ClaimStrings{"nvuto-customer-services"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = keyID
	raw, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer "+raw)
	principal, err := FromRequest(request)
	if err != nil || principal.SubjectID != "cus-1" || !HasAnyRole(principal, "self_service") {
		t.Fatalf("valid Authentik-style token rejected: principal=%+v err=%v", principal, err)
	}

	claims.Issuer = "https://attacker.example/"
	token = jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = keyID
	raw, err = token.SignedString(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+raw)
	if _, err := FromRequest(request); err == nil {
		t.Fatal("wrong issuer must be rejected")
	}
}

func TestValidateConfig(t *testing.T) {
	clearEnv := func() {
		t.Setenv("JWT_JWKS_URL", "")
		t.Setenv("JWT_ISSUER", "")
		t.Setenv("JWT_AUDIENCE", "")
		t.Setenv("JWT_HMAC_SECRET", "")
		t.Setenv("ALLOW_HMAC_AUTH", "false")
		t.Setenv("ALLOW_DEV_HEADERS", "false")
	}

	t.Run("unconfigured fails closed", func(t *testing.T) {
		clearEnv()
		if err := ValidateConfig(); err == nil {
			t.Fatal("expected error for unconfigured auth, got nil")
		}
	})

	t.Run("valid dev headers mode", func(t *testing.T) {
		clearEnv()
		t.Setenv("ALLOW_DEV_HEADERS", "true")
		if err := ValidateConfig(); err != nil {
			t.Fatalf("expected nil for ALLOW_DEV_HEADERS=true, got %v", err)
		}
	})

	t.Run("valid HMAC mode", func(t *testing.T) {
		clearEnv()
		t.Setenv("JWT_HMAC_SECRET", "super-secret-key-12345")
		t.Setenv("ALLOW_HMAC_AUTH", "true")
		if err := ValidateConfig(); err != nil {
			t.Fatalf("expected nil for valid HMAC config, got %v", err)
		}
	})

	t.Run("HMAC secret set without ALLOW_HMAC_AUTH fails", func(t *testing.T) {
		clearEnv()
		t.Setenv("JWT_HMAC_SECRET", "super-secret-key-12345")
		t.Setenv("ALLOW_HMAC_AUTH", "false")
		if err := ValidateConfig(); err == nil {
			t.Fatal("expected error when ALLOW_HMAC_AUTH is false, got nil")
		}
	})

	t.Run("ALLOW_HMAC_AUTH true without secret fails", func(t *testing.T) {
		clearEnv()
		t.Setenv("ALLOW_HMAC_AUTH", "true")
		t.Setenv("JWT_HMAC_SECRET", "")
		if err := ValidateConfig(); err == nil {
			t.Fatal("expected error when JWT_HMAC_SECRET is empty, got nil")
		}
	})

	t.Run("valid JWKS mode", func(t *testing.T) {
		clearEnv()
		t.Setenv("JWT_JWKS_URL", "https://auth.example.com/.well-known/jwks.json")
		t.Setenv("JWT_ISSUER", "https://auth.example.com/")
		t.Setenv("JWT_AUDIENCE", "consent-service")
		if err := ValidateConfig(); err != nil {
			t.Fatalf("expected nil for valid JWKS config, got %v", err)
		}
	})

	t.Run("JWKS missing issuer fails", func(t *testing.T) {
		clearEnv()
		t.Setenv("JWT_JWKS_URL", "https://auth.example.com/.well-known/jwks.json")
		t.Setenv("JWT_AUDIENCE", "consent-service")
		if err := ValidateConfig(); err == nil {
			t.Fatal("expected error for missing issuer, got nil")
		}
	})

	t.Run("JWKS missing audience fails", func(t *testing.T) {
		clearEnv()
		t.Setenv("JWT_JWKS_URL", "https://auth.example.com/.well-known/jwks.json")
		t.Setenv("JWT_ISSUER", "https://auth.example.com/")
		if err := ValidateConfig(); err == nil {
			t.Fatal("expected error for missing audience, got nil")
		}
	})

	t.Run("JWKS invalid URL scheme fails", func(t *testing.T) {
		clearEnv()
		t.Setenv("JWT_JWKS_URL", "ftp://auth.example.com/jwks")
		t.Setenv("JWT_ISSUER", "https://auth.example.com/")
		t.Setenv("JWT_AUDIENCE", "consent-service")
		if err := ValidateConfig(); err == nil {
			t.Fatal("expected error for non-http/https URL, got nil")
		}
	})

	t.Run("JWKS http scheme fails when dev headers off", func(t *testing.T) {
		clearEnv()
		t.Setenv("JWT_JWKS_URL", "http://auth.example.com/jwks")
		t.Setenv("JWT_ISSUER", "https://auth.example.com/")
		t.Setenv("JWT_AUDIENCE", "consent-service")
		t.Setenv("ALLOW_DEV_HEADERS", "false")
		if err := ValidateConfig(); err == nil {
			t.Fatal("expected error for http:// scheme without ALLOW_DEV_HEADERS=true, got nil")
		}
	})

	t.Run("JWKS http scheme succeeds when dev headers on", func(t *testing.T) {
		clearEnv()
		t.Setenv("JWT_JWKS_URL", "http://auth.example.com/jwks")
		t.Setenv("JWT_ISSUER", "https://auth.example.com/")
		t.Setenv("JWT_AUDIENCE", "consent-service")
		t.Setenv("ALLOW_DEV_HEADERS", "true")
		if err := ValidateConfig(); err != nil {
			t.Fatalf("expected nil for http:// with ALLOW_DEV_HEADERS=true, got %v", err)
		}
	})
}
