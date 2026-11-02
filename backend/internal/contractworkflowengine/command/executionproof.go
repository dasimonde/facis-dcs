package command

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
)

// ExecutionProofRecorder writes the proof of contract execution over every
// signature (DCS-FR-SM-10) when the last counterparty signature arrives after
// this instance archived the contract. The archive entry proves the
// signatures this instance held when its own signing completed; this proof
// adds the counterparty signatures and the fully signed PDF, and is notarized
// and timestamped like the archive entry.
type ExecutionProofRecorder struct {
	CRepo  db.ContractRepo
	Storer ArchiveSnapshotStorer
	Notary ArchiveNotary
	TSA    ArchiveTimestampIssuer
}

// ExecutionProofInput is the counterparty signature that just arrived.
type ExecutionProofInput struct {
	DID       string
	LocalPeer string
	// PeerDID is the counterparty whose signature arrived, and PeerJAdES its
	// JAdES over the contract payload.
	PeerDID   string
	PeerJAdES string
	// SignedPDF is the PDF that carries every signature.
	SignedPDF []byte
	StoredBy  string
}

// Record stores the proof once every declared signature field is signed and
// an archive entry exists. It returns false when no proof was due or the same
// proof is already stored.
func (r ExecutionProofRecorder) Record(ctx context.Context, tx *sqlx.Tx, in ExecutionProofInput) (bool, error) {
	contract, err := r.CRepo.ReadDataByDID(ctx, tx, in.DID)
	if err != nil {
		return false, fmt.Errorf("read contract %s for its execution proof: %w", in.DID, err)
	}
	if contract.ContractData == nil || !contract.ContractData.IsNotNullValue() {
		return false, nil
	}
	declared := validation.RequiredSignatureFields(*contract.ContractData)
	if len(declared) == 0 {
		return false, nil
	}
	signedLocally, err := r.CRepo.ReadSignedSignatureFieldNames(ctx, tx, in.DID)
	if err != nil {
		return false, fmt.Errorf("read the signed fields of %s: %w", in.DID, err)
	}
	evidence := contractstate.SignatureEvidence{Declared: declared, SignedLocally: signedLocally, LocalPeer: in.LocalPeer, PeerSigners: []string{in.PeerDID}}
	if contract.Responsible != nil {
		evidence.Parties = []string{contract.Responsible.Creator, contract.Responsible.Counterparty}
	}
	if !evidence.AllSignaturesCollected() {
		return false, nil
	}
	entry, err := r.CRepo.ReadLatestArchiveEntry(ctx, tx, in.DID)
	if err != nil {
		return false, fmt.Errorf("read the archive entry of %s: %w", in.DID, err)
	}
	if entry == nil {
		// The archive entry, written when this instance's signing completes,
		// then already holds the counterparty signature.
		return false, nil
	}

	var metadata struct {
		Signatures     json.RawMessage `json:"signatures"`
		PeerSignatures json.RawMessage `json:"peer_signatures"`
	}
	if entry.SignatureMeta != nil {
		if err := json.Unmarshal(*entry.SignatureMeta, &metadata); err != nil {
			return false, fmt.Errorf("decode the archived signatures of %s: %w", in.DID, err)
		}
	}
	pdfSum := sha256.Sum256(in.SignedPDF)
	jadesSum := sha256.Sum256([]byte(in.PeerJAdES))
	proof, err := datatype.NewJSON(map[string]any{
		"did":                  in.DID,
		"archive_content_hash": entry.ContentHash,
		"signatures":           metadata.Signatures,
		"peer_signatures":      metadata.PeerSignatures,
		"peer_signature":       map[string]any{"signer": in.PeerDID, "jades_hash": "sha256:" + hex.EncodeToString(jadesSum[:])},
		"pdf_hash":             "sha256:" + hex.EncodeToString(pdfSum[:]),
	})
	if err != nil {
		return false, err
	}
	contentHash, err := HashArchiveSnapshot(proof)
	if err != nil {
		return false, err
	}

	storedAt := time.Now().UTC()
	record := db.ExecutionProof{DID: in.DID, ContentHash: contentHash, Proof: proof}
	var notaryReceipt *contractevents.ArchiveNotaryReceipt
	var tsaReceipt *contractevents.ArchiveTSAReceipt
	if r.Notary != nil {
		if r.Storer == nil {
			return false, fmt.Errorf("an execution proof of %s needs a store to notarize it", in.DID)
		}
		proofCID, err := r.Storer.Put(ctx, artifactstore.ContractScope(in.DID), proof)
		if err != nil {
			return false, fmt.Errorf("store the execution proof of %s: %w", in.DID, err)
		}
		var tsaJSON *datatype.JSON
		notaryReceipt, tsaReceipt, tsaJSON, err = notarize(ctx, r.Notary, r.TSA, ArchiveNotaryPayload{
			EventType: "EXECUTION_PROOF", ArchiveEntryID: in.DID + "#execution#" + contentHash, DID: in.DID,
			ContractVersion: entry.ContractVersion, ContentHash: contentHash, SnapshotCID: proofCID,
			StoredBy: in.StoredBy, StoredAt: storedAt,
		})
		if err != nil {
			return false, err
		}
		receiptJSON, err := datatype.NewJSON(notaryReceipt)
		if err != nil {
			return false, err
		}
		record.NotaryReceipt, record.TSAReceipt = &receiptJSON, tsaJSON
	}
	stored, err := r.CRepo.StoreExecutionProof(ctx, tx, record)
	if err != nil || !stored {
		return false, err
	}
	return true, event.Create(ctx, tx, contractevents.StoreExecutionProofEvent{
		DID: in.DID, ContentHash: contentHash, ArchiveContentHash: entry.ContentHash, PDFHash: "sha256:" + hex.EncodeToString(pdfSum[:]),
		NotaryReceipt: notaryReceipt, TSAReceipt: tsaReceipt, OccurredAt: storedAt,
	}, componenttype.ContractStorageArchive)
}
