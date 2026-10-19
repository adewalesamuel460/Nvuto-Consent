package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
	"github.com/nats-io/nats.go"
)

type event = store.OutboxRecord

type cloudEvent struct {
	SpecVersion   string          `json:"specversion"`
	ID            string          `json:"id"`
	Source        string          `json:"source"`
	Type          string          `json:"type"`
	Subject       string          `json:"subject"`
	Time          time.Time       `json:"time"`
	DataType      string          `json:"datacontenttype"`
	SchemaVersion int             `json:"nvutoschemaversion"`
	TenantID      string          `json:"nvutotenantid"`
	Data          json.RawMessage `json:"data"`
}

const (
	defaultBatchSize      = 50
	defaultPollInterval   = time.Second
	defaultPublishTimeout = 10 * time.Second
	// defaultShutdownGrace fits inside the Kubernetes default
	// terminationGracePeriodSeconds (30s) with headroom for process exit.
	defaultShutdownGrace = 25 * time.Second
	// leaseSafetyMargin is subtracted from the lease to absorb clock skew
	// between worker and database and the final SQL round trip. A batch never
	// starts a publish after lease-margin, so it never publishes on a lease
	// another worker may already own.
	leaseSafetyMargin = 30 * time.Second
	// jetStreamDuplicateWindow must exceed the lease: when a worker publishes
	// but dies before completing, the row is re-claimed after the lease and
	// re-published under the same Msg-Id, which JetStream then drops.
	jetStreamDuplicateWindow = 10 * time.Minute
	completionAttempts       = 3
)

// publishFunc publishes one event and returns once the broker acknowledged
// it or ctx ended.
type publishFunc func(ctx context.Context, item event) error

// worker drains the transactional outbox with fenced leases. Every write it
// makes after claiming presents the claim token, so a worker whose lease was
// re-claimed can no longer complete, reschedule or release the row.
type worker struct {
	leaser         store.OutboxLeaser
	publish        publishFunc
	id             string
	batchSize      int
	tenantFilter   string
	pollInterval   time.Duration
	publishTimeout time.Duration
	leaseMargin    time.Duration
	shutdownGrace  time.Duration
	now            func() time.Time
	logf           func(format string, args ...any)
}

func newWorker(leaser store.OutboxLeaser, publish publishFunc, id string) *worker {
	return &worker{
		leaser: leaser, publish: publish, id: id,
		batchSize: defaultBatchSize, pollInterval: defaultPollInterval,
		publishTimeout: defaultPublishTimeout, leaseMargin: leaseSafetyMargin,
		shutdownGrace: defaultShutdownGrace, now: time.Now, logf: log.Printf,
	}
}

// releaseReserve is the tail of the shutdown grace kept for releasing rows
// that were claimed but not published.
func (w *worker) releaseReserve() time.Duration {
	return min(5*time.Second, w.shutdownGrace/5)
}

// shutdownContexts derives the two contexts used for in-flight work. Until
// stop fires neither ends. After stop, soft ends at grace-reserve (no further
// publishing) and hard ends at grace (no further SQL), so the worker settles
// within the approved shutdown budget.
func (w *worker) shutdownContexts(stop context.Context) (soft, hard context.Context, cleanup func()) {
	hard, cancelHard := context.WithCancel(context.Background())
	soft, cancelSoft := context.WithCancel(hard)
	unwatch := context.AfterFunc(stop, func() {
		time.AfterFunc(w.shutdownGrace-w.releaseReserve(), cancelSoft)
		time.AfterFunc(w.shutdownGrace, cancelHard)
	})
	return soft, hard, func() {
		unwatch()
		cancelSoft()
		cancelHard()
	}
}

// run claims and publishes batches until stop ends. Once stop ends it claims
// nothing new; the batch in flight keeps publishing until the drain deadline,
// its unpublished rows are then released, and run returns.
func (w *worker) run(stop context.Context) {
	soft, hard, cleanup := w.shutdownContexts(stop)
	defer cleanup()
	wait := time.Duration(0)
	for {
		if wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-stop.Done():
				timer.Stop()
			case <-timer.C:
			}
		}
		if stop.Err() != nil {
			w.logf("outbox worker %s: stop requested, claiming no new batches", w.id)
			return
		}
		full := w.cycle(soft, hard)
		wait = w.pollInterval
		if full {
			wait = 0
		}
	}
}

// cycle claims one batch and settles it. It reports whether the batch was
// full, in which case more work is likely waiting.
func (w *worker) cycle(soft, hard context.Context) bool {
	started := w.now()
	claim, err := w.leaser.ClaimOutbox(hard, w.id, w.batchSize, w.tenantFilter)
	if err != nil {
		w.logf("outbox claim failed: %v", err)
		return false
	}
	w.process(soft, hard, claim, started)
	return len(claim.Events) >= w.batchSize
}

// process publishes the claimed rows in order. started is the worker clock
// before the claim, so started+lease is no later than the database lease
// expiry (up to clock skew, absorbed by leaseMargin).
func (w *worker) process(soft, hard context.Context, claim store.OutboxClaim, started time.Time) {
	budget := started.Add(claim.Lease - w.leaseMargin)
	for i, item := range claim.Events {
		if soft.Err() != nil {
			w.release(hard, claim.Token, claim.Events[i:], "shutdown")
			return
		}
		remaining := budget.Sub(w.now())
		if remaining <= 0 {
			w.release(hard, claim.Token, claim.Events[i:], "lease budget exhausted")
			return
		}
		limit := min(remaining, w.publishTimeout)
		publishCtx, cancel := context.WithTimeout(soft, limit)
		err := w.publish(publishCtx, item)
		budgetHit := errors.Is(publishCtx.Err(), context.DeadlineExceeded) && limit < w.publishTimeout
		cancel()
		if err == nil {
			w.complete(hard, claim.Token, item)
			continue
		}
		switch {
		case soft.Err() != nil:
			// The outcome of the interrupted publish is unknown. Releasing lets
			// the next owner publish again under the same Msg-Id.
			w.release(hard, claim.Token, claim.Events[i:], "shutdown during publish")
			return
		case budgetHit:
			w.release(hard, claim.Token, claim.Events[i:], "lease budget exhausted during publish")
			return
		}
		w.logf("publish %s failed: %v", item.ID, err)
		w.retry(hard, claim.Token, item)
	}
}

// complete records publication. A lost lease means a newer claim owns the
// row; the write is refused and the row is left to that owner. Other failures
// leave the row claimed: it is re-claimed after the lease and re-published,
// and JetStream drops the duplicate Msg-Id within jetStreamDuplicateWindow.
func (w *worker) complete(ctx context.Context, token string, item event) {
	for attempt := 1; ; attempt++ {
		err := w.leaser.CompleteOutbox(ctx, item.ID, token)
		switch {
		case err == nil:
			return
		case errors.Is(err, store.ErrLeaseLost):
			w.logf("complete %s refused: lease is owned by a newer claim", item.ID)
			return
		case attempt >= completionAttempts || ctx.Err() != nil:
			w.logf("complete %s failed, row will be re-claimed after lease expiry: %v", item.ID, err)
			return
		}
		select {
		case <-ctx.Done():
		case <-time.After(time.Duration(attempt) * 100 * time.Millisecond):
		}
	}
}

func (w *worker) retry(ctx context.Context, token string, item event) {
	err := w.leaser.RetryOutbox(ctx, item.ID, token, retryDelay(item.Attempts))
	switch {
	case errors.Is(err, store.ErrLeaseLost):
		w.logf("retry scheduling %s refused: lease is owned by a newer claim", item.ID)
	case err != nil:
		w.logf("retry scheduling %s failed, row will be re-claimed after lease expiry: %v", item.ID, err)
	}
}

// release unlocks rows this claim will not publish so they can be claimed
// at once. Rows not released before ctx ends stay claimed until the lease
// expires, which is equally safe.
func (w *worker) release(ctx context.Context, token string, items []event, reason string) {
	released := 0
	for i, item := range items {
		err := w.leaser.ReleaseOutbox(ctx, item.ID, token)
		if err == nil {
			released++
			continue
		}
		if ctx.Err() != nil {
			w.logf("release deadline reached (%s): %d outbox rows left to lease expiry", reason, len(items)-i)
			break
		}
		if !errors.Is(err, store.ErrLeaseLost) {
			w.logf("release %s failed: %v", item.ID, err)
		}
	}
	w.logf("released %d of %d claimed outbox rows (%s)", released, len(items), reason)
}

// retryDelay is the bounded exponential backoff after a failed publish.
func retryDelay(previousAttempts int) time.Duration {
	attempt := previousAttempts + 1
	seconds := 1 << min(attempt, 10)
	if seconds > 900 {
		seconds = 900
	}
	return time.Duration(seconds) * time.Second
}

func subject(prefix string, item event) string {
	prefix = strings.TrimSuffix(strings.TrimSpace(prefix), ".")
	return prefix + "." + strings.TrimPrefix(item.EventType, "consent.")
}

func envelope(item event) ([]byte, error) {
	return json.Marshal(cloudEvent{
		SpecVersion:   "1.0",
		ID:            item.ID,
		Source:        item.Source,
		Type:          item.EventType,
		Subject:       item.AggregateType + "/" + item.AggregateID,
		Time:          item.OccurredAt,
		DataType:      "application/json",
		SchemaVersion: item.SchemaVersion,
		TenantID:      item.TenantID,
		Data:          item.Payload,
	})
}

// publish sends one CloudEvent and waits for the JetStream acknowledgement,
// bounded by ctx.
func publish(ctx context.Context, js nats.JetStreamContext, prefix string, item event) error {
	body, err := envelope(item)
	if err != nil {
		return err
	}
	message := nats.NewMsg(subject(prefix, item))
	message.Header.Set(nats.MsgIdHdr, item.ID)
	message.Header.Set("Content-Type", "application/cloudevents+json")
	message.Header.Set("X-Nvuto-Tenant-ID", item.TenantID)
	message.Data = body
	_, err = js.PublishMsg(message, nats.Context(ctx))
	return err
}

func ensureStream(js nats.JetStreamContext, stream, prefix string, replicas int) error {
	_, err := js.StreamInfo(stream)
	if err == nil {
		return nil
	}
	if !errors.Is(err, nats.ErrStreamNotFound) {
		return err
	}
	_, err = js.AddStream(&nats.StreamConfig{Name: stream, Subjects: []string{strings.TrimSuffix(prefix, ".") + ".>"}, Storage: nats.FileStorage, Retention: nats.LimitsPolicy, Duplicates: jetStreamDuplicateWindow, Replicas: replicas})
	return err
}

func durationEnv(name string, fallback, lowest, highest time.Duration) (time.Duration, error) {
	configured := strings.TrimSpace(os.Getenv(name))
	if configured == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(configured)
	if err != nil || parsed < lowest || parsed > highest {
		return 0, fmt.Errorf("%s must be a duration between %s and %s", name, lowest, highest)
	}
	return parsed, nil
}

func main() {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	natsURL := strings.TrimSpace(os.Getenv("NATS_URL"))
	if databaseURL == "" || natsURL == "" {
		log.Fatal("DATABASE_URL and NATS_URL are required")
	}
	stream := strings.TrimSpace(os.Getenv("NATS_STREAM"))
	if stream == "" {
		stream = "NVUTO_CONSENT"
	}
	prefix := strings.TrimSpace(os.Getenv("NATS_SUBJECT_PREFIX"))
	if prefix == "" {
		prefix = "nvuto.consent"
	}
	replicas := 3
	if configured := strings.TrimSpace(os.Getenv("NATS_REPLICAS")); configured != "" {
		parsed, err := strconv.Atoi(configured)
		if err != nil || parsed < 1 || parsed > 5 {
			log.Fatal("NATS_REPLICAS must be between 1 and 5")
		}
		replicas = parsed
	}
	// The grace must stay below the pod's terminationGracePeriodSeconds.
	shutdownGrace, err := durationEnv("OUTBOX_SHUTDOWN_GRACE", defaultShutdownGrace, time.Second, 5*time.Minute)
	if err != nil {
		log.Fatal(err)
	}
	publishTimeout, err := durationEnv("OUTBOX_PUBLISH_TIMEOUT", defaultPublishTimeout, time.Second, time.Minute)
	if err != nil {
		log.Fatal(err)
	}
	workerID, _ := os.Hostname()
	if workerID == "" {
		workerID = "consent-outbox-worker"
	}
	stop, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	connectCtx, cancelConnect := context.WithTimeout(stop, 30*time.Second)
	repo, err := store.NewPostgres(connectCtx, databaseURL)
	cancelConnect()
	if err != nil {
		log.Fatal(err)
	}
	defer repo.Close()
	natsOptions := []nats.Option{nats.Name(workerID), nats.Timeout(10 * time.Second), nats.MaxReconnects(-1)}
	if credentialsFile := strings.TrimSpace(os.Getenv("NATS_CREDS_FILE")); credentialsFile != "" {
		natsOptions = append(natsOptions, nats.UserCredentials(credentialsFile))
	} else if !strings.EqualFold(os.Getenv("ALLOW_INSECURE_NATS"), "true") {
		log.Fatal("NATS_CREDS_FILE is required unless ALLOW_INSECURE_NATS=true")
	}
	if rootCA := strings.TrimSpace(os.Getenv("NATS_TLS_CA_FILE")); rootCA != "" {
		natsOptions = append(natsOptions, nats.RootCAs(rootCA))
	}
	connection, err := nats.Connect(natsURL, natsOptions...)
	if err != nil {
		log.Fatal(err)
	}
	defer connection.Close()
	js, err := connection.JetStream(nats.PublishAsyncMaxPending(256))
	if err != nil {
		log.Fatal(err)
	}
	if err := ensureStream(js, stream, prefix, replicas); err != nil {
		log.Fatal(err)
	}
	w := newWorker(repo, func(ctx context.Context, item event) error { return publish(ctx, js, prefix, item) }, workerID)
	w.shutdownGrace, w.publishTimeout = shutdownGrace, publishTimeout
	log.Printf("consent outbox worker %s started (lease %s, shutdown grace %s)", workerID, store.OutboxLeaseDuration, shutdownGrace)
	w.run(stop)
	log.Printf("consent outbox worker %s stopped", workerID)
}
