package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/jackc/pgx/v5/pgconn"
)

type outboxRow struct {
	token, lockedBy   string
	locked, published bool
	attempts          int
	deferred          bool
}

func readOutboxRow(t *testing.T, p *Postgres, id string) outboxRow {
	t.Helper()
	var row outboxRow
	var token, lockedBy *string
	err := p.pool.QueryRow(context.Background(), `SELECT claim_token,locked_by,locked_at IS NOT NULL,published_at IS NOT NULL,attempts,next_attempt_at>now()+interval '30 minutes'
		FROM consent_outbox_events WHERE id=$1`, id).Scan(&token, &lockedBy, &row.locked, &row.published, &row.attempts, &row.deferred)
	if err != nil {
		t.Fatal(err)
	}
	if token != nil {
		row.token = *token
	}
	if lockedBy != nil {
		row.lockedBy = *lockedBy
	}
	return row
}

func claimIDs(claim OutboxClaim) []string {
	ids := make([]string, len(claim.Events))
	for i, item := range claim.Events {
		ids[i] = item.ID
	}
	return ids
}

// AT-009 on real YSQL: token-fenced complete/retry/release after a genuine
// lease expiry, plus the migration 005 fence against pre-005 worker SQL.
func TestPostgresOutboxLeaseFencing(t *testing.T) {
	dsn := integrationDSN(t)
	ctx := context.Background()
	p, err := NewPostgres(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	p.SetOutboxLease(time.Second)
	run := runID(t)
	tenant := "outbox-fence-" + run
	ids := []string{"evt-" + run + "-a", "evt-" + run + "-b", "evt-" + run + "-legacy"}
	base := time.Now().UTC().Add(-time.Minute)
	for i, id := range ids {
		if err := p.AppendOutbox(context.Background(), domain.OutboxEvent{ID: id, TenantID: tenant, EventType: "consent.granted", AggregateID: "con-" + id, OccurredAt: base.Add(time.Duration(i) * time.Millisecond)}); err != nil {
			t.Fatal(err)
		}
	}

	stale, err := p.ClaimOutbox(ctx, "worker-a", 2, tenant)
	if err != nil || len(stale.Events) != 2 || stale.Events[0].ID != ids[0] || len(stale.Token) != 32 {
		t.Fatalf("first claim: %+v %v", claimIDs(stale), err)
	}
	if early, err := p.ClaimOutbox(ctx, "worker-b", 2, tenant); err != nil || len(early.Events) != 1 || early.Events[0].ID != ids[2] {
		t.Fatalf("live leases must be skipped: %v %v", claimIDs(early), err)
	} else if err := p.ReleaseOutbox(ctx, ids[2], early.Token); err != nil {
		t.Fatal(err)
	}

	time.Sleep(1500 * time.Millisecond)
	fresh, err := p.ClaimOutbox(ctx, "worker-b", 2, tenant)
	if err != nil || len(fresh.Events) != 2 || fresh.Token == stale.Token || fresh.Events[0].Attempts != 1 {
		t.Fatalf("expired leases must be re-claimed under a new token: %+v %v", fresh, err)
	}
	for _, id := range ids[:2] {
		for name, write := range map[string]func() error{
			"complete": func() error { return p.CompleteOutbox(ctx, id, stale.Token) },
			"retry":    func() error { return p.RetryOutbox(ctx, id, stale.Token, time.Hour) },
			"release":  func() error { return p.ReleaseOutbox(ctx, id, stale.Token) },
		} {
			if err := write(); !errors.Is(err, ErrLeaseLost) {
				t.Fatalf("stale %s of %s must be refused, got %v", name, id, err)
			}
		}
		if row := readOutboxRow(t, p, id); row.token != fresh.Token || row.lockedBy != "worker-b" || row.published || row.attempts != 2 || row.deferred {
			t.Fatalf("stale writes disturbed the newer claim on %s: %+v", id, row)
		}
	}

	if err := p.RetryOutbox(ctx, ids[0], fresh.Token, time.Hour); err != nil {
		t.Fatal(err)
	}
	if row := readOutboxRow(t, p, ids[0]); row.locked || row.token != "" || !row.deferred {
		t.Fatalf("owner retry must unlock and defer: %+v", row)
	}
	if err := p.ReleaseOutbox(ctx, ids[1], fresh.Token); err != nil {
		t.Fatal(err)
	}
	if row := readOutboxRow(t, p, ids[1]); row.locked || row.token != "" || row.attempts != 1 {
		t.Fatalf("owner release must unlock without counting the attempt: %+v", row)
	}
	next, err := p.ClaimOutbox(ctx, "worker-c", 1, tenant)
	if err != nil || len(next.Events) != 1 || next.Events[0].ID != ids[1] {
		t.Fatalf("released row must be claimable at once, deferred row must not: %v %v", claimIDs(next), err)
	}
	if err := p.CompleteOutbox(ctx, ids[1], next.Token); err != nil {
		t.Fatal(err)
	}
	if err := p.CompleteOutbox(ctx, ids[1], next.Token); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("completion must be single-use, got %v", err)
	}
	if row := readOutboxRow(t, p, ids[1]); !row.published || row.token != "" || row.locked {
		t.Fatalf("completed row must be published with no lease: %+v", row)
	}

	// Migration 005 fence: SQL issued by a pre-005 worker image.
	owner, err := p.ClaimOutbox(ctx, "worker-new", 1, tenant)
	if err != nil || len(owner.Events) != 1 || owner.Events[0].ID != ids[2] {
		t.Fatalf("claim of legacy-fence row: %v %v", claimIDs(owner), err)
	}
	legacyComplete := `UPDATE consent_outbox_events SET published_at=now(),locked_at=NULL,locked_by=NULL WHERE id=$1 AND published_at IS NULL`
	legacyRetry := `UPDATE consent_outbox_events SET locked_at=NULL,locked_by=NULL,next_attempt_at=now()+interval '1 second' WHERE id=$1 AND published_at IS NULL`
	for name, statement := range map[string]string{"complete": legacyComplete, "retry": legacyRetry} {
		_, err := p.pool.Exec(ctx, statement, ids[2])
		var pe *pgconn.PgError
		if !errors.As(err, &pe) || pe.Code != "55000" {
			t.Fatalf("legacy %s of a token-owned row must be rejected with 55000, got %v", name, err)
		}
	}
	if row := readOutboxRow(t, p, ids[2]); row.token != owner.Token || row.published {
		t.Fatalf("rejected legacy writes must not change the row: %+v", row)
	}
	// A legacy re-claim (after expiry) revokes the token-holding claim.
	if _, err := p.pool.Exec(ctx, `UPDATE consent_outbox_events SET locked_at=now(),locked_by='legacy-worker',attempts=attempts+1 WHERE id=$1`, ids[2]); err != nil {
		t.Fatal(err)
	}
	if err := p.CompleteOutbox(ctx, ids[2], owner.Token); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("legacy re-claim must fence the previous token owner, got %v", err)
	}
	// Legacy workers keep working on rows they own (no token).
	if _, err := p.pool.Exec(ctx, legacyComplete, ids[2]); err != nil {
		t.Fatalf("legacy completion of a legacy-owned row must still succeed: %v", err)
	}

	var tokenOnPublished int
	if err := p.pool.QueryRow(ctx, `SELECT count(*) FROM consent_outbox_events WHERE tenant_id=$1 AND published_at IS NOT NULL AND claim_token IS NOT NULL`, tenant).Scan(&tokenOnPublished); err != nil || tokenOnPublished != 0 {
		t.Fatalf("published rows must never hold a claim token: %d %v", tokenOnPublished, err)
	}
}

func TestIntegration_PostgresOutboxLeaseFencing(t *testing.T) {
	TestPostgresOutboxLeaseFencing(t)
}
