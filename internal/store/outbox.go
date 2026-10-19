package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"
)

// OutboxLeaseDuration is how long a claim owns its rows. After it elapses
// another worker may re-claim them with a new token, which fences the old
// claim. It must stay well below the JetStream duplicate window so a row
// re-published after a lost completion is still de-duplicated by Msg-Id.
const OutboxLeaseDuration = 2 * time.Minute

// ErrLeaseLost is returned when a completion, retry or release presents a
// claim token that no longer owns the row: the lease expired and was
// re-claimed, or the row was already completed or released. It matches
// ErrConflict. The caller must not retry the write.
var ErrLeaseLost = ErrConflict.Derive("outbox_lease_lost", "outbox lease is owned by a newer claim")

// OutboxRecord is one pending outbox event as seen by a publisher.
type OutboxRecord struct {
	ID            string          `json:"id"`
	SchemaVersion int             `json:"schema_version"`
	Source        string          `json:"source"`
	TenantID      string          `json:"tenant_id"`
	EventType     string          `json:"event_type"`
	AggregateType string          `json:"aggregate_type"`
	AggregateID   string          `json:"aggregate_id"`
	Payload       json.RawMessage `json:"payload"`
	OccurredAt    time.Time       `json:"occurred_at"`
	// Attempts is the number of earlier claims of this row (excluding this one).
	Attempts int `json:"attempts"`
}

// OutboxClaim is a batch of rows locked under a single token.
type OutboxClaim struct {
	// Token is unguessable and unique to this claim. Every completion, retry
	// and release must present it.
	Token string
	// Lease is how long the claim owns its rows, measured from before the
	// claim transaction started.
	Lease  time.Duration
	Events []OutboxRecord
}

// OutboxLeaser is the fenced lease protocol used by the outbox worker.
type OutboxLeaser interface {
	// ClaimOutbox locks up to limit due, unpublished rows that are unlocked
	// or whose lease has expired, and stamps them with a fresh token.
	ClaimOutbox(ctx context.Context, workerID string, limit int, tenantFilter string) (OutboxClaim, error)
	// CompleteOutbox marks a published row done. ErrLeaseLost if token no
	// longer owns the row.
	CompleteOutbox(ctx context.Context, id, token string) error
	// RetryOutbox unlocks the row and defers its next attempt by delay.
	// ErrLeaseLost if token no longer owns the row.
	RetryOutbox(ctx context.Context, id, token string, delay time.Duration) error
	// ReleaseOutbox unlocks an unprocessed row so it can be claimed at once,
	// and does not count the claim as an attempt. ErrLeaseLost if token no
	// longer owns the row.
	ReleaseOutbox(ctx context.Context, id, token string) error
}

// newClaimToken returns 128 random bits as 32 lowercase hex characters.
func newClaimToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
