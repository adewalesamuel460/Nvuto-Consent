package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/service"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
)

func TestConsentAPIIdempotencyKeyReplaysAndConflicts(t *testing.T) {
	t.Setenv("JWT_HMAC_SECRET", "")
	t.Setenv("ALLOW_DEV_HEADERS", "true")
	repo := store.NewMemory()
	svc := service.New(repo)
	now := time.Now().UTC()
	if _, err := svc.RegisterPurpose(context.Background(), "tenant-api", "admin", domain.Purpose{Code: "marketing", Category: "marketing", AllowedChannels: []string{"email"}, RequiresAffirmativeConsent: true, Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RegisterLawfulBasis(context.Background(), "tenant-api", "admin", domain.LawfulBasis{Code: "consent", AllowedCategories: []string{"marketing"}, RequiresEvidence: true, Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PublishNotice(context.Background(), "tenant-api", "admin", domain.PrivacyNotice{Version: "2026-08", Jurisdiction: "NG", PublishedAt: now, Active: true}); err != nil {
		t.Fatal(err)
	}
	handler := newMux(svc)
	body := map[string]any{
		"subject_id": "cus-1", "purpose": "marketing", "channel": "email", "legal_basis": "consent", "status": "granted", "source": "customer360", "jurisdiction": "NG", "notice_version": "2026-08",
		"scope":    map[string]string{"source_record_id": "local-consent-1"},
		"evidence": map[string]any{"method": "preference_center", "artifact_hash": "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", "collected_at": now},
	}
	post := func(payload map[string]any) *httptest.ResponseRecorder {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/v1/consents", bytes.NewReader(encoded))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nvuto-Tenant-ID", "tenant-api")
		req.Header.Set("X-Nvuto-Actor-ID", "actor-api")
		req.Header.Set("Idempotency-Key", "customer360:local-consent-1")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}

	first := post(body)
	if first.Code != http.StatusCreated {
		t.Fatalf("first idempotent consent returned %d: %s", first.Code, first.Body.String())
	}
	second := post(body)
	if second.Code != http.StatusCreated {
		t.Fatalf("exact replay returned %d: %s", second.Code, second.Body.String())
	}
	var firstConsent, secondConsent domain.Consent
	if err := json.Unmarshal(first.Body.Bytes(), &firstConsent); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondConsent); err != nil {
		t.Fatal(err)
	}
	if firstConsent.ID != secondConsent.ID || firstConsent.ReceiptID == "" || firstConsent.ReceiptID != secondConsent.ReceiptID {
		t.Fatalf("HTTP replay did not return original consent/receipt: first=%+v second=%+v", firstConsent, secondConsent)
	}

	changed := map[string]any{}
	for key, value := range body {
		changed[key] = value
	}
	changed["status"] = "denied"
	conflict := post(changed)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("semantic key reuse returned %d, want 409: %s", conflict.Code, conflict.Body.String())
	}
}
