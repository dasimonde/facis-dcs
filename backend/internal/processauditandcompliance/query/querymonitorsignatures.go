package qry

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"

	"digital-contracting-service/internal/base/validation"
	event2 "digital-contracting-service/internal/processauditandcompliance/event"
)

// RiskTypeLateSignature flags a contract whose start date passed before it
// was fully signed (DCS-FR-PACM-05).
const RiskTypeLateSignature = "LATE_SIGNATURE"

// RiskTypeInvalidCredential flags a signature that relies on a certificate
// the CRL check found revoked (DCS-FR-PACM-05).
const RiskTypeInvalidCredential = "INVALID_CREDENTIAL" // #nosec G101 -- risk type name, not a credential

// RiskTypeSigningDeadlineMissed flags a contract that is not signed although
// the signing deadline its document states has passed (DCS-FR-SM-13).
const RiskTypeSigningDeadlineMissed = "SIGNING_DEADLINE_MISSED"

// RiskTypeSigningConflict flags a contract whose signed copy was refused
// between this instance and a peer, because each instance holds a signature
// over the same revision (ADR-38 §2).
const RiskTypeSigningConflict = "SIGNING_CONFLICT"

// unsignedStates are the states of a contract on its way to signature.
var unsignedStates = []string{"OFFERED", "NEGOTIATION", "SUBMITTED", "REVIEWED", "APPROVED"}

// lateSignature is a contract that waits for signatures after its start date.
// When the last signature arrives the risk resolves; the register keeps it.
type lateSignature struct {
	DID       string    `db:"did"`
	StartDate time.Time `db:"start_date"`
}

type revokedCredential struct {
	DID       string    `db:"did"`
	Signer    string    `db:"signer_did"`
	Field     string    `db:"field_name"`
	RevokedAt time.Time `db:"cert_revoked_at"`
}

// signatureRisks reads the contracts that wait for signatures after their
// start date and the signatures on revoked certificates. The details name
// only persisted facts, so a risk hashes the same on every sweep.
func signatureRisks(ctx context.Context, tx *sqlx.Tx, checkedAt time.Time) ([]event2.ComplianceRisk, error) {
	var late []lateSignature
	if err := tx.SelectContext(ctx, &late, `
        SELECT did, start_date FROM contracts_effective
        WHERE state = 'APPROVED' AND start_date < $1
        ORDER BY did`, checkedAt); err != nil {
		return nil, fmt.Errorf("could not read late signatures: %w", err)
	}
	var revoked []revokedCredential
	if err := tx.SelectContext(ctx, &revoked, `
        SELECT contract_did AS did, signer_did, COALESCE(field_name, '') AS field_name, cert_revoked_at
        FROM contract_signatures
        WHERE status = 'SIGNED' AND cert_revoked_at IS NOT NULL
        ORDER BY id`); err != nil {
		return nil, fmt.Errorf("could not read signatures on revoked certificates: %w", err)
	}
	var unsigned []activeContract
	if err := tx.SelectContext(ctx, &unsigned, `
        SELECT did, contract_data FROM contracts_effective
        WHERE state = ANY($1) AND contract_data IS NOT NULL
        ORDER BY did`, pq.Array(unsignedStates)); err != nil {
		return nil, fmt.Errorf("could not read contracts waiting for signatures: %w", err)
	}
	var conflicts []signingConflict
	if err := tx.SelectContext(ctx, &conflicts, `
        SELECT did, event_data ->> 'peer_did' AS peer_did, event_data ->> 'direction' AS direction
        FROM outbox_events
        WHERE event_type = $1
        ORDER BY id`, event2.SigningConflictEvent{}.EventType()); err != nil {
		return nil, fmt.Errorf("could not read signing conflicts: %w", err)
	}
	risks := signatureRisksFrom(late, revoked, checkedAt)
	risks = append(risks, signingConflictRisksFrom(conflicts, checkedAt)...)
	return append(risks, signingDeadlineRisksFrom(unsigned, checkedAt)...), nil
}

// signingConflict is one recorded refusal of a signed copy between this
// instance and a peer.
type signingConflict struct {
	DID       string `db:"did"`
	PeerDID   string `db:"peer_did"`
	Direction string `db:"direction"`
}

// signingConflictRisksFrom flags each recorded signing conflict. A recorded
// conflict stays an open risk.
func signingConflictRisksFrom(conflicts []signingConflict, checkedAt time.Time) []event2.ComplianceRisk {
	risks := make([]event2.ComplianceRisk, 0, len(conflicts))
	for _, conflict := range conflicts {
		detail := fmt.Sprintf("%s refused the signed copy of this instance: both instances hold a signature over the same revision", conflict.PeerDID)
		if conflict.Direction == event2.SigningConflictReceived {
			detail = fmt.Sprintf("this instance refused the signed copy of %s: both instances hold a signature over the same revision", conflict.PeerDID)
		}
		risks = append(risks, event2.ComplianceRisk{
			DID:        conflict.DID,
			RiskType:   RiskTypeSigningConflict,
			Detail:     detail,
			DetectedAt: checkedAt,
		})
	}
	return risks
}

// signingDeadlineRisksFrom flags each contract whose signing deadline lies
// before checkedAt. The detail names the deadline only, so the same miss
// hashes the same on every sweep.
func signingDeadlineRisksFrom(unsigned []activeContract, checkedAt time.Time) []event2.ComplianceRisk {
	var risks []event2.ComplianceRisk
	for _, contract := range unsigned {
		deadline, ok := validation.SigningDeadline(contract.ContractData)
		if !ok || !deadline.Before(checkedAt) {
			continue
		}
		risks = append(risks, event2.ComplianceRisk{
			DID:      contract.DID,
			RiskType: RiskTypeSigningDeadlineMissed,
			Detail: fmt.Sprintf("contract is not signed although its signing deadline %s has passed",
				deadline.Format(time.RFC3339)),
			DetectedAt: checkedAt,
		})
	}
	return risks
}

func signatureRisksFrom(late []lateSignature, revoked []revokedCredential, checkedAt time.Time) []event2.ComplianceRisk {
	risks := make([]event2.ComplianceRisk, 0, len(late)+len(revoked))
	for _, contract := range late {
		risks = append(risks, event2.ComplianceRisk{
			DID:      contract.DID,
			RiskType: RiskTypeLateSignature,
			Detail: fmt.Sprintf("contract is not fully signed although its start date %s has passed",
				contract.StartDate.UTC().Format(time.RFC3339)),
			DetectedAt: checkedAt,
		})
	}
	for _, sig := range revoked {
		risks = append(risks, event2.ComplianceRisk{
			DID:      sig.DID,
			RiskType: RiskTypeInvalidCredential,
			Detail: fmt.Sprintf("signature of %s for %q relies on a certificate revoked at %s",
				sig.Signer, sig.Field, sig.RevokedAt.UTC().Format(time.RFC3339)),
			DetectedAt: checkedAt,
		})
	}
	return risks
}
