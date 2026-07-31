package command

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

const archiveSnapshotHashAlgorithm = "SHA-256"

type ArchiveSigningEvidence struct {
	Signer, CredentialType, CeremonyID, Field, PDFCID, PDFHash string
	SignedAt                                                   time.Time
	CredentialHashes                                           map[string]string
}

// BuildArchiveEntry freezes the signed contract state for archive
// persistence (DCS-FR-CWE-20): the archive entry is created once the
// signature workflow completes (SIGNED), not at APPROVED.
func BuildArchiveEntry(contract *db.Contract, storedBy string, signing ArchiveSigningEvidence) (db.ContractArchiveEntry, error) {
	if contract == nil {
		return db.ContractArchiveEntry{}, fmt.Errorf("contract is required")
	}
	if contract.State != contractstate.Signed.String() {
		return db.ContractArchiveEntry{}, fmt.Errorf("contract %s must be signed before archive storage", contract.DID)
	}

	snapshotJSON, err := buildContractSnapshot(contract)
	if err != nil {
		return db.ContractArchiveEntry{}, err
	}
	contentHash, err := HashArchiveSnapshot(snapshotJSON)
	if err != nil {
		return db.ContractArchiveEntry{}, err
	}

	signatureMetadata, err := datatype.NewJSON(map[string]any{
		"status": "SIGNED", "signer": signing.Signer, "credential_type": signing.CredentialType,
		"ceremony_id": signing.CeremonyID, "field": signing.Field, "signed_at": signing.SignedAt.UTC().Format(time.RFC3339Nano),
		"pdf_cid": signing.PDFCID, "pdf_hash": "sha256:" + strings.TrimPrefix(signing.PDFHash, "sha256:"),
	})
	if err != nil {
		return db.ContractArchiveEntry{}, err
	}
	credentialHashes, err := datatype.NewJSON(signing.CredentialHashes)
	if err != nil {
		return db.ContractArchiveEntry{}, err
	}
	evidence, err := datatype.NewJSON(map[string]any{
		"source":                  "SIGNING_WORKFLOW_COMPLETION",
		"stored_by":               storedBy,
		"stored_state":            contractstate.Signed.String(),
		"snapshot_hash_algorithm": archiveSnapshotHashAlgorithm,
	})
	if err != nil {
		return db.ContractArchiveEntry{}, err
	}

	return db.ContractArchiveEntry{
		DID:              contract.DID,
		ContractVersion:  contract.ContractVersion,
		StoredBy:         storedBy,
		StoredAt:         time.Now().UTC(),
		ContractSnapshot: snapshotJSON,
		ContentHash:      contentHash,
		SignatureMeta:    &signatureMetadata,
		CredentialHashes: &credentialHashes,
		Evidence:         &evidence,
	}, nil
}

func buildContractSnapshot(contract *db.Contract) (datatype.JSON, error) {
	contractData := json.RawMessage(`{}`)
	if contract.ContractData != nil && contract.ContractData.IsNotNullValue() {
		contractData = json.RawMessage(*contract.ContractData)
	}

	snapshot := map[string]any{
		"did":               contract.DID,
		"contract_version":  contract.ContractVersion,
		"state":             contract.State,
		"name":              stringPtrValue(contract.Name),
		"description":       stringPtrValue(contract.Description),
		"created_by":        contract.CreatedBy,
		"created_at":        formatArchiveTime(&contract.CreatedAt),
		"updated_at":        formatArchiveTime(&contract.UpdatedAt),
		"start_date":        formatArchiveTime(contract.StartDate),
		"exp_date":          formatArchiveTime(contract.ExpDate),
		"exp_policy":        stringPtrValue(contract.ExpPolicy),
		"exp_notice_period": intPtrValue(contract.ExpNoticePeriod),
		"responsible":       contract.Responsible,
		"contract_data":     contractData,
	}

	return datatype.NewJSON(snapshot)
}

func HashArchiveSnapshot(snapshot datatype.JSON) (string, error) {
	canonicalSnapshot, err := CanonicalizeArchiveSnapshot(snapshot)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonicalSnapshot)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func CanonicalizeArchiveSnapshot(snapshot datatype.JSON) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(snapshot))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("decode archive snapshot JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("decode archive snapshot JSON: multiple JSON values")
		}
		return nil, fmt.Errorf("decode archive snapshot JSON: %w", err)
	}

	canonicalSnapshot, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("canonicalize archive snapshot JSON: %w", err)
	}
	return canonicalSnapshot, nil
}

func formatArchiveTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func intPtrValue(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
