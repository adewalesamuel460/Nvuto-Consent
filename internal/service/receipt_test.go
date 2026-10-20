package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
)

// vectorReceipt exercises non-ASCII text, HTML-significant characters, JSON
// quoting, a JavaScript line separator, a non-UTC offset and sub-microsecond
// digits that canonicalization must discard.
func vectorReceipt() domain.ConsentReceipt {
	lagos := time.FixedZone("UTC+1", 3600)
	return domain.ConsentReceipt{
		ID: "rcp_vector", TenantID: "tenant-a", ConsentID: "con_vector", SubjectID: "cus-é<&>\"q\"",
		Purpose: "marketing", Channel: "email", NoticeVersion: "2026-08", Jurisdiction: "NG",
		Evidence: domain.Evidence{
			Method:       "signup_form\u2028",
			ArtifactHash: "sha256:abc",
			CollectedAt:  time.Date(2026, 10, 19, 9, 0, 0, 999, lagos),
		},
		IssuedAt:    time.Date(2026, 10, 19, 10, 11, 12, 123456789, lagos),
		HashVersion: ReceiptHashVersionCanonical,
	}
}

const (
	// vectorCanonical is the exact v2 byte sequence for vectorReceipt.
	vectorCanonical = `{"v":2,"id":"rcp_vector","tenant_id":"tenant-a","consent_id":"con_vector","subject_id":"cus-é<&>\"q\"","purpose":"marketing","channel":"email","notice_version":"2026-08","jurisdiction":"NG","evidence":{"method":"signup_form\u2028","artifact_hash":"sha256:abc","collected_at":"2026-10-19T08:00:00.000000Z"},"issued_at":"2026-10-19T09:11:12.123456Z"}`
	// vectorHash is frozen: any change to it is a breaking format change and
	// requires a new hash_version.
	vectorHash = "2f69d6c5c69da4fa2d1fb4ed19774552d7b14558d6a35cbd90e428cdbdb4a5c3"
)

func sealed(t *testing.T, r domain.ConsentReceipt) domain.ConsentReceipt {
	t.Helper()
	hash, err := canonicalReceiptHash(r)
	if err != nil {
		t.Fatal(err)
	}
	r.ReceiptHash = hash
	return r
}

func TestCanonicalReceiptBytesVector(t *testing.T) {
	encoded, err := CanonicalReceiptBytes(vectorReceipt())
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != vectorCanonical {
		t.Fatalf("canonical bytes changed\n got: %s\nwant: %s", encoded, vectorCanonical)
	}
	hash, err := canonicalReceiptHash(vectorReceipt())
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(vectorCanonical))
	if hash != hex.EncodeToString(digest[:]) {
		t.Fatalf("hash is not sha256 of the canonical bytes: %s", hash)
	}
	if hash != vectorHash {
		t.Fatalf("frozen v2 vector hash changed: got %s want %s", hash, vectorHash)
	}
}

func TestCanonicalReceiptHashNormalizesOffsetAndPrecision(t *testing.T) {
	base := vectorReceipt()
	want, err := canonicalReceiptHash(base)
	if err != nil {
		t.Fatal(err)
	}
	instant := base.IssuedAt
	for name, issuedAt := range map[string]time.Time{
		"utc":                instant.UTC(),
		"minus five":         instant.In(time.FixedZone("UTC-5", -5*3600)),
		"plus five thirty":   instant.In(time.FixedZone("UTC+5:30", 5*3600+1800)),
		"truncated to micro": instant.Truncate(time.Microsecond),
		"other nanos":        instant.Truncate(time.Microsecond).Add(999 * time.Nanosecond),
		"local":              instant.Local(),
		"monotonic stripped": instant.Round(0),
	} {
		variant := base
		variant.IssuedAt = issuedAt
		variant.Evidence.CollectedAt = base.Evidence.CollectedAt.UTC()
		got, err := canonicalReceiptHash(variant)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%s: hash %s differs from %s", name, got, want)
		}
	}

	shifted := base
	shifted.IssuedAt = base.IssuedAt.Truncate(time.Microsecond).Add(time.Microsecond)
	if got, _ := canonicalReceiptHash(shifted); got == want {
		t.Fatal("a one microsecond difference must change the hash")
	}
	changed := base
	changed.SubjectID = "cus-other"
	if got, _ := canonicalReceiptHash(changed); got == want {
		t.Fatal("a field change must change the hash")
	}
}

func TestCanonicalReceiptRejectsInvalidUTF8(t *testing.T) {
	r := vectorReceipt()
	r.SubjectID = "cus-\xff"
	if _, err := CanonicalReceiptBytes(r); !errors.Is(err, ErrInvalidReceipt) {
		t.Fatalf("invalid UTF-8 must be rejected, got %v", err)
	}
	consent := domain.Consent{ID: "con", TenantID: "tenant-a", SubjectID: "cus-\xff"}
	if _, err := makeReceipt(consent, domain.Evidence{}, time.Now()); !errors.Is(err, ErrValidation) {
		t.Fatalf("makeReceipt must refuse invalid UTF-8, got %v", err)
	}
}

func TestMakeReceiptStoresNormalizedInstant(t *testing.T) {
	at := time.Date(2026, 10, 19, 10, 11, 12, 123456789, time.FixedZone("UTC+1", 3600))
	consent := domain.Consent{ID: "con", TenantID: "tenant-a", SubjectID: "cus", Purpose: "marketing", Channel: "email"}
	receipt, err := makeReceipt(consent, domain.Evidence{Method: "form", CollectedAt: at}, at)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.HashVersion != ReceiptHashVersionCanonical {
		t.Fatalf("new receipts must be v2, got %d", receipt.HashVersion)
	}
	if !receipt.IssuedAt.Equal(at.Truncate(time.Microsecond)) || receipt.IssuedAt.Location() != time.UTC {
		t.Fatalf("IssuedAt must be stored as UTC microseconds: %v", receipt.IssuedAt)
	}
	if v := VerifyReceipt(receipt); v.Status != ReceiptHashVerified {
		t.Fatalf("receipt must verify before storage: %+v", v)
	}
}

func TestReceiptHashSurvivesJSONExportImport(t *testing.T) {
	original := sealed(t, vectorReceipt())
	exported, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var imported domain.ConsentReceipt
	if err := json.Unmarshal(exported, &imported); err != nil {
		t.Fatal(err)
	}
	if imported.HashVersion != ReceiptHashVersionCanonical {
		t.Fatalf("hash_version must survive export: %s", exported)
	}
	if v := VerifyReceipt(imported); v.Status != ReceiptHashVerified {
		t.Fatalf("receipt must verify after JSON import: %+v (%s)", v, exported)
	}
}

// pgxTimestamptzRoundTrip pushes t through the same binary timestamptz codec
// pgx uses on the wire, which truncates to microseconds and returns local time.
func pgxTimestamptzRoundTrip(t *testing.T, value time.Time) time.Time {
	t.Helper()
	m := pgtype.NewMap()
	wire, err := m.Encode(pgtype.TimestamptzOID, pgtype.BinaryFormatCode, value, nil)
	if err != nil {
		t.Fatal(err)
	}
	var decoded time.Time
	if err := m.Scan(pgtype.TimestamptzOID, pgtype.BinaryFormatCode, wire, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func TestReceiptHashSurvivesPgxTimestamptzCodec(t *testing.T) {
	at := time.Date(2026, 10, 19, 10, 11, 12, 123456789, time.FixedZone("UTC+1", 3600))
	consent := domain.Consent{ID: "con", TenantID: "tenant-a", SubjectID: "cus", Purpose: "marketing", Channel: "email", NoticeVersion: "2026-08", Jurisdiction: "NG"}
	receipt, err := makeReceipt(consent, domain.Evidence{Method: "form", ArtifactHash: "sha256:x", CollectedAt: at}, at)
	if err != nil {
		t.Fatal(err)
	}
	read := receipt
	read.IssuedAt = pgxTimestamptzRoundTrip(t, receipt.IssuedAt)
	read.Evidence.CollectedAt = pgxTimestamptzRoundTrip(t, at)
	if !read.IssuedAt.Equal(receipt.IssuedAt) {
		t.Fatalf("normalized IssuedAt must be exact under the codec: %v vs %v", read.IssuedAt, receipt.IssuedAt)
	}
	if v := VerifyReceipt(read); v.Status != ReceiptHashVerified {
		t.Fatalf("v2 receipt must verify after a timestamptz round trip: %+v", v)
	}

	// The defect being fixed: a v1 hash over nanoseconds does not survive.
	legacy := domain.ConsentReceipt{ID: "rcp_old", TenantID: "tenant-a", ConsentID: "con", SubjectID: "cus", Purpose: "marketing", Channel: "email", IssuedAt: at.UTC()}
	legacy.ReceiptHash = legacyReceiptHash(legacy, legacy.IssuedAt)
	if v := VerifyReceipt(legacy); v.Status != ReceiptHashLegacyVerified {
		t.Fatalf("legacy receipt must verify before storage: %+v", v)
	}
	legacy.IssuedAt = pgxTimestamptzRoundTrip(t, legacy.IssuedAt)
	if v := VerifyReceipt(legacy); v.Status != ReceiptHashLegacyUnverifiable || v.Trusted() {
		t.Fatalf("truncated legacy receipt must be classified as precision loss: %+v", v)
	}
}

func TestVerifyReceiptClassifiesFormats(t *testing.T) {
	microAt := time.Date(2026, 1, 2, 3, 4, 5, 123456000, time.UTC)
	nanoAt := microAt.Add(789)
	legacyAt := func(at time.Time) domain.ConsentReceipt {
		r := domain.ConsentReceipt{ID: "rcp_l", TenantID: "tenant-a", ConsentID: "con", SubjectID: "cus", Purpose: "marketing", Channel: "email", IssuedAt: at}
		r.ReceiptHash = legacyReceiptHash(r, at)
		return r
	}

	cases := map[string]struct {
		receipt domain.ConsentReceipt
		want    ReceiptHashStatus
	}{
		"v2 verified": {receipt: sealed(t, vectorReceipt()), want: ReceiptHashVerified},
		"v2 tampered": {receipt: func() domain.ConsentReceipt {
			r := sealed(t, vectorReceipt())
			r.Purpose = "service_updates"
			return r
		}(), want: ReceiptHashMismatch},
		"v2 invalid utf8": {receipt: func() domain.ConsentReceipt {
			r := sealed(t, vectorReceipt())
			r.Channel = "\xff"
			return r
		}(), want: ReceiptHashMismatch},
		"v1 explicit": {receipt: func() domain.ConsentReceipt {
			r := legacyAt(nanoAt)
			r.HashVersion = ReceiptHashVersionLegacy
			return r
		}(), want: ReceiptHashLegacyVerified},
		"v1 implicit (hash_version absent)": {receipt: legacyAt(nanoAt), want: ReceiptHashLegacyVerified},
		"v1 read back in local zone":        {receipt: func() domain.ConsentReceipt { r := legacyAt(microAt); r.IssuedAt = r.IssuedAt.Local(); return r }(), want: ReceiptHashLegacyVerified},
		"v1 precision loss": {receipt: func() domain.ConsentReceipt {
			r := legacyAt(nanoAt)
			r.IssuedAt = nanoAt.Truncate(time.Microsecond)
			return r
		}(), want: ReceiptHashLegacyUnverifiable},
		"v1 tampered with nanos": {receipt: func() domain.ConsentReceipt {
			r := legacyAt(nanoAt)
			r.SubjectID = "cus-other"
			return r
		}(), want: ReceiptHashMismatch},
		"v2 hash labelled v1": {receipt: func() domain.ConsentReceipt {
			r := sealed(t, vectorReceipt())
			r.HashVersion = ReceiptHashVersionLegacy
			r.IssuedAt = r.IssuedAt.Add(789) // keep sub-µs digits so this is not excusable
			return r
		}(), want: ReceiptHashMismatch},
		"unknown version": {receipt: func() domain.ConsentReceipt { r := sealed(t, vectorReceipt()); r.HashVersion = 3; return r }(), want: ReceiptHashUnsupportedVersion},
	}
	for name, tc := range cases {
		original := tc.receipt
		got := VerifyReceipt(tc.receipt)
		if got.Status != tc.want {
			t.Errorf("%s: got %s (%s), want %s", name, got.Status, got.Detail, tc.want)
		}
		if got.Trusted() != (tc.want == ReceiptHashVerified || tc.want == ReceiptHashLegacyVerified) {
			t.Errorf("%s: Trusted()=%v inconsistent with %s", name, got.Trusted(), got.Status)
		}
		if tc.receipt.ReceiptHash != original.ReceiptHash || !tc.receipt.IssuedAt.Equal(original.IssuedAt) {
			t.Errorf("%s: verification must not modify the receipt", name)
		}
	}
}

func TestReconcileReceiptsReportsUntrustedOnly(t *testing.T) {
	good := sealed(t, vectorReceipt())
	tampered := good
	tampered.ID, tampered.Jurisdiction = "rcp_tampered", "GH"
	nanoAt := time.Date(2026, 1, 2, 3, 4, 5, 123456789, time.UTC)
	lossy := domain.ConsentReceipt{ID: "rcp_lossy", TenantID: "tenant-a", IssuedAt: nanoAt}
	lossy.ReceiptHash = legacyReceiptHash(lossy, nanoAt)
	lossy.IssuedAt = nanoAt.Truncate(time.Microsecond)

	report := ReconcileReceipts([]domain.ConsentReceipt{good, tampered, lossy})
	if report.Total != 3 || report.Counts[ReceiptHashVerified] != 1 || report.Counts[ReceiptHashMismatch] != 1 || report.Counts[ReceiptHashLegacyUnverifiable] != 1 {
		t.Fatalf("unexpected counts: %+v", report)
	}
	if len(report.Findings) != 2 || report.Findings[0].ReceiptID != "rcp_tampered" || report.Findings[1].ReceiptID != "rcp_lossy" {
		t.Fatalf("findings must list exactly the untrusted receipts in input order: %+v", report.Findings)
	}
}

func TestServiceReceiptsVerifyAfterStorage(t *testing.T) {
	_, svc, evidence := newDOIFixture(t)
	granted, err := svc.RecordGoverned(context.Background(), doiTenant, "actor", "cus-receipt", "marketing", "email", "consent", "granted", "portal", nil, nil, "NG", "2026-08", evidence)
	if err != nil {
		t.Fatal(err)
	}
	challenge := startChallenge(t, svc, "cus-receipt", evidence)
	confirmed, err := svc.ConfirmDoubleOptIn(context.Background(), doiTenant, "actor", TenantWide(), challenge.ConfirmationToken)
	if err != nil {
		t.Fatal(err)
	}
	for _, receiptID := range []string{granted.ReceiptID, confirmed.ReceiptID} {
		if receiptID == "" {
			t.Fatal("expected a receipt")
		}
		verification, err := svc.VerifyReceipt(context.Background(), doiTenant, receiptID)
		if err != nil {
			t.Fatal(err)
		}
		if verification.Status != ReceiptHashVerified || verification.HashVersion != ReceiptHashVersionCanonical {
			t.Fatalf("stored receipt %s must verify as v2: %+v", receiptID, verification)
		}
		receipt, err := svc.Receipt(context.Background(), doiTenant, receiptID)
		if err != nil {
			t.Fatal(err)
		}
		if receipt.IssuedAt.Nanosecond()%1000 != 0 {
			t.Fatalf("stored IssuedAt must be microsecond-normalized: %v", receipt.IssuedAt)
		}
	}
	if _, err := svc.VerifyReceipt(context.Background(), "tenant-b", granted.ReceiptID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("receipts must stay tenant-scoped, got %v", err)
	}
}

func TestYSQLReceiptHashSurvivesRealRead(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		if strings.EqualFold(os.Getenv("NVUTO_REQUIRE_INTEGRATION"), "true") {
			t.Fatal("TEST_DATABASE_URL (or DATABASE_URL) is required when NVUTO_REQUIRE_INTEGRATION=true")
		}
		t.Skip("TEST_DATABASE_URL / DATABASE_URL not configured")
	}
	repo, err := store.NewPostgres(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repo.Close() })
	svc := New(repo)
	tenantID := fmt.Sprintf("tenant-ysql-rcp-%d", time.Now().UnixNano())
	now := time.Now().UTC()
	installPolicyFixture(t, svc, tenantID)

	evidence := domain.Evidence{
		Method:       "integration",
		ArtifactHash: "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
		CollectedAt:  now,
	}
	consent, err := svc.RecordGoverned(context.Background(), tenantID, "admin", "cus-ysql-rcp", "marketing", "email", "consent", "granted", "portal", nil, nil, "NG", "2026-08", evidence)
	if err != nil {
		t.Fatal(err)
	}
	verification, err := svc.VerifyReceipt(context.Background(), tenantID, consent.ReceiptID)
	if err != nil {
		t.Fatal(err)
	}
	if verification.Status != ReceiptHashVerified || !verification.Trusted() {
		t.Fatalf("receipt %s must verify after a real YSQL read: %+v", consent.ReceiptID, verification)
	}
	receipt, err := svc.Receipt(context.Background(), tenantID, consent.ReceiptID)
	if err != nil {
		t.Fatal(err)
	}
	if v := VerifyReceipt(receipt); v.Status != ReceiptHashVerified {
		t.Fatalf("receipt must verify from entity: %+v", v)
	}
	exported, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	var imported domain.ConsentReceipt
	if err := json.Unmarshal(exported, &imported); err != nil {
		t.Fatal(err)
	}
	if v := VerifyReceipt(imported); v.Status != ReceiptHashVerified {
		t.Fatalf("receipt must verify after JSON export/import: %+v", v)
	}
}
