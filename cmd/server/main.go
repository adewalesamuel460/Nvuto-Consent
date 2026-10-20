package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Atlasfsp/Nvuto-Consent/internal/auth"
	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/service"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
)

const (
	// maxBodyBytes bounds every request body (413 beyond it).
	maxBodyBytes = 1 << 20
	// maxJSONDepth bounds nesting of request documents; no endpoint needs
	// more than a few levels.
	maxJSONDepth = 32
	// defaultRequestTimeout bounds the work of a single request, including
	// all database calls, and stays below the server WriteTimeout so the
	// client receives a typed 503 rather than a dropped connection.
	defaultRequestTimeout = 10 * time.Second
	// maxLoggedCause bounds the operator log line for one failure.
	maxLoggedCause = 512
	// statusClientClosedRequest (nginx convention) records requests the client
	// abandoned. Nothing is delivered; it exists for logs and metrics.
	statusClientClosedRequest = 499
	correlationHeader         = "X-Correlation-ID"
)

type api struct {
	svc *service.Service
	log *slog.Logger
}

// Public request contracts (T-027). Every policy boolean is a *bool so an
// omitted value is rejected instead of silently decoding as false, and the
// structs carry only client-settable fields: tenant, version and timestamps
// come from the authenticated principal and the server.

type purposeRequest struct {
	Code                       string   `json:"code"`
	Category                   string   `json:"category"`
	AllowedChannels            []string `json:"allowed_channels"`
	RequiresAffirmativeConsent *bool    `json:"requires_affirmative_consent"`
	RequiresDoubleOptIn        *bool    `json:"requires_double_opt_in"`
	Active                     *bool    `json:"active"`
}

type lawfulBasisRequest struct {
	Code              string   `json:"code"`
	AllowedCategories []string `json:"allowed_categories"`
	RequiresEvidence  *bool    `json:"requires_evidence"`
	Active            *bool    `json:"active"`
}

type noticeRequest struct {
	Version      string    `json:"version"`
	Jurisdiction string    `json:"jurisdiction"`
	PublishedAt  time.Time `json:"published_at"`
	Active       *bool     `json:"active"`
}

type suppressionRequest struct {
	SubjectID string                  `json:"subject_id"`
	Scope     domain.SuppressionScope `json:"scope"`
	Purpose   string                  `json:"purpose"`
	Channel   string                  `json:"channel"`
	Reason    string                  `json:"reason"`
	Active    *bool                   `json:"active"`
}

type recordRequest struct {
	SubjectID     string            `json:"subject_id"`
	Purpose       string            `json:"purpose"`
	Channel       string            `json:"channel"`
	LegalBasis    string            `json:"legal_basis"`
	Status        string            `json:"status"`
	Source        string            `json:"source"`
	Scope         map[string]string `json:"scope"`
	ExpiresAt     *time.Time        `json:"expires_at"`
	Jurisdiction  string            `json:"jurisdiction"`
	NoticeVersion string            `json:"notice_version"`
	Evidence      domain.Evidence   `json:"evidence"`
}

type evaluateRequest struct {
	SubjectID string     `json:"subject_id"`
	Purpose   string     `json:"purpose"`
	Channel   string     `json:"channel"`
	At        *time.Time `json:"at"`
}

type batchEvaluateRequest struct {
	Requests []domain.EvaluationRequest `json:"requests"`
}

type doubleOptInRequest struct {
	SubjectID     string          `json:"subject_id"`
	Purpose       string          `json:"purpose"`
	Channel       string          `json:"channel"`
	Source        string          `json:"source"`
	Jurisdiction  string          `json:"jurisdiction"`
	NoticeVersion string          `json:"notice_version"`
	Evidence      domain.Evidence `json:"evidence"`
	ExpiresAt     time.Time       `json:"expires_at"`
}

type confirmRequest struct {
	Token string `json:"token"`
}

type boolField struct {
	name  string
	value *bool
}

// requireBools rejects the first omitted or null boolean.
func requireBools(fields ...boolField) error {
	for _, f := range fields {
		if f.value == nil {
			return errRequiredField.WithField(f.name, "is required and must be true or false")
		}
	}
	return nil
}

func (in purposeRequest) toDomain() (domain.Purpose, error) {
	if err := requireBools(
		boolField{"requires_affirmative_consent", in.RequiresAffirmativeConsent},
		boolField{"requires_double_opt_in", in.RequiresDoubleOptIn},
		boolField{"active", in.Active},
	); err != nil {
		return domain.Purpose{}, err
	}
	return domain.Purpose{Code: in.Code, Category: in.Category, AllowedChannels: in.AllowedChannels, RequiresAffirmativeConsent: *in.RequiresAffirmativeConsent, RequiresDoubleOptIn: *in.RequiresDoubleOptIn, Active: *in.Active}, nil
}

func (in lawfulBasisRequest) toDomain() (domain.LawfulBasis, error) {
	if err := requireBools(boolField{"requires_evidence", in.RequiresEvidence}, boolField{"active", in.Active}); err != nil {
		return domain.LawfulBasis{}, err
	}
	return domain.LawfulBasis{Code: in.Code, AllowedCategories: in.AllowedCategories, RequiresEvidence: *in.RequiresEvidence, Active: *in.Active}, nil
}

func (in noticeRequest) toDomain() (domain.PrivacyNotice, error) {
	if err := requireBools(boolField{"active", in.Active}); err != nil {
		return domain.PrivacyNotice{}, err
	}
	return domain.PrivacyNotice{Version: in.Version, Jurisdiction: in.Jurisdiction, PublishedAt: in.PublishedAt, Active: *in.Active}, nil
}

func (in suppressionRequest) toDomain() (domain.Suppression, error) {
	if err := requireBools(boolField{"active", in.Active}); err != nil {
		return domain.Suppression{}, err
	}
	return domain.Suppression{SubjectID: in.SubjectID, Scope: in.Scope, Purpose: in.Purpose, Channel: in.Channel, Reason: in.Reason, Active: *in.Active}, nil
}

// Stable public error codes produced by the HTTP layer itself.
var (
	errInvalidJSON      = domain.ErrValidation.Derive("invalid_json", "the request body must be a single valid JSON object")
	errTrailingData     = domain.ErrValidation.Derive("trailing_data", "the request body must contain exactly one JSON object and nothing after it")
	errUnknownField     = domain.ErrValidation.Derive("unknown_field", "the request contains an unrecognized field")
	errDuplicateField   = domain.ErrValidation.Derive("duplicate_field", "the request repeats a field")
	errInvalidType      = domain.ErrValidation.Derive("invalid_type", "a field has the wrong JSON type")
	errRequiredField    = domain.ErrValidation.Derive("required_field", "a required field is missing")
	errRoleForbidden    = domain.ErrForbidden.Derive("role_not_authorized", "role not authorized")
	errSubjectForbidden = domain.ErrForbidden.Derive("subject_not_authorized", "subject not authorized")
)

type errorBody struct {
	Error         string `json:"error"`
	Message       string `json:"message"`
	Field         string `json:"field,omitempty"`
	CorrelationID string `json:"correlation_id"`
}

func write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

type correlationKey struct{}

func newCorrelationID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("t%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func correlationID(ctx context.Context) string {
	id, _ := ctx.Value(correlationKey{}).(string)
	return id
}

// httpStatus is the single mapping from error kind to public status (T-013).
func httpStatus(kind domain.ErrorKind) int {
	switch kind {
	case domain.KindValidation:
		return http.StatusBadRequest
	case domain.KindPolicy:
		return http.StatusUnprocessableEntity
	case domain.KindUnauthenticated:
		return http.StatusUnauthorized
	case domain.KindForbidden:
		return http.StatusForbidden
	case domain.KindNotFound:
		return http.StatusNotFound
	case domain.KindConflict:
		return http.StatusConflict
	case domain.KindGone:
		return http.StatusGone
	case domain.KindTooLarge:
		return http.StatusRequestEntityTooLarge
	case domain.KindUnavailable:
		return http.StatusServiceUnavailable
	case domain.KindCanceled:
		return statusClientClosedRequest
	default:
		return http.StatusInternalServerError
	}
}

// sanitizeCause renders an internal error for the operator log: control
// characters are replaced and the text is bounded. It is never sent to the
// client.
func sanitizeCause(err error) string {
	if err == nil {
		return ""
	}
	cause := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, err.Error())
	if len(cause) > maxLoggedCause {
		cut := maxLoggedCause
		for cut > 0 && !utf8.RuneStart(cause[cut]) {
			cut--
		}
		cause = cause[:cut] + "...(truncated)"
	}
	return cause
}

// writeError is the only path that renders a failure. The client receives a
// stable status, code, safe message, optional field and the correlation ID;
// the classified cause is logged locally under the same correlation ID.
// Unclassified errors are internal faults (500) and never fall through as a
// 4xx carrying raw driver, SQL or schema text.
func (a api) writeError(w http.ResponseWriter, r *http.Request, err error) {
	kind := domain.KindOf(err)
	public := domain.Public(err)
	if kind == domain.KindCanceled && !errors.Is(r.Context().Err(), context.Canceled) {
		// Not abandoned by the client (for example server shutdown): a
		// retryable outage from the client's point of view.
		kind, public = domain.KindUnavailable, domain.ErrUnavailable
	}
	status := httpStatus(kind)
	cid := correlationID(r.Context())
	cause := sanitizeCause(err)
	if status >= 500 && status != statusClientClosedRequest {
		a.log.ErrorContext(r.Context(), "request_failed",
			slog.String("correlation_id", cid),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", status),
			slog.String("kind", kind.String()),
			slog.String("code", public.Code()),
			slog.String("cause", cause),
		)
	} else {
		a.log.InfoContext(r.Context(), "request_failed",
			slog.String("correlation_id", cid),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", status),
			slog.String("kind", kind.String()),
			slog.String("code", public.Code()),
			slog.String("cause", cause),
		)
	}
	if status == http.StatusServiceUnavailable {
		w.Header().Set("Retry-After", "1")
	}
	w.Header().Set("Cache-Control", "no-store")
	write(w, status, errorBody{Error: public.Code(), Message: public.Message(), Field: public.Field(), CorrelationID: cid})
}

func (a api) writeMethodNotAllowed(w http.ResponseWriter, r *http.Request, allowed string) {
	cid := correlationID(r.Context())
	w.Header().Set("Allow", allowed)
	write(w, http.StatusMethodNotAllowed, errorBody{Error: "method_not_allowed", Message: "method not allowed", CorrelationID: cid})
}

// decodeRequest reads exactly one JSON object into dst (T-027). It rejects:
// bodies over maxBodyBytes (413); anything that is not a JSON object,
// including a top-level null; duplicate member names, including the
// case-variant duplicates encoding/json would silently merge; unknown fields;
// wrong JSON types; excessive nesting; and any data after the first object.
func decodeRequest(w http.ResponseWriter, r *http.Request, dst any) error {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return domain.ErrTooLarge.Wrap(err)
		}
		if ctxErr := r.Context().Err(); ctxErr != nil {
			if errors.Is(ctxErr, context.Canceled) {
				return domain.ErrCanceled.Wrap(err)
			}
			return domain.ErrUnavailable.Wrap(err)
		}
		return errInvalidJSON.Wrap(err)
	}
	trimmed := bytes.TrimLeft(body, " \t\r\n")
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return errInvalidJSON
	}
	if err := scanJSONObject(body); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return describeDecodeError(err)
	}
	// Exactly one value: More() catches a following value, and the EOF check
	// also catches stray closing delimiters that More() reports as "no more".
	if decoder.More() {
		return errTrailingData
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errTrailingData
	}
	return nil
}

// scanJSONObject walks the first JSON value with the token decoder to reject
// duplicate member names and excessive depth before struct decoding.
func scanJSONObject(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	return scanJSONValue(decoder, "", 0, true)
}

// foldKey approximates the case folding encoding/json applies when matching
// member names to struct fields (including the Kelvin and long-s forms).
func foldKey(key string) string { return strings.ToLower(strings.ToUpper(key)) }

func joinPath(parent, key string) string {
	if parent == "" {
		return key
	}
	return parent + "." + key
}

func scanJSONValue(decoder *json.Decoder, path string, depth int, foldNames bool) error {
	if depth > maxJSONDepth {
		return errInvalidJSON.WithField(path, fmt.Sprintf("must not nest deeper than %d levels", maxJSONDepth))
	}
	token, err := decoder.Token()
	if err != nil {
		return errInvalidJSON.Wrap(err)
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return errInvalidJSON.Wrap(err)
			}
			key, ok := keyToken.(string)
			if !ok {
				return errInvalidJSON
			}
			name := key
			if foldNames {
				name = foldKey(key)
			}
			if seen[name] {
				return errDuplicateField.WithField(joinPath(path, boundedName(key)), "must appear at most once (member names are matched case-insensitively)")
			}
			seen[name] = true
			// Map-typed members (scope) keep exact, case-sensitive keys.
			childFold := foldNames && foldKey(key) != "scope"
			if err := scanJSONValue(decoder, joinPath(path, key), depth+1, childFold); err != nil {
				return err
			}
		}
		if _, err := decoder.Token(); err != nil {
			return errInvalidJSON.Wrap(err)
		}
	case '[':
		for i := 0; decoder.More(); i++ {
			if err := scanJSONValue(decoder, fmt.Sprintf("%s[%d]", path, i), depth+1, foldNames); err != nil {
				return err
			}
		}
		if _, err := decoder.Token(); err != nil {
			return errInvalidJSON.Wrap(err)
		}
	}
	return nil
}

func boundedName(name string) string {
	if len(name) > 64 {
		cut := 64
		for cut > 0 && !utf8.RuneStart(name[cut]) {
			cut--
		}
		return name[:cut]
	}
	return name
}

func jsonTypeName(t reflect.Type) string {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil {
		return "a valid value"
	}
	if t == reflect.TypeOf(time.Time{}) {
		return "an RFC 3339 timestamp string"
	}
	switch t.Kind() {
	case reflect.Bool:
		return "true or false"
	case reflect.String:
		return "a string"
	case reflect.Slice, reflect.Array:
		return "an array"
	case reflect.Map, reflect.Struct:
		return "an object"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		return "a number"
	}
	return "a valid value"
}

// describeDecodeError maps a decoder failure to a typed 400 that names the
// field without echoing the offending value.
func describeDecodeError(err error) error {
	var typeErr *json.UnmarshalTypeError
	var syntaxErr *json.SyntaxError
	var timeErr *time.ParseError
	switch {
	case errors.As(err, &typeErr):
		return errInvalidType.WithField(typeErr.Field, "must be "+jsonTypeName(typeErr.Type))
	case errors.As(err, &timeErr):
		return errInvalidType.WithField("", "timestamps must be RFC 3339 strings").Wrap(err)
	case errors.As(err, &syntaxErr), errors.Is(err, io.ErrUnexpectedEOF):
		return errInvalidJSON.Wrap(err)
	}
	if message := err.Error(); strings.HasPrefix(message, "json: unknown field ") {
		name := strings.Trim(strings.TrimPrefix(message, "json: unknown field "), `"`)
		return errUnknownField.WithField(boundedName(name), "is not a recognized field")
	}
	return errInvalidJSON.Wrap(err)
}

func (a api) decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := decodeRequest(w, r, dst); err != nil {
		a.writeError(w, r, err)
		return false
	}
	return true
}

func (a api) principal(w http.ResponseWriter, r *http.Request, roles ...string) (auth.Principal, bool) {
	principal, err := auth.FromRequest(r)
	if err != nil {
		// The reason is logged, not returned, so token problems cannot be
		// probed through response text.
		a.writeError(w, r, domain.ErrUnauthenticated.Wrap(err))
		return auth.Principal{}, false
	}
	if len(roles) > 0 && !auth.HasAnyRole(principal, roles...) {
		a.writeError(w, r, errRoleForbidden)
		return auth.Principal{}, false
	}
	return principal, true
}

func (a api) requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	a.writeMethodNotAllowed(w, r, method)
	return false
}

var adminRoles = []string{"privacy_admin", "consent_admin", "developer"}
var writerRoles = []string{"privacy_admin", "consent_admin", "consent_writer", auth.RoleSelfService, "developer"}
var readerRoles = []string{"privacy_admin", "consent_admin", "consent_writer", "consent_reader", "service", auth.RoleSelfService, "developer"}

// Roles that carry tenant-wide subject scope for a given action. A role only
// widens scope for the actions it is itself allowed to perform, so a read-only
// role (service, consent_reader) never widens a self_service writer (T-030).
var tenantWideWriterRoles = []string{"privacy_admin", "consent_admin", "consent_writer", "developer"}
var tenantWideReaderRoles = []string{"privacy_admin", "consent_admin", "consent_writer", "consent_reader", "service", "developer"}

type action int

const (
	actionRead action = iota
	actionWrite
)

// subjectScope derives the subjects a principal may act on for the requested
// action. Tenant-wide scope requires a role that is tenant-wide for that same
// action; otherwise self_service is restricted to its canonical subject.
func subjectScope(p auth.Principal, act action) (service.SubjectScope, bool) {
	tenantWide := tenantWideReaderRoles
	if act == actionWrite {
		tenantWide = tenantWideWriterRoles
	}
	if auth.HasAnyRole(p, tenantWide...) {
		return service.TenantWide(), true
	}
	if auth.HasAnyRole(p, auth.RoleSelfService) && strings.TrimSpace(p.SubjectID) != "" {
		return service.OwnSubject(p.SubjectID), true
	}
	return service.SubjectScope{}, false
}

func subjectAuthorized(p auth.Principal, act action, subjectID string) bool {
	scope, ok := subjectScope(p, act)
	return ok && scope.Permits(subjectID)
}

func (a api) purposes(w http.ResponseWriter, r *http.Request) {
	if !a.requireMethod(w, r, http.MethodPost) {
		return
	}
	p, ok := a.principal(w, r, adminRoles...)
	if !ok {
		return
	}
	var input purposeRequest
	if !a.decode(w, r, &input) {
		return
	}
	purpose, err := input.toDomain()
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	output, err := a.svc.RegisterPurpose(r.Context(), p.TenantID, p.ActorID, purpose)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	write(w, http.StatusCreated, output)
}

func (a api) lawfulBases(w http.ResponseWriter, r *http.Request) {
	if !a.requireMethod(w, r, http.MethodPost) {
		return
	}
	p, ok := a.principal(w, r, adminRoles...)
	if !ok {
		return
	}
	var input lawfulBasisRequest
	if !a.decode(w, r, &input) {
		return
	}
	basis, err := input.toDomain()
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	output, err := a.svc.RegisterLawfulBasis(r.Context(), p.TenantID, p.ActorID, basis)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	write(w, http.StatusCreated, output)
}

func (a api) notices(w http.ResponseWriter, r *http.Request) {
	if !a.requireMethod(w, r, http.MethodPost) {
		return
	}
	p, ok := a.principal(w, r, adminRoles...)
	if !ok {
		return
	}
	var input noticeRequest
	if !a.decode(w, r, &input) {
		return
	}
	notice, err := input.toDomain()
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	output, err := a.svc.PublishNotice(r.Context(), p.TenantID, p.ActorID, notice)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	write(w, http.StatusCreated, output)
}

func (a api) consents(w http.ResponseWriter, r *http.Request) {
	if !a.requireMethod(w, r, http.MethodPost) {
		return
	}
	p, ok := a.principal(w, r, writerRoles...)
	if !ok {
		return
	}
	var input recordRequest
	if !a.decode(w, r, &input) {
		return
	}
	scope, ok := subjectScope(p, actionWrite)
	if !ok || !scope.Permits(input.SubjectID) {
		a.writeError(w, r, errSubjectForbidden)
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	var output domain.Consent
	var err error
	if idempotencyKey == "" {
		output, err = a.svc.RecordGoverned(r.Context(), p.TenantID, p.ActorID, input.SubjectID, input.Purpose, input.Channel, input.LegalBasis, input.Status, input.Source, input.Scope, input.ExpiresAt, input.Jurisdiction, input.NoticeVersion, input.Evidence)
	} else {
		output, err = a.svc.RecordGovernedIdempotent(r.Context(), idempotencyKey, p.TenantID, p.ActorID, input.SubjectID, input.Purpose, input.Channel, input.LegalBasis, input.Status, input.Source, input.Scope, input.ExpiresAt, input.Jurisdiction, input.NoticeVersion, input.Evidence, scope)
	}
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	write(w, http.StatusCreated, output)
}

func (a api) consent(w http.ResponseWriter, r *http.Request) {
	if !a.requireMethod(w, r, http.MethodPost) {
		return
	}
	p, ok := a.principal(w, r, writerRoles...)
	if !ok {
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/consents/"), "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "withdraw" {
		a.writeError(w, r, domain.ErrNotFound)
		return
	}
	consent, err := a.svc.Consent(r.Context(), p.TenantID, parts[0])
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	if !subjectAuthorized(p, actionWrite, consent.SubjectID) {
		a.writeError(w, r, errSubjectForbidden)
		return
	}
	output, err := a.svc.Withdraw(r.Context(), p.TenantID, p.ActorID, parts[0])
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	write(w, http.StatusOK, output)
}

func (a api) suppressions(w http.ResponseWriter, r *http.Request) {
	if !a.requireMethod(w, r, http.MethodPost) {
		return
	}
	p, ok := a.principal(w, r, writerRoles...)
	if !ok {
		return
	}
	var input suppressionRequest
	if !a.decode(w, r, &input) {
		return
	}
	if !subjectAuthorized(p, actionWrite, input.SubjectID) {
		a.writeError(w, r, errSubjectForbidden)
		return
	}
	suppression, err := input.toDomain()
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	output, err := a.svc.SetSuppression(r.Context(), p.TenantID, p.ActorID, suppression)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	write(w, http.StatusCreated, output)
}

func (a api) evaluate(w http.ResponseWriter, r *http.Request) {
	if !a.requireMethod(w, r, http.MethodPost) {
		return
	}
	p, ok := a.principal(w, r, readerRoles...)
	if !ok {
		return
	}
	var input evaluateRequest
	if !a.decode(w, r, &input) {
		return
	}
	if !subjectAuthorized(p, actionRead, input.SubjectID) {
		a.writeError(w, r, errSubjectForbidden)
		return
	}
	at := time.Time{}
	if input.At != nil {
		at = *input.At
	}
	output, err := a.svc.Evaluate(r.Context(), p.TenantID, input.SubjectID, input.Purpose, input.Channel, at)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	write(w, http.StatusOK, output)
}

func (a api) batchEvaluate(w http.ResponseWriter, r *http.Request) {
	if !a.requireMethod(w, r, http.MethodPost) {
		return
	}
	p, ok := a.principal(w, r, readerRoles...)
	if !ok {
		return
	}
	var input batchEvaluateRequest
	if !a.decode(w, r, &input) {
		return
	}
	for _, request := range input.Requests {
		if !subjectAuthorized(p, actionRead, request.SubjectID) {
			a.writeError(w, r, errSubjectForbidden)
			return
		}
	}
	output, err := a.svc.BatchEvaluate(r.Context(), p.TenantID, input.Requests)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	write(w, http.StatusOK, map[string]any{"decisions": output})
}

func (a api) startDoubleOptIn(w http.ResponseWriter, r *http.Request) {
	if !a.requireMethod(w, r, http.MethodPost) {
		return
	}
	p, ok := a.principal(w, r, writerRoles...)
	if !ok {
		return
	}
	var input doubleOptInRequest
	if !a.decode(w, r, &input) {
		return
	}
	if !subjectAuthorized(p, actionWrite, input.SubjectID) {
		a.writeError(w, r, errSubjectForbidden)
		return
	}
	output, err := a.svc.StartDoubleOptIn(r.Context(), p.TenantID, p.ActorID, input.SubjectID, input.Purpose, input.Channel, input.Source, input.Jurisdiction, input.NoticeVersion, input.Evidence, input.ExpiresAt)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	write(w, http.StatusCreated, map[string]any{
		"challenge_id": output.Consent.ID,
		"status":       output.Consent.Status,
		"consent":      output.Consent,
	})
}

func (a api) confirmDoubleOptIn(w http.ResponseWriter, r *http.Request) {
	if !a.requireMethod(w, r, http.MethodPost) {
		return
	}
	p, ok := a.principal(w, r, writerRoles...)
	if !ok {
		return
	}
	// Confirmation is a write: a self_service caller may only redeem challenges
	// for its own canonical subject; tenant-wide writers may redeem any.
	scope, ok := subjectScope(p, actionWrite)
	if !ok {
		a.writeError(w, r, errSubjectForbidden)
		return
	}
	var input confirmRequest
	if !a.decode(w, r, &input) {
		return
	}
	output, err := a.svc.ConfirmDoubleOptIn(r.Context(), p.TenantID, p.ActorID, scope, input.Token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	write(w, http.StatusOK, output)
}

func (a api) receipt(w http.ResponseWriter, r *http.Request) {
	if !a.requireMethod(w, r, http.MethodGet) {
		return
	}
	p, ok := a.principal(w, r, readerRoles...)
	if !ok {
		return
	}
	receiptID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/receipts/"), "/")
	output, err := a.svc.Receipt(r.Context(), p.TenantID, receiptID)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	if !subjectAuthorized(p, actionRead, output.SubjectID) {
		a.writeError(w, r, errSubjectForbidden)
		return
	}
	write(w, http.StatusOK, output)
}

// ValidateAuthConfig exposes the authentication configuration validation check (T-015).
func ValidateAuthConfig() error {
	return auth.ValidateConfig()
}

func (a api) ready(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.Ready(r.Context()); err != nil {
		a.log.WarnContext(r.Context(), "readiness_failed",
			slog.String("correlation_id", correlationID(r.Context())),
			slog.String("kind", domain.KindOf(err).String()),
			slog.String("cause", sanitizeCause(err)),
		)
		write(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	if err := auth.ValidateConfig(); err != nil {
		a.log.WarnContext(r.Context(), "readiness_auth_unconfigured",
			slog.String("correlation_id", correlationID(r.Context())),
			slog.String("cause", sanitizeCause(err)),
		)
		write(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	write(w, http.StatusOK, map[string]string{"status": "ready"})
}

// withRequestContext assigns a server-generated correlation ID (returned in
// X-Correlation-ID and in every error body), bounds the request with timeout
// and converts handler panics into a correlated 500. The request context is
// derived from the client connection, so a disconnect cancels database work
// (T-012).
func (a api) withRequestContext(next http.Handler, timeout time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cid := newCorrelationID()
		w.Header().Set(correlationHeader, cid)
		ctx := context.WithValue(r.Context(), correlationKey{}, cid)
		if timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}
		r = r.WithContext(ctx)
		defer func() {
			if recovered := recover(); recovered != nil {
				if recovered == http.ErrAbortHandler {
					panic(recovered)
				}
				recoveredErr := fmt.Errorf("panic: %v", recovered)
				a.log.ErrorContext(r.Context(), "request_panic",
					slog.String("correlation_id", cid),
					slog.String("cause", sanitizeCause(recoveredErr)),
				)
				a.writeError(w, r, domain.ErrInternal.Wrap(recoveredErr))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func newJSONLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

func newMux(svc *service.Service) http.Handler {
	return newHandler(svc, defaultRequestTimeout, newJSONLogger())
}

func newHandler(svc *service.Service, timeout time.Duration, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = newJSONLogger()
	}
	a := api{svc: svc, log: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		write(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/readyz", a.ready)
	mux.HandleFunc("/v1/purposes", a.purposes)
	mux.HandleFunc("/v1/lawful-bases", a.lawfulBases)
	mux.HandleFunc("/v1/notices", a.notices)
	mux.HandleFunc("/v1/consents", a.consents)
	mux.HandleFunc("/v1/consents/", a.consent)
	mux.HandleFunc("/v1/suppressions", a.suppressions)
	mux.HandleFunc("/v1/evaluate", a.evaluate)
	mux.HandleFunc("/v1/evaluate:batch", a.batchEvaluate)
	mux.HandleFunc("/v1/double-opt-in", a.startDoubleOptIn)
	mux.HandleFunc("/v1/double-opt-in/confirm", a.confirmDoubleOptIn)
	mux.HandleFunc("/v1/receipts/", a.receipt)
	return a.withRequestContext(mux, timeout)
}

// requestTimeout reads REQUEST_TIMEOUT (a Go duration). Invalid or
// non-positive values fall back to the default.
func requestTimeout(logger *slog.Logger) time.Duration {
	raw := strings.TrimSpace(os.Getenv("REQUEST_TIMEOUT"))
	if raw == "" {
		return defaultRequestTimeout
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		if logger != nil {
			logger.Warn("invalid_request_timeout", slog.String("raw", raw), slog.Duration("default", defaultRequestTimeout))
		}
		return defaultRequestTimeout
	}
	return value
}

func main() {
	logger := newJSONLogger()
	if err := auth.ValidateConfig(); err != nil {
		logger.Error("auth_configuration_invalid", slog.String("cause", sanitizeCause(err)))
		os.Exit(1)
	}

	var repo store.Repository
	closeStore := func() {}
	if databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL")); databaseURL != "" {
		startupCtx, cancelStartup := context.WithTimeout(context.Background(), 30*time.Second)
		postgres, err := store.NewPostgres(startupCtx, databaseURL)
		cancelStartup()
		if err != nil {
			logger.Error("postgres_unavailable", slog.String("cause", sanitizeCause(err)))
			os.Exit(1)
		}
		repo = postgres
		closeStore = postgres.Close
		logger.Info("using_postgresql_storage")
	} else {
		if !strings.EqualFold(os.Getenv("ALLOW_MEMORY_STORE"), "true") {
			logger.Error("database_url_required", slog.String("message", "DATABASE_URL is required unless ALLOW_MEMORY_STORE=true"))
			os.Exit(1)
		}
		repo = store.NewMemory()
		logger.Warn("using_non_durable_memory_storage")
	}
	defer closeStore()

	svc := service.New(repo)
	timeout := requestTimeout(logger)
	server := &http.Server{
		Addr:              ":8080",
		Handler:           newHandler(svc, timeout, logger),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      timeout + 5*time.Second,
		IdleTimeout:       60 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		logger.Info("server_listening", slog.String("addr", server.Addr), slog.Duration("request_timeout", timeout))
		errCh <- server.ListenAndServe()
	}()

	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-signalChannel:
		logger.Info("shutdown_requested")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server_failed", slog.String("error", err.Error()))
		}
	}
	shutdownContext, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		logger.Error("graceful_shutdown_failed", slog.String("error", err.Error()))
	}
}
