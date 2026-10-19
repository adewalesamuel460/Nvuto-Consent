package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
)

// Receipt hash formats.
//
// Version 1 (legacy, read-only): SHA-256 over encoding/json of the Go
// ConsentReceipt struct with an empty receipt_hash. Timestamps were RFC 3339
// with nanoseconds and the value's own zone, so a receipt read back through the
// pgx timestamptz codec (microsecond precision, local zone) no longer hashes to
// its stored value (T-004). Version 1 is never produced again.
//
// Version 2 (canonical): SHA-256, lowercase hex, over the UTF-8 bytes of a JSON
// object with exactly these members, in this order, without insignificant
// whitespace:
//
//	{"v":2,"id":…,"tenant_id":…,"consent_id":…,"subject_id":…,"purpose":…,
//	 "channel":…,"notice_version":…,"jurisdiction":…,
//	 "evidence":{"method":…,"artifact_hash":…,"collected_at":…},"issued_at":…}
//
// Canonicalization rules:
//
//   - Strings are hashed exactly as stored (no Unicode normalization) and must
//     be valid UTF-8. They are escaped per RFC 8259 by encoding/json with HTML
//     escaping disabled ('<', '>', '&' are literal; U+2028/U+2029 are \u-escaped).
//   - Timestamps are converted to UTC, truncated to whole microseconds (the
//     precision of YSQL/PostgreSQL timestamptz and of the pgx binary codec) and
//     formatted as YYYY-MM-DDTHH:MM:SS.ffffffZ with exactly six fraction digits.
//     The same instant therefore hashes identically regardless of zone offset
//     or sub-microsecond digits.
//   - The receipt contains no maps, so no key-ordering rule is required.
const (
	ReceiptHashVersionLegacy    = 1
	ReceiptHashVersionCanonical = 2

	receiptTimeLayout = "2006-01-02T15:04:05.000000Z"
)

// ErrInvalidReceipt is returned when a receipt field cannot be canonicalized.
var ErrInvalidReceipt = errors.New("receipt is not canonicalizable")

// normalizeReceiptTime returns t at the precision and zone the canonical form
// uses. Receipts store the normalized value so the stored instant is exact.
func normalizeReceiptTime(t time.Time) time.Time {
	return t.UTC().Truncate(time.Microsecond)
}

type canonicalEvidenceV2 struct {
	Method       string `json:"method"`
	ArtifactHash string `json:"artifact_hash"`
	CollectedAt  string `json:"collected_at"`
}

type canonicalReceiptV2 struct {
	V             int                 `json:"v"`
	ID            string              `json:"id"`
	TenantID      string              `json:"tenant_id"`
	ConsentID     string              `json:"consent_id"`
	SubjectID     string              `json:"subject_id"`
	Purpose       string              `json:"purpose"`
	Channel       string              `json:"channel"`
	NoticeVersion string              `json:"notice_version"`
	Jurisdiction  string              `json:"jurisdiction"`
	Evidence      canonicalEvidenceV2 `json:"evidence"`
	IssuedAt      string              `json:"issued_at"`
}

// CanonicalReceiptBytes returns the version 2 canonical serialization.
func CanonicalReceiptBytes(r domain.ConsentReceipt) ([]byte, error) {
	for _, field := range []string{r.ID, r.TenantID, r.ConsentID, r.SubjectID, r.Purpose, r.Channel, r.NoticeVersion, r.Jurisdiction, r.Evidence.Method, r.Evidence.ArtifactHash} {
		if !utf8.ValidString(field) {
			return nil, ErrInvalidReceipt
		}
	}
	canonical := canonicalReceiptV2{
		V: ReceiptHashVersionCanonical, ID: r.ID, TenantID: r.TenantID, ConsentID: r.ConsentID, SubjectID: r.SubjectID,
		Purpose: r.Purpose, Channel: r.Channel, NoticeVersion: r.NoticeVersion, Jurisdiction: r.Jurisdiction,
		Evidence: canonicalEvidenceV2{
			Method:       r.Evidence.Method,
			ArtifactHash: r.Evidence.ArtifactHash,
			CollectedAt:  normalizeReceiptTime(r.Evidence.CollectedAt).Format(receiptTimeLayout),
		},
		IssuedAt: normalizeReceiptTime(r.IssuedAt).Format(receiptTimeLayout),
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(canonical); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), nil
}

func canonicalReceiptHash(r domain.ConsentReceipt) (string, error) {
	encoded, err := CanonicalReceiptBytes(r)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

// legacyReceiptV1 freezes the field set and JSON layout the version 1 hash was
// computed over, independent of later changes to domain.ConsentReceipt.
type legacyReceiptV1 struct {
	ID            string          `json:"id"`
	TenantID      string          `json:"tenant_id"`
	ConsentID     string          `json:"consent_id"`
	SubjectID     string          `json:"subject_id"`
	Purpose       string          `json:"purpose"`
	Channel       string          `json:"channel"`
	NoticeVersion string          `json:"notice_version"`
	Jurisdiction  string          `json:"jurisdiction"`
	Evidence      domain.Evidence `json:"evidence"`
	ReceiptHash   string          `json:"receipt_hash"`
	IssuedAt      time.Time       `json:"issued_at"`
}

func legacyReceiptHash(r domain.ConsentReceipt, issuedAt time.Time) string {
	encoded, _ := json.Marshal(legacyReceiptV1{
		ID: r.ID, TenantID: r.TenantID, ConsentID: r.ConsentID, SubjectID: r.SubjectID, Purpose: r.Purpose,
		Channel: r.Channel, NoticeVersion: r.NoticeVersion, Jurisdiction: r.Jurisdiction, Evidence: r.Evidence,
		IssuedAt: issuedAt,
	})
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

// makeReceipt issues a version 2 receipt for consent. IssuedAt is stored at
// canonical precision so the persisted instant is exactly the hashed instant.
func makeReceipt(consent domain.Consent, evidence domain.Evidence, at time.Time) (domain.ConsentReceipt, error) {
	receipt := domain.ConsentReceipt{
		ID: id("rcp"), TenantID: consent.TenantID, ConsentID: consent.ID, SubjectID: consent.SubjectID,
		Purpose: consent.Purpose, Channel: consent.Channel, NoticeVersion: consent.NoticeVersion,
		Jurisdiction: consent.Jurisdiction, Evidence: evidence, IssuedAt: normalizeReceiptTime(at),
		HashVersion: ReceiptHashVersionCanonical,
	}
	hash, err := canonicalReceiptHash(receipt)
	if err != nil {
		return domain.ConsentReceipt{}, ErrValidation
	}
	receipt.ReceiptHash = hash
	return receipt, nil
}

// ReceiptHashStatus classifies a receipt hash check.
type ReceiptHashStatus string

const (
	// ReceiptHashVerified: version 2 receipt whose canonical hash matches.
	ReceiptHashVerified ReceiptHashStatus = "verified"
	// ReceiptHashLegacyVerified: version 1 receipt whose legacy hash matches.
	ReceiptHashLegacyVerified ReceiptHashStatus = "legacy_verified"
	// ReceiptHashLegacyUnverifiable: version 1 receipt whose hash cannot be
	// reproduced, and whose issued_at carries only microsecond precision. This
	// is the expected outcome of the T-004 storage truncation; the discarded
	// nanoseconds are unrecoverable, so the receipt can be neither confirmed
	// nor refuted from its own contents and needs reconciliation.
	ReceiptHashLegacyUnverifiable ReceiptHashStatus = "legacy_unverifiable_precision_loss"
	// ReceiptHashMismatch: the hash does not match and precision loss cannot
	// explain it. Treat as tampering or corruption.
	ReceiptHashMismatch ReceiptHashStatus = "mismatch"
	// ReceiptHashUnsupportedVersion: unknown hash_version.
	ReceiptHashUnsupportedVersion ReceiptHashStatus = "unsupported_version"
)

// ReceiptVerification is the outcome of verifying one receipt. It never
// modifies the receipt.
type ReceiptVerification struct {
	ReceiptID   string            `json:"receipt_id"`
	TenantID    string            `json:"tenant_id"`
	HashVersion int               `json:"hash_version"`
	Status      ReceiptHashStatus `json:"status"`
	Detail      string            `json:"detail,omitempty"`
}

// Trusted reports whether the stored hash was reproduced from the receipt.
func (v ReceiptVerification) Trusted() bool {
	return v.Status == ReceiptHashVerified || v.Status == ReceiptHashLegacyVerified
}

func hashesEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(strings.ToLower(a)), []byte(strings.ToLower(b))) == 1
}

// VerifyReceipt recomputes the receipt hash using the format named by its
// hash_version (absent/0 is treated as legacy version 1).
func VerifyReceipt(r domain.ConsentReceipt) ReceiptVerification {
	result := ReceiptVerification{ReceiptID: r.ID, TenantID: r.TenantID, HashVersion: r.HashVersion}
	switch r.HashVersion {
	case ReceiptHashVersionCanonical:
		expected, err := canonicalReceiptHash(r)
		if err != nil {
			result.Status, result.Detail = ReceiptHashMismatch, "receipt fields are not canonicalizable"
			return result
		}
		if hashesEqual(expected, r.ReceiptHash) {
			result.Status = ReceiptHashVerified
			return result
		}
		result.Status, result.Detail = ReceiptHashMismatch, "canonical v2 hash does not match stored receipt_hash"
		return result
	case 0, ReceiptHashVersionLegacy:
		result.HashVersion = ReceiptHashVersionLegacy
		// Try the value as read and in UTC: v1 hashes were computed over a UTC
		// time, while pgx returns timestamptz in the local zone.
		for _, issuedAt := range []time.Time{r.IssuedAt, r.IssuedAt.UTC()} {
			if hashesEqual(legacyReceiptHash(r, issuedAt), r.ReceiptHash) {
				result.Status = ReceiptHashLegacyVerified
				return result
			}
		}
		if r.IssuedAt.Nanosecond()%1000 == 0 {
			result.Status = ReceiptHashLegacyUnverifiable
			result.Detail = "legacy v1 hash not reproducible; issued_at has microsecond precision consistent with timestamptz truncation, original nanoseconds unrecoverable"
			return result
		}
		result.Status, result.Detail = ReceiptHashMismatch, "legacy v1 hash does not match and issued_at retains sub-microsecond precision"
		return result
	default:
		result.Status = ReceiptHashUnsupportedVersion
		return result
	}
}

// ReceiptReconciliation is a read-only, reviewable report over a set of
// receipts. It lists every receipt that is not Trusted so reviewers can
// annotate historic receipts without rewriting them.
type ReceiptReconciliation struct {
	GeneratedAt time.Time                 `json:"generated_at"`
	Total       int                       `json:"total"`
	Counts      map[ReceiptHashStatus]int `json:"counts"`
	Findings    []ReceiptVerification     `json:"findings"`
}

func ReconcileReceipts(receipts []domain.ConsentReceipt) ReceiptReconciliation {
	report := ReceiptReconciliation{GeneratedAt: time.Now().UTC(), Total: len(receipts), Counts: map[ReceiptHashStatus]int{}, Findings: []ReceiptVerification{}}
	for _, receipt := range receipts {
		verification := VerifyReceipt(receipt)
		report.Counts[verification.Status]++
		if !verification.Trusted() {
			report.Findings = append(report.Findings, verification)
		}
	}
	return report
}

// VerifyReceipt loads a stored receipt and verifies its hash.
func (s *Service) VerifyReceipt(ctx context.Context, tenantID, receiptID string) (ReceiptVerification, error) {
	receipt, err := s.Receipt(ctx, tenantID, receiptID)
	if err != nil {
		return ReceiptVerification{}, err
	}
	return VerifyReceipt(receipt), nil
}
