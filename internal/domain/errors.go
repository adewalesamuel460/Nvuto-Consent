package domain

import (
	"context"
	"errors"
)

// ErrorKind classifies a failure for the public API contract (T-013). Every
// kind maps to exactly one HTTP status in cmd/server, so a client can rely on
// the status and the stable Code without parsing messages.
type ErrorKind uint8

const (
	// KindInternal is an unexpected server fault. Never retryable by
	// contract, never rendered with its cause.
	KindInternal ErrorKind = iota
	// KindValidation is malformed, ambiguous or out-of-bounds input (400).
	KindValidation
	// KindPolicy is well-formed input rejected by the tenant's consent policy
	// registries (422).
	KindPolicy
	// KindUnauthenticated means no valid principal could be established (401).
	KindUnauthenticated
	// KindForbidden means the principal may not perform the action (403).
	KindForbidden
	// KindNotFound means the resource does not exist in the caller's scope (404).
	KindNotFound
	// KindConflict means the request conflicts with current state (409).
	KindConflict
	// KindGone means the resource existed but has expired (410).
	KindGone
	// KindTooLarge means the request body exceeds the accepted size (413).
	KindTooLarge
	// KindUnavailable is a transient dependency failure, timeout or
	// contention the client may retry (503).
	KindUnavailable
	// KindCanceled means the caller abandoned the request before it finished.
	KindCanceled
)

func (k ErrorKind) String() string {
	switch k {
	case KindValidation:
		return "validation"
	case KindPolicy:
		return "policy"
	case KindUnauthenticated:
		return "unauthenticated"
	case KindForbidden:
		return "forbidden"
	case KindNotFound:
		return "not_found"
	case KindConflict:
		return "conflict"
	case KindGone:
		return "gone"
	case KindTooLarge:
		return "too_large"
	case KindUnavailable:
		return "unavailable"
	case KindCanceled:
		return "canceled"
	default:
		return "internal"
	}
}

// Error is a typed failure with a stable public code and message and an
// optional internal cause. The cause participates in errors.Is/As and in
// Error() (for operator logs) but is never part of the public rendering.
type Error struct {
	kind    ErrorKind
	code    string
	message string
	field   string
	cause   error
	base    *Error
}

// NewError returns a sentinel error. Sentinels are compared by identity; every
// error derived from a sentinel also matches it under errors.Is.
func NewError(kind ErrorKind, code, message string) *Error {
	return &Error{kind: kind, code: code, message: message}
}

// Kind, Code, Message and Field expose the public contract of the error.
func (e *Error) Kind() ErrorKind { return e.kind }
func (e *Error) Code() string    { return e.code }
func (e *Error) Message() string { return e.message }
func (e *Error) Field() string   { return e.field }

// Error renders code, message, field and internal cause for operator logs.
// It must not be sent to clients; use Code/Message/Field instead.
func (e *Error) Error() string {
	s := e.code + ": " + e.message
	if e.field != "" {
		s += " (field " + e.field + ")"
	}
	if e.cause != nil {
		s += ": " + e.cause.Error()
	}
	return s
}

func (e *Error) Unwrap() error { return e.cause }

// Is reports whether target is e or a sentinel e was derived from.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	for current := e; current != nil; current = current.base {
		if current == t {
			return true
		}
	}
	return false
}

func (e *Error) derive() *Error {
	derived := *e
	derived.base = e
	return &derived
}

// Derive returns a more specific sentinel of the same kind that still matches
// e under errors.Is.
func (e *Error) Derive(code, message string) *Error {
	derived := e.derive()
	derived.code, derived.message = code, message
	return derived
}

// WithField returns an error that matches e and names the offending input
// field with a safe, actionable public message.
func (e *Error) WithField(field, message string) *Error {
	derived := e.derive()
	derived.field = field
	if message != "" {
		derived.message = message
	}
	return derived
}

// Wrap attaches an internal cause for logging and errors.Is/As while keeping
// the public code and message of e. A nil cause returns e itself.
func (e *Error) Wrap(cause error) error {
	if cause == nil {
		return e
	}
	derived := e.derive()
	derived.cause = cause
	return derived
}

// Sentinels shared by every layer. Store and service re-export them so
// existing errors.Is checks keep working.
var (
	ErrInternal        = NewError(KindInternal, "internal_error", "an internal error occurred")
	ErrValidation      = NewError(KindValidation, "validation_failed", "the request failed validation")
	ErrPolicyRejected  = &Error{kind: KindPolicy, code: "policy_rejected", message: "the request is not permitted by the tenant consent policy", base: ErrValidation}
	ErrUnauthenticated = NewError(KindUnauthenticated, "unauthenticated", "valid authentication is required")
	ErrForbidden       = NewError(KindForbidden, "forbidden", "the principal is not authorized for this action")
	ErrNotFound        = NewError(KindNotFound, "not_found", "resource not found")
	ErrConflict        = NewError(KindConflict, "conflict", "the request conflicts with the current state")
	ErrExpired         = NewError(KindGone, "expired", "the resource has expired")
	ErrTooLarge        = NewError(KindTooLarge, "body_too_large", "the request body is too large")
	ErrUnavailable     = NewError(KindUnavailable, "unavailable", "the service is temporarily unavailable; retry later")
	ErrContention      = ErrUnavailable.Derive("contention", "the request conflicted with concurrent updates; retry later")
	ErrCanceled        = NewError(KindCanceled, "canceled", "the request was canceled")
)

// KindOf classifies any error. Unclassified context errors are mapped so a
// deadline is retryable and a cancellation is attributed to the caller;
// everything else unknown is internal (fail closed: never a 4xx that blames
// the client for a server fault).
func KindOf(err error) ErrorKind {
	if err == nil {
		return KindInternal
	}
	var typed *Error
	if errors.As(err, &typed) {
		return typed.kind
	}
	switch {
	case errors.Is(err, context.Canceled):
		return KindCanceled
	case errors.Is(err, context.DeadlineExceeded):
		return KindUnavailable
	}
	return KindInternal
}

// Public returns the error whose code and message may be shown to a client.
// Internal faults always collapse to ErrInternal so no cause text leaks.
func Public(err error) *Error {
	var typed *Error
	if errors.As(err, &typed) && typed.kind != KindInternal {
		return typed
	}
	switch KindOf(err) {
	case KindCanceled:
		return ErrCanceled
	case KindUnavailable:
		return ErrUnavailable
	}
	return ErrInternal
}

// Retryable reports whether the client may safely retry the same request.
func Retryable(err error) bool { return KindOf(err) == KindUnavailable }
