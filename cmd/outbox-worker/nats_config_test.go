package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
	"github.com/nats-io/nats.go"
)

func natsURLForTest(t *testing.T) string {
	t.Helper()
	u := os.Getenv("TEST_NATS_URL")
	if u == "" {
		u = os.Getenv("NATS_URL")
	}
	if u == "" {
		if strings.EqualFold(os.Getenv("NVUTO_REQUIRE_INTEGRATION"), "true") {
			t.Fatal("TEST_NATS_URL (or NATS_URL) is required when NVUTO_REQUIRE_INTEGRATION=true")
		}
		t.Skip("TEST_NATS_URL is not configured")
	}
	return u
}

// TestAT020_MismatchedStream_MemoryStorage_RejectsStartup (AT-020) proves that
// an existing stream configured with MemoryStorage instead of FileStorage is
// rejected at startup with an actionable error and without mutating broker state.
func TestAT020_MismatchedStream_MemoryStorage_RejectsStartup(t *testing.T) {
	nc, err := nats.Connect(natsURLForTest(t))
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	js, err := nc.JetStream()
	if err != nil {
		t.Fatal(err)
	}

	stream := fmt.Sprintf("TEST_STREAM_MEM_%d", time.Now().UnixNano())
	prefix := fmt.Sprintf("nvuto.test.mem.%d", time.Now().UnixNano())
	expectedSubject := prefix + ".>"

	// Manually create mismatched stream with MemoryStorage
	_, err = js.AddStream(&nats.StreamConfig{
		Name:       stream,
		Subjects:   []string{expectedSubject},
		Storage:    nats.MemoryStorage, // MISMATCH: must be FileStorage
		Retention:  nats.LimitsPolicy,
		Duplicates: 10 * time.Minute,
		Replicas:   1,
	})
	if err != nil {
		t.Fatalf("failed to create test stream: %v", err)
	}
	defer js.DeleteStream(stream)

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	err = validateStreamContract(logger, js, stream, prefix, 1)
	if err == nil {
		t.Fatal("expected validateStreamContract to fail on MemoryStorage, got nil")
	}
	if !strings.Contains(err.Error(), "storage type mismatch") {
		t.Fatalf("expected 'storage type mismatch' error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "FileStorage required") {
		t.Fatalf("expected 'FileStorage required' in error, got: %v", err)
	}

	// Verify structured error log was emitted
	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "nats_stream_configuration_drift_detected") {
		t.Fatalf("expected structured drift log, got: %s", logOutput)
	}

	// Verify the stream in NATS was NOT mutated or silently updated
	info, err := js.StreamInfo(stream)
	if err != nil {
		t.Fatal(err)
	}
	if info.Config.Storage != nats.MemoryStorage {
		t.Fatalf("stream storage was silently mutated to %v; expected to remain MemoryStorage", info.Config.Storage)
	}
}

// TestAT020_MismatchedStream_ReplicaCount_RejectsStartup (AT-020) proves that
// an existing stream with fewer replicas than the required baseline (e.g. 1 vs 3)
// is rejected at startup.
func TestAT020_MismatchedStream_ReplicaCount_RejectsStartup(t *testing.T) {
	nc, err := nats.Connect(natsURLForTest(t))
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	js, err := nc.JetStream()
	if err != nil {
		t.Fatal(err)
	}

	stream := fmt.Sprintf("TEST_STREAM_REP_%d", time.Now().UnixNano())
	prefix := fmt.Sprintf("nvuto.test.rep.%d", time.Now().UnixNano())
	expectedSubject := prefix + ".>"

	// Create stream with 1 replica
	_, err = js.AddStream(&nats.StreamConfig{
		Name:       stream,
		Subjects:   []string{expectedSubject},
		Storage:    nats.FileStorage,
		Retention:  nats.LimitsPolicy,
		Duplicates: 10 * time.Minute,
		Replicas:   1, // 1 replica in cluster
	})
	if err != nil {
		t.Fatalf("failed to create test stream: %v", err)
	}
	defer js.DeleteStream(stream)

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Attempt to validate with production requirement of 3 replicas
	err = validateStreamContract(logger, js, stream, prefix, 3)
	if err == nil {
		t.Fatal("expected validateStreamContract to fail when stream has 1 replica but 3 are required")
	}
	if !strings.Contains(err.Error(), "replica count mismatch") {
		t.Fatalf("expected 'replica count mismatch' in error, got: %v", err)
	}
}

// TestAT020_MismatchedStream_DeduplicationWindow_RejectsStartup (AT-020) proves
// that a stream with an insufficient deduplication window (e.g. 5m instead of 10m)
// is rejected at startup.
func TestAT020_MismatchedStream_DeduplicationWindow_RejectsStartup(t *testing.T) {
	nc, err := nats.Connect(natsURLForTest(t))
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	js, err := nc.JetStream()
	if err != nil {
		t.Fatal(err)
	}

	stream := fmt.Sprintf("TEST_STREAM_DEDUP_%d", time.Now().UnixNano())
	prefix := fmt.Sprintf("nvuto.test.dedup.%d", time.Now().UnixNano())
	expectedSubject := prefix + ".>"

	// Create stream with 5-minute dedup window (MISMATCH: must be 10 minutes)
	_, err = js.AddStream(&nats.StreamConfig{
		Name:       stream,
		Subjects:   []string{expectedSubject},
		Storage:    nats.FileStorage,
		Retention:  nats.LimitsPolicy,
		Duplicates: 5 * time.Minute,
		Replicas:   1,
	})
	if err != nil {
		t.Fatalf("failed to create test stream: %v", err)
	}
	defer js.DeleteStream(stream)

	err = validateStreamContract(nil, js, stream, prefix, 1)
	if err == nil {
		t.Fatal("expected validateStreamContract to fail on dedup window mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "deduplication window mismatch") {
		t.Fatalf("expected 'deduplication window mismatch' in error, got: %v", err)
	}
}

// TestAT020_MismatchedStream_Subjects_RejectsStartup (AT-020) proves that
// a stream configured for a different subject namespace is rejected.
func TestAT020_MismatchedStream_Subjects_RejectsStartup(t *testing.T) {
	nc, err := nats.Connect(natsURLForTest(t))
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	js, err := nc.JetStream()
	if err != nil {
		t.Fatal(err)
	}

	stream := fmt.Sprintf("TEST_STREAM_SUB_%d", time.Now().UnixNano())
	prefix := fmt.Sprintf("nvuto.test.sub.%d", time.Now().UnixNano())

	// Create stream with wrong subjects
	_, err = js.AddStream(&nats.StreamConfig{
		Name:       stream,
		Subjects:   []string{"unrelated.other.namespace.>"},
		Storage:    nats.FileStorage,
		Retention:  nats.LimitsPolicy,
		Duplicates: 10 * time.Minute,
		Replicas:   1,
	})
	if err != nil {
		t.Fatalf("failed to create test stream: %v", err)
	}
	defer js.DeleteStream(stream)

	err = validateStreamContract(nil, js, stream, prefix, 1)
	if err == nil {
		t.Fatal("expected validateStreamContract to fail on subjects mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "subjects mismatch") {
		t.Fatalf("expected 'subjects mismatch' in error, got: %v", err)
	}
}

// TestAT020_MismatchedStream_RetentionPolicy_RejectsStartup (AT-020) proves that
// a stream configured with InterestPolicy or WorkQueuePolicy is rejected.
func TestAT020_MismatchedStream_RetentionPolicy_RejectsStartup(t *testing.T) {
	nc, err := nats.Connect(natsURLForTest(t))
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	js, err := nc.JetStream()
	if err != nil {
		t.Fatal(err)
	}

	stream := fmt.Sprintf("TEST_STREAM_RET_%d", time.Now().UnixNano())
	prefix := fmt.Sprintf("nvuto.test.ret.%d", time.Now().UnixNano())
	expectedSubject := prefix + ".>"

	// Create stream with InterestPolicy (MISMATCH: must be LimitsPolicy)
	_, err = js.AddStream(&nats.StreamConfig{
		Name:       stream,
		Subjects:   []string{expectedSubject},
		Storage:    nats.FileStorage,
		Retention:  nats.InterestPolicy,
		Duplicates: 10 * time.Minute,
		Replicas:   1,
	})
	if err != nil {
		t.Fatalf("failed to create test stream: %v", err)
	}
	defer js.DeleteStream(stream)

	err = validateStreamContract(nil, js, stream, prefix, 1)
	if err == nil {
		t.Fatal("expected validateStreamContract to fail on retention policy mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "retention policy mismatch") {
		t.Fatalf("expected 'retention policy mismatch' in error, got: %v", err)
	}
}

// TestAT020_ApprovedContract_SucceedsAndBindsWorker (AT-020) proves that a
// correctly configured stream passes validation and allows worker binding and publishing.
func TestAT020_ApprovedContract_SucceedsAndBindsWorker(t *testing.T) {
	nc, err := nats.Connect(natsURLForTest(t))
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	js, err := nc.JetStream()
	if err != nil {
		t.Fatal(err)
	}

	stream := fmt.Sprintf("TEST_STREAM_OK_%d", time.Now().UnixNano())
	prefix := fmt.Sprintf("nvuto.test.ok.%d", time.Now().UnixNano())
	expectedSubject := prefix + ".>"

	// Provision stream exactly matching approved contract
	_, err = js.AddStream(&nats.StreamConfig{
		Name:       stream,
		Subjects:   []string{expectedSubject},
		Storage:    nats.FileStorage,
		Retention:  nats.LimitsPolicy,
		Duplicates: 10 * time.Minute,
		Replicas:   1,
	})
	if err != nil {
		t.Fatalf("failed to create test stream: %v", err)
	}
	defer js.DeleteStream(stream)

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Must pass validation cleanly
	if err := validateStreamContract(logger, js, stream, prefix, 1); err != nil {
		t.Fatalf("expected validateStreamContract to succeed on approved contract, got: %v", err)
	}

	// Verify worker can bind and publish to this validated stream
	mem := store.NewMemory()
	w := newWorker(mem, func(ctx context.Context, item event) error {
		return publish(ctx, js, prefix, item)
	}, "test-worker-binding")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub, err := js.PullSubscribe(expectedSubject, "test-consumer", nats.BindStream(stream))
	if err != nil {
		t.Fatalf("failed to create consumer subscription: %v", err)
	}

	// Claim and publish one event
	testEvent := store.OutboxRecord{
		ID:          fmt.Sprintf("evt-bind-%d", time.Now().UnixNano()),
		TenantID:    "tenant-bind-test",
		EventType:   "consent.granted",
		AggregateID: "con-123",
		Payload:     []byte(`{"test":"binding"}`),
		OccurredAt:  time.Now().UTC(),
	}

	err = w.publish(ctx, testEvent)
	if err != nil {
		t.Fatalf("failed to publish to validated stream: %v", err)
	}

	msgs, err := sub.Fetch(1, nats.MaxWait(5*time.Second))
	if err != nil {
		t.Fatalf("failed to fetch message from validated stream: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	_ = msgs[0].Ack()

	if msgs[0].Header.Get(nats.MsgIdHdr) != testEvent.ID {
		t.Fatalf("expected Msg-Id %s, got %s", testEvent.ID, msgs[0].Header.Get(nats.MsgIdHdr))
	}
	if msgs[0].Header.Get("X-Nvuto-Tenant-ID") != "tenant-bind-test" {
		t.Fatalf("expected tenant header, got %s", msgs[0].Header.Get("X-Nvuto-Tenant-ID"))
	}
}

// TestIntegration_AT020_StreamContractIntegrationAliases matches the -run Integration filter.
func TestIntegration_AT020_StreamContractValidation(t *testing.T) {
	TestAT020_MismatchedStream_MemoryStorage_RejectsStartup(t)
	TestAT020_MismatchedStream_ReplicaCount_RejectsStartup(t)
	TestAT020_MismatchedStream_DeduplicationWindow_RejectsStartup(t)
	TestAT020_MismatchedStream_Subjects_RejectsStartup(t)
	TestAT020_MismatchedStream_RetentionPolicy_RejectsStartup(t)
	TestAT020_ApprovedContract_SucceedsAndBindsWorker(t)
}
