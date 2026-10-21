package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/auth"
	"github.com/Atlasfsp/Nvuto-Consent/internal/observability"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
)

type event = store.OutboxRecord

type cloudEvent struct {
	SpecVersion       string          `json:"specversion"`
	ID                string          `json:"id"`
	Source            string          `json:"source"`
	Type              string          `json:"type"`
	Subject           string          `json:"subject"`
	Time              time.Time       `json:"time"`
	DataType          string          `json:"datacontenttype"`
	SchemaVersion     int             `json:"nvutoschemaversion"`
	TenantID          string          `json:"nvutotenantid"`
	AggregateType     string          `json:"nvutoaggregatetype,omitempty"`
	AggregateRevision int64           `json:"nvutoaggregaterevision,omitempty"`
	Data              json.RawMessage `json:"data"`
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
	defaultMaxAttempts       = 5
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
	maxAttempts    int
	tenantFilter   string
	pollInterval   time.Duration
	publishTimeout time.Duration
	leaseMargin    time.Duration
	shutdownGrace  time.Duration
	now            func() time.Time
	logger         *slog.Logger
	logf           func(format string, args ...any)
}

func newWorker(leaser store.OutboxLeaser, publish publishFunc, id string) *worker {
	return &worker{
		leaser: leaser, publish: publish, id: id,
		batchSize: defaultBatchSize, maxAttempts: defaultMaxAttempts,
		pollInterval:   defaultPollInterval,
		publishTimeout: defaultPublishTimeout, leaseMargin: leaseSafetyMargin,
		shutdownGrace: defaultShutdownGrace, now: time.Now,
		logger: slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})),
		logf:   nil,
	}
}

func (w *worker) logEvent(level slog.Level, msg string, attrs ...slog.Attr) {
	if w.logger != nil {
		w.logger.LogAttrs(context.Background(), level, msg, attrs...)
	}
}

func (w *worker) emitLogf(format string, args ...any) {
	if w.logf != nil {
		w.logf(format, args...)
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
			w.logEvent(slog.LevelInfo, "stop_requested", slog.String("worker_id", w.id))
			w.emitLogf("outbox worker %s: stop requested, claiming no new batches", w.id)
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
		w.logEvent(slog.LevelError, "claim_failed", slog.String("worker_id", w.id), slog.String("error", err.Error()))
		w.emitLogf("outbox claim failed: %v", err)
		return false
	}
	if len(claim.Events) > 0 {
		w.logEvent(slog.LevelInfo, "claim_succeeded", slog.String("worker_id", w.id), slog.Int("claimed_count", len(claim.Events)), slog.Duration("lease", claim.Lease))
	}
	processed := w.process(soft, hard, claim, started)
	if lag, err := w.leaser.OutboxLag(hard, w.tenantFilter); err == nil {
		observability.DefaultMetrics.SetOutboxLag(w.tenantFilter, lag.Seconds())
	}
	if count, err := w.leaser.QuarantinedOutboxCount(hard, w.tenantFilter); err == nil {
		observability.DefaultMetrics.SetOutboxPendingCount(w.tenantFilter, float64(count))
	}
	if len(claim.Events) > 0 {
		w.logEvent(slog.LevelInfo, "batch_completed", slog.String("worker_id", w.id), slog.Int("claimed_count", len(claim.Events)), slog.Int("processed_count", processed), slog.Duration("duration", w.now().Sub(started)))
	}
	return len(claim.Events) >= w.batchSize
}

// process publishes the claimed rows in order. started is the worker clock
// before the claim, so started+lease is no later than the database lease
// expiry (up to clock skew, absorbed by leaseMargin).
func (w *worker) process(soft, hard context.Context, claim store.OutboxClaim, started time.Time) int {
	budget := started.Add(claim.Lease - w.leaseMargin)
	processed := 0
	for i, item := range claim.Events {
		if soft.Err() != nil {
			w.release(hard, claim.Token, claim.Events[i:], "shutdown")
			return processed
		}
		remaining := budget.Sub(w.now())
		if remaining <= 0 {
			w.release(hard, claim.Token, claim.Events[i:], "lease budget exhausted")
			return processed
		}
		limit := min(remaining, w.publishTimeout)
		publishCtx, cancel := context.WithTimeout(soft, limit)
		observability.DefaultMetrics.RecordOutboxPublishAttempt(item.TenantID, item.EventType)
		err := w.publish(publishCtx, item)
		budgetHit := errors.Is(publishCtx.Err(), context.DeadlineExceeded) && limit < w.publishTimeout
		cancel()
		if err == nil {
			observability.DefaultMetrics.RecordOutboxPublishSuccess(item.TenantID, item.EventType)
			w.logEvent(slog.LevelInfo, "publish_succeeded", slog.String("worker_id", w.id), slog.String("event_id", item.ID), slog.String("tenant_id", item.TenantID), slog.String("event_type", item.EventType), slog.String("aggregate_id", item.AggregateID))
			w.complete(hard, claim.Token, item)
			processed++
			continue
		}
		switch {
		case soft.Err() != nil:
			// The outcome of the interrupted publish is unknown. Releasing lets
			// the next owner publish again under the same Msg-Id.
			w.release(hard, claim.Token, claim.Events[i:], "shutdown during publish")
			return processed
		case budgetHit:
			w.release(hard, claim.Token, claim.Events[i:], "lease budget exhausted during publish")
			return processed
		}
		w.logEvent(slog.LevelWarn, "publish_failed", slog.String("worker_id", w.id), slog.String("event_id", item.ID), slog.String("tenant_id", item.TenantID), slog.String("event_type", item.EventType), slog.Int("attempts", item.Attempts+1), slog.String("error", err.Error()))
		w.emitLogf("publish %s failed: %v", item.ID, err)
		observability.DefaultMetrics.RecordOutboxPublishFailure(item.TenantID, item.EventType, "publish_error")
		if item.Attempts+1 >= w.maxAttempts {
			w.quarantine(hard, claim.Token, item, err.Error())
		} else {
			w.retry(hard, claim.Token, item)
		}
		processed++
	}
	return processed
}

func (w *worker) quarantine(ctx context.Context, token string, item event, reason string) {
	err := w.leaser.QuarantineOutbox(ctx, item.ID, token, reason)
	switch {
	case errors.Is(err, store.ErrLeaseLost) || errors.Is(err, store.ErrOutboxClaimLost):
		w.logEvent(slog.LevelWarn, "quarantine_refused", slog.String("worker_id", w.id), slog.String("event_id", item.ID), slog.String("tenant_id", item.TenantID), slog.String("reason", "lease_lost"))
		w.emitLogf("quarantine %s refused: lease is owned by a newer claim", item.ID)
	case err != nil:
		w.logEvent(slog.LevelError, "quarantine_failed", slog.String("worker_id", w.id), slog.String("event_id", item.ID), slog.String("tenant_id", item.TenantID), slog.String("error", err.Error()))
		w.emitLogf("quarantine %s failed: %v", item.ID, err)
	default:
		w.logEvent(slog.LevelError, "event_quarantined", slog.String("worker_id", w.id), slog.String("event_id", item.ID), slog.String("tenant_id", item.TenantID), slog.String("event_type", item.EventType), slog.Int("attempts", item.Attempts+1), slog.String("reason", reason))
		w.emitLogf("event %s quarantined after %d attempts: %s", item.ID, item.Attempts+1, reason)
		observability.DefaultMetrics.RecordOutboxQuarantined(item.TenantID, item.EventType)
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
			w.logEvent(slog.LevelInfo, "complete_succeeded", slog.String("worker_id", w.id), slog.String("event_id", item.ID), slog.String("tenant_id", item.TenantID), slog.Int("attempt", attempt))
			return
		case errors.Is(err, store.ErrLeaseLost) || errors.Is(err, store.ErrOutboxClaimLost):
			w.logEvent(slog.LevelWarn, "complete_refused", slog.String("worker_id", w.id), slog.String("event_id", item.ID), slog.String("tenant_id", item.TenantID), slog.String("reason", "lease_lost"))
			w.emitLogf("complete %s refused: lease is owned by a newer claim", item.ID)
			return
		case attempt >= completionAttempts || ctx.Err() != nil:
			w.logEvent(slog.LevelError, "complete_failed", slog.String("worker_id", w.id), slog.String("event_id", item.ID), slog.String("tenant_id", item.TenantID), slog.Int("attempt", attempt), slog.String("error", err.Error()))
			w.emitLogf("complete %s failed, row will be re-claimed after lease expiry: %v", item.ID, err)
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
	case errors.Is(err, store.ErrLeaseLost) || errors.Is(err, store.ErrOutboxClaimLost):
		w.logEvent(slog.LevelWarn, "retry_refused", slog.String("worker_id", w.id), slog.String("event_id", item.ID), slog.String("tenant_id", item.TenantID), slog.String("reason", "lease_lost"))
		w.emitLogf("retry scheduling %s refused: lease is owned by a newer claim", item.ID)
	case err != nil:
		w.logEvent(slog.LevelError, "retry_failed", slog.String("worker_id", w.id), slog.String("event_id", item.ID), slog.String("tenant_id", item.TenantID), slog.String("error", err.Error()))
		w.emitLogf("retry scheduling %s failed, row will be re-claimed after lease expiry: %v", item.ID, err)
	default:
		w.logEvent(slog.LevelInfo, "retry_scheduled", slog.String("worker_id", w.id), slog.String("event_id", item.ID), slog.String("tenant_id", item.TenantID), slog.Int("attempts", item.Attempts+1))
	}
}

// release unlocks rows this claim will not publish so they can be claimed
// at once. Rows not released before ctx ends stay claimed until the lease
// expires, which is equally safe.
func (w *worker) release(ctx context.Context, token string, items []event, reason string) {
	if len(items) == 0 {
		return
	}
	if err := w.leaser.ReleaseOutboxBatch(ctx, w.id, items); err == nil {
		w.logEvent(slog.LevelInfo, "release_completed", slog.String("worker_id", w.id), slog.Int("released_count", len(items)), slog.Int("total_items", len(items)), slog.String("reason", reason))
		w.emitLogf("released %d of %d claimed outbox rows (%s)", len(items), len(items), reason)
		return
	}
	released := 0
	for i, item := range items {
		err := w.leaser.ReleaseOutbox(ctx, item.ID, token)
		if err == nil {
			released++
			continue
		}
		if ctx.Err() != nil {
			w.logEvent(slog.LevelWarn, "release_deadline_reached", slog.String("worker_id", w.id), slog.String("reason", reason), slog.Int("remaining_count", len(items)-i))
			w.emitLogf("release deadline reached (%s): %d outbox rows left to lease expiry", reason, len(items)-i)
			break
		}
		if !errors.Is(err, store.ErrLeaseLost) && !errors.Is(err, store.ErrOutboxClaimLost) {
			w.logEvent(slog.LevelWarn, "release_failed", slog.String("worker_id", w.id), slog.String("event_id", item.ID), slog.String("error", err.Error()))
			w.emitLogf("release %s failed: %v", item.ID, err)
		}
	}
	w.logEvent(slog.LevelInfo, "release_completed", slog.String("worker_id", w.id), slog.Int("released_count", released), slog.Int("total_items", len(items)), slog.String("reason", reason))
	w.emitLogf("released %d of %d claimed outbox rows (%s)", released, len(items), reason)
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
		SpecVersion:       "1.0",
		ID:                item.ID,
		Source:            item.Source,
		Type:              item.EventType,
		Subject:           item.AggregateType + "/" + item.AggregateID,
		Time:              item.OccurredAt,
		DataType:          "application/json",
		SchemaVersion:     item.SchemaVersion,
		TenantID:          item.TenantID,
		AggregateType:     item.AggregateType,
		AggregateRevision: item.AggregateRevision,
		Data:              item.Payload,
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
	message.Header.Set("X-Nvuto-Aggregate-Type", item.AggregateType)
	message.Header.Set("X-Nvuto-Aggregate-Revision", strconv.FormatInt(item.AggregateRevision, 10))
	message.Data = body
	_, err = js.PublishMsg(message, nats.Context(ctx))
	return err
}

// verifyStreamConfig verifies that an existing JetStream stream matches the
// required configuration (subjects, storage type, replica count, and deduplication
// window) to detect configuration drift (T-020).
func verifyStreamConfig(cfg nats.StreamConfig, stream, expectedSubject string, replicas int) error {
	expectedSubjects := []string{expectedSubject}
	if len(cfg.Subjects) != len(expectedSubjects) || cfg.Subjects[0] != expectedSubjects[0] {
		return fmt.Errorf("stream %q subjects mismatch: got %v, want %v", stream, cfg.Subjects, expectedSubjects)
	}
	if cfg.Storage != nats.FileStorage {
		return fmt.Errorf("stream %q storage type mismatch: got %v, want %v", stream, cfg.Storage, nats.FileStorage)
	}
	if cfg.Replicas != replicas {
		return fmt.Errorf("stream %q replica count mismatch: got %d, want %d", stream, cfg.Replicas, replicas)
	}
	if cfg.Duplicates != jetStreamDuplicateWindow {
		return fmt.Errorf("stream %q deduplication window mismatch: got %v, want %v", stream, cfg.Duplicates, jetStreamDuplicateWindow)
	}
	return nil
}

func ensureStream(js nats.JetStreamContext, stream, prefix string, replicas int) error {
	expectedSubject := strings.TrimSuffix(prefix, ".") + ".>"
	info, err := js.StreamInfo(stream)
	if err == nil {
		return verifyStreamConfig(info.Config, stream, expectedSubject, replicas)
	}
	if !errors.Is(err, nats.ErrStreamNotFound) {
		return err
	}
	_, err = js.AddStream(&nats.StreamConfig{
		Name:       stream,
		Subjects:   []string{expectedSubject},
		Storage:    nats.FileStorage,
		Retention:  nats.LimitsPolicy,
		Duplicates: jetStreamDuplicateWindow,
		Replicas:   replicas,
	})
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

// workerHealthHandler provides operational liveness and readiness probe endpoints (T-015).
type workerHealthHandler struct {
	repo        store.Repository
	nc          *nats.Conn
	js          nats.JetStreamContext
	stream      string
	customReady func(ctx context.Context) error
}

func (h *workerHealthHandler) checkReadiness(ctx context.Context) error {
	if h.customReady != nil {
		return h.customReady(ctx)
	}
	if h.repo != nil {
		if err := h.repo.Ping(ctx); err != nil {
			return fmt.Errorf("database unreachable: %w", err)
		}
		if err := h.repo.VerifySchema(ctx); err != nil {
			return fmt.Errorf("database schema unverified: %w", err)
		}
	}
	if h.nc != nil && h.nc.Status() != nats.CONNECTED {
		return fmt.Errorf("nats disconnected (status=%v)", h.nc.Status())
	}
	if h.js != nil && h.stream != "" {
		if _, err := h.js.StreamInfo(h.stream); err != nil {
			return fmt.Errorf("jetstream stream %s check failed: %w", h.stream, err)
		}
	}
	return nil
}

func (h *workerHealthHandler) healthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *workerHealthHandler) readyz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := h.checkReadiness(r.Context()); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "not_ready", "reason": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

func newWorkerHealthMux(h *workerHealthHandler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.healthz)
	mux.HandleFunc("/readyz", h.readyz)
	mux.Handle("/metrics", observability.Handler())
	return mux
}

func newWorkerHealthServer(repo store.Repository, nc *nats.Conn, js nats.JetStreamContext, stream string) http.Handler {
	return newWorkerHealthMux(&workerHealthHandler{repo: repo, nc: nc, js: js, stream: stream})
}

func validateDatabaseTLS(databaseURL string, allowDev, isProduction bool) error {
	u, err := url.Parse(databaseURL)
	var sslmode string
	if err == nil && u != nil {
		sslmode = strings.ToLower(strings.TrimSpace(u.Query().Get("sslmode")))
	}
	if sslmode == "" {
		for _, part := range strings.Fields(databaseURL) {
			if strings.HasPrefix(strings.ToLower(part), "sslmode=") {
				sslmode = strings.ToLower(strings.TrimPrefix(part, "sslmode="))
				break
			}
		}
	}
	if sslmode == "disable" && !allowDev {
		return errors.New("sslmode=disable is prohibited in production configuration (requires ALLOW_DEV_HEADERS=true)")
	}
	if isProduction && sslmode != "verify-full" && sslmode != "require" && sslmode != "verify-ca" {
		return errors.New("DATABASE_URL requires sslmode=verify-full or sslmode=require in production")
	}
	return nil
}

type natsConfig struct {
	URL           string
	CredsFile     string
	NkeyFile      string
	ClientCert    string
	ClientKey     string
	RootCA        string
	AllowInsecure bool
	WorkerID      string
}

func configureNATS(cfg natsConfig) ([]nats.Option, error) {
	if cfg.URL == "" {
		return nil, errors.New("NATS_URL is required")
	}
	if !cfg.AllowInsecure && !strings.HasPrefix(cfg.URL, "tls://") {
		return nil, errors.New("NATS_URL must use tls:// scheme unless ALLOW_INSECURE_NATS=true")
	}
	opts := []nats.Option{
		nats.Name(cfg.WorkerID),
		nats.Timeout(10 * time.Second),
		nats.MaxReconnects(-1),
	}
	if strings.HasPrefix(cfg.URL, "tls://") {
		opts = append(opts, nats.Secure())
	}
	hasAuth := false
	if cfg.CredsFile != "" {
		opts = append(opts, nats.UserCredentials(cfg.CredsFile))
		hasAuth = true
	}
	if cfg.NkeyFile != "" {
		nkeyOpt, err := nats.NkeyOptionFromSeed(cfg.NkeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load NKEY seed file: %w", err)
		}
		opts = append(opts, nkeyOpt)
		hasAuth = true
	}
	if cfg.ClientCert != "" || cfg.ClientKey != "" {
		if cfg.ClientCert == "" || cfg.ClientKey == "" {
			return nil, errors.New("both NATS_CLIENT_CERT and NATS_CLIENT_KEY must be provided for mTLS")
		}
		opts = append(opts, nats.ClientCert(cfg.ClientCert, cfg.ClientKey))
		hasAuth = true
	}
	if !hasAuth && !cfg.AllowInsecure {
		return nil, errors.New("NATS authentication credentials (creds, nkey, or client cert/key) are required unless ALLOW_INSECURE_NATS=true")
	}
	if cfg.RootCA != "" {
		opts = append(opts, nats.RootCAs(cfg.RootCA))
	}
	return opts, nil
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	fatal := func(msg string, err error) {
		if err != nil {
			logger.Error(msg, slog.String("error", err.Error()))
		} else {
			logger.Error(msg)
		}
		os.Exit(1)
	}

	if err := auth.ValidateConfig(); err != nil {
		fatal("auth configuration invalid", err)
	}

	allowDev := strings.EqualFold(os.Getenv("ALLOW_DEV_HEADERS"), "true")
	env := strings.ToLower(strings.TrimSpace(os.Getenv("ENVIRONMENT")))
	if env == "" {
		env = strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	}
	if env == "" {
		env = strings.ToLower(strings.TrimSpace(os.Getenv("NVUTO_ENV")))
	}
	isProduction := env == "production" || env == "prod"

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	natsURL := strings.TrimSpace(os.Getenv("NATS_URL"))
	if databaseURL == "" || natsURL == "" {
		fatal("DATABASE_URL and NATS_URL are required", nil)
	}
	if _, err := pgxpool.ParseConfig(databaseURL); err != nil {
		fatal("malformed DATABASE_URL", err)
	}
	if err := validateDatabaseTLS(databaseURL, allowDev, isProduction); err != nil {
		fatal("database security configuration invalid", err)
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
			fatal("NATS_REPLICAS must be between 1 and 5", err)
		}
		replicas = parsed
	}
	// The grace must stay below the pod's terminationGracePeriodSeconds.
	shutdownGrace, err := durationEnv("OUTBOX_SHUTDOWN_GRACE", defaultShutdownGrace, time.Second, 5*time.Minute)
	if err != nil {
		fatal("invalid OUTBOX_SHUTDOWN_GRACE", err)
	}
	publishTimeout, err := durationEnv("OUTBOX_PUBLISH_TIMEOUT", defaultPublishTimeout, time.Second, time.Minute)
	if err != nil {
		fatal("invalid OUTBOX_PUBLISH_TIMEOUT", err)
	}
	workerID, _ := os.Hostname()
	if workerID == "" {
		workerID = "consent-outbox-worker"
	}
	natsOpts, err := configureNATS(natsConfig{
		URL:           natsURL,
		CredsFile:     strings.TrimSpace(os.Getenv("NATS_CREDS_FILE")),
		NkeyFile:      strings.TrimSpace(os.Getenv("NATS_NKEY_FILE")),
		ClientCert:    strings.TrimSpace(os.Getenv("NATS_CLIENT_CERT")),
		ClientKey:     strings.TrimSpace(os.Getenv("NATS_CLIENT_KEY")),
		RootCA:        strings.TrimSpace(os.Getenv("NATS_TLS_CA_FILE")),
		AllowInsecure: strings.EqualFold(os.Getenv("ALLOW_INSECURE_NATS"), "true"),
		WorkerID:      workerID,
	})
	if err != nil {
		fatal("nats security configuration invalid", err)
	}

	stop, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	connectCtx, cancelConnect := context.WithTimeout(stop, 30*time.Second)
	repo, err := store.NewPostgres(connectCtx, databaseURL)
	cancelConnect()
	if err != nil {
		fatal("postgres connection failed", err)
	}
	defer repo.Close()

	connection, err := nats.Connect(natsURL, natsOpts...)
	if err != nil {
		fatal("nats connection failed", err)
	}
	defer connection.Close()
	js, err := connection.JetStream(nats.PublishAsyncMaxPending(256))
	if err != nil {
		fatal("jetstream context failed", err)
	}
	if err := ensureStream(js, stream, prefix, replicas); err != nil {
		fatal("ensure stream failed", err)
	}

	healthPort := strings.TrimSpace(os.Getenv("HEALTH_PORT"))
	if healthPort == "" {
		healthPort = strings.TrimSpace(os.Getenv("PORT"))
	}
	if healthPort == "" {
		healthPort = "8081"
	}
	if !strings.HasPrefix(healthPort, ":") {
		healthPort = ":" + healthPort
	}
	healthHandler := newWorkerHealthServer(repo, connection, js, stream)
	healthServer := &http.Server{
		Addr:              healthPort,
		Handler:           healthHandler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := healthServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("health_server_failed", slog.String("error", err.Error()))
		}
	}()
	defer func() {
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelShutdown()
		_ = healthServer.Shutdown(shutdownCtx)
	}()

	maxAttempts := defaultMaxAttempts
	if configured := strings.TrimSpace(os.Getenv("OUTBOX_MAX_ATTEMPTS")); configured != "" {
		if parsed, err := strconv.Atoi(configured); err == nil && parsed > 0 {
			maxAttempts = parsed
		}
	}

	w := newWorker(repo, func(ctx context.Context, item event) error { return publish(ctx, js, prefix, item) }, workerID)
	w.logger = logger
	w.maxAttempts = maxAttempts
	w.shutdownGrace, w.publishTimeout = shutdownGrace, publishTimeout
	logger.Info("outbox_worker_started",
		slog.String("worker_id", workerID),
		slog.Duration("lease", store.OutboxLeaseDuration),
		slog.Duration("shutdown_grace", shutdownGrace),
		slog.String("stream", stream),
		slog.String("subject_prefix", prefix),
		slog.Int("replicas", replicas),
	)
	w.run(stop)
	logger.Info("outbox_worker_stopped", slog.String("worker_id", workerID))
}
