package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/service"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
	"github.com/jackc/pgx/v5/pgconn"
)

// transientPG reports YSQL serialization/deadlock aborts that callers are
// expected to retry under snapshot isolation.
func transientPG(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && (pe.Code == "40001" || pe.Code == "40P01")
}

func retryTransient[T any](t *testing.T, op func() (T, error)) (T, error) {
	t.Helper()
	for attempt := 0; ; attempt++ {
		value, err := op()
		if err == nil || !transientPG(err) || attempt >= 20 {
			return value, err
		}
		time.Sleep(time.Duration(attempt+1) * 5 * time.Millisecond)
	}
}

// assertStoredReceiptVerifies checks the T-004 hash after a real YSQL read and
// after a JSON export/import of the read value.
func assertStoredReceiptVerifies(t *testing.T, svc *service.Service, tenantID, receiptID string) {
	t.Helper()
	receipt, err := svc.Receipt(context.Background(), tenantID, receiptID)
	if err != nil {
		t.Fatal(err)
	}
	if v := service.VerifyReceipt(receipt); v.Status != service.ReceiptHashVerified || receipt.HashVersion != service.ReceiptHashVersionCanonical {
		t.Fatalf("receipt %s must verify after a YSQL read: %+v (issued_at=%s)", receiptID, v, receipt.IssuedAt.Format(time.RFC3339Nano))
	}
	exported, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	var imported domain.ConsentReceipt
	if err := json.Unmarshal(exported, &imported); err != nil {
		t.Fatal(err)
	}
	if v := service.VerifyReceipt(imported); v.Status != service.ReceiptHashVerified {
		t.Fatalf("receipt %s must verify after JSON export/import: %+v", receiptID, v)
	}
}

func TestPostgresPolicyTransactionsAndSingleUseConfirmation(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	repo, err := store.NewPostgres(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	svc := service.New(repo)
	tenantID := fmt.Sprintf("tenant-pg-%d", time.Now().UnixNano())
	now := time.Now().UTC()

	if _, err := svc.RegisterPurpose(context.Background(), tenantID, "admin", domain.Purpose{Code: "marketing", Category: "marketing", AllowedChannels: []string{"email"}, RequiresAffirmativeConsent: true, Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RegisterPurpose(context.Background(), tenantID, "admin", domain.Purpose{Code: "double_marketing", Category: "marketing", AllowedChannels: []string{"email"}, RequiresAffirmativeConsent: true, RequiresDoubleOptIn: true, Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RegisterLawfulBasis(context.Background(), tenantID, "admin", domain.LawfulBasis{Code: "consent", AllowedCategories: []string{"marketing"}, RequiresEvidence: true, Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PublishNotice(context.Background(), tenantID, "admin", domain.PrivacyNotice{Version: "2026-08", Jurisdiction: "NG", PublishedAt: now, Active: true}); err != nil {
		t.Fatal(err)
	}
	evidence := domain.Evidence{Method: "integration", ArtifactHash: "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", CollectedAt: now}
	consent, err := svc.RecordGoverned(context.Background(), tenantID, "admin", "cus-1", "marketing", "email", "consent", "granted", "portal", nil, nil, "NG", "2026-08", evidence)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := svc.Receipt(context.Background(), tenantID, consent.ReceiptID)
	if err != nil || receipt.ConsentID != consent.ID || receipt.ReceiptHash == "" {
		t.Fatalf("durable receipt mismatch: receipt=%+v err=%v", receipt, err)
	}
	if _, err := svc.SetSuppression(context.Background(), tenantID, "admin", domain.Suppression{SubjectID: "cus-1", Scope: domain.SuppressionPurposeChannel, Purpose: "marketing", Channel: "email", Reason: "integration", Active: true}); err != nil {
		t.Fatal(err)
	}
	decision, err := svc.Evaluate(context.Background(), tenantID, "cus-1", "marketing", "email", time.Time{})
	if err != nil || decision.Reason != "purpose_channel_suppression" {
		t.Fatalf("durable suppression mismatch: decision=%+v err=%v", decision, err)
	}

	challenge, err := svc.StartDoubleOptIn(context.Background(), tenantID, "admin", "cus-2", "double_marketing", "email", "portal", "NG", "2026-08", evidence, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var successes atomic.Int32
	var wait sync.WaitGroup
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if _, err := svc.ConfirmDoubleOptIn(context.Background(), tenantID, "admin", service.TenantWide(), challenge.ConfirmationToken); err == nil {
				successes.Add(1)
			}
		}()
	}
	wait.Wait()
	if successes.Load() != 1 {
		t.Fatalf("confirmation token succeeded %d times, want exactly one", successes.Load())
	}
	decision, err = svc.Evaluate(context.Background(), tenantID, "cus-2", "double_marketing", "email", time.Time{})
	if err != nil || !decision.Allowed || decision.ReceiptID == "" {
		t.Fatalf("confirmed consent unavailable: decision=%+v err=%v", decision, err)
	}

	// T-004: governed and double-opt-in receipts verify after a real read.
	assertStoredReceiptVerifies(t, svc, tenantID, consent.ReceiptID)
	assertStoredReceiptVerifies(t, svc, tenantID, decision.ReceiptID)

	// T-001: challenge -> later denial -> confirming the older challenge fails.
	stale, err := svc.StartDoubleOptIn(context.Background(), tenantID, "admin", "cus-3", "double_marketing", "email", "portal", "NG", "2026-08", evidence, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RecordGoverned(context.Background(), tenantID, "admin", "cus-3", "double_marketing", "email", "consent", "denied", "portal", nil, nil, "NG", "2026-08", evidence); err != nil {
		t.Fatal(err)
	}
	if confirmed, err := svc.ConfirmDoubleOptIn(context.Background(), tenantID, "admin", service.TenantWide(), stale.ConfirmationToken); err == nil {
		t.Fatalf("stale challenge overrode a later denial: %+v", confirmed)
	}
	if decision, err := svc.Evaluate(context.Background(), tenantID, "cus-3", "double_marketing", "email", time.Time{}); err != nil || decision.Allowed || decision.Reason != string(domain.StatusDenied) {
		t.Fatalf("later denial must remain effective: decision=%+v err=%v", decision, err)
	}

	// T-001: challenge1 -> challenge2 -> confirm2 -> withdraw -> confirm1 fails.
	older, err := svc.StartDoubleOptIn(context.Background(), tenantID, "admin", "cus-4", "double_marketing", "email", "portal", "NG", "2026-08", evidence, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	newer, err := svc.StartDoubleOptIn(context.Background(), tenantID, "admin", "cus-4", "double_marketing", "email", "portal", "NG", "2026-08", evidence, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	granted, err := svc.ConfirmDoubleOptIn(context.Background(), tenantID, "admin", service.TenantWide(), newer.ConfirmationToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Withdraw(context.Background(), tenantID, "admin", granted.ID); err != nil {
		t.Fatal(err)
	}
	if confirmed, err := svc.ConfirmDoubleOptIn(context.Background(), tenantID, "admin", service.TenantWide(), older.ConfirmationToken); err == nil {
		t.Fatalf("superseded challenge restored a withdrawn consent: %+v", confirmed)
	}
	if decision, err := svc.Evaluate(context.Background(), tenantID, "cus-4", "double_marketing", "email", time.Time{}); err != nil || decision.Allowed || decision.Reason != string(domain.StatusWithdrawn) {
		t.Fatalf("withdrawal must remain effective: decision=%+v err=%v", decision, err)
	}

	// T-001: concurrent confirm and later denial always converge to denied.
	for i := range 5 {
		subject := fmt.Sprintf("cus-race-%d", i)
		racing, err := svc.StartDoubleOptIn(context.Background(), tenantID, "admin", subject, "double_marketing", "email", "portal", "NG", "2026-08", evidence, now.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		var confirmed domain.Consent
		var confirmErr, denyErr error
		var race sync.WaitGroup
		race.Add(2)
		go func() {
			defer race.Done()
			confirmed, confirmErr = retryTransient(t, func() (domain.Consent, error) {
				return svc.ConfirmDoubleOptIn(context.Background(), tenantID, "admin", service.TenantWide(), racing.ConfirmationToken)
			})
		}()
		go func() {
			defer race.Done()
			_, denyErr = retryTransient(t, func() (domain.Consent, error) {
				return svc.RecordGoverned(context.Background(), tenantID, "admin", subject, "double_marketing", "email", "consent", "denied", "portal", nil, nil, "NG", "2026-08", evidence)
			})
		}()
		race.Wait()
		if denyErr != nil {
			t.Fatalf("%s: denial failed: %v", subject, denyErr)
		}
		if confirmErr == nil {
			assertStoredReceiptVerifies(t, svc, tenantID, confirmed.ReceiptID)
		} else if transientPG(confirmErr) {
			t.Fatalf("%s: confirmation kept aborting: %v", subject, confirmErr)
		}
		if decision, err := svc.Evaluate(context.Background(), tenantID, subject, "double_marketing", "email", time.Time{}); err != nil || decision.Allowed || decision.Reason != string(domain.StatusDenied) {
			t.Fatalf("%s: denial must win regardless of ordering: decision=%+v err=%v", subject, decision, err)
		}
	}
}
