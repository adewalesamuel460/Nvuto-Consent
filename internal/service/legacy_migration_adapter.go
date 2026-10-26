package service

import (
	"context"
	"errors"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
)

// LegacyMigrationAdapter provides a strictly quarantined, isolated boundary for
// offline historical data migrations.
//
// CRITICAL ARCHITECTURAL INVARIANT (T-031 / F-031 / AT-031):
// This adapter is strictly WALLED OFF from production HTTP/REST API endpoints,
// background workers, and evaluation request flows. Under no circumstances may
// any production route in cmd/server or background consumer invoke this adapter.
// All production consent creations MUST execute through RecordGoverned or
// RecordGovernedIdempotent to guarantee atomic state, receipt, audit, and outbox writes.
type LegacyMigrationAdapter struct {
	repo RawConsentStore
}

// RawConsentStore defines the direct persistence capability reserved solely
// for offline batch data migration tooling. It is deliberately separated from
// the runtime store.Repository interface.
type RawConsentStore interface {
	// RawInsertLegacy is reserved strictly for offline batch data migration.
	RawInsertLegacy(ctx context.Context, consent domain.Consent) error
}

// NewLegacyMigrationAdapter constructs an isolated migration adapter.
func NewLegacyMigrationAdapter(repo RawConsentStore) *LegacyMigrationAdapter {
	return &LegacyMigrationAdapter{repo: repo}
}

// MigrateLegacyConsent records a historical legacy consent record prior to policy
// registry enforcement, tagging it explicitly as a migrated historical artifact.
func (a *LegacyMigrationAdapter) MigrateLegacyConsent(ctx context.Context, consent domain.Consent) error {
	if a.repo == nil {
		return errors.New("legacy_migration_adapter: unconfigured repository")
	}
	if consent.TenantID == "" || consent.SubjectID == "" || consent.Purpose == "" || consent.Channel == "" {
		return domain.ErrValidation.WithField("consent", "tenant_id, subject_id, purpose, and channel are required")
	}
	if consent.Source == "" {
		consent.Source = "legacy_migration"
	}
	if consent.CreatedAt.IsZero() {
		consent.CreatedAt = time.Now().UTC()
	}
	consent.UpdatedAt = consent.CreatedAt
	return a.repo.RawInsertLegacy(ctx, consent)
}
