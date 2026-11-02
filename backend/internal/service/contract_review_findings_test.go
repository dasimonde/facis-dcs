package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/datatype/actionflag"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
)

func submitEntry(t *testing.T, evt contractevents.SubmitEvent) datatype.AuditLogEntry {
	t.Helper()
	data, err := json.Marshal(evt)
	require.NoError(t, err)
	return datatype.AuditLogEntry{EventType: evt.EventType(), EventData: data}
}

// DCS-IR-CWE-08, DCS-FR-CWE-25: the approver reads the reviewer decisions with
// their comments and block annotations, newest first. Submits of other states
// carry no reviewer finding and are left out.
func TestApproverReadsTheLatestReviewFindings(t *testing.T) {
	approval, reject := actionflag.Approval, actionflag.Reject
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	entries := []datatype.AuditLogEntry{
		submitEntry(t, contractevents.SubmitEvent{DID: "c", PreviousState: "NEGOTIATION", NewState: "SUBMITTED", SubmittedBy: "Org A", OccurredAt: at, Comments: []string{"not a finding"}}),
		submitEntry(t, contractevents.SubmitEvent{DID: "c", PreviousState: "SUBMITTED", NewState: "NEGOTIATION", SubmittedBy: "Org A", HolderDID: "did:key:reviewer", OccurredAt: at.Add(time.Hour), ContractVersion: 1, ActionFlag: &reject, Comments: []string{"clause 3 is unclear"}, Annotations: []contractevents.BlockAnnotation{{BlockID: "c#block-3", Comment: "rephrase"}}}),
		submitEntry(t, contractevents.SubmitEvent{DID: "c", PreviousState: "SUBMITTED", NewState: "REVIEWED", SubmittedBy: "Org A", OccurredAt: at.Add(2 * time.Hour), ContractVersion: 2, ActionFlag: &approval, Comments: []string{"fine now"}}),
		{EventType: "APPROVE_CONTRACT", EventData: json.RawMessage(`{"did":"c"}`)},
	}

	findings := reviewFindings(entries)

	require.Len(t, findings, 2)
	require.Equal(t, "APPROVAL", *findings[0].Decision)
	require.Equal(t, []string{"fine now"}, findings[0].Comments)
	require.Empty(t, findings[0].Annotations)
	require.Equal(t, "REJECT", *findings[1].Decision)
	require.Equal(t, 1, findings[1].ContractVersion)
	require.Equal(t, "did:key:reviewer", *findings[1].HolderDid)
	require.Equal(t, []string{"clause 3 is unclear"}, findings[1].Comments)
	require.Equal(t, "c#block-3", findings[1].Annotations[0].BlockID)
	require.Equal(t, "rephrase", findings[1].Annotations[0].Comment)
}
