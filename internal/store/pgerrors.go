package store

import (
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net"
	"strings"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrInvalidValue is returned when the database rejects a value through a
// data or check constraint that input validation did not catch. It is a
// client-correctable 4xx; the constraint detail is only logged.
var ErrInvalidValue = domain.ErrValidation.Derive("invalid_value", "a value was rejected by storage constraints")

// errNoChange lets a transaction body request a rollback without reporting a
// failure (an idempotent replay or an already-applied withdrawal).
var errNoChange = errors.New("store: no change; transaction rolled back")

// transientSQLState reports the SQLSTATEs after which the server has aborted
// the whole transaction and the same transaction may simply be re-run:
// serialization_failure (also YSQL read-restart and transaction conflicts)
// and deadlock_detected.
func transientSQLState(code string) bool { return code == "40001" || code == "40P01" }

func unavailableSQLState(code string) bool {
	switch {
	case strings.HasPrefix(code, "08"): // connection exception
		return true
	case strings.HasPrefix(code, "53"): // insufficient resources
		return true
	case strings.HasPrefix(code, "58"): // system error (e.g. I/O)
		return true
	}
	switch code {
	case "57014", // query_canceled, including statement_timeout
		"57P01", "57P02", "57P03", // admin/crash shutdown, cannot connect now
		"55P03": // lock_not_available (lock_timeout)
		return true
	}
	return false
}

// classify maps a driver or database error onto the typed domain errors that
// define the public contract (T-013). It is idempotent: typed errors pass
// through unchanged. The raw error is kept only as the internal cause, so SQL
// text, constraint names and schema details never reach a client.
func classify(err error) error {
	if err == nil {
		return nil
	}
	var typed *domain.Error
	if errors.As(err, &typed) {
		return err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if errors.Is(err, context.Canceled) {
		return domain.ErrCanceled.Wrap(err)
	}
	if errors.Is(err, context.DeadlineExceeded) || pgconn.Timeout(err) {
		return domain.ErrUnavailable.Wrap(err)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch {
		case pgErr.Code == "23505", pgErr.Code == "23503": // unique / foreign key
			return ErrConflict.Wrap(err)
		case transientSQLState(pgErr.Code):
			return domain.ErrContention.Wrap(err)
		case unavailableSQLState(pgErr.Code):
			return domain.ErrUnavailable.Wrap(err)
		case strings.HasPrefix(pgErr.Code, "22"), pgErr.Code == "23502", pgErr.Code == "23514":
			return ErrInvalidValue.Wrap(err)
		}
		return domain.ErrInternal.Wrap(err)
	}
	var connectErr *pgconn.ConnectError
	var netErr net.Error
	if errors.As(err, &connectErr) || errors.As(err, &netErr) ||
		errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) ||
		pgconn.SafeToRetry(err) {
		return domain.ErrUnavailable.Wrap(err)
	}
	return domain.ErrInternal.Wrap(err)
}

// isTransient reports whether err carries a server-aborted transaction state.
// Only those are retried: the server discarded every effect of the attempt,
// so a re-run cannot duplicate receipt, audit or outbox rows. Errors whose
// outcome is unknown (a lost commit acknowledgement, a dropped connection)
// are never retried here; they surface as retryable 503s and the client's
// Idempotency-Key replay resolves them.
func isTransient(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && transientSQLState(pgErr.Code)
}

// retryPolicy bounds transaction retries (T-012). The total time is further
// bounded by the caller's context deadline.
type retryPolicy struct {
	attempts int
	base     time.Duration
	max      time.Duration
}

var defaultTxRetry = retryPolicy{attempts: 4, base: 20 * time.Millisecond, max: 250 * time.Millisecond}

func (p retryPolicy) delay(attempt int) time.Duration {
	d := p.base << (attempt - 1)
	if d <= 0 || d > p.max {
		d = p.max
	}
	// Equal jitter: spread contending writers without dropping below d/2.
	half := d / 2
	return half + rand.N(half+1)
}

// withTxRetry runs attempt until it succeeds, fails non-transiently, exhausts
// the policy, or ctx ends. The backoff wait is cancellation-aware, so an
// abandoned request stops retrying immediately.
func withTxRetry(ctx context.Context, policy retryPolicy, attempt func(context.Context) error) error {
	for n := 1; ; n++ {
		if err := ctx.Err(); err != nil {
			return contextError(err)
		}
		err := attempt(ctx)
		if err == nil || !isTransient(err) || n >= policy.attempts {
			return err
		}
		timer := time.NewTimer(policy.delay(n))
		select {
		case <-ctx.Done():
			timer.Stop()
			return contextError(ctx.Err())
		case <-timer.C:
		}
	}
}

// rollbackTimeout bounds the cleanup of an abandoned transaction.
const rollbackTimeout = 2 * time.Second

// rollback runs on a context detached from the (possibly cancelled) request
// so the server-side transaction ends and the connection returns to the pool
// promptly instead of lingering until the session is reaped.
func rollback(ctx context.Context, tx pgx.Tx) {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
	defer cancel()
	_ = tx.Rollback(cleanup)
}
