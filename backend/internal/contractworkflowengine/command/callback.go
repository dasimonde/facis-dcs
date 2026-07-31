package command

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

// ErrDeploymentCallbackUnauthorized is returned when the caller is not the
// target system the deployment was dispatched to (DCS-IR-SI-05).
var ErrDeploymentCallbackUnauthorized = errors.New("caller is not the target system this deployment was dispatched to")

// ErrDeploymentNotFound is returned when a callback references a correlation
// ID that was never dispatched by Deployer.
var ErrDeploymentNotFound = errors.New("deployment correlation id not found")

// DeploymentReceiptPayload is the target's execution-evidence receipt
// carried in an acknowledgement callback.
type DeploymentReceiptPayload struct {
	CorrelationID string `json:"correlation_id"`
	PayloadHash   string `json:"payload_hash"`
	ActivatedAt   string `json:"activated_at"`
}

// DeploymentCallbackCmd carries a POST /contract/deployment/callback
// request: either an ack/status update (Status/Receipt set) or a KPI report
// (KPIMetric set), or both.
//
// KPIVerdict is what the target system concluded about the rule KPIRule names
// (ADR-33). The verdict is recorded as reported; an empty one records as
// not_evaluated, and a stated one has to name a rule of this contract.
type DeploymentCallbackCmd struct {
	// CallerClientID is the OAuth2 client the request authenticated as, taken
	// from the validated access token. The callback is accepted only when it
	// matches the credential of the target the deployment went to, so a target
	// can report on its own deployments and on no one else's.
	CallerClientID string
	DID            string
	CorrelationID  string
	Status         string
	Receipt        *DeploymentReceiptPayload
	KPIMetric      string
	KPIValue       string
	KPIVerdict     string
	KPIRule        string
}

// DeploymentCallbackHandler checks the caller is the target the deployment was
// dispatched to (ADR-27), then applies an ack (sealing an RFC-3161-timestamped
// execution-evidence receipt into the archive entry and moving the contract
// SIGNED -> ACTIVE, DCS-FR-SM-10/DCS-FR-SM-12) and/or records a reported KPI
// observation together with the verdict the target reached on it
// (DCS-FR-CWE-09, ADR-33).
type DeploymentCallbackHandler struct {
	DB             *sqlx.DB
	CRepo          db.ContractRepo
	DeploymentRepo db.DeploymentRepo
	TargetRepo     db.ContractTargetRepo
	ArchiveTSA     ArchiveTimestampIssuer
}

func (h *DeploymentCallbackHandler) Handle(ctx context.Context, cmd DeploymentCallbackCmd) error {
	if strings.TrimSpace(cmd.CallerClientID) == "" {
		return ErrDeploymentCallbackUnauthorized
	}

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	deployment, err := h.DeploymentRepo.FindDeploymentByCorrelationID(ctx, tx, cmd.CorrelationID)
	if err != nil {
		return fmt.Errorf("could not read deployment %s: %w", cmd.CorrelationID, err)
	}
	if deployment == nil {
		return ErrDeploymentNotFound
	}

	// One shared secret proved only that SOME target was calling. Binding the
	// caller's own credential to the registry entry the deployment was
	// dispatched to means a target can acknowledge its own deployments and
	// nothing else, and a compromised target cannot speak for the others.
	if err := h.authorizeCaller(ctx, tx, deployment, cmd.CallerClientID); err != nil {
		return err
	}

	if cmd.Receipt != nil || strings.TrimSpace(cmd.Status) != "" {
		if err := h.applyAcknowledgement(ctx, tx, deployment, cmd); err != nil {
			return err
		}
	}

	if strings.TrimSpace(cmd.KPIMetric) != "" {
		if err := h.applyKPIReport(ctx, tx, deployment, cmd); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// authorizeCaller refuses a callback that does not come from the registry entry
// the deployment was dispatched to. A deployment with no target recorded, or a
// target with no credential issued, cannot be acknowledged at all: there is
// nothing to check the caller against, and accepting it would restore exactly
// the "some target said so" property the shared secret had.
func (h *DeploymentCallbackHandler) authorizeCaller(ctx context.Context, tx *sqlx.Tx, deployment *db.ContractDeployment, callerClientID string) error {
	if deployment.TargetID == nil || strings.TrimSpace(*deployment.TargetID) == "" {
		return ErrDeploymentCallbackUnauthorized
	}

	target, err := h.TargetRepo.ReadTarget(ctx, tx, *deployment.TargetID)
	if err != nil {
		return fmt.Errorf("could not read the target of deployment %s: %w", deployment.CorrelationID, err)
	}
	if target == nil || target.OAuthClientID == nil {
		return ErrDeploymentCallbackUnauthorized
	}
	if strings.TrimSpace(*target.OAuthClientID) != strings.TrimSpace(callerClientID) {
		return ErrDeploymentCallbackUnauthorized
	}
	return nil
}

func (h *DeploymentCallbackHandler) applyAcknowledgement(ctx context.Context, tx *sqlx.Tx, deployment *db.ContractDeployment, cmd DeploymentCallbackCmd) error {
	activatedAt := time.Now().UTC()
	receipt := DeploymentReceiptPayload{
		CorrelationID: cmd.CorrelationID,
		PayloadHash:   deployment.ContentHash,
	}
	if cmd.Receipt != nil {
		if cmd.Receipt.PayloadHash != "" {
			receipt.PayloadHash = cmd.Receipt.PayloadHash
		}
		if cmd.Receipt.ActivatedAt != "" {
			receipt.ActivatedAt = cmd.Receipt.ActivatedAt
		}
	}
	if receipt.ActivatedAt == "" {
		receipt.ActivatedAt = activatedAt.Format(time.RFC3339Nano)
	}

	receiptBytes, err := json.Marshal(receipt)
	if err != nil {
		return fmt.Errorf("marshal execution-evidence receipt: %w", err)
	}
	receiptSum := sha256.Sum256(receiptBytes)
	receiptHash := "sha256:" + hex.EncodeToString(receiptSum[:])

	tsaToken := ""
	if h.ArchiveTSA != nil && h.ArchiveTSA.Enabled() {
		tsaReceipt, err := h.ArchiveTSA.TimestampBytes(ctx, receiptBytes)
		if err != nil {
			return fmt.Errorf("could not timestamp execution-evidence receipt: %w", err)
		}
		tsaToken = tsaReceipt.Token
	}

	if err := h.DeploymentRepo.AcknowledgeDeployment(ctx, tx, cmd.CorrelationID, receiptHash, tsaToken, activatedAt); err != nil {
		return fmt.Errorf("could not acknowledge deployment %s: %w", cmd.CorrelationID, err)
	}

	processData, err := h.CRepo.ReadProcessDataByDID(ctx, tx, deployment.DID)
	if err != nil {
		return fmt.Errorf("could not read contract %s: %w", deployment.DID, err)
	}
	if err := contractstate.ValidateTransition(contractstate.ContractState(processData.State), contractstate.EventDeploy); err != nil {
		return err
	}
	if err := h.CRepo.UpdateState(ctx, tx, deployment.DID, contractstate.Active.String()); err != nil {
		return fmt.Errorf("could not activate contract %s: %w", deployment.DID, err)
	}

	return nil
}

// applyKPIReport records the observation and the target system's verdict on it
// as reported (ADR-33). The DCS does not re-adjudicate the contract's terms: it
// holds them, the target holds the events. A report that carries no verdict is
// stored as not_evaluated.
func (h *DeploymentCallbackHandler) applyKPIReport(ctx context.Context, tx *sqlx.Tx, deployment *db.ContractDeployment, cmd DeploymentCallbackCmd) error {
	verdict, err := reportedVerdict(cmd.KPIVerdict)
	if err != nil {
		return err
	}

	ruleID, err := h.attributedRule(ctx, tx, deployment.DID, cmd)
	if err != nil {
		return err
	}

	correlationID := cmd.CorrelationID

	if err := h.DeploymentRepo.CreateKPI(ctx, tx, db.ContractKPI{
		DID:           deployment.DID,
		CorrelationID: &correlationID,
		Metric:        cmd.KPIMetric,
		Value:         cmd.KPIValue,
		ObservedAt:    time.Now().UTC(),
		Verdict:       verdict,
		RuleID:        ruleID,
	}); err != nil {
		return fmt.Errorf("could not store KPI report: %w", err)
	}

	return nil
}

// attributedRule resolves the rule a reported verdict is about against the
// rules this contract deployed. Attribution is the DCS's own competence under
// ADR-33, and it is the whole traceability of the row: a verdict is evidence
// about one term of the signed contract, so the @id it names has to be a rule
// that travelled to the target in the deployment envelope (deploymentPolicy).
//
// A report that states no verdict at all is silence rather than a conclusion,
// and names nothing; anything it did name is still resolved, so a target may
// say "I could not evaluate rule X".
func (h *DeploymentCallbackHandler) attributedRule(ctx context.Context, tx *sqlx.Tx, did string, cmd DeploymentCallbackCmd) (*string, error) {
	rule := strings.TrimSpace(cmd.KPIRule)
	if rule == "" {
		if strings.TrimSpace(cmd.KPIVerdict) != "" {
			return nil, fmt.Errorf("%w: verdict %q names no rule", ErrKPIRuleMissing, strings.TrimSpace(cmd.KPIVerdict))
		}
		return nil, nil
	}

	contract, err := h.CRepo.ReadDataByDID(ctx, tx, did)
	if err != nil {
		return nil, fmt.Errorf("could not read contract %s: %w", did, err)
	}
	if contract == nil || contract.ContractData == nil || !contract.ContractData.IsNotNullValue() {
		return nil, fmt.Errorf("could not resolve the reported rule against contract %s: it holds no document", did)
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(*contract.ContractData), &document); err != nil {
		return nil, fmt.Errorf("could not decode contract document for %s: %w", did, err)
	}
	if !deployedRuleIDs(document)[rule] {
		return nil, fmt.Errorf("%w: %s", ErrKPIRuleUnknown, rule)
	}
	return &rule, nil
}

// Malformed KPI reports, which ADR-33 leaves the DCS free to refuse. Neither is
// an opinion on the verdict itself.
var (
	// ErrKPIVerdictUnknown is returned for a report naming a verdict outside
	// the three ADR-33 defines. It is a malformed report, not an absent
	// verdict.
	ErrKPIVerdictUnknown = errors.New("unknown KPI verdict")
	// ErrKPIRuleMissing is returned for a stated verdict that names no rule:
	// a conclusion about nothing cannot be attributed or traced.
	ErrKPIRuleMissing = errors.New("KPI verdict names no ODRL rule")
	// ErrKPIRuleUnknown is returned for a rule @id that is not among the rules
	// this contract deployed to the target.
	ErrKPIRuleUnknown = errors.New("KPI verdict names a rule this contract does not carry")
)

func reportedVerdict(reported string) (string, error) {
	switch strings.TrimSpace(reported) {
	case "":
		return db.KPIVerdictNotEvaluated, nil
	case db.KPIVerdictSatisfied:
		return db.KPIVerdictSatisfied, nil
	case db.KPIVerdictViolated:
		return db.KPIVerdictViolated, nil
	case db.KPIVerdictNotEvaluated:
		return db.KPIVerdictNotEvaluated, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrKPIVerdictUnknown, reported)
	}
}
