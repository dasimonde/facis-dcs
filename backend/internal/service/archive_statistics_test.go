package service

import (
	"testing"

	"digital-contracting-service/internal/base/datatype"
)

func TestAuditEntryActorLifecycleOperations(t *testing.T) {
	for _, raw := range []string{`{"stored_by":"alice"}`, `{"approved_by":"alice"}`, `{"signed_by":"alice"}`} {
		if got := auditEntryActor([]byte(raw)); got != "alice" {
			t.Fatalf("actor for %s = %q", raw, got)
		}
	}
	if got := auditEntryActor([]byte(`{"approved_by":"alice","signed_by":"bob"}`)); got != "" {
		t.Fatalf("ambiguous actor = %q", got)
	}
}

// DCS-FR-PACM-04: the actor of a recent archive action is shown only to a
// request that gives a justification.
func TestRecentArchiveActionsNameTheActorOnlyWithAJustification(t *testing.T) {
	did := "did"
	chains := [][]datatype.AuditLogEntry{{{DID: &did, EventType: "STORE_ARCHIVED_CONTRACT", EventData: []byte(`{"stored_by":"alice"}`)}}}
	if actions := recentArchiveActions(chains, 10, false); actions[0].Actor != nil {
		t.Fatalf("actor without justification = %q", *actions[0].Actor)
	}
	if actions := recentArchiveActions(chains, 10, true); actions[0].Actor == nil || *actions[0].Actor != "alice" {
		t.Fatalf("actor with justification = %v", actions[0].Actor)
	}
}
