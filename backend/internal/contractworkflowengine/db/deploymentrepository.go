package db

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

// ContractDeployment is one dispatch of a contract to the configured
// Contract Target System (UC-05-01), keyed by CorrelationID for matching the
// target's later ack/status/KPI callbacks.
type ContractDeployment struct {
	ID              int64      `db:"id"`
	DID             string     `db:"did"`
	ContractVersion int        `db:"contract_version"`
	CorrelationID   string     `db:"correlation_id"`
	ContentHash     string     `db:"content_hash"`
	TargetID        *string    `db:"target_id"`
	TargetURL       *string    `db:"target_url"`
	Status          string     `db:"status"`
	RequestedBy     string     `db:"requested_by"`
	RequestedAt     time.Time  `db:"requested_at"`
	AcknowledgedAt  *time.Time `db:"acknowledged_at"`
	ReceiptHash     *string    `db:"receipt_hash"`
	TSAToken        *string    `db:"tsa_token"`
	DispatchError   *string    `db:"dispatch_error"`
}

// KPI verdicts a Contract Target System may report (ADR-33). The target
// executes and observes the contract, so it classifies; a report that carries
// no verdict is recorded as KPIVerdictNotEvaluated, which is neither a
// violation nor compliance.
const (
	KPIVerdictSatisfied    = "satisfied"
	KPIVerdictViolated     = "violated"
	KPIVerdictNotEvaluated = "not_evaluated"
)

// ContractKPI is a single KPI report received via the deployment callback for
// an ACTIVE contract: what the target system observed (Metric/Value) and what
// it concluded (Verdict) about the ODRL rule RuleID names
// (DCS-FR-CWE-09, DCS-FR-CWE-31, ADR-33). RuleID is nil when the report named
// no rule.
type ContractKPI struct {
	ID            int64     `db:"id"`
	DID           string    `db:"did"`
	CorrelationID *string   `db:"correlation_id"`
	Metric        string    `db:"metric"`
	Value         string    `db:"value"`
	ObservedAt    time.Time `db:"observed_at"`
	Verdict       string    `db:"verdict"`
	RuleID        *string   `db:"rule_id"`
}

// ContractTarget is one configured Contract Target System deployments may be
// dispatched to (ADR-25). It replaces the single CONTRACT_TARGET_URL: a DCS
// serving several execution environments could not otherwise say where a
// contract should go, and a failed dispatch had no target to name.
type ContractTarget struct {
	ID          string  `db:"id"`
	Name        string  `db:"name"`
	URL         string  `db:"url"`
	Description *string `db:"description"`
	Enabled     bool    `db:"enabled"`
	// OAuthClientID is the Hydra client this target authenticates its callbacks
	// with (ADR-27). Nil until a credential is issued; the secret is never
	// stored, only Hydra's hash of it.
	OAuthClientID  *string    `db:"oauth_client_id"`
	SecretIssuedAt *time.Time `db:"secret_issued_at"`
	CreatedBy      string     `db:"created_by"`
	CreatedAt      time.Time  `db:"created_at"`
	UpdatedAt      time.Time  `db:"updated_at"`
}

type ContractTargetRepo interface {
	ListTargets(ctx context.Context, tx *sqlx.Tx) ([]ContractTarget, error)
	ReadTarget(ctx context.Context, tx *sqlx.Tx, id string) (*ContractTarget, error)
	CreateTarget(ctx context.Context, tx *sqlx.Tx, data ContractTarget) (*ContractTarget, error)
	UpdateTarget(ctx context.Context, tx *sqlx.Tx, data ContractTarget) (*ContractTarget, error)
	DeleteTarget(ctx context.Context, tx *sqlx.Tx, id string) error
	// CountContractsDesignating reports how many contracts still name this
	// target, so removing it can be refused rather than leaving them
	// undeployable with no record of where they were meant to go.
	CountContractsDesignating(ctx context.Context, tx *sqlx.Tx, id string) (int, error)
	// DesignateForContract sets (or with a nil targetID clears) the target a
	// contract deploys to. Staleness is checked by the caller against the
	// stored updated_at, the same second-granularity comparison every other
	// contract mutation uses.
	DesignateForContract(ctx context.Context, tx *sqlx.Tx, did string, targetID *string) (bool, error)
	// SetCredential records the OAuth2 client this target authenticates its
	// callbacks as, and when its current secret was issued. The secret itself
	// is never stored: Hydra holds only a hash of it (ADR-27).
	SetCredential(ctx context.Context, tx *sqlx.Tx, id string, oauthClientID string, issuedAt time.Time) error
}

type DeploymentRepo interface {
	CreateDeployment(ctx context.Context, tx *sqlx.Tx, data ContractDeployment) error
	FindDeploymentByCorrelationID(ctx context.Context, tx *sqlx.Tx, correlationID string) (*ContractDeployment, error)
	AcknowledgeDeployment(ctx context.Context, tx *sqlx.Tx, correlationID string, receiptHash string, tsaToken string, acknowledgedAt time.Time) error
	// MarkDispatchFailed records that the outbound call never reached the
	// target. Without it the row keeps the 'DISPATCHED' status it was written
	// with before the call was attempted, making a deployment the target never
	// received indistinguishable from one it acknowledged.
	MarkDispatchFailed(ctx context.Context, tx *sqlx.Tx, correlationID string, reason string) error
	CreateKPI(ctx context.Context, tx *sqlx.Tx, data ContractKPI) error
	ReadKPIsByDID(ctx context.Context, tx *sqlx.Tx, did string) ([]ContractKPI, error)
}
