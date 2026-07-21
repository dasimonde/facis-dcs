// This file serves the audit trail's tamper-evidence surface: the checkpoint
// head that external notaries poll, and the inclusion proof that ties one
// entry to a timestamped root (see ADR-16).
package qry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/db"
)

// CheckpointHead is the publishable part of a checkpoint: hashes, counts and a
// trusted timestamp, and nothing derived from the entries themselves. It is
// what an external anchor (ORCE, a notary, a chain) is given. Because every
// root chains to its predecessor, publishing one head transitively commits to
// the entire log before it.
type CheckpointHead struct {
	Seq           int64
	Root          string
	PrevRoot      *string
	LeafCount     int
	CreatedAt     time.Time
	TsaTimestamp  *string
	TimestampedAt *time.Time
}

// CheckpointProof shows that one anchored entry is committed to by a
// timestamped root. The verifier hashes the entry bytes it holds (nonce
// included), walks the sibling path, and compares the result with a root it
// obtained independently — from the external anchor, not from us.
type CheckpointProof struct {
	EntryCID  string
	LeafHash  string
	LeafIndex int
	Siblings  []string
	Head      CheckpointHead
}

type CheckpointAuditor struct {
	DB    *sqlx.DB
	ARepo db.AuditTrailRepository
}

func headOf(record datatype.AuditCheckpointRecord) CheckpointHead {
	return CheckpointHead{
		Seq:           record.Seq,
		Root:          record.Root,
		PrevRoot:      record.PrevRoot,
		LeafCount:     record.LeafCount,
		CreatedAt:     record.CreatedAt,
		TsaTimestamp:  record.TsaSignature,
		TimestampedAt: record.TimestampedAt,
	}
}

// Head returns the newest checkpoint head, or nil when nothing has been
// anchored yet.
func (h *CheckpointAuditor) Head(ctx context.Context) (*CheckpointHead, error) {
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	record, err := h.ARepo.ReadLatestCheckpoint(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("could not read the latest checkpoint: %w", err)
	}
	if record == nil {
		return nil, nil
	}
	head := headOf(*record)
	return &head, nil
}

// Proof builds the inclusion proof for one anchored entry. It verifies the
// proof it just built against the stored root before handing it out, so a
// corrupted leaf index surfaces here rather than at the auditor.
func (h *CheckpointAuditor) Proof(ctx context.Context, entryCID string) (*CheckpointProof, error) {
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	record, leafHashes, index, err := h.ARepo.ReadCheckpointForEntry(ctx, tx, entryCID)
	if err != nil {
		return nil, fmt.Errorf("could not locate the checkpoint of entry %s: %w", entryCID, err)
	}
	if record == nil {
		return nil, nil
	}

	siblings, err := base.MerkleInclusionProof(leafHashes, index)
	if err != nil {
		return nil, fmt.Errorf("could not build the inclusion proof for entry %s: %w", entryCID, err)
	}
	if !base.VerifyMerkleInclusion(leafHashes[index], siblings, index, len(leafHashes), record.Root) {
		return nil, fmt.Errorf("inclusion proof for entry %s does not reproduce checkpoint %d's root", entryCID, record.Seq)
	}

	return &CheckpointProof{
		EntryCID:  entryCID,
		LeafHash:  leafHashes[index],
		LeafIndex: index,
		Siblings:  siblings,
		Head:      headOf(*record),
	}, nil
}
