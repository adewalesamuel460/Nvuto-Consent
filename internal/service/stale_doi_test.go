package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
)

const doiTenant = "tenant-a"

func newDOIFixture(t *testing.T) (*store.Memory, *Service, domain.Evidence) {
	t.Helper()
	repo := store.NewMemory()
	svc := New(repo)
	installPolicyFixture(t, svc, doiTenant)
	return repo, svc, domain.Evidence{Method: "signup_form", ArtifactHash: "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", CollectedAt: time.Now().UTC()}
}

func startChallenge(t *testing.T, svc *Service, subject string, evidence domain.Evidence) domain.DoubleOptInChallenge {
	t.Helper()
	challenge, err := svc.StartDoubleOptIn(context.Background(), doiTenant, "actor", subject, "double_marketing", "email", "portal", "NG", "2026-08", evidence, time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return challenge
}

func deny(t *testing.T, svc *Service, subject string, evidence domain.Evidence) domain.Consent {
	t.Helper()
	denied, err := svc.RecordGoverned(context.Background(), doiTenant, "actor", subject, "double_marketing", "email", "consent", "denied", "portal", nil, nil, "NG", "2026-08", evidence)
	if err != nil {
		t.Fatal(err)
	}
	return denied
}

func evaluate(t *testing.T, svc *Service, subject string) domain.SuppressionDecision {
	t.Helper()
	decision, err := svc.Evaluate(context.Background(), doiTenant, subject, "double_marketing", "email", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	return decision
}

type sideEffects struct{ audit, outbox int }

func snapshot(repo *store.Memory) sideEffects {
	return sideEffects{audit: len(repo.Audit()), outbox: len(repo.Outbox())}
}

// assertRejectedConfirmation confirms token and requires a rejection that
// leaves the challenge ungranted and writes no receipt, audit or outbox.
func assertRejectedConfirmation(t *testing.T, repo *store.Memory, svc *Service, challenge domain.DoubleOptInChallenge) error {
	t.Helper()
	before := snapshot(repo)
	confirmed, err := svc.ConfirmDoubleOptIn(context.Background(), doiTenant, "actor", TenantWide(), challenge.ConfirmationToken)
	if err == nil {
		t.Fatalf("stale challenge was confirmed: %+v", confirmed)
	}
	if after := snapshot(repo); after != before {
		t.Fatalf("rejected confirmation wrote side effects: before=%+v after=%+v", before, after)
	}
	stored, getErr := repo.Get(context.Background(), doiTenant, challenge.Consent.ID)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if stored.Status == domain.StatusGranted || stored.ReceiptID != "" || stored.ConfirmedAt != nil {
		t.Fatalf("rejected challenge was mutated into a grant: %+v", stored)
	}
	return err
}

// AT-001 assertion 1.
func TestStaleChallengeCannotOverrideLaterDenial(t *testing.T) {
	repo, svc, evidence := newDOIFixture(t)
	challenge := startChallenge(t, svc, "cus-deny", evidence)
	denied := deny(t, svc, "cus-deny", evidence)

	stored, _ := repo.Get(context.Background(), doiTenant, challenge.Consent.ID)
	if stored.ConfirmationTokenHash != "" {
		t.Fatal("denial must atomically invalidate the earlier challenge token")
	}
	err := assertRejectedConfirmation(t, repo, svc, challenge)
	if !errors.Is(err, store.ErrNotFound) && !errors.Is(err, store.ErrConflict) {
		t.Fatalf("unexpected rejection error: %v", err)
	}
	decision := evaluate(t, svc, "cus-deny")
	if decision.Allowed || decision.Reason != string(domain.StatusDenied) || decision.ConsentID != denied.ID {
		t.Fatalf("later denial must remain effective: %+v", decision)
	}
}

// AT-001 assertion 2.
func TestStaleChallengeCannotRestoreWithdrawnConsent(t *testing.T) {
	repo, svc, evidence := newDOIFixture(t)
	older := startChallenge(t, svc, "cus-withdraw", evidence)
	newer := startChallenge(t, svc, "cus-withdraw", evidence)
	granted, err := svc.ConfirmDoubleOptIn(context.Background(), doiTenant, "actor", TenantWide(), newer.ConfirmationToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Withdraw(context.Background(), doiTenant, "actor", granted.ID); err != nil {
		t.Fatal(err)
	}
	assertRejectedConfirmation(t, repo, svc, older)
	decision := evaluate(t, svc, "cus-withdraw")
	if decision.Allowed || decision.Reason != string(domain.StatusWithdrawn) || decision.ConsentID != granted.ID {
		t.Fatalf("withdrawal must remain effective: %+v", decision)
	}
}

func TestSupersedingChallengeInvalidatesPriorChallenge(t *testing.T) {
	repo, svc, evidence := newDOIFixture(t)
	first := startChallenge(t, svc, "cus-supersede", evidence)
	second := startChallenge(t, svc, "cus-supersede", evidence)
	assertRejectedConfirmation(t, repo, svc, first)
	confirmed, err := svc.ConfirmDoubleOptIn(context.Background(), doiTenant, "actor", TenantWide(), second.ConfirmationToken)
	if err != nil {
		t.Fatalf("current challenge rejected: %v", err)
	}
	if decision := evaluate(t, svc, "cus-supersede"); !decision.Allowed || decision.ConsentID != confirmed.ID {
		t.Fatalf("current challenge should grant: %+v", decision)
	}
}

func TestWithdrawingPendingChallengeInvalidatesIt(t *testing.T) {
	repo, svc, evidence := newDOIFixture(t)
	challenge := startChallenge(t, svc, "cus-pending-withdraw", evidence)
	if _, err := svc.Withdraw(context.Background(), doiTenant, "actor", challenge.Consent.ID); err != nil {
		t.Fatal(err)
	}
	before := snapshot(repo)
	if _, err := svc.ConfirmDoubleOptIn(context.Background(), doiTenant, "actor", TenantWide(), challenge.ConfirmationToken); err == nil {
		t.Fatal("withdrawn challenge was confirmed")
	}
	if snapshot(repo) != before {
		t.Fatal("rejected confirmation wrote side effects")
	}
	if decision := evaluate(t, svc, "cus-pending-withdraw"); decision.Allowed || decision.Reason != string(domain.StatusWithdrawn) {
		t.Fatalf("withdrawal must be effective: %+v", decision)
	}
}

func TestConfirmationRechecksPolicyInsideTransaction(t *testing.T) {
	repo, svc, evidence := newDOIFixture(t)
	challenge := startChallenge(t, svc, "cus-policy", evidence)
	if _, err := svc.RegisterPurpose(context.Background(), doiTenant, "admin", domain.Purpose{Code: "double_marketing", Category: "marketing", AllowedChannels: []string{"email"}, RequiresAffirmativeConsent: true, RequiresDoubleOptIn: true, Active: false}); err != nil {
		t.Fatal(err)
	}
	before := snapshot(repo)
	if _, err := svc.ConfirmDoubleOptIn(context.Background(), doiTenant, "actor", TenantWide(), challenge.ConfirmationToken); !errors.Is(err, ErrValidation) {
		t.Fatalf("deactivated purpose: got %v, want ErrValidation", err)
	}
	if snapshot(repo) != before {
		t.Fatal("policy rejection wrote side effects")
	}
	// Policy changes do not supersede intent: once reactivated, the still
	// current challenge can be confirmed.
	if _, err := svc.RegisterPurpose(context.Background(), doiTenant, "admin", domain.Purpose{Code: "double_marketing", Category: "marketing", AllowedChannels: []string{"email"}, RequiresAffirmativeConsent: true, RequiresDoubleOptIn: true, Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConfirmDoubleOptIn(context.Background(), doiTenant, "actor", TenantWide(), challenge.ConfirmationToken); err != nil {
		t.Fatalf("current challenge rejected after policy restored: %v", err)
	}
}

func TestExpiredChallengeIsRejected(t *testing.T) {
	repo, svc, evidence := newDOIFixture(t)
	challenge, err := svc.StartDoubleOptIn(context.Background(), doiTenant, "actor", "cus-expiry", "double_marketing", "email", "portal", "NG", "2026-08", evidence, time.Now().UTC().Add(50*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	before := snapshot(repo)
	if _, err := svc.ConfirmDoubleOptIn(context.Background(), doiTenant, "actor", TenantWide(), challenge.ConfirmationToken); !errors.Is(err, store.ErrExpired) {
		t.Fatalf("expired challenge: got %v, want ErrExpired", err)
	}
	if snapshot(repo) != before {
		t.Fatal("expired confirmation wrote side effects")
	}
}

// AT-001 assertion 3: whichever of confirm and deny commits first, the denial
// is the later intent, so the converged state is always denied, and exactly
// one atomic outcome is recorded per successful operation.
func TestConcurrentConfirmAndDenyConvergeToDenied(t *testing.T) {
	repo, svc, evidence := newDOIFixture(t)
	for i := range 100 {
		subject := fmt.Sprintf("cus-race-%d", i)
		challenge := startChallenge(t, svc, subject, evidence)
		before := snapshot(repo)
		var wg sync.WaitGroup
		start := make(chan struct{})
		var confirmed domain.Consent
		var confirmErr, denyErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			confirmed, confirmErr = svc.ConfirmDoubleOptIn(context.Background(), doiTenant, "actor", TenantWide(), challenge.ConfirmationToken)
		}()
		go func() {
			defer wg.Done()
			<-start
			_, denyErr = svc.RecordGoverned(context.Background(), doiTenant, "actor", subject, "double_marketing", "email", "consent", "denied", "portal", nil, nil, "NG", "2026-08", evidence)
		}()
		close(start)
		wg.Wait()
		if denyErr != nil {
			t.Fatalf("iteration %d: deny failed: %v", i, denyErr)
		}
		if decision := evaluate(t, svc, subject); decision.Allowed || decision.Reason != string(domain.StatusDenied) {
			t.Fatalf("iteration %d: converged to %+v (confirmErr=%v)", i, decision, confirmErr)
		}
		wantWrites := 1 // deny
		if confirmErr == nil {
			wantWrites++
			receipt, err := repo.GetReceipt(context.Background(), doiTenant, confirmed.ReceiptID)
			if err != nil || !VerifyReceipt(receipt).Trusted() {
				t.Fatalf("iteration %d: confirmed grant lacks a verifiable receipt: %v", i, err)
			}
		}
		if after := snapshot(repo); after.audit-before.audit != wantWrites || after.outbox-before.outbox != wantWrites {
			t.Fatalf("iteration %d: side effects %+v -> %+v, want %d of each", i, before, after, wantWrites)
		}
	}
}

func TestConcurrentConfirmAndWithdrawConvergeToWithdrawn(t *testing.T) {
	_, svc, evidence := newDOIFixture(t)
	for i := range 100 {
		subject := fmt.Sprintf("cus-race-withdraw-%d", i)
		challenge := startChallenge(t, svc, subject, evidence)
		var wg sync.WaitGroup
		start := make(chan struct{})
		var withdrawErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, _ = svc.ConfirmDoubleOptIn(context.Background(), doiTenant, "actor", TenantWide(), challenge.ConfirmationToken)
		}()
		go func() {
			defer wg.Done()
			<-start
			_, withdrawErr = svc.Withdraw(context.Background(), doiTenant, "actor", challenge.Consent.ID)
		}()
		close(start)
		wg.Wait()
		if withdrawErr != nil {
			t.Fatalf("iteration %d: withdraw failed: %v", i, withdrawErr)
		}
		if decision := evaluate(t, svc, subject); decision.Allowed || decision.Reason != string(domain.StatusWithdrawn) {
			t.Fatalf("iteration %d: converged to %+v", i, decision)
		}
	}
}
