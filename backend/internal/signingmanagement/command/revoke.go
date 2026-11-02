package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"digital-contracting-service/internal/base/datatype/userrole"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/pdfgeneration/provenance"
	"digital-contracting-service/internal/pdfgeneration/statuspublication"
	"digital-contracting-service/internal/signingmanagement/db"
	signingmanagementevents "digital-contracting-service/internal/signingmanagement/event"
)

// ErrRevocationReasonRequired prevents a revocation without an auditable reason.
var ErrRevocationReasonRequired = errors.New("revocation reason is required")

// NormalizeRevocationReason removes transport whitespace and rejects an empty reason.
func NormalizeRevocationReason(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "", ErrRevocationReasonRequired
	}
	return reason, nil
}

type RevokeCmd struct {
	DID       string
	SignerDID string
	Reason    string
	RevokedBy string
	HolderDID string
	UserRoles userrole.UserRoles
}

type Revoker struct {
	DB    *sqlx.DB
	CRepo db.ContractRepo
}

// Handle revokes the signature. A failed attempt is audited as
// REVOKE_SIGNATURE_FAILED (DCS-FR-SM-19).
func (h *Revoker) Handle(ctx context.Context, cmd RevokeCmd) error {
	err := h.revoke(ctx, cmd)
	if err != nil {
		failed := revokeEvent(cmd, 0, time.Now().UTC())
		failed.Outcome = signingmanagementevents.OutcomeFailed
		failed.Error = err.Error()
		if auditErr := event.CreateInOwnTx(ctx, h.DB, failed, componenttype.SignatureManagement); auditErr != nil {
			log.Printf("could not audit failed revocation of %s: %v", cmd.DID, auditErr)
		}
	}
	return err
}

func revokeEvent(cmd RevokeCmd, contractVersion int, occurredAt time.Time) signingmanagementevents.RevokeEvent {
	return signingmanagementevents.RevokeEvent{
		DID:             cmd.DID,
		ContractVersion: contractVersion,
		SignerDID:       cmd.SignerDID,
		Outcome:         signingmanagementevents.OutcomeCompleted,
		Reason:          cmd.Reason,
		RevokedBy:       cmd.RevokedBy,
		OccurredAt:      occurredAt,
		HolderDID:       cmd.HolderDID,
		UserRoles:       cmd.UserRoles,
	}
}

func (h *Revoker) revoke(ctx context.Context, cmd RevokeCmd) error {
	reason, err := NormalizeRevocationReason(cmd.Reason)
	if err != nil {
		return err
	}
	cmd.Reason = reason

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	// The per-contract lock of the signing path: a revocation waits for a
	// signature in progress and that signature then reads REVOKED.
	if err := acquireRegenerationLock(ctx, tx, cmd.DID); err != nil {
		return err
	}
	processData, err := h.CRepo.ReadProcessDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read process data: %w", err)
	}

	err = h.CRepo.RevokeSignature(ctx, tx, cmd.DID, cmd.SignerDID)
	if err != nil {
		return fmt.Errorf("could not revoke signature: %w", err)
	}

	// Beyond flipping the signature row's own status, revoking a signature
	// transitions the contract's own lifecycle state to REVOKED (C2PA lifecycle
	// banner "suspended", DCS-OR-C2PA-006). The Signed/Active -> Revoked
	// edge is validated against the single-source-of-truth transition table
	// (contractstate.Transitions), analogous to command/apply.go's
	// APPROVED -> SIGNED transition — no hardcoded SQL state literal decides it.
	if err := contractstate.ValidateTransition(contractstate.ContractState(processData.State), contractstate.EventRevoke); err != nil {
		return err
	}
	if err := h.CRepo.UpdateState(ctx, tx, cmd.DID, processData.State, contractstate.Revoked.String()); err != nil {
		return fmt.Errorf("could not update contract state to revoked: %w", err)
	}

	occurredAt := time.Now().UTC()
	evt := revokeEvent(cmd, processData.ContractVersion, occurredAt)
	err = event.Create(ctx, tx, evt, componenttype.SignatureManagement)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}
	status, err := provenance.MapCWEStateToC2PA(contractstate.Revoked.String())
	if err != nil {
		return err
	}
	if err := statuspublication.EnqueueTx(ctx, tx, cmd.DID, status, cmd.Reason, occurredAt); err != nil {
		return err
	}

	return tx.Commit()
}
