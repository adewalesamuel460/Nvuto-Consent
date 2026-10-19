package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEnvelopeIsVersionedCloudEventOnStableSubject(t *testing.T) {
	item := event{ID: "evt-1", SchemaVersion: 1, Source: "nvuto/consent", TenantID: "tenant-a", EventType: "consent.granted", AggregateType: "consent", AggregateID: "con-1", Payload: json.RawMessage(`{"subject_id":"cus-1"}`), OccurredAt: time.Now().UTC()}
	if got := subject("nvuto.consent", item); got != "nvuto.consent.granted" {
		t.Fatalf("unexpected subject %q", got)
	}
	body, err := envelope(item)
	if err != nil {
		t.Fatal(err)
	}
	var output cloudEvent
	if err := json.Unmarshal(body, &output); err != nil {
		t.Fatal(err)
	}
	if output.SpecVersion != "1.0" || output.SchemaVersion != 1 || output.TenantID != "tenant-a" || output.Subject != "consent/con-1" {
		t.Fatalf("incomplete event envelope: %+v", output)
	}
}
