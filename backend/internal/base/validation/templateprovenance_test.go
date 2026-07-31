package validation

import (
	"encoding/json"
	"testing"
	"time"

	"digital-contracting-service/internal/base/datatype"

	"github.com/stretchr/testify/require"
)

func TestAuditTemplateApprovalProvenanceAcceptsValidLifecycle(t *testing.T) {
	did := "did:facis:template:001"
	entries := newestFirstTemplateEntries(
		templateAuditEntry(1, did, "CREATE_CONTRACT_TEMPLATE", map[string]any{"did": did, "created_by": "alice"}),
		templateAuditEntry(2, did, "SUBMIT_CONTRACT_TEMPLATE", map[string]any{
			"did":            did,
			"previous_state": "DRAFT",
			"new_state":      "SUBMITTED",
			"submitted_by":   "alice",
			"responsible_persons": map[string]any{
				"Creator":   "alice",
				"Reviewers": []any{"bob"},
				"Approver":  "carol",
			},
		}),
		templateAuditEntry(3, did, "VERIFY_CONTRACT_TEMPLATE", map[string]any{"did": did, "verified_by": "bob"}),
		templateAuditEntry(4, did, "SUBMIT_CONTRACT_TEMPLATE", map[string]any{"did": did, "previous_state": "SUBMITTED", "new_state": "REVIEWED", "submitted_by": "bob"}),
		templateAuditEntry(5, did, "APPROVE_CONTRACT_TEMPLATE", map[string]any{"did": did, "approved_by": "carol"}),
		templateAuditEntry(6, did, "REGISTER_CONTRACT_TEMPLATE", map[string]any{"did": did, "registered_by": "alice"}),
	)

	findings := AuditTemplateApprovalProvenance(did, entries)

	require.True(t, hasFindingSeverity(findings, "FACIS-TPL-PROV-000", SeveritySatisfied))
	require.False(t, hasFindingSeverity(findings, "FACIS-TPL-PROV-004", "error"))
}

func TestAuditTemplateApprovalProvenanceFlagsApprovalBeforeReview(t *testing.T) {
	did := "did:facis:template:001"
	entries := newestFirstTemplateEntries(
		templateAuditEntry(1, did, "CREATE_CONTRACT_TEMPLATE", map[string]any{"did": did, "created_by": "alice"}),
		templateAuditEntry(2, did, "SUBMIT_CONTRACT_TEMPLATE", map[string]any{"did": did, "previous_state": "DRAFT", "new_state": "SUBMITTED", "submitted_by": "alice"}),
		templateAuditEntry(3, did, "APPROVE_CONTRACT_TEMPLATE", map[string]any{"did": did, "approved_by": "carol"}),
	)

	findings := AuditTemplateApprovalProvenance(did, entries)

	require.True(t, hasFindingSeverity(findings, "FACIS-TPL-PROV-004", "error"))
}

func TestAuditTemplateApprovalProvenanceFlagsWrongApproverAndDidMismatch(t *testing.T) {
	did := "did:facis:template:001"
	entries := newestFirstTemplateEntries(
		templateAuditEntry(1, did, "CREATE_CONTRACT_TEMPLATE", map[string]any{"did": did, "created_by": "alice"}),
		templateAuditEntry(2, did, "SUBMIT_CONTRACT_TEMPLATE", map[string]any{
			"did":            did,
			"previous_state": "DRAFT",
			"new_state":      "SUBMITTED",
			"submitted_by":   "alice",
			"responsible_persons": map[string]any{
				"Creator":   "alice",
				"Reviewers": []any{"bob"},
				"Approver":  "carol",
			},
		}),
		templateAuditEntry(3, did, "SUBMIT_CONTRACT_TEMPLATE", map[string]any{"did": did, "previous_state": "SUBMITTED", "new_state": "REVIEWED", "submitted_by": "bob"}),
		templateAuditEntry(4, did, "APPROVE_CONTRACT_TEMPLATE", map[string]any{"did": "did:facis:template:other", "approved_by": "mallory"}),
	)

	findings := AuditTemplateApprovalProvenance(did, entries)

	require.True(t, hasFindingSeverity(findings, "FACIS-TPL-PROV-002", "error"))
	require.True(t, hasFindingSeverity(findings, "FACIS-TPL-PROV-007", "error"))
}

func newestFirstTemplateEntries(entries ...datatype.AuditLogEntry) []datatype.AuditLogEntry {
	result := make([]datatype.AuditLogEntry, len(entries))
	copy(result, entries)
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}

func templateAuditEntry(id int64, did string, eventType string, eventData map[string]any) datatype.AuditLogEntry {
	bytes, err := json.Marshal(eventData)
	if err != nil {
		panic(err)
	}
	return datatype.AuditLogEntry{
		ID:        id,
		Component: "CONTRACT_TEMPLATE_REPO",
		EventType: eventType,
		EventData: bytes,
		DID:       &did,
		CreatedAt: time.Date(2026, 5, 20, 12, int(id), 0, 0, time.UTC),
	}
}
