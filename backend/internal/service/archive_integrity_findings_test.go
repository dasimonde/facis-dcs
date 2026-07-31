package service

import (
	"context"
	"errors"
	"testing"

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
