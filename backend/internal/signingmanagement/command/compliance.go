package command

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/signingmanagement/db"
	signingmanagementevents "digital-contracting-service/internal/signingmanagement/event"
)

// poaComplianceFindings walks the sealed agreement's party nodes and reports any
// signed party (dcs:hasSignatory present) that signed with no Power of Attorney,
// or under one authorizing a different organization than the party it signed as
// (UC-14, FR-SM-03/-04). The organization rides the party node so this holds for
// a counterparty's signature synced from another instance.
//
// It returns the parties it judged, so the caller knows which of this instance's
// own signatures the document accounts for and which it must judge from the
// retained ceremony evidence instead.
func poaComplianceFindings(raw datatype.JSON) (findings []string, attributed map[string]bool) {
	attributed = map[string]bool{}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, attributed
	}
	nodes, _ := doc["dcs:parties"].([]any)
	for _, rawNode := range nodes {
		node, ok := rawNode.(map[string]any)
		if !ok {
			continue
		}
		signatory := nodeIRI(node["dcs:hasSignatory"])
		if signatory == "" {
			continue
		}
		party, _ := node["@id"].(string)
		attributed[strings.TrimSpace(party)] = true
		findings = append(findings, poaFinding(party, signatory, nodeIRI(node["dcs:hasPowerOfAttorney"]))...)
	}
	return findings, attributed
}

// poaFinding applies the Power-of-Attorney rule to one attributed signature,
// wherever the attribution was read from.
func poaFinding(party, signatory, poaOrg string) []string {
	switch {
	case poaOrg == "":
		return []string{fmt.Sprintf("Party %s signed with no Power of Attorney (signatory %s)", party, signatory)}
	case poaOrg != party:
		return []string{fmt.Sprintf("Party %s signed under a Power of Attorney authorizing %s, not this party (signatory %s)", party, poaOrg, signatory)}
	}
	return nil
}

// signatureAuthorities resolves the ceremony an applied signature was made
// under — the record of the authority behind a signature the frozen contract
// document could not be given. db.CeremonyRepo satisfies it.
type signatureAuthorities interface {
	GetCeremonyByID(ctx context.Context, tx *sqlx.Tx, id string) (*db.SignatureCeremony, error)
}

// appliedSignatureFindings judges the signatures this instance applied that the
// contract document does not account for.
//
// A signature made once the artifact is frozen cannot be written into the
// document — the bytes are signed and can never be re-rendered — so its
// signatory and its authority live on the ceremony that produced it and in the
// signing summary issued from it, which is also what ships to the counterparty.
// Reading only the document would leave a countersignature judged by nobody:
// silently no finding, which reads exactly like a compliant signature.
func appliedSignatureFindings(
	ctx context.Context, tx *sqlx.Tx, ceremonies signatureAuthorities, signatures []db.SignatureRecord, attributed map[string]bool,
) ([]string, error) {
	if ceremonies == nil {
		return nil, errors.New("the compliance viewer needs the signing ceremonies to judge a signature the contract document does not record")
	}
	var findings []string
	for _, sig := range signatures {
		if sig.Status != "SIGNED" || sig.FieldName == nil || sig.CeremonyID == nil {
			continue
		}
		// The signature field IS the party it was signed for: the ceremony
		// refuses unless the Power of Attorney authorizes exactly that name
		// (ceremony.go), and the seeded fields are named for the party DIDs.
		party := strings.TrimSpace(*sig.FieldName)
		if party == "" || attributed[party] {
			continue
		}
		ceremony, err := ceremonies.GetCeremonyByID(ctx, tx, *sig.CeremonyID)
		if err != nil {
			return nil, fmt.Errorf("could not resolve the ceremony behind the signature for %q: %w", party, err)
		}
		if ceremony == nil {
			continue
		}
		poaOrg := ""
		if ceremony.PoAOrganization != nil {
			poaOrg = strings.TrimSpace(*ceremony.PoAOrganization)
		}
		findings = append(findings, poaFinding(party, sig.SignerDID, poaOrg)...)
	}
	return findings, nil
}

// nodeIRI reads an IRI from a JSON-LD value that is either {"@id": iri} or a
// bare string.
func nodeIRI(v any) string {
	switch t := v.(type) {
	case map[string]any:
		id, _ := t["@id"].(string)
		return strings.TrimSpace(id)
	case string:
		return strings.TrimSpace(t)
	}
	return ""
}

type ComplianceCmd struct {
	DID       string
	CheckedBy string
	HolderDID string
	UserRoles userrole.UserRoles
}

type ComplianceValidator struct {
	DB    *sqlx.DB
	CRepo db.ContractRepo
	// CeremonyRepo resolves the authority behind a signature the contract
	// document does not record, which is every signature applied once the
	// artifact was already signed.
	CeremonyRepo db.CeremonyRepo
}

// Handle evaluates the contract's signatures against the signature
// compliance policy (DCS-FR-SM-21: signature level SES/AES/QES, signature
// status, presence of active signed credentials) and returns the findings;
// the check itself — findings included — is recorded as an audit event.
func (h *ComplianceValidator) Handle(ctx context.Context, cmd ComplianceCmd) ([]string, error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	processData, err := h.CRepo.ReadProcessDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("could not read process data: %w", err)
	}

	findings, err := h.CRepo.CollectComplianceFindings(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("could not collect compliance findings: %w", err)
	}

	// Power of Attorney (UC-14, FR-SM-03/-04): every signed party — this
	// instance's own and any counterparty whose signature arrived over the peer
	// sync — must have signed under a PoA authorizing the very party it signed as.
	// The organization travels on the party node (dcs:hasPowerOfAttorney) for
	// every signature the document could still carry, so a counterparty running a
	// misconfigured or malicious DCS is caught here; a signature applied to an
	// already-signed artifact is judged from the ceremony it was made under,
	// which is where its authority is retained instead.
	contract, err := h.CRepo.ReadDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("could not read contract data: %w", err)
	}
	attributed := map[string]bool{}
	if contract.ContractData != nil {
		var documentFindings []string
		documentFindings, attributed = poaComplianceFindings(*contract.ContractData)
		findings = append(findings, documentFindings...)
	}
	signatures, err := h.CRepo.LoadSignatures(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("could not load signatures: %w", err)
	}
	appliedFindings, err := appliedSignatureFindings(ctx, tx, h.CeremonyRepo, signatures, attributed)
	if err != nil {
		return nil, err
	}
	findings = append(findings, appliedFindings...)

	evt := signingmanagementevents.ComplianceValidationEvent{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		CheckedBy:       cmd.CheckedBy,
		Findings:        findings,
		OccurredAt:      time.Now().UTC(),
		HolderDID:       cmd.HolderDID,
		UserRoles:       cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.SignatureManagement)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return findings, nil
}
