package command

import (
	"bytes"
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

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	db2 "digital-contracting-service/internal/dcstodcs/db"
)

// ErrSigningIncomplete rejects deployment of a multi-signer contract whose
// declared signature fields are not all signed yet (DCS-FR-SM-07/-17).
var ErrSigningIncomplete = errors.New("signing workflow incomplete")

// DeployCmd carries the inputs for deploying a SIGNED contract to the
// configured Contract Target System (UC-05-01).
// Deployment refusals that are the operator's to fix, not internal errors
// (ADR-25). A contract that reaches signing with no destination does not deploy,
// and saying so is the point — silently doing nothing is what this replaced.
var (
	ErrNoTargetDesignated  = errors.New("this contract designates no target system to deploy to")
	ErrTargetNotRegistered = errors.New("no such target system is registered")
	ErrTargetDisabled      = errors.New("target system is disabled and cannot receive deployments")
)

type DeployCmd struct {
	DID         string
	UpdatedAt   time.Time
	RequestedBy string
	// LocalPeer is this instance's own did:web. Signature slots are named by
	// the signing party, so it identifies which declared slot is ours.
	LocalPeer string
	// TargetIDOverride re-dispatches to a different registered target than the
	// one the contract designates (ADR-25). The designation itself is unchanged:
	// this is the operator directing one delivery, not editing the contract.
	TargetIDOverride string
}

// DeployResult is what both the manual /contract/deploy endpoint and the
// automatic post-signing subscriber receive back from Deployer.Handle.
type DeployResult struct {
	DID             string
	ContractVersion int
	ContentHash     string
	Timestamp       time.Time
	CorrelationID   string
	Payload         map[string]any
	TargetID        string
	TargetName      string
}

// Deployer handles DeployCmd: it gates on the contract being SIGNED (the
// same EventDeploy edge the ack-driven SIGNED -> ACTIVE transition uses,
// declared once in contractstate.Transitions), builds the machine-readable
// deployment payload (JSON-LD contract document, DID, version, content hash,
// timestamp, and an enclosing odrl:Set describing the deployment's own
// authorization), records the dispatch, and best-effort forwards it to the
// configured Contract Target System.
type Deployer struct {
	DB             *sqlx.DB
	CRepo          db.ContractRepo
	DeploymentRepo db.DeploymentRepo
	TargetRepo     db.ContractTargetRepo
	Target         ContractTargetClient
	// PeerSigs supplies the counterparty signature evidence the multi-signer
	// gate needs for a federated contract. Required: a deployer without it
	// cannot tell a countersigned contract from a half-signed one.
	PeerSigs PeerSignatures
}

// PeerSignatures reads the cross-instance signature artifact a peer ships with
// its own signed copy of a contract (DCS-FR-SM-02): a JAdES over the contract
// payload, verified against the peer's published assertion key before it is
// stored (internal/service/dcs_to_dcs.go verifyShippedJades).
type PeerSignatures interface {
	GetSyncSignature(ctx context.Context, tx *sqlx.Tx, did string) (*db2.SyncSignature, error)
}

func (h *Deployer) Handle(ctx context.Context, cmd DeployCmd) (*DeployResult, error) {
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	data, err := h.CRepo.ReadDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("could not read contract %s: %w", cmd.DID, err)
	}

	if err := contractstate.ValidateTransition(contractstate.ContractState(data.State), contractstate.EventDeploy); err != nil {
		return nil, err
	}

	// Multi-signer gate (DCS-FR-SM-07/-17, DCS-NFR-BR-03): a contract that
	// declares signature fields may only deploy once EVERY declared field is
	// signed. The auto-deploy subscriber fires after each signature, so a
	// partially signed contract hits this gate until the last signatory
	// signs.
	if data.ContractData != nil && data.ContractData.IsNotNullValue() {
		required := validation.RequiredSignatureFields([]byte(*data.ContractData))
		if len(required) > 0 {
			signedFields, err := h.CRepo.ReadSignedSignatureFieldNames(ctx, tx, cmd.DID)
			if err != nil {
				return nil, fmt.Errorf("could not read signed signature fields: %w", err)
			}
			if h.PeerSigs == nil {
				return nil, fmt.Errorf("could not check counterparty signatures for %s: no peer signature store is configured", cmd.DID)
			}
			peerSig, err := h.PeerSigs.GetSyncSignature(ctx, tx, cmd.DID)
			if err != nil {
				return nil, fmt.Errorf("could not read the counterparty signature for %s: %w", cmd.DID, err)
			}
			if missing := signatureEvidence(required, signedFields, data.Responsible, cmd.LocalPeer, peerSig).Unsigned(); len(missing) > 0 {
				return nil, fmt.Errorf("%w: unsigned signature fields: %s", ErrSigningIncomplete, strings.Join(missing, ", "))
			}
		}
	}

	contractDataBytes := []byte(`{}`)
	if data.ContractData != nil && data.ContractData.IsNotNullValue() {
		contractDataBytes = []byte(*data.ContractData)
	}
	// Decode the contract document instead of embedding the raw jsonb bytes:
	// the content hash must be reproducible by the RECEIVING target system
	// from the parsed JSON (canonical form = recursively key-sorted, compact,
	// no HTML escaping — see hashDeploymentPayload), and Postgres jsonb's
	// length-then-bytewise key order would otherwise leak into the hash.
	var contractDocument map[string]any
	if err := json.Unmarshal(contractDataBytes, &contractDocument); err != nil {
		return nil, fmt.Errorf("could not decode contract document for %s: %w", cmd.DID, err)
	}

	correlationID := uuid.NewString()
	now := time.Now().UTC()

	payload := map[string]any{
		"@context": map[string]string{
			"dcs":  "https://w3id.org/facis/dcs/ontology/v1#",
			"odrl": "http://www.w3.org/ns/odrl/2/",
		},
		"@type":                "dcs:ContractDeployment",
		"dcs:contractDid":      cmd.DID,
		"dcs:contractVersion":  data.ContractVersion,
		"dcs:timestamp":        now.Format(time.RFC3339Nano),
		"dcs:correlationId":    correlationID,
		"dcs:contractDocument": contractDocument,
		"odrl:policy":          deploymentPolicy(contractDocument, correlationID),
	}

	contentHash, err := hashDeploymentPayload(payload)
	if err != nil {
		return nil, fmt.Errorf("could not hash deployment payload: %w", err)
	}
	payload["dcs:contentHash"] = contentHash

	// Where this contract goes is the contract's own property (ADR-25), so the
	// automatic trigger on signing completion has a destination without a human
	// present to choose one. A re-dispatch may be directed elsewhere.
	targetID := strings.TrimSpace(cmd.TargetIDOverride)
	if targetID == "" && data.TargetID != nil {
		targetID = strings.TrimSpace(*data.TargetID)
	}
	if targetID == "" {
		return nil, ErrNoTargetDesignated
	}
	target, err := h.TargetRepo.ReadTarget(ctx, tx, targetID)
	if err != nil {
		return nil, fmt.Errorf("could not read target system %s: %w", targetID, err)
	}
	if target == nil {
		return nil, fmt.Errorf("%w: %s", ErrTargetNotRegistered, targetID)
	}
	if !target.Enabled {
		return nil, fmt.Errorf("%w: %s", ErrTargetDisabled, target.Name)
	}
	// The endpoint is copied as it stands now, beside the reference: editing the
	// registry entry later must not rewrite what this deployment actually did.
	targetURL := target.URL
	targetURLPtr := &targetURL
	targetIDPtr := &target.ID

	if err := h.DeploymentRepo.CreateDeployment(ctx, tx, db.ContractDeployment{
		DID:             cmd.DID,
		ContractVersion: data.ContractVersion,
		CorrelationID:   correlationID,
		ContentHash:     contentHash,
		TargetID:        targetIDPtr,
		TargetURL:       targetURLPtr,
		Status:          "DISPATCHED",
		RequestedBy:     cmd.RequestedBy,
		RequestedAt:     now,
	}); err != nil {
		return nil, fmt.Errorf("could not store deployment record: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	// The target's own callback (POST /contract/deployment/callback) remains the
	// authoritative signal of a successful deployment, so a failed outbound call
	// is not fatal to the request. It is recorded, though: the row was written
	// DISPATCHED before this ran, and leaving it that way made a deployment the
	// target never received indistinguishable from one it acknowledged. The
	// compliance monitor reads these back and alerts on them (DCS-FR-CWE-31).
	if h.Target != nil {
		if err := h.Target.DeployTo(ctx, targetURL, payload); err != nil {
			log.Printf("contractworkflowengine: could not dispatch deployment %s for contract %s to %s: %v", correlationID, cmd.DID, targetURL, err)
			h.recordDispatchFailure(ctx, correlationID, err)
		}
	}

	return &DeployResult{
		DID:             cmd.DID,
		ContractVersion: data.ContractVersion,
		ContentHash:     contentHash,
		Timestamp:       now,
		CorrelationID:   correlationID,
		Payload:         payload,
		TargetID:        target.ID,
		TargetName:      target.Name,
	}, nil
}

// recordDispatchFailure marks the deployment row so the failure survives the
// process log. Its own failure is logged and swallowed: the deployment already
// happened as far as the caller is concerned, and losing the request over a
// bookkeeping error would be worse than losing the alert.
func (h *Deployer) recordDispatchFailure(ctx context.Context, correlationID string, cause error) {
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		log.Printf("contractworkflowengine: could not record dispatch failure for %s: %v", correlationID, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	if err := h.DeploymentRepo.MarkDispatchFailed(ctx, tx, correlationID, cause.Error()); err != nil {
		log.Printf("contractworkflowengine: could not record dispatch failure for %s: %v", correlationID, err)
		return
	}
	if err := tx.Commit(); err != nil {
		log.Printf("contractworkflowengine: could not record dispatch failure for %s: %v", correlationID, err)
	}
}

// odrlRuleProperties are the ODRL rule collections a contract's policy set may
// carry. A deployed policy names the same rules the parties signed; it does not
// restate or summarise them.
var odrlRuleProperties = [...]string{"odrl:permission", "odrl:prohibition", "odrl:obligation"}

// deploymentPolicy is the policy the target system enforces (SRS §1.2: the
// target system is where automated runtime enforcement happens; DCS-IR-SI-05
// is the interface that hands it over). It carries the rules of the signed
// contract's own policy set, so an integrator reading the documented odrl:policy
// slot finds the terms rather than an empty Set and has to go digging in
// dcs:contractDocument for them.
//
// The set is emitted even when the contract states no rules: an empty policy is
// the honest answer for a contract that constrains nothing, and is distinct from
// the slot being absent.
func deploymentPolicy(contractDocument map[string]any, correlationID string) map[string]any {
	policy := map[string]any{
		"@id":   "urn:uuid:deployment-policy-" + correlationID,
		"@type": "odrl:Set",
	}
	policies, ok := contractDocument["dcs:policies"].(map[string]any)
	if !ok {
		return policy
	}
	if profile, ok := policies["odrl:profile"]; ok {
		policy["odrl:profile"] = profile
	}
	for _, property := range odrlRuleProperties {
		if rules, ok := policies[property]; ok {
			policy[property] = rules
		}
	}
	return policy
}

// nestedRuleProperties are the rule slots a rule may itself carry: a duty
// attached to a permission, and the consequence of a breached duty. Both travel
// inside the rule they hang off, so the target can conclude about them too.
var nestedRuleProperties = [...]string{"odrl:duty", "odrl:consequence"}

// deployedRuleIDs collects the @id of every ODRL rule the deployment envelope
// hands the target — the buckets deploymentPolicy copies, plus the duties and
// consequences nested inside them. A reported verdict names one of these; an
// @id outside the set is a conclusion the DCS cannot attribute to a term of
// this contract (ADR-33).
func deployedRuleIDs(contractDocument map[string]any) map[string]bool {
	ids := map[string]bool{}
	policies, ok := contractDocument["dcs:policies"].(map[string]any)
	if !ok {
		return ids
	}
	for _, property := range odrlRuleProperties {
		collectRuleIDs(policies[property], ids)
	}
	return ids
}

func collectRuleIDs(rules any, ids map[string]bool) {
	for _, rule := range ruleNodes(rules) {
		if id, ok := rule["@id"].(string); ok && strings.TrimSpace(id) != "" {
			ids[strings.TrimSpace(id)] = true
		}
		for _, property := range nestedRuleProperties {
			collectRuleIDs(rule[property], ids)
		}
	}
}

// ruleNodes reads a rule slot, which JSON-LD lets hold either a single node or
// an array of them.
func ruleNodes(raw any) []map[string]any {
	switch value := raw.(type) {
	case map[string]any:
		return []map[string]any{value}
	case []any:
		nodes := make([]map[string]any, 0, len(value))
		for _, item := range value {
			if node, ok := item.(map[string]any); ok {
				nodes = append(nodes, node)
			}
		}
		return nodes
	}
	return nil
}

// hashDeploymentPayload computes the payload's canonical content hash:
// recursively key-sorted (Go marshals maps sorted), compact, WITHOUT HTML
// escaping — so a receiving target system can reproduce it from the parsed
// JSON with a plain deep-sort + stringify (the shipped ORCE
// contract-target-flow and the BDD harness both do exactly that).
func hashDeploymentPayload(payload map[string]any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(payload); err != nil {
		return "", err
	}
	canonical := bytes.TrimRight(buf.Bytes(), "\n")
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// signatureEvidence expresses what this instance holds about a contract's
// signatures in the shared form the "have all declared signatures been
// collected" predicate takes (contractstate.SignatureEvidence). The predicate
// itself lives there and is answered once, for the extrinsic lifecycle
// projection and for this gate alike: the same rule stated twice is how a
// half-signed contract came to be deployable on one path while a fully
// countersigned one was undeployable on the other.
//
// Only the mapping is local. A contract with no recorded parties has no remote
// slot to exempt, and only one cross-instance signature is stored per contract
// (contract_sync_signatures is keyed by contract, with no field column), so at
// most one remote party can be evidenced and any further one stays unsigned.
func signatureEvidence(required, signedLocally []string, resp *db.Responsible, localPeer string, peerSig *db2.SyncSignature) contractstate.SignatureEvidence {
	evidence := contractstate.SignatureEvidence{
		Declared:      required,
		SignedLocally: signedLocally,
		LocalPeer:     localPeer,
	}
	if resp != nil {
		evidence.Parties = []string{resp.Creator, resp.Counterparty}
	}
	if peerSig != nil {
		evidence.PeerSigners = []string{peerSig.FromPeerDID}
	}
	return evidence
}
