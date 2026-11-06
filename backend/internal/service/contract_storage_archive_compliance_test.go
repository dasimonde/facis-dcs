package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype"
	cwecommand "digital-contracting-service/internal/contractworkflowengine/command"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

func complianceJSON(t *testing.T, value any) *datatype.JSON {
	t.Helper()
	j, err := datatype.NewJSON(value)
	require.NoError(t, err)
	return &j
}

func complianceVerdicts(t *testing.T, entry db.ContractArchiveEntry, meta *db.ContractMetadata, now time.Time) map[string]string {
	t.Helper()
	verdicts := map[string]string{}
	for _, check := range archiveComplianceChecks(entry, meta, now) {
		if check.Passed {
			verdicts[check.Rule] = "pass"
			continue
		}
		require.NotNil(t, check.Reason, "a failed rule must say why")
		verdicts[check.Rule] = *check.Reason
	}
	return verdicts
}

// signedArchiveEntry builds the entry the signing workflow stores: field
// did:web:a requires QES, field did:web:b the default AES.
func signedArchiveEntry(t *testing.T, signatures ...cwecommand.ArchiveSigningEvidence) db.ContractArchiveEntry {
	t.Helper()
	data := datatype.JSON(`{"dcs:signatureFields":[{"dcs:signatoryName":"did:web:a","dcs:requiredCredentialType":"QES"},{"dcs:signatoryName":"did:web:b"}]}`)
	entry, err := cwecommand.BuildArchiveEntry(&db.Contract{DID: "did:web:dcs:contract:c", State: "SIGNED", ContractVersion: 1, ContractData: &data},
		nil, "did:web:a", signatures, json.RawMessage(`[{"party":"did:web:peer","signatory":"did:key:peer"}]`))
	require.NoError(t, err)
	entry.TSAReceipt = complianceJSON(t, map[string]any{"token": "t"})
	return entry
}

// DCS-FR-CSA-19: an entry built by the signing workflow passes every rule
// when each signature meets the level its field requires.
func TestCompliantArchiveEntryPassesEveryRule(t *testing.T) {
	now := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	start, end := now.AddDate(-1, 0, 0), now.AddDate(1, 0, 0)
	name := "Supply agreement"
	entry := signedArchiveEntry(t,
		cwecommand.ArchiveSigningEvidence{Signer: "did:key:jane", CredentialType: "QES", CeremonyID: "c1", Field: "did:web:a",
			PDFHash: "abc", SignedAt: now, CredentialHashes: map[string]string{"presentation": "sha256:00"}},
		cwecommand.ArchiveSigningEvidence{Signer: "did:key:john", CredentialType: "AES", CeremonyID: "c2", Field: "did:web:b",
			PDFHash: "abd", SignedAt: now, CredentialHashes: map[string]string{"presentation": "sha256:01"}})
	meta := &db.ContractMetadata{Name: &name, StartDate: &start, ExpDate: &end,
		Responsible: &db.Responsible{Creator: "did:web:a", Counterparty: "did:web:b"}}

	for rule, verdict := range complianceVerdicts(t, entry, meta, now) {
		require.Equal(t, "pass", verdict, rule)
	}
}

// DCS-FR-CSA-19: a signature below the level its field requires, or without
// a recorded level, flags the entry.
func TestArchiveSignatureBelowTheRequiredLevelIsFlagged(t *testing.T) {
	now := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	entry := signedArchiveEntry(t,
		cwecommand.ArchiveSigningEvidence{Signer: "did:key:jane", CredentialType: "AES", CeremonyID: "c1", Field: "did:web:a", PDFHash: "abc", SignedAt: now,
			CredentialHashes: map[string]string{"presentation": "sha256:00"}},
		cwecommand.ArchiveSigningEvidence{Signer: "did:key:john", CeremonyID: "c2", Field: "did:web:b", PDFHash: "abd", SignedAt: now})

	verdict := complianceVerdicts(t, entry, nil, now)["signature_policy"]
	require.Contains(t, verdict, `signature of did:key:jane for "did:web:a" reached AES, the contract requires QES`)
	require.Contains(t, verdict, `signature of did:key:john for "did:web:b" records no credential type`)
}

func TestNonCompliantArchiveEntryIsFlaggedPerRule(t *testing.T) {
	now := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	retention := now.AddDate(0, 0, -1)
	entry := db.ContractArchiveEntry{
		ContractSnapshot: datatype.JSON(`{"did":"x"}`),
		ContentHash:      "sha256:" + zeroHash,
		SignatureMeta:    complianceJSON(t, map[string]any{}),
		CredentialHashes: complianceJSON(t, map[string]any{}),
		TSAReceipt:       complianceJSON(t, map[string]any{}),
		RetentionUntil:   &retention,
	}
	meta := &db.ContractMetadata{Responsible: &db.Responsible{Creator: "did:web:a"}}

	verdicts := complianceVerdicts(t, entry, meta, now)
	require.Contains(t, verdicts["retention"], "retention period ended on 2026-10-29")
	require.Contains(t, verdicts["signature_policy"], "no completed signature recorded")
	require.Contains(t, verdicts["signature_policy"], "no signer credential hashes recorded")
	require.Equal(t, "missing name, parties, validity period", verdicts["metadata_completeness"])
	require.Equal(t, "no TSA receipt", verdicts["evidence_completeness"])
}
