package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
)

func TestPostgresProtectedAdministrativeSuppression(t *testing.T) {
	databaseURL := integrationDSN(t)
	repo, err := NewPostgres(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	run := runID(t)
	tenant := "tenant-" + run
	subject := "cus-" + run

	audit := func(action, id string) domain.AuditEvent {
		return domain.AuditEvent{
			ID:           "aud-" + runID(t),
			TenantID:     tenant,
			ActorID:      "actor",
			Action:       action,
			ConsentID:    id,
			ResourceType: "suppression",
			OccurredAt:   now,
		}
	}
	outbox := func(id string, active bool) domain.OutboxEvent {
		return domain.OutboxEvent{
			ID:          "evt-" + runID(t),
			TenantID:    tenant,
			EventType:   "consent.suppression.changed",
			AggregateID: id,
			Payload:     map[string]any{"subject_id": subject, "active": active},
			OccurredAt:  now,
		}
	}

	// 1. Admin creates an administrative global suppression
	adminSup := domain.Suppression{
		ID:            "sup-" + run + "-global",
		TenantID:      tenant,
		SubjectID:     subject,
		Scope:         domain.SuppressionGlobal,
		Reason:        "regulatory_hold",
		Active:        true,
		CreatedByRole: "privacy_admin",
		UpdatedAt:     now,
	}
	stored, err := repo.SetSuppression(ctx, adminSup, audit("suppression.created", adminSup.ID), outbox(adminSup.ID, true))
	if err != nil {
		t.Fatalf("failed to create administrative suppression: %v", err)
	}
	if stored.CreatedByRole != "privacy_admin" {
		t.Fatalf("expected created_by_role privacy_admin, got %s", stored.CreatedByRole)
	}

	// 2. Self_service caller attempts to clear / revoke the administrative suppression
	revokeAttempt := domain.Suppression{
		ID:            "sup-" + run + "-revoke",
		TenantID:      tenant,
		SubjectID:     subject,
		Scope:         domain.SuppressionGlobal,
		Reason:        "user_opt_in",
		Active:        false,
		CreatedByRole: "self_service",
		UpdatedAt:     time.Now().UTC(),
	}
	_, err = repo.SetSuppression(ctx, revokeAttempt, audit("suppression.revoked", adminSup.ID), outbox(adminSup.ID, false))
	if !errors.Is(err, domain.ErrSuppressionProtected) {
		t.Fatalf("self_service revoke must fail with ErrSuppressionProtected, got %v", err)
	}

	// Verify suppression is STILL active
	found, err := repo.FindSuppression(ctx, tenant, subject, "marketing", "email")
	if err != nil {
		t.Fatalf("FindSuppression failed: %v", err)
	}
	if !found.Active || found.CreatedByRole != "privacy_admin" {
		t.Fatalf("expected active suppression with created_by_role privacy_admin, got %+v", found)
	}

	// 3. Self_service creates their own channel suppression
	selfChannelSup := domain.Suppression{
		ID:            "sup-" + run + "-channel",
		TenantID:      tenant,
		SubjectID:     subject,
		Scope:         domain.SuppressionChannel,
		Channel:       "email",
		Reason:        "user_preference",
		Active:        true,
		CreatedByRole: "self_service",
		UpdatedAt:     time.Now().UTC(),
	}
	storedChannel, err := repo.SetSuppression(ctx, selfChannelSup, audit("suppression.created", selfChannelSup.ID), outbox(selfChannelSup.ID, true))
	if err != nil {
		t.Fatalf("self_service create own channel suppression failed: %v", err)
	}
	if storedChannel.CreatedByRole != "self_service" {
		t.Fatalf("expected created_by_role self_service, got %s", storedChannel.CreatedByRole)
	}

	// 4. Self_service CAN revoke their own channel suppression
	selfChannelSup.Active = false
	selfChannelSup.Reason = "user_re_opt_in"
	selfChannelSup.UpdatedAt = time.Now().UTC()
	revokedChannel, err := repo.SetSuppression(ctx, selfChannelSup, audit("suppression.revoked", selfChannelSup.ID), outbox(selfChannelSup.ID, false))
	if err != nil {
		t.Fatalf("self_service revoke own channel suppression failed: %v", err)
	}
	if revokedChannel.Active {
		t.Fatalf("expected channel suppression active to be false")
	}

	// 5. Admin can revoke the administrative global suppression
	adminRevoke := domain.Suppression{
		ID:            adminSup.ID,
		TenantID:      tenant,
		SubjectID:     subject,
		Scope:         domain.SuppressionGlobal,
		Reason:        "hold_released",
		Active:        false,
		CreatedByRole: "consent_admin",
		UpdatedAt:     time.Now().UTC(),
	}
	revokedAdmin, err := repo.SetSuppression(ctx, adminRevoke, audit("suppression.revoked", adminSup.ID), outbox(adminSup.ID, false))
	if err != nil {
		t.Fatalf("admin revoke administrative suppression failed: %v", err)
	}
	if revokedAdmin.Active {
		t.Fatalf("expected admin global suppression active to be false")
	}
}

func TestIntegration_PostgresProtectedAdministrativeSuppression(t *testing.T) {
	TestPostgresProtectedAdministrativeSuppression(t)
}

func TestMemoryProtectedAdministrativeSuppression(t *testing.T) {
	repo := NewMemory()
	ctx := context.Background()
	now := time.Now().UTC()
	run := runID(t)
	tenant := "tenant-mem-" + run
	subject := "cus-mem-" + run

	audit := func(action, id string) domain.AuditEvent {
		return domain.AuditEvent{
			ID:           "aud-" + runID(t),
			TenantID:     tenant,
			ActorID:      "actor",
			Action:       action,
			ConsentID:    id,
			ResourceType: "suppression",
			OccurredAt:   now,
		}
	}
	outbox := func(id string, active bool) domain.OutboxEvent {
		return domain.OutboxEvent{
			ID:          "evt-" + runID(t),
			TenantID:    tenant,
			EventType:   "consent.suppression.changed",
			AggregateID: id,
			Payload:     map[string]any{"subject_id": subject, "active": active},
			OccurredAt:  now,
		}
	}

	// 1. Admin creates an administrative global suppression
	adminSup := domain.Suppression{
		ID:            "sup-" + run + "-global",
		TenantID:      tenant,
		SubjectID:     subject,
		Scope:         domain.SuppressionGlobal,
		Reason:        "compliance_hold",
		Active:        true,
		CreatedByRole: "privacy_admin",
		UpdatedAt:     now,
	}
	stored, err := repo.SetSuppression(ctx, adminSup, audit("suppression.created", adminSup.ID), outbox(adminSup.ID, true))
	if err != nil {
		t.Fatalf("failed to create administrative suppression: %v", err)
	}
	if stored.CreatedByRole != "privacy_admin" {
		t.Fatalf("expected created_by_role privacy_admin, got %s", stored.CreatedByRole)
	}

	// 2. Self_service caller attempts to revoke the administrative suppression
	revokeAttempt := domain.Suppression{
		ID:            "sup-" + run + "-revoke",
		TenantID:      tenant,
		SubjectID:     subject,
		Scope:         domain.SuppressionGlobal,
		Reason:        "user_opt_in",
		Active:        false,
		CreatedByRole: "self_service",
		UpdatedAt:     time.Now().UTC(),
	}
	_, err = repo.SetSuppression(ctx, revokeAttempt, audit("suppression.revoked", adminSup.ID), outbox(adminSup.ID, false))
	if !errors.Is(err, domain.ErrSuppressionProtected) {
		t.Fatalf("self_service revoke must fail with ErrSuppressionProtected, got %v", err)
	}

	// 3. Self-service caller creates their own purpose_channel suppression
	selfSup := domain.Suppression{
		ID:            "sup-" + run + "-purpose",
		TenantID:      tenant,
		SubjectID:     subject,
		Scope:         domain.SuppressionPurposeChannel,
		Purpose:       "marketing",
		Channel:       "email",
		Reason:        "opt_out",
		Active:        true,
		CreatedByRole: "self_service",
		UpdatedAt:     time.Now().UTC(),
	}
	if _, err := repo.SetSuppression(ctx, selfSup, audit("suppression.created", selfSup.ID), outbox(selfSup.ID, true)); err != nil {
		t.Fatalf("self_service create own suppression failed: %v", err)
	}

	// 4. Self-service caller can revoke their own suppression
	selfSup.Active = false
	if _, err := repo.SetSuppression(ctx, selfSup, audit("suppression.revoked", selfSup.ID), outbox(selfSup.ID, false)); err != nil {
		t.Fatalf("self_service revoke own suppression failed: %v", err)
	}

	// 5. Admin can revoke the administrative suppression
	adminSup.Active = false
	adminSup.CreatedByRole = "consent_admin"
	if _, err := repo.SetSuppression(ctx, adminSup, audit("suppression.revoked", adminSup.ID), outbox(adminSup.ID, false)); err != nil {
		t.Fatalf("admin revoke administrative suppression failed: %v", err)
	}
}
