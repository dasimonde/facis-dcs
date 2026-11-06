package pg

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

type PostgresNegotiationRepo struct {
}

func (r PostgresNegotiationRepo) Create(ctx context.Context, tx *sqlx.Tx, data db.NegotiationCreateData, negotiators []string) (string, *time.Time, error) {
	statement := `
        INSERT INTO contract_negotiations (
            did, contract_version, change_request, created_by
        ) VALUES ($1, $2, $3, $4)
        RETURNING id, created_at
    `

	var result struct {
		ID        string    `db:"id"`
		CreatedAt time.Time `db:"created_at"`
	}
	err := tx.GetContext(ctx, &result, statement,
		data.DID, data.ContractVersion, data.ChangeRequest, data.CreatedBy)
	if err != nil {
		return "", nil, err
	}

	for _, negotiator := range negotiators {
		decisionStatement := `
            INSERT INTO contract_negotiation_decisions (
                negotiation_id, negotiator
            ) VALUES ($1, $2)
        `
		_, err = tx.ExecContext(ctx, decisionStatement, result.ID, negotiator)
		if err != nil {
			return "", nil, err
		}
	}

	return result.ID, &result.CreatedAt, nil
}

func (r PostgresNegotiationRepo) ReadDecisionID(ctx context.Context, tx *sqlx.Tx, negotiationID, negotiator string) (string, error) {
	query := `
        SELECT id FROM contract_negotiation_decisions
        WHERE negotiation_id = $1 AND negotiator = $2
    `
	var id string
	err := tx.GetContext(ctx, &id, query, negotiationID, negotiator)

	if errors.Is(err, sql.ErrNoRows) {
		return "", db.ErrNoMatchingDecision
	}

	return id, err
}

func (r PostgresNegotiationRepo) ReadNegotiationByID(ctx context.Context, tx *sqlx.Tx, id string) (*db.NegotiationData, error) {
	query := `
        SELECT id, did, contract_version, change_request, created_by, created_at, superseded_by
        FROM contract_negotiations
        WHERE id = $1
    `

	var row db.NegotiationData
	err := tx.GetContext(ctx, &row, query, id)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return &row, nil
}

func (r PostgresNegotiationRepo) ReadDecisionByID(ctx context.Context, tx *sqlx.Tx, id string) (*db.NegotiationDecisionData, error) {
	query := `
        SELECT id, negotiation_id, negotiator, decision, rejection_reason
        FROM contract_negotiation_decisions
        WHERE id = $1
    `

	var row db.NegotiationDecisionData
	err := tx.GetContext(ctx, &row, query, id)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return &row, nil
}

func (r PostgresNegotiationRepo) UpsertDraft(ctx context.Context, tx *sqlx.Tx, contractDID string, savedBy string, changeRequest *datatype.JSON) error {
	statement := `
        INSERT INTO contract_negotiation_drafts (contract_did, saved_by, change_request)
        VALUES ($1, $2, $3)
        ON CONFLICT (contract_did, saved_by)
        DO UPDATE SET change_request = EXCLUDED.change_request, updated_at = CURRENT_TIMESTAMP
    `
	_, err := tx.ExecContext(ctx, statement, contractDID, savedBy, changeRequest)
	return err
}

func (r PostgresNegotiationRepo) ReadDraft(ctx context.Context, tx *sqlx.Tx, contractDID string, savedBy string) (*db.NegotiationDraftData, error) {
	statement := `
        SELECT contract_did, saved_by, change_request, updated_at
        FROM contract_negotiation_drafts
        WHERE contract_did = $1 AND saved_by = $2
    `
	var draft db.NegotiationDraftData
	err := tx.GetContext(ctx, &draft, statement, contractDID, savedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &draft, nil
}

func (r PostgresNegotiationRepo) DeleteDraft(ctx context.Context, tx *sqlx.Tx, contractDID string, savedBy string) error {
	statement := `
        DELETE FROM contract_negotiation_drafts
        WHERE contract_did = $1 AND saved_by = $2
    `
	_, err := tx.ExecContext(ctx, statement, contractDID, savedBy)
	return err
}

func (r PostgresNegotiationRepo) Accept(ctx context.Context, tx *sqlx.Tx, id string, acceptedBy string) error {
	statement := `
        UPDATE contract_negotiation_decisions cnd
        SET decision = 'ACCEPTED'
        FROM contract_negotiations cn
        WHERE
            cn.id = cnd.negotiation_id AND
            cn.id = $1 AND
            decision IS NULL AND
            negotiator = $2
    `
	result, err := tx.ExecContext(ctx, statement, id, acceptedBy)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return db.ErrNoMatchingDecision
	}

	return nil
}

func (r PostgresNegotiationRepo) Reject(ctx context.Context, tx *sqlx.Tx, id string, rejectedBy string, rejectionReason *string) error {
	statement := `
        UPDATE contract_negotiation_decisions cnd
        SET
            decision = CASE
                WHEN negotiator = $2 THEN 'REJECTED'::contract_negotiation_decision
        		ELSE 'CLOSED'::contract_negotiation_decision
            END,
            rejection_reason = CASE
                WHEN negotiator = $2 THEN $3
            END
        FROM contract_negotiations cn
        WHERE cn.id = cnd.negotiation_id 
          AND cn.id = $1
          AND cnd.decision IS NULL
    `
	result, err := tx.ExecContext(ctx, statement, id, rejectedBy, rejectionReason)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return db.ErrNoMatchingDecision
	}

	return nil
}

func (r PostgresNegotiationRepo) DeleteByID(ctx context.Context, tx *sqlx.Tx, id string) error {
	statement := `DELETE FROM contract_negotiations WHERE id = $1`
	_, err := tx.ExecContext(ctx, statement, id)

	return err
}

func (r PostgresNegotiationRepo) HasAcceptedDecision(ctx context.Context, tx *sqlx.Tx, negotiationID string) (bool, error) {
	query := `
        SELECT EXISTS (
            SELECT 1 FROM contract_negotiation_decisions
            WHERE negotiation_id = $1 AND decision = 'ACCEPTED'
        )
    `
	var exists bool
	err := tx.GetContext(ctx, &exists, query, negotiationID)

	return exists, err
}

func (r PostgresNegotiationRepo) RecordWithdrawn(ctx context.Context, tx *sqlx.Tx, negotiationID, did string) error {
	statement := `
        INSERT INTO contract_negotiation_withdrawn (negotiation_id, did)
        VALUES ($1, $2)
        ON CONFLICT (negotiation_id) DO NOTHING
    `
	_, err := tx.ExecContext(ctx, statement, negotiationID, did)

	return err
}

func (r PostgresNegotiationRepo) IsWithdrawn(ctx context.Context, tx *sqlx.Tx, negotiationID string) (bool, error) {
	query := `SELECT EXISTS (SELECT 1 FROM contract_negotiation_withdrawn WHERE negotiation_id = $1)`
	var exists bool
	err := tx.GetContext(ctx, &exists, query, negotiationID)

	return exists, err
}

// ReadCreatedByByNegotiationID returns the created_by of the
// contract_negotiations row identified by id — the individual/organization
// identity that authored the change_request, used by the conflict-of-
// interest check (FR-CWE-07: a reviewer may not approve their own redline
// proposal, see command.NegotiationAcceptor.Handle).
func (r PostgresNegotiationRepo) ReadCreatedByByNegotiationID(ctx context.Context, tx *sqlx.Tx, id string) (string, error) {
	query := `SELECT created_by FROM contract_negotiations WHERE id = $1`
	var createdBy string
	err := tx.GetContext(ctx, &createdBy, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", db.ErrNoMatchingDecision
		}
		return "", err
	}
	return createdBy, nil
}

func (r PostgresNegotiationRepo) ReadAllByContractDID(ctx context.Context, tx *sqlx.Tx, did string) ([]db.NegotiationData, error) {
	// Include negotiations even when no local decision row exists yet, so a
	// peer-received Proposal is still returned (otherwise retrieve omitted it
	// and the Proposals tab looked like the peer was missing data).
	query := `
        SELECT cn.id, cn.did, cn.contract_version, cn.change_request, cnd.negotiator, cnd.decision,
               cnd.rejection_reason, cn.created_by, cn.created_at, cn.superseded_by
        FROM contract_negotiations cn
            LEFT JOIN contract_negotiation_decisions cnd ON cnd.negotiation_id = cn.id
            WHERE cn.did = $1
    `
	var negotiations []db.NegotiationData
	err := tx.SelectContext(ctx, &negotiations, query, did)
	if err != nil {
		return nil, err
	}
	return negotiations, nil
}

func (r PostgresNegotiationRepo) ReadAllAcceptedByContractDIDAndVersion(ctx context.Context, tx *sqlx.Tx, did string, contractVersion int) ([]db.NegotiationChangeData, error) {
	// Without ORDER BY the row order is a GROUP BY artefact — a planner
	// detail — and the merge that folds these rows is last-write-wins per
	// field, so it would resolve conflicts differently run to run.
	// (created_at, id) is a total order: CURRENT_TIMESTAMP is the transaction
	// clock, so requests proposed in one transaction share a created_at and
	// the primary key separates them.
	// Author instance rows stay NULL (FR-CWE-07 refuses self-accept). Require at
	// least one ACCEPTED and no REJECTED so a peer Accept still folds on Submit.
	query := `
        SELECT cn.id, cn.change_request, cn.created_at
		FROM contract_negotiations cn
		JOIN contract_negotiation_decisions cnd ON cnd.negotiation_id = cn.id
		WHERE cn.did = $1
		  AND cn.contract_version = $2
		GROUP BY cn.id, cn.change_request, cn.created_at
		HAVING COUNT(*) FILTER (WHERE cnd.decision = 'REJECTED') = 0
		   AND COUNT(*) FILTER (WHERE cnd.decision = 'ACCEPTED') > 0
		ORDER BY cn.created_at, cn.id
    `
	var negotiations []db.NegotiationChangeData
	err := tx.SelectContext(ctx, &negotiations, query, did, contractVersion)
	if err != nil {
		return nil, err
	}
	return negotiations, nil
}

func (r PostgresNegotiationRepo) HasOpenNegotiationDecisions(ctx context.Context, tx *sqlx.Tx, did string, contractVersion int, negotiator string, caller string) (bool, error) {
	// Submit needs every proposal handled. Count unresolved nulls:
	//  - we still owe Accept/Reject on someone else's proposal, or
	//  - our proposal still waits on a counterparty decision.
	// Author "Not required" nulls stay out: FR-CWE-07 refuses self-accept, so
	// (created_by = caller AND negotiator = us) and the mirrored author slot on
	// the peer (created_by ≠ caller AND negotiator ≠ us) must not block Submit.
	query := `
        SELECT EXISTS (
            SELECT 1
            FROM contract_negotiations cn
            JOIN contract_negotiation_decisions cnd ON cnd.negotiation_id = cn.id
            WHERE cn.did = $1
              AND cn.contract_version = $2
              AND cnd.decision IS NULL
              AND (
                    (cn.created_by IS DISTINCT FROM $4 AND cnd.negotiator = $3)
                 OR (cn.created_by = $4 AND cnd.negotiator IS DISTINCT FROM $3)
              )
        )
    `
	var exists bool
	err := tx.GetContext(ctx, &exists, query, did, contractVersion, negotiator, caller)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (r PostgresNegotiationRepo) HasNegotiationForContractVersion(ctx context.Context, tx *sqlx.Tx, did string, contractVersion int) (bool, error) {

	query := `
        SELECT EXISTS (
            SELECT 1
            FROM contract_negotiations cn
            JOIN contract_negotiation_decisions cnd ON cnd.negotiation_id = cn.id
            WHERE cn.did = $1
              AND contract_version = $2
        )
    `
	var exists bool
	err := tx.GetContext(ctx, &exists, query, did, contractVersion)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (r PostgresNegotiationRepo) Delete(ctx context.Context, tx *sqlx.Tx, did string) error {
	statement := `
        DELETE FROM contract_review_task
        WHERE did = $1
    `
	_, err := tx.ExecContext(ctx, statement, did)
	return err
}

func (r PostgresNegotiationRepo) ReadAllNegotiationDecisionsByContractDID(ctx context.Context, tx *sqlx.Tx, did string) ([]db.NegotiationDecisionData, error) {
	query := `
        SELECT cnd.id, negotiation_id, negotiator, decision, rejection_reason
        FROM contract_negotiations cn
            JOIN contract_negotiation_decisions cnd ON cnd.negotiation_id = cn.id
            WHERE cn.did = $1
    `
	var decisions []db.NegotiationDecisionData
	err := tx.SelectContext(ctx, &decisions, query, did)
	if err != nil {
		return nil, err
	}
	return decisions, nil
}

func (r PostgresNegotiationRepo) MarkSuperseded(ctx context.Context, tx *sqlx.Tx, supersessions []db.NegotiationSupersession) error {
	// One row can lose different fields to different later requests, so the
	// entries are grouped per superseded request and stored as one array.
	byNegotiation := map[string][]db.NegotiationSupersession{}
	order := make([]string, 0, len(supersessions))
	for _, supersession := range supersessions {
		if _, seen := byNegotiation[supersession.NegotiationID]; !seen {
			order = append(order, supersession.NegotiationID)
		}
		byNegotiation[supersession.NegotiationID] = append(byNegotiation[supersession.NegotiationID], supersession)
	}

	statement := `
        UPDATE contract_negotiations
        SET superseded_by = $2
        WHERE id = $1
    `
	for _, negotiationID := range order {
		annotation, err := json.Marshal(byNegotiation[negotiationID])
		if err != nil {
			return fmt.Errorf("could not marshal supersession record: %w", err)
		}
		result, err := tx.ExecContext(ctx, statement, negotiationID, annotation)
		if err != nil {
			return err
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rowsAffected == 0 {
			return fmt.Errorf("%w: negotiation %s", db.ErrNoMatchingDecision, negotiationID)
		}
	}
	return nil
}

func (r PostgresNegotiationRepo) RemoteCreateOrUpdateNegotiation(ctx context.Context, tx *sqlx.Tx, data db.NegotiationData) error {
	statement := `
        INSERT INTO contract_negotiations (
            id, did, contract_version, change_request, created_by, created_at
        ) VALUES ($1, $2, $3, $4, $5, $6)
        ON CONFLICT (id) DO UPDATE SET
            contract_version = EXCLUDED.contract_version,
            change_request = EXCLUDED.change_request
    `
	_, err := tx.ExecContext(ctx, statement,
		data.ID, data.DID, data.ContractVersion, data.ChangeRequest, data.CreatedBy, data.CreatedAt)
	return err
}

func (r PostgresNegotiationRepo) RemoteCreateOrUpdateNegotiationDecision(ctx context.Context, tx *sqlx.Tx, data db.NegotiationDecisionData) error {
	statement := `
        INSERT INTO contract_negotiation_decisions (
            id, negotiation_id, negotiator, decision, rejection_reason
        ) VALUES ($1, $2, $3, $4, $5)
        ON CONFLICT (id) DO UPDATE SET
            decision = EXCLUDED.decision,
            rejection_reason = EXCLUDED.rejection_reason
    `
	_, err := tx.ExecContext(ctx, statement,
		data.ID, data.NegotiationID, data.Negotiator, data.Decision, data.RejectionReason)
	return err
}

func (r PostgresNegotiationRepo) EnsureOpenDecision(ctx context.Context, tx *sqlx.Tx, negotiationID, negotiator string) error {
	_, err := r.ReadDecisionID(ctx, tx, negotiationID, negotiator)
	if err == nil {
		return nil
	}

	if !errors.Is(err, db.ErrNoMatchingDecision) {
		return err
	}

	return r.RemoteCreateOrUpdateNegotiationDecision(ctx, tx, db.NegotiationDecisionData{
		ID:            uuid.NewString(),
		NegotiationID: negotiationID,
		Negotiator:    negotiator,
	})
}
