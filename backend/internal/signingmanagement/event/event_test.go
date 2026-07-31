package event

import (
	"encoding/json"
	"testing"
)

func TestRevokeEventJSONRecordsReason(t *testing.T) {
	t.Parallel()

	body, err := json.Marshal(RevokeEvent{
		DID:    "did:web:example.test:contracts:1",
		Reason: "Signer credential compromised",
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var eventBody map[string]any
	if err := json.Unmarshal(body, &eventBody); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got := eventBody["reason"]; got != "Signer credential compromised" {
		t.Fatalf("reason = %#v, want %q", got, "Signer credential compromised")
	}
}
