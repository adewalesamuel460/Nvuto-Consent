package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// fakeIdP is a controllable JWKS source that counts outbound fetches.
type fakeIdP struct {
	mu      sync.Mutex
	keys    map[string]*rsa.PublicKey
	fail    bool
	gate    chan struct{} // when non-nil, fetches block until closed
	fetches atomic.Int32
}

func (f *fakeIdP) fetch(ctx context.Context, _ string) (map[string]*rsa.PublicKey, error) {
	f.fetches.Add(1)
	f.mu.Lock()
	gate := f.gate
	f.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return nil, errors.New("identity provider unavailable")
	}
	out := make(map[string]*rsa.PublicKey, len(f.keys))
	for k, v := range f.keys {
		out[k] = v
	}
	return out, nil
}

func (f *fakeIdP) set(fn func(*fakeIdP)) {
	f.mu.Lock()
	fn(f)
	f.mu.Unlock()
}

func newTestProvider(idp *fakeIdP, clock *fakeClock) *jwksProvider {
	p := newJWKSProvider("https://idp.test/jwks")
	p.fetch = idp.fetch
	p.now = clock.Now
	return p
}

func testRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestJWKSUnknownKidFloodIsBoundedByCooldown(t *testing.T) {
	signing := testRSAKey(t)
	idp := &fakeIdP{keys: map[string]*rsa.PublicKey{"kid-good": &signing.PublicKey}}
	clock := &fakeClock{now: time.Unix(1_800_000_000, 0)}
	provider := newTestProvider(idp, clock)
	ctx := context.Background()

	// Burst of 500 concurrent, distinct, forged kids.
	var wg sync.WaitGroup
	var accepted atomic.Int32
	for i := range 500 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if key, err := provider.key(ctx, fmt.Sprintf("forged-%d", i)); err == nil || key != nil {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != 0 {
		t.Fatalf("%d forged kids resolved to a key; unknown kids must fail closed", accepted.Load())
	}
	if got := idp.fetches.Load(); got != 1 {
		t.Fatalf("burst of distinct unknown kids caused %d JWKS fetches, want 1", got)
	}

	// A second burst inside the cooldown window must not reach the IdP.
	for i := range 200 {
		_, _ = provider.key(ctx, fmt.Sprintf("forged-second-%d", i))
	}
	if got := idp.fetches.Load(); got != 1 {
		t.Fatalf("burst inside cooldown caused %d fetches, want 1", got)
	}

	// After the cooldown elapses, at most one more fetch is permitted per burst.
	clock.Advance(provider.minRefreshInterval)
	for i := range 200 {
		_, _ = provider.key(ctx, fmt.Sprintf("forged-third-%d", i))
	}
	if got := idp.fetches.Load(); got != 2 {
		t.Fatalf("burst after cooldown caused %d total fetches, want 2", got)
	}

	// The valid key is still served from cache throughout.
	if key, err := provider.key(ctx, "kid-good"); err != nil || key == nil {
		t.Fatalf("valid cached key rejected during flood: %v", err)
	}
	if got := idp.fetches.Load(); got != 2 {
		t.Fatalf("valid cached key triggered a fetch: %d", got)
	}
}

func TestJWKSConcurrentColdStartIsSingleFlight(t *testing.T) {
	signing := testRSAKey(t)
	gate := make(chan struct{})
	idp := &fakeIdP{keys: map[string]*rsa.PublicKey{"kid-good": &signing.PublicKey}, gate: gate}
	clock := &fakeClock{now: time.Unix(1_800_000_000, 0)}
	provider := newTestProvider(idp, clock)

	const callers = 64
	var wg sync.WaitGroup
	var started sync.WaitGroup
	errs := make(chan error, callers)
	started.Add(callers)
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			started.Done()
			if _, err := provider.key(context.Background(), "kid-good"); err != nil {
				errs <- err
			}
		}()
	}
	started.Wait()
	// Give goroutines a moment to pile up on the in-flight refresh.
	time.Sleep(20 * time.Millisecond)
	close(gate)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("caller sharing the in-flight refresh failed: %v", err)
	}
	if got := idp.fetches.Load(); got != 1 {
		t.Fatalf("%d concurrent cold-start callers caused %d fetches, want 1", callers, got)
	}
}

func TestJWKSCancelledCallerDoesNotAbortSharedRefresh(t *testing.T) {
	signing := testRSAKey(t)
	gate := make(chan struct{})
	idp := &fakeIdP{keys: map[string]*rsa.PublicKey{"kid-good": &signing.PublicKey}, gate: gate}
	clock := &fakeClock{now: time.Unix(1_800_000_000, 0)}
	provider := newTestProvider(idp, clock)

	cancelled, cancel := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		_, err := provider.key(cancelled, "kid-good")
		firstDone <- err
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()
	if err := <-firstDone; err == nil {
		t.Fatal("cancelled caller should observe an error")
	}

	secondDone := make(chan error, 1)
	go func() {
		_, err := provider.key(context.Background(), "kid-good")
		secondDone <- err
	}()
	time.Sleep(10 * time.Millisecond)
	close(gate)
	if err := <-secondDone; err != nil {
		t.Fatalf("second caller should share the surviving refresh: %v", err)
	}
	if got := idp.fetches.Load(); got != 1 {
		t.Fatalf("cancellation caused %d fetches, want 1", got)
	}
}

func TestJWKSNegativeCacheRejectsRepeatedUnknownKidWithoutFetch(t *testing.T) {
	signing := testRSAKey(t)
	idp := &fakeIdP{keys: map[string]*rsa.PublicKey{"kid-good": &signing.PublicKey}}
	clock := &fakeClock{now: time.Unix(1_800_000_000, 0)}
	provider := newTestProvider(idp, clock)
	ctx := context.Background()

	if _, err := provider.key(ctx, "forged"); !errors.Is(err, errUnknownKeyID) {
		t.Fatalf("unknown kid: got %v, want errUnknownKeyID", err)
	}
	if got := idp.fetches.Load(); got != 1 {
		t.Fatalf("first unknown kid fetches = %d, want 1", got)
	}
	// Cooldown has elapsed, but the kid was proven absent: no new fetch.
	clock.Advance(provider.minRefreshInterval + time.Second)
	if _, err := provider.key(ctx, "forged"); !errors.Is(err, errUnknownKeyID) {
		t.Fatalf("negatively cached kid: got %v", err)
	}
	if got := idp.fetches.Load(); got != 1 {
		t.Fatalf("negatively cached kid triggered a fetch: %d", got)
	}
	// Once the negative entry expires the kid is re-checked exactly once.
	clock.Advance(provider.unknownKeyTTL)
	_, _ = provider.key(ctx, "forged")
	if got := idp.fetches.Load(); got != 2 {
		t.Fatalf("expired negative entry fetches = %d, want 2", got)
	}
}

func TestJWKSNegativeCacheIsBounded(t *testing.T) {
	signing := testRSAKey(t)
	idp := &fakeIdP{keys: map[string]*rsa.PublicKey{"kid-good": &signing.PublicKey}}
	clock := &fakeClock{now: time.Unix(1_800_000_000, 0)}
	provider := newTestProvider(idp, clock)
	provider.maxUnknownKeys = 8
	provider.minRefreshInterval = 0 // let every miss fetch so the cache would fill
	for i := range 100 {
		_, _ = provider.key(context.Background(), fmt.Sprintf("forged-%d", i))
	}
	provider.mu.Lock()
	size := len(provider.unknown)
	provider.mu.Unlock()
	if size > provider.maxUnknownKeys {
		t.Fatalf("negative cache grew to %d entries, bound is %d", size, provider.maxUnknownKeys)
	}
}

func TestJWKSCachedKeysSurviveIdentityProviderOutage(t *testing.T) {
	signing := testRSAKey(t)
	idp := &fakeIdP{keys: map[string]*rsa.PublicKey{"kid-good": &signing.PublicKey}}
	clock := &fakeClock{now: time.Unix(1_800_000_000, 0)}
	provider := newTestProvider(idp, clock)
	ctx := context.Background()

	if _, err := provider.key(ctx, "kid-good"); err != nil {
		t.Fatal(err)
	}
	idp.set(func(f *fakeIdP) { f.fail = true })

	// Cache is now stale and the IdP is down: the cached key must keep working.
	clock.Advance(provider.ttl + time.Second)
	for i := range 50 {
		if key, err := provider.key(ctx, "kid-good"); err != nil || key == nil {
			t.Fatalf("cached key rejected during IdP outage (iteration %d): %v", i, err)
		}
	}
	// Refresh attempts during the outage are still rate-limited.
	if got := idp.fetches.Load(); got != 2 {
		t.Fatalf("outage caused %d fetches, want 2 (initial + one throttled retry)", got)
	}
	// Unknown kids still fail closed during the outage.
	clock.Advance(provider.minRefreshInterval)
	if _, err := provider.key(ctx, "forged"); err == nil {
		t.Fatal("unknown kid accepted during IdP outage")
	}
	// Beyond the maximum staleness window, cached keys are no longer trusted.
	clock.Advance(provider.maxStale)
	if _, err := provider.key(ctx, "kid-good"); err == nil {
		t.Fatal("key older than maxStale must be rejected while IdP is unreachable")
	}
	// Recovery: once the IdP is back, keys work again after the cooldown.
	idp.set(func(f *fakeIdP) { f.fail = false })
	clock.Advance(provider.minRefreshInterval)
	if _, err := provider.key(ctx, "kid-good"); err != nil {
		t.Fatalf("key rejected after IdP recovery: %v", err)
	}
}

func TestJWKSKeyRotationIsPickedUpAndRevokedKeysDropped(t *testing.T) {
	oldKey, newKey := testRSAKey(t), testRSAKey(t)
	idp := &fakeIdP{keys: map[string]*rsa.PublicKey{"kid-old": &oldKey.PublicKey}}
	clock := &fakeClock{now: time.Unix(1_800_000_000, 0)}
	provider := newTestProvider(idp, clock)
	ctx := context.Background()

	if _, err := provider.key(ctx, "kid-old"); err != nil {
		t.Fatal(err)
	}
	idp.set(func(f *fakeIdP) { f.keys = map[string]*rsa.PublicKey{"kid-new": &newKey.PublicKey} })
	clock.Advance(provider.minRefreshInterval)
	if key, err := provider.key(ctx, "kid-new"); err != nil || key.N.Cmp(newKey.N) != 0 {
		t.Fatalf("rotated key not picked up within cooldown: %v", err)
	}
	// The successful refresh replaced the key set, so the revoked kid fails.
	if _, err := provider.key(ctx, "kid-old"); err == nil {
		t.Fatal("revoked key must not be served after a successful refresh")
	}
}

func signRS256(t *testing.T, key *rsa.PrivateKey, kid string, claims Claims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	raw, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestFromRequestUnknownKidFloodMakesOneOutboundRequest(t *testing.T) {
	ResetJWKSProviders()
	t.Cleanup(ResetJWKSProviders)
	signing := testRSAKey(t)
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": "kid-good", "kty": "RSA", "use": "sig",
			"n": base64.RawURLEncoding.EncodeToString(signing.PublicKey.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(signing.PublicKey.E)).Bytes()),
		}}})
	}))
	defer server.Close()
	const issuer, audience = "https://auth.nvuto.example/", "nvuto-consent"
	t.Setenv("ALLOW_DEV_HEADERS", "true")
	t.Setenv("JWT_JWKS_URL", server.URL)
	t.Setenv("JWT_ISSUER", issuer)
	t.Setenv("JWT_AUDIENCE", audience)
	claims := Claims{TenantID: "tenant-a", Roles: []string{"consent_reader"}, RegisteredClaims: jwt.RegisteredClaims{
		Subject: "svc", Issuer: issuer, Audience: jwt.ClaimStrings{audience}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
	}}

	tokens := make([]string, 300)
	for i := range tokens {
		tokens[i] = signRS256(t, signing, fmt.Sprintf("forged-%d", i), claims)
	}
	var wg sync.WaitGroup
	var accepted atomic.Int32
	for _, raw := range tokens {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", "Bearer "+raw)
			if _, err := FromRequest(req); err == nil {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != 0 {
		t.Fatalf("%d tokens with unknown kids were accepted", accepted.Load())
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("unknown-kid flood made %d JWKS requests, want 1", got)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+signRS256(t, signing, "kid-good", claims))
	if _, err := FromRequest(req); err != nil {
		t.Fatalf("valid token rejected after flood: %v", err)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("valid cached key caused extra JWKS request: %d", got)
	}
}

func TestFromRequestRejectsOversizedKid(t *testing.T) {
	signing := testRSAKey(t)
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hits.Add(1) }))
	defer server.Close()
	t.Setenv("JWT_JWKS_URL", server.URL)
	t.Setenv("JWT_ISSUER", "iss")
	t.Setenv("JWT_AUDIENCE", "aud")
	claims := Claims{TenantID: "tenant-a", RegisteredClaims: jwt.RegisteredClaims{Subject: "x", Issuer: "iss", Audience: jwt.ClaimStrings{"aud"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute))}}
	kid := string(make([]byte, maxKeyIDLength+1))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+signRS256(t, signing, kid, claims))
	if _, err := FromRequest(req); err == nil {
		t.Fatal("oversized kid must be rejected")
	}
	if hits.Load() != 0 {
		t.Fatal("oversized kid must be rejected before any JWKS request")
	}
}

func TestSelfServiceTokenRequiresCustomerSubjectClaim(t *testing.T) {
	const secret = "self-service-claim-test-secret"
	t.Setenv("JWT_JWKS_URL", "")
	t.Setenv("JWT_HMAC_SECRET", secret)
	t.Setenv("ALLOW_HMAC_AUTH", "true")
	expires := jwt.NewNumericDate(time.Now().Add(time.Minute))
	cases := []struct {
		name    string
		claims  Claims
		wantErr bool
	}{
		{"self_service without customer_subject_id", Claims{TenantID: "t", Roles: []string{"self_service"}, RegisteredClaims: jwt.RegisteredClaims{Subject: "u", ExpiresAt: expires}}, true},
		{"self_service with blank customer_subject_id", Claims{TenantID: "t", SubjectID: "   ", Roles: []string{"self_service"}, RegisteredClaims: jwt.RegisteredClaims{Subject: "u", ExpiresAt: expires}}, true},
		{"mixed self_service+service without customer_subject_id", Claims{TenantID: "t", Roles: []string{"self_service", "service"}, RegisteredClaims: jwt.RegisteredClaims{Subject: "u", ExpiresAt: expires}}, true},
		{"self_service with customer_subject_id", Claims{TenantID: "t", SubjectID: "cus-1", Roles: []string{"self_service"}, RegisteredClaims: jwt.RegisteredClaims{Subject: "u", ExpiresAt: expires}}, false},
		{"service without customer_subject_id", Claims{TenantID: "t", Roles: []string{"service"}, RegisteredClaims: jwt.RegisteredClaims{Subject: "u", ExpiresAt: expires}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", "Bearer "+signedToken(t, jwt.SigningMethodHS256, secret, tc.claims))
			_, err := FromRequest(req)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

// AT-014: Concurrency test proving a burst of 100 requests with unknown kids
// triggers exactly one outbound HTTP request to the mock IdP and fails closed.
func TestAT014_Burst100UnknownKidsTriggersExactlyOneOutboundRequest(t *testing.T) {
	ResetJWKSProviders()
	t.Cleanup(ResetJWKSProviders)
	signing := testRSAKey(t)
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": "good-key", "kty": "RSA", "use": "sig",
			"n": base64.RawURLEncoding.EncodeToString(signing.PublicKey.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(signing.PublicKey.E)).Bytes()),
		}}})
	}))
	defer server.Close()

	const issuer, audience = "https://auth.nvuto.example/", "nvuto-consent"
	t.Setenv("ALLOW_DEV_HEADERS", "true")
	t.Setenv("JWT_JWKS_URL", server.URL)
	t.Setenv("JWT_ISSUER", issuer)
	t.Setenv("JWT_AUDIENCE", audience)

	claims := Claims{
		TenantID: "tenant-a",
		Roles:    []string{"consent_reader"},
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "svc",
			Issuer:    issuer,
			Audience:  jwt.ClaimStrings{audience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
	}

	const requestCount = 100
	tokens := make([]string, requestCount)
	for i := range requestCount {
		tokens[i] = signRS256(t, signing, fmt.Sprintf("unknown-kid-%d", i), claims)
	}

	var wg sync.WaitGroup
	var accepted atomic.Int32
	for _, raw := range tokens {
		wg.Add(1)
		go func(token string) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			if _, err := FromRequest(req); err == nil {
				accepted.Add(1)
			}
		}(raw)
	}
	wg.Wait()

	if accepted.Load() != 0 {
		t.Fatalf("%d tokens with unknown kids were accepted; must fail closed", accepted.Load())
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("burst of 100 unknown kids made %d HTTP requests to mock IdP, want exactly 1", got)
	}

	// Repeat inside cooldown window: must make zero additional outbound requests
	for i := range 50 {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+tokens[i])
		if _, err := FromRequest(req); err == nil {
			t.Fatal("token accepted inside cooldown window")
		}
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("requests within cooldown caused %d HTTP requests, want 1", got)
	}
}

// AT-014: Proves that legitimate key rotation succeeds after cooldown expires,
// and revoked keys fail closed.
func TestAT014_LegitimateKeyRotationAfterCooldown(t *testing.T) {
	oldKey, newKey := testRSAKey(t), testRSAKey(t)
	idp := &fakeIdP{keys: map[string]*rsa.PublicKey{"kid-initial": &oldKey.PublicKey}}
	clock := &fakeClock{now: time.Unix(1_800_000_000, 0)}
	provider := newTestProvider(idp, clock)
	ctx := context.Background()

	// Initial fetch
	if _, err := provider.key(ctx, "kid-initial"); err != nil {
		t.Fatalf("initial key resolution failed: %v", err)
	}
	if got := idp.fetches.Load(); got != 1 {
		t.Fatalf("initial key resolution made %d fetches, want 1", got)
	}

	// IdP rotates keys
	idp.set(func(f *fakeIdP) {
		f.keys = map[string]*rsa.PublicKey{"kid-rotated": &newKey.PublicKey}
	})

	// Before cooldown: request for rotated key is throttled / fails closed
	if _, err := provider.key(ctx, "kid-rotated"); err == nil {
		t.Fatal("rotated key accepted before cooldown has elapsed")
	}
	if got := idp.fetches.Load(); got != 1 {
		t.Fatalf("request before cooldown caused fetch: %d, want 1", got)
	}

	// Advance clock past cooldown duration (e.g. 10s)
	clock.Advance(provider.minRefreshInterval)

	// Rotated key should trigger a singleflight refresh and succeed
	key, err := provider.key(ctx, "kid-rotated")
	if err != nil || key == nil {
		t.Fatalf("legitimate key rotation failed after cooldown: %v", err)
	}
	if key.N.Cmp(newKey.N) != 0 {
		t.Fatal("retrieved key does not match rotated key")
	}
	if got := idp.fetches.Load(); got != 2 {
		t.Fatalf("rotation fetch count = %d, want 2", got)
	}

	// Revoked key must fail closed
	if _, err := provider.key(ctx, "kid-initial"); err == nil {
		t.Fatal("revoked key must not be served after rotation")
	}
}

// AT-014: Proves that identity-provider outages do not clear the last-known-good
// cache, and unknown keys fail closed during outages.
func TestAT014_IdPOutagePreservesLastKnownGoodAndUnknownKeysFailClosed(t *testing.T) {
	signing := testRSAKey(t)
	idp := &fakeIdP{keys: map[string]*rsa.PublicKey{"kid-valid": &signing.PublicKey}}
	clock := &fakeClock{now: time.Unix(1_800_000_000, 0)}
	provider := newTestProvider(idp, clock)
	ctx := context.Background()

	// Initial warm cache
	if _, err := provider.key(ctx, "kid-valid"); err != nil {
		t.Fatalf("failed to cache valid key: %v", err)
	}
	if got := idp.fetches.Load(); got != 1 {
		t.Fatalf("warmup fetches = %d, want 1", got)
	}

	// Simulate IdP outage
	idp.set(func(f *fakeIdP) { f.fail = true })

	// Advance past normal cache TTL so keys become stale
	clock.Advance(provider.ttl + time.Second)

	// Valid cached key must still be accepted from the last-known-good cache
	key, err := provider.key(ctx, "kid-valid")
	if err != nil || key == nil {
		t.Fatalf("valid cached key rejected during IdP outage: %v", err)
	}

	// Unknown key during outage must fail closed
	clock.Advance(provider.minRefreshInterval)
	if _, err := provider.key(ctx, "kid-forged-during-outage"); err == nil {
		t.Fatal("unknown key accepted during IdP outage; must fail closed")
	}

	// Valid cached key still works
	keyAfter, err := provider.key(ctx, "kid-valid")
	if err != nil || keyAfter == nil {
		t.Fatalf("valid cached key lost after unknown key failure during outage: %v", err)
	}
}
