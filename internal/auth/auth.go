package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Principal struct {
	TenantID  string
	ActorID   string
	SubjectID string
	Roles     []string
}

type Claims struct {
	TenantID  string   `json:"tenant_id"`
	SubjectID string   `json:"customer_subject_id"`
	Roles     []string `json:"roles"`
	jwt.RegisteredClaims
}

// RoleSelfService is the customer-facing role whose authority is bound to the
// canonical customer_subject_id claim.
const RoleSelfService = "self_service"

// JWKS refresh budget (T-014). A provider performs at most one outbound fetch
// per jwksMinRefreshInterval regardless of how many distinct unknown key IDs
// arrive, concurrent callers share a single in-flight fetch, and key IDs that a
// successful fetch proved absent are rejected locally for jwksUnknownKeyTTL.
const (
	jwksCacheTTL           = 5 * time.Minute
	jwksMinRefreshInterval = 10 * time.Second
	jwksMaxStale           = 24 * time.Hour
	jwksUnknownKeyTTL      = time.Minute
	jwksMaxUnknownKeys     = 1024
	jwksFetchTimeout       = 5 * time.Second
	maxKeyIDLength         = 256
)

var (
	errUnknownKeyID     = errors.New("JWT signing key not found")
	errJWKSUnavailable  = errors.New("JWT signing keys unavailable")
	errRefreshThrottled = errors.New("JWKS refresh throttled")
)

var jwksHTTPClient = &http.Client{Timeout: jwksFetchTimeout}

func fetchJWKS(ctx context.Context, url string) (map[string]*rsa.PublicKey, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := jwksHTTPClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("JWKS endpoint returned " + response.Status)
	}
	var document struct {
		Keys []struct {
			KeyID string `json:"kid"`
			Type  string `json:"kty"`
			Use   string `json:"use"`
			N     string `json:"n"`
			E     string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&document); err != nil {
		return nil, err
	}
	keys := map[string]*rsa.PublicKey{}
	for _, raw := range document.Keys {
		if raw.KeyID == "" || raw.Type != "RSA" || (raw.Use != "" && raw.Use != "sig") {
			continue
		}
		modulus, err := base64.RawURLEncoding.DecodeString(raw.N)
		if err != nil {
			return nil, err
		}
		exponentBytes, err := base64.RawURLEncoding.DecodeString(raw.E)
		if err != nil {
			return nil, err
		}
		if len(exponentBytes) > 4 {
			continue
		}
		exponent := 0
		for _, value := range exponentBytes {
			exponent = exponent<<8 + int(value)
		}
		if exponent == 0 || len(modulus) == 0 {
			continue
		}
		keys[raw.KeyID] = &rsa.PublicKey{N: new(big.Int).SetBytes(modulus), E: exponent}
	}
	if len(keys) == 0 {
		return nil, errors.New("JWKS contained no signing keys")
	}
	return keys, nil
}

// jwksRefresh is a single in-flight JWKS fetch shared by all concurrent callers.
type jwksRefresh struct {
	done chan struct{}
	err  error
}

// jwksProvider caches the signing keys of one JWKS endpoint and bounds how
// often that endpoint is contacted.
type jwksProvider struct {
	url   string
	fetch func(context.Context, string) (map[string]*rsa.PublicKey, error)
	now   func() time.Time

	ttl                time.Duration // keys younger than this are served without a refresh attempt
	minRefreshInterval time.Duration // minimum spacing between fetch attempts, successful or not
	maxStale           time.Duration // cached keys remain usable this long while the IdP is unreachable
	unknownKeyTTL      time.Duration // how long a key ID proven absent is rejected without refresh
	maxUnknownKeys     int           // upper bound on the negative cache
	fetchTimeout       time.Duration

	mu          sync.Mutex
	keys        map[string]*rsa.PublicKey
	fetchedAt   time.Time
	lastAttempt time.Time
	inflight    *jwksRefresh
	unknown     map[string]time.Time
}

func newJWKSProvider(url string) *jwksProvider {
	return &jwksProvider{
		url:                url,
		fetch:              fetchJWKS,
		now:                time.Now,
		ttl:                jwksCacheTTL,
		minRefreshInterval: jwksMinRefreshInterval,
		maxStale:           jwksMaxStale,
		unknownKeyTTL:      jwksUnknownKeyTTL,
		maxUnknownKeys:     jwksMaxUnknownKeys,
		fetchTimeout:       jwksFetchTimeout,
		keys:               map[string]*rsa.PublicKey{},
		unknown:            map[string]time.Time{},
	}
}

// jwksProviders holds one provider per configured JWKS URL so that the
// singleflight and cooldown state is scoped to each identity provider.
var jwksProviders = struct {
	sync.Mutex
	byURL map[string]*jwksProvider
}{byURL: map[string]*jwksProvider{}}

func jwksProviderFor(url string) *jwksProvider {
	jwksProviders.Lock()
	defer jwksProviders.Unlock()
	provider := jwksProviders.byURL[url]
	if provider == nil {
		provider = newJWKSProvider(url)
		jwksProviders.byURL[url] = provider
	}
	return provider
}

// key resolves a signing key. Fresh cached keys are served without I/O. Unknown
// or stale keys trigger at most one shared, rate-limited refresh. When the
// identity provider is unreachable, previously cached keys remain valid for
// maxStale; unknown key IDs always fail closed.
func (p *jwksProvider) key(ctx context.Context, keyID string) (*rsa.PublicKey, error) {
	p.mu.Lock()
	now := p.now()
	key, known := p.keys[keyID]
	if known && now.Sub(p.fetchedAt) < p.ttl {
		p.mu.Unlock()
		return key, nil
	}
	if !known {
		if until, cached := p.unknown[keyID]; cached {
			if now.Before(until) {
				p.mu.Unlock()
				return nil, errUnknownKeyID
			}
			delete(p.unknown, keyID)
		}
	}
	p.mu.Unlock()

	refreshErr := p.refresh(ctx)

	p.mu.Lock()
	defer p.mu.Unlock()
	now = p.now()
	if key, ok := p.keys[keyID]; ok {
		if now.Sub(p.fetchedAt) < p.maxStale {
			return key, nil
		}
		return nil, errJWKSUnavailable
	}
	if refreshErr == nil {
		// A successful fetch proved the key ID absent; remember that so repeats
		// of the same forged kid do not consume the refresh budget.
		p.rememberUnknownLocked(keyID, now)
	}
	return nil, errUnknownKeyID
}

// refresh joins the in-flight fetch if there is one, otherwise starts a new
// fetch unless the cooldown since the last attempt has not elapsed.
func (p *jwksProvider) refresh(ctx context.Context) error {
	p.mu.Lock()
	call := p.inflight
	if call == nil {
		now := p.now()
		if !p.lastAttempt.IsZero() && now.Sub(p.lastAttempt) < p.minRefreshInterval {
			p.mu.Unlock()
			return errRefreshThrottled
		}
		call = &jwksRefresh{done: make(chan struct{})}
		p.inflight = call
		p.lastAttempt = now
		go p.runRefresh(call)
	}
	p.mu.Unlock()
	select {
	case <-call.done:
		return call.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// runRefresh performs the fetch detached from any single request context so
// that one cancelled caller cannot abort the refresh shared by the others.
func (p *jwksProvider) runRefresh(call *jwksRefresh) {
	ctx, cancel := context.WithTimeout(context.Background(), p.fetchTimeout)
	defer cancel()
	keys, err := p.fetch(ctx, p.url)
	p.mu.Lock()
	if err == nil {
		p.keys = keys
		p.fetchedAt = p.now()
		for keyID := range keys {
			delete(p.unknown, keyID)
		}
	}
	call.err = err
	p.inflight = nil
	p.mu.Unlock()
	close(call.done)
}

func (p *jwksProvider) rememberUnknownLocked(keyID string, now time.Time) {
	if len(p.unknown) >= p.maxUnknownKeys {
		for cachedID, until := range p.unknown {
			if !now.Before(until) {
				delete(p.unknown, cachedID)
			}
		}
		if len(p.unknown) >= p.maxUnknownKeys {
			// Cache is full of live entries. The global refresh cooldown still
			// bounds outbound fetches, so skipping the insert is safe.
			return
		}
	}
	p.unknown[keyID] = now.Add(p.unknownKeyTTL)
}

func bearerToken(r *http.Request) (string, error) {
	authorization := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(strings.ToLower(authorization), "bearer ") {
		return "", errors.New("bearer token required")
	}
	raw := strings.TrimSpace(authorization[len("Bearer "):])
	if raw == "" {
		return "", errors.New("bearer token required")
	}
	return raw, nil
}

func principalFromClaims(token *jwt.Token, claims *Claims, err error) (Principal, error) {
	if err != nil || !token.Valid || claims.TenantID == "" || claims.Subject == "" {
		return Principal{}, errors.New("invalid token")
	}
	principal := Principal{TenantID: claims.TenantID, ActorID: claims.Subject, SubjectID: claims.SubjectID, Roles: claims.Roles}
	// A self-service token is only meaningful when bound to a canonical
	// customer subject; without it the subject boundary cannot be enforced.
	if HasAnyRole(principal, RoleSelfService) && strings.TrimSpace(claims.SubjectID) == "" {
		return Principal{}, errors.New("self_service token requires customer_subject_id")
	}
	return principal, nil
}

func FromRequest(r *http.Request) (Principal, error) {
	jwksURL := strings.TrimSpace(os.Getenv("JWT_JWKS_URL"))
	secret := strings.TrimSpace(os.Getenv("JWT_HMAC_SECRET"))
	if jwksURL != "" {
		issuer := strings.TrimSpace(os.Getenv("JWT_ISSUER"))
		audience := strings.TrimSpace(os.Getenv("JWT_AUDIENCE"))
		if issuer == "" || audience == "" {
			return Principal{}, errors.New("JWKS authentication requires issuer and audience")
		}
		raw, err := bearerToken(r)
		if err != nil {
			return Principal{}, err
		}
		claims := &Claims{}
		options := []jwt.ParserOption{jwt.WithExpirationRequired()}
		options = append(options, jwt.WithIssuer(issuer), jwt.WithAudience(audience))
		token, parseErr := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
			if token.Method.Alg() != jwt.SigningMethodRS256.Alg() {
				return nil, errors.New("unexpected signing method")
			}
			keyID, _ := token.Header["kid"].(string)
			if keyID == "" {
				return nil, errors.New("JWT key id required")
			}
			if len(keyID) > maxKeyIDLength {
				return nil, errors.New("JWT key id too long")
			}
			return jwksProviderFor(jwksURL).key(r.Context(), keyID)
		}, options...)
		return principalFromClaims(token, claims, parseErr)
	}
	if secret != "" && strings.EqualFold(os.Getenv("ALLOW_HMAC_AUTH"), "true") {
		raw, err := bearerToken(r)
		if err != nil {
			return Principal{}, err
		}
		claims := &Claims{}
		token, parseErr := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
			if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, errors.New("unexpected signing method")
			}
			return []byte(secret), nil
		}, jwt.WithExpirationRequired())
		return principalFromClaims(token, claims, parseErr)
	}
	if strings.EqualFold(os.Getenv("ALLOW_DEV_HEADERS"), "true") {
		tenantID := strings.TrimSpace(r.Header.Get("X-Nvuto-Tenant-ID"))
		actorID := strings.TrimSpace(r.Header.Get("X-Nvuto-Actor-ID"))
		if tenantID == "" || actorID == "" {
			return Principal{}, errors.New("development tenant and actor headers required")
		}
		return Principal{TenantID: tenantID, ActorID: actorID, SubjectID: actorID, Roles: []string{"developer"}}, nil
	}
	return Principal{}, errors.New("authentication is not configured")
}

func HasAnyRole(principal Principal, roles ...string) bool {
	for _, have := range principal.Roles {
		for _, want := range roles {
			if have == want {
				return true
			}
		}
	}
	return false
}

// ValidateConfig checks that at least one supported authentication mode is
// properly configured and usable (T-015). Supported modes are:
//  1. JWKS: JWT_JWKS_URL (valid http/https URL), JWT_ISSUER, and JWT_AUDIENCE.
//  2. HMAC: ALLOW_HMAC_AUTH=true and non-empty JWT_HMAC_SECRET.
//  3. Dev:  ALLOW_DEV_HEADERS=true (explicitly allowed dev headers mode).
//
// If none of these modes are enabled or if an enabled mode has invalid settings,
// ValidateConfig returns an error explaining the configuration failure.
func ValidateConfig() error {
	jwksURL := strings.TrimSpace(os.Getenv("JWT_JWKS_URL"))
	hmacAllowed := strings.EqualFold(os.Getenv("ALLOW_HMAC_AUTH"), "true")
	devAllowed := strings.EqualFold(os.Getenv("ALLOW_DEV_HEADERS"), "true")
	secret := strings.TrimSpace(os.Getenv("JWT_HMAC_SECRET"))

	if jwksURL != "" {
		issuer := strings.TrimSpace(os.Getenv("JWT_ISSUER"))
		audience := strings.TrimSpace(os.Getenv("JWT_AUDIENCE"))
		if issuer == "" || audience == "" {
			return errors.New("JWKS authentication requires JWT_ISSUER and JWT_AUDIENCE")
		}
		parsed, err := url.ParseRequestURI(jwksURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return errors.New("JWT_JWKS_URL must be a valid HTTP or HTTPS URL")
		}
		return nil
	}

	if hmacAllowed {
		if secret == "" {
			return errors.New("ALLOW_HMAC_AUTH=true requires non-empty JWT_HMAC_SECRET")
		}
		return nil
	}

	if devAllowed {
		return nil
	}

	if secret != "" {
		return errors.New("JWT_HMAC_SECRET is set but ALLOW_HMAC_AUTH is not true; HMAC authentication is disabled")
	}

	return errors.New("authentication is not configured: must configure JWKS (JWT_JWKS_URL, JWT_ISSUER, JWT_AUDIENCE), HMAC (JWT_HMAC_SECRET with ALLOW_HMAC_AUTH=true), or ALLOW_DEV_HEADERS=true")
}
