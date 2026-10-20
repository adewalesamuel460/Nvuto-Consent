package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
)

func insertOutboxRow(ctx context.Context, pool *pgxpool.Pool, id, tenantID, aggregateID string) error {
	_, err := pool.Exec(ctx, `INSERT INTO consent_outbox_events(id,schema_version,source,tenant_id,event_type,aggregate_type,aggregate_id,payload,occurred_at) VALUES($1,1,'nvuto/consent',$2,'consent.granted','consent',$3,'{}'::jsonb,now())`, id, tenantID, aggregateID)
	return err
}

func countPublished(ctx context.Context, pool *pgxpool.Pool, pattern string) (int, error) {
	var published int
	err := pool.QueryRow(ctx, `SELECT count(*) FROM consent_outbox_events WHERE id LIKE $1 AND published_at IS NOT NULL`, pattern).Scan(&published)
	return published, err
}

// requireEnv returns the named settings or skips; with
// NVUTO_REQUIRE_INTEGRATION=true a missing setting fails the test instead, so
// a required CI job cannot pass by silently skipping.
func requireEnv(t *testing.T, names ...string) []string {
	t.Helper()
	values := make([]string, len(names))
	for i, name := range names {
		values[i] = os.Getenv(name)
		if values[i] == "" {
			if strings.EqualFold(os.Getenv("NVUTO_REQUIRE_INTEGRATION"), "true") {
				t.Fatalf("%s is required when NVUTO_REQUIRE_INTEGRATION=true", name)
			}
			t.Skipf("%s is not configured", name)
		}
	}
	return values
}

func TestPostgresJetStreamOutboxConcurrency(t *testing.T) {
	env := requireEnv(t, "TEST_DATABASE_URL", "TEST_NATS_URL")
	ctx := context.Background()
	repo, err := store.NewPostgres(ctx, env[0])
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	pool, err := pgxpool.New(ctx, env[0])
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	connection, err := nats.Connect(env[1])
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	js, err := connection.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantID := "tenant-outbox-" + suffix
	stream := "NVUTO_CONSENT_TEST_" + suffix
	prefix := "nvuto.consent.test" + suffix
	if err := ensureStream(js, stream, prefix, 1); err != nil {
		t.Fatal(err)
	}
	defer js.DeleteStream(stream)
	subscription, err := js.PullSubscribe(prefix+".>", "test-consumer", nats.BindStream(stream))
	if err != nil {
		t.Fatal(err)
	}

	const total = 30
	for i := range total {
		id := fmt.Sprintf("evt-test-%s-%02d", suffix, i)
		if err := insertOutboxRow(ctx, pool, id, tenantID, fmt.Sprintf("con-%s-%02d", suffix, i)); err != nil {
			t.Fatal(err)
		}
	}
	var wait sync.WaitGroup
	claimed := make(chan store.OutboxClaim, 2)
	errorsChannel := make(chan error, 2)
	for worker := range 2 {
		wait.Add(1)
		go func(worker int) {
			defer wait.Done()
			claim, err := repo.ClaimOutbox(ctx, fmt.Sprintf("worker-%d", worker), 20, tenantID)
			if err != nil {
				errorsChannel <- err
				return
			}
			claimed <- claim
		}(worker)
	}
	wait.Wait()
	close(claimed)
	close(errorsChannel)
	for err := range errorsChannel {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	claims := make([]store.OutboxClaim, 0, 2)
	for claim := range claimed {
		claims = append(claims, claim)
		for _, item := range claim.Events {
			if seen[item.ID] {
				t.Fatalf("event %s was claimed by multiple workers", item.ID)
			}
			seen[item.ID] = true
		}
	}
	if len(seen) != total {
		t.Fatalf("claimed %d events, want %d", len(seen), total)
	}
	if len(claims) == 2 && claims[0].Token == claims[1].Token && len(claims[0].Events) > 0 && len(claims[1].Events) > 0 {
		t.Fatal("concurrent claims must carry distinct tokens")
	}
	for _, claim := range claims {
		other := ""
		for _, candidate := range claims {
			if candidate.Token != claim.Token {
				other = candidate.Token
			}
		}
		for _, item := range claim.Events {
			if other != "" {
				if err := repo.CompleteOutbox(ctx, item.ID, other); !errors.Is(err, store.ErrLeaseLost) {
					t.Fatalf("another claim's token must not complete %s: %v", item.ID, err)
				}
			}
			if err := publish(ctx, js, prefix, item); err != nil {
				t.Fatal(err)
			}
			if err := repo.CompleteOutbox(ctx, item.ID, claim.Token); err != nil {
				t.Fatal(err)
			}
		}
	}
	messages, err := subscription.Fetch(total, nats.MaxWait(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != total {
		t.Fatalf("received %d JetStream events, want %d", len(messages), total)
	}
	for _, message := range messages {
		if err := message.Ack(); err != nil {
			t.Fatal(err)
		}
	}
	published, err := countPublished(ctx, pool, "evt-test-"+suffix+"-%")
	if err != nil {
		t.Fatal(err)
	}
	if published != total {
		t.Fatalf("published %d rows, want %d", published, total)
	}

	// Ack/commit gap on real YSQL + JetStream: a stale worker published but
	// stalled past its lease and was re-claimed; the new owner re-publishes
	// under the same Msg-Id, JetStream drops the duplicate, and the stale
	// completion is refused. A dedicated store with a 1s lease lets the lease
	// expire in real database time without rewriting lock columns.
	shortLease, err := store.NewPostgres(ctx, env[0])
	if err != nil {
		t.Fatal(err)
	}
	defer shortLease.Close()
	shortLease.SetOutboxLease(time.Second)
	gapID := "evt-test-" + suffix + "-gap"
	if err := insertOutboxRow(ctx, pool, gapID, tenantID, "con-"+suffix+"-gap"); err != nil {
		t.Fatal(err)
	}
	stale, err := shortLease.ClaimOutbox(ctx, "worker-stale", 1, tenantID)
	if err != nil || len(stale.Events) != 1 {
		t.Fatalf("stale claim: %+v %v", stale, err)
	}
	if err := publish(ctx, js, prefix, stale.Events[0]); err != nil {
		t.Fatal(err)
	}
	if early, err := shortLease.ClaimOutbox(ctx, "worker-fresh", 1, tenantID); err != nil || len(early.Events) != 0 {
		t.Fatalf("a live lease must not be re-claimed: %+v %v", early, err)
	}
	time.Sleep(1500 * time.Millisecond)
	fresh, err := shortLease.ClaimOutbox(ctx, "worker-fresh", 1, tenantID)
	if err != nil || len(fresh.Events) != 1 || fresh.Token == stale.Token {
		t.Fatalf("expired lease must be re-claimed with a new token: %+v %v", fresh, err)
	}
	if err := publish(ctx, js, prefix, fresh.Events[0]); err != nil {
		t.Fatal(err)
	}
	for name, write := range map[string]func() error{
		"retry":    func() error { return shortLease.RetryOutbox(ctx, gapID, stale.Token, time.Hour) },
		"release":  func() error { return shortLease.ReleaseOutbox(ctx, gapID, stale.Token) },
		"complete": func() error { return shortLease.CompleteOutbox(ctx, gapID, stale.Token) },
	} {
		if err := write(); !errors.Is(err, store.ErrLeaseLost) {
			t.Fatalf("stale %s must be refused on YSQL, got %v", name, err)
		}
	}
	if err := shortLease.CompleteOutbox(ctx, gapID, fresh.Token); err != nil {
		t.Fatal(err)
	}
	replayed, err := subscription.Fetch(2, nats.MaxWait(2*time.Second))
	if err != nil && !errors.Is(err, nats.ErrTimeout) {
		t.Fatal(err)
	}
	if len(replayed) != 1 {
		t.Fatalf("JetStream must de-duplicate the replay by Msg-Id, received %d messages", len(replayed))
	}
	_ = replayed[0].Ack()
}

func TestPostgresJetStreamEnsureStreamDriftDetection(t *testing.T) {
	env := requireEnv(t, "TEST_NATS_URL")
	connection, err := nats.Connect(env[0])
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	js, err := connection.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	stream := "NVUTO_CONSENT_DRIFT_" + suffix
	prefix := "nvuto.consent.drift" + suffix

	// Initial creation should succeed.
	if err := ensureStream(js, stream, prefix, 1); err != nil {
		t.Fatalf("initial ensureStream failed: %v", err)
	}
	defer js.DeleteStream(stream)

	// Idempotent re-check with identical config should succeed.
	if err := ensureStream(js, stream, prefix, 1); err != nil {
		t.Fatalf("idempotent ensureStream failed: %v", err)
	}

	// Mismatched replica count must be detected and return an error.
	if err := ensureStream(js, stream, prefix, 3); err == nil {
		t.Fatal("ensureStream must return error when replica count drifts")
	} else if !strings.Contains(err.Error(), "replica count mismatch") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// Mismatched subject prefix must be detected and return an error.
	if err := ensureStream(js, stream, "other.prefix", 1); err == nil {
		t.Fatal("ensureStream must return error when subjects drift")
	} else if !strings.Contains(err.Error(), "subjects mismatch") {
		t.Fatalf("unexpected error message: %v", err)
	}
}
