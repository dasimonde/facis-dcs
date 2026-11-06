package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

func TestArchiveIntegrityRuleForError(t *testing.T) {
	tests := map[string]string{
		"content_hash mismatch":                   "ARCHIVE_CONTENT_HASH",
		"fetch archive snapshot from IPFS":        "ARCHIVE_IPFS_SNAPSHOT",
		"stored notary receipt missing":           "ARCHIVE_ORCE_RECEIPT",
		"ORCE previousHash invalid":               "ARCHIVE_ORCE_CHAIN",
		"archive TSA receipt verification failed": "ARCHIVE_TSA_RFC3161",
		"contract_snapshot is empty":              "ARCHIVE_DB_SNAPSHOT",
	}
	for message, want := range tests {
		if got := archiveIntegrityRuleForError(errors.New(message)); got != want {
			t.Errorf("%q: got %s, want %s", message, got, want)
		}
	}
}

// An entry whose content was erased with the contract's key, as a peer's
// erase request leaves it, gets the verdict ERASED for every check that
// needs the content. The notary chain stays verifiable and passes.
func TestArchiveIntegrityReportsAnErasedEntryAsErased(t *testing.T) {
	did := "did:web:dcs.example:contract:erased"
	entry := db.ContractArchiveEntry{DID: did, ContractVersion: 1, ContractSnapshot: datatype.JSON(`{}`), ContentHash: "sha256:" + zeroHash, SnapshotCID: "bafy-erased"}
	storeEvents := []datatype.AuditLogEntry{{ID: 7, EventType: "STORE_ARCHIVED_CONTRACT", DID: &did, EventData: datatype.ErasedEventData}}
	chain := map[string][]archiveNotaryEvent{did + "#1": {{ArchiveEntryID: did + "#1", EventHash: "sha256:" + zeroHash}}}

	findings := (&processAuditAndCompliancesrvc{}).archiveIntegrityTrailEntries(context.Background(), entry, 0, storeEvents, chain, nil)
	results := map[string]string{}
	for _, finding := range findings {
		results[*finding.RuleID] = *finding.Result
	}
	require.Equal(t, map[string]string{
		"ARCHIVE_DB_SNAPSHOT":   "ERASED",
		"ARCHIVE_CONTENT_HASH":  "ERASED",
		"ARCHIVE_IPFS_SNAPSHOT": "ERASED",
		"ARCHIVE_ORCE_RECEIPT":  "ERASED",
		"ARCHIVE_ORCE_CHAIN":    "PASSED",
		"ARCHIVE_TSA_RFC3161":   "ERASED",
	}, results)
}

func TestArchiveIntegrityFindingsNeverPassUnevaluatedChecks(t *testing.T) {
	invalidSnapshot := datatype.JSON(`not-json`)
	entry := db.ContractArchiveEntry{DID: "did:web:damaged", ContractVersion: 1, ContractSnapshot: invalidSnapshot, ContentHash: "broken"}
	service := &processAuditAndCompliancesrvc{}
	findings := service.archiveIntegrityTrailEntries(context.Background(), entry, 0, nil, nil, errors.New("ORCE chain unavailable"))
	if len(findings) != len(archiveIntegrityRules) {
		t.Fatalf("got %d findings", len(findings))
	}
	for _, finding := range findings {
		if finding.Result == nil || *finding.Result != "FAILED" {
			t.Fatalf("false PASSED/unevaluated finding: %+v", finding)
		}
		if finding.Reason == nil || *finding.Reason == "" {
			t.Fatalf("finding has no reason: %+v", finding)
		}
	}
}
