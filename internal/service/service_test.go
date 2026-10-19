package service

import (
	"context"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
)

func TestConsentGrantEvaluateWithdraw(t *testing.T) {
	repo := store.NewMemory()
	svc := New(repo)
	c, err := svc.Record(context.Background(), "tenant-a", "user-1", "cus-1", "marketing", "whatsapp", "consent", "granted", "portal", map[string]string{"brand": "nvuto"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	d, err := svc.Evaluate(context.Background(), "tenant-a", "cus-1", "marketing", "whatsapp", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !d.Allowed || d.ConsentID != c.ID {
		t.Fatalf("expected allowed decision, got %+v", d)
	}
	c2, err := svc.Withdraw(context.Background(), "tenant-a", "user-1", c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(c2.Status) != "withdrawn" {
		t.Fatalf("expected withdrawn, got %s", c2.Status)
	}
	d, err = svc.Evaluate(context.Background(), "tenant-a", "cus-1", "marketing", "whatsapp", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if d.Allowed || d.Reason != "withdrawn" {
		t.Fatalf("expected suppression after withdrawal, got %+v", d)
	}
	audit := repo.Audit()
	if len(audit) != 2 {
		t.Fatalf("expected 2 audit events, got %d", len(audit))
	}
	if audit[1].ID == "" || audit[1].ConsentID != c.ID || audit[1].Action != "consent.withdrawn" {
		t.Fatalf("withdrawal audit must reference the consent with its own event ID: %+v", audit[1])
	}
	outbox := repo.Outbox()
	if len(outbox) != 2 {
		t.Fatalf("expected 2 outbox events, got %d", len(outbox))
	}
	if outbox[1].ID == "" || outbox[1].AggregateID != c.ID || outbox[1].EventType != "consent.withdrawn" {
		t.Fatalf("withdrawal outbox must reference the consent with its own event ID: %+v", outbox[1])
	}
}

func TestNoConsentAndExpirySuppress(t *testing.T) {
	svc := New(store.NewMemory())
	d, err := svc.Evaluate(context.Background(), "tenant-a", "cus-x", "marketing", "email", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if d.Allowed || d.Reason != "no_consent" {
		t.Fatalf("unexpected no-consent decision %+v", d)
	}
	expires := time.Now().Add(time.Hour)
	_, err = svc.Record(context.Background(), "tenant-a", "user-1", "cus-x", "marketing", "email", "consent", "granted", "portal", nil, &expires)
	if err != nil {
		t.Fatal(err)
	}
	d, err = svc.Evaluate(context.Background(), "tenant-a", "cus-x", "marketing", "email", time.Now().Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if d.Allowed || d.Reason != "expired" {
		t.Fatalf("expected expiry suppression, got %+v", d)
	}
}

func TestTenantIsolation(t *testing.T) {
	repo := store.NewMemory()
	svc := New(repo)
	_, err := svc.Record(context.Background(), "tenant-a", "user-1", "cus-1", "service_updates", "sms", "contract", "granted", "admin", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	d, err := svc.Evaluate(context.Background(), "tenant-b", "cus-1", "service_updates", "sms", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if d.Allowed || d.Reason != "no_consent" {
		t.Fatalf("cross tenant decision leaked: %+v", d)
	}
}
