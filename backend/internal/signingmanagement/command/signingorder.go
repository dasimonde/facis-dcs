package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/signingmanagement/db"
)

// ErrSigningOutOfOrder refuses a signature for a field whose position in the
// contract's signing order comes after a field that is not signed yet
// (DCS-FR-SM-07, DCS-FR-SM-13).
var ErrSigningOutOfOrder = errors.New("the contract's signing order requires other signature fields to be signed first")

// ErrSigningDeadlinePassed refuses a signature after the contract's signing
// deadline (DCS-FR-SM-13).
var ErrSigningDeadlinePassed = errors.New("the contract's signing deadline has passed")

// assertSigningDeadline refuses when the contract states a signing deadline
// and now is after it.
func assertSigningDeadline(contractData []byte, now time.Time) error {
	deadline, ok := validation.SigningDeadline(contractData)
	if ok && now.After(deadline) {
		return fmt.Errorf("%w: the deadline was %s", ErrSigningDeadlinePassed, deadline.Format(time.RFC3339))
	}
	return nil
}

// assertSigningOrder refuses field while a field with a lower position in the
// contract's signing order is not in signed. A field without a position is
// not ordered.
func assertSigningOrder(contractData []byte, field string, signed map[string]bool) error {
	order := validation.SigningOrder(contractData)
	position, ok := order[field]
	if !ok {
		return nil
	}
	var waiting []string
	for other, otherPosition := range order {
		if otherPosition < position && !signed[other] {
			waiting = append(waiting, other)
		}
	}
	if len(waiting) == 0 {
		return nil
	}
	slices.Sort(waiting)
	return fmt.Errorf("%w: %q (position %d) waits for %s", ErrSigningOutOfOrder, field, position, strings.Join(waiting, ", "))
}

// signedFieldSet returns the fields this instance holds a signature for: a
// local SIGNED record, or the peer signature recorded when the counterparty's
// signed PDF arrived (dcstodcs.PeerSignatures).
func signedFieldSet(ctx context.Context, tx *sqlx.Tx, repo db.ContractRepo, did string, records []db.SignatureRecord) (map[string]bool, error) {
	signed := map[string]bool{}
	for _, rec := range records {
		if rec.Status == "SIGNED" && rec.FieldName != nil {
			signed[*rec.FieldName] = true
		}
	}
	raw, err := repo.ReadPeerSignatureEvidence(ctx, tx, did)
	if err != nil {
		return nil, fmt.Errorf("could not read the peer signatures of %s: %w", did, err)
	}
	if len(raw) == 0 {
		return signed, nil
	}
	var peers []struct {
		Party string `json:"party"`
	}
	if err := json.Unmarshal(raw, &peers); err != nil {
		return nil, fmt.Errorf("decode peer signature evidence: %w", err)
	}
	for _, peer := range peers {
		if party := strings.TrimSpace(peer.Party); party != "" {
			signed[party] = true
		}
	}
	return signed, nil
}

// assertSigningWorkflow applies the contract's signing deadline and signing
// order to a signature for field.
func assertSigningWorkflow(ctx context.Context, tx *sqlx.Tx, repo db.ContractRepo, did string, contractData []byte, field string, records []db.SignatureRecord) error {
	if err := assertSigningDeadline(contractData, time.Now().UTC()); err != nil {
		return err
	}
	if len(validation.SigningOrder(contractData)) == 0 {
		return nil
	}
	signed, err := signedFieldSet(ctx, tx, repo, did, records)
	if err != nil {
		return err
	}
	return assertSigningOrder(contractData, field, signed)
}
