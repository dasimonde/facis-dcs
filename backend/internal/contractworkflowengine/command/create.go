package command

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/identity"

	"digital-contracting-service/internal/contractworkflowengine/datatype/reviewtaskstate"

	"digital-contracting-service/internal/base/datatype/userrole"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
	"digital-contracting-service/internal/contractworkflowengine/query/contracttemplate"
	"digital-contracting-service/internal/semantichub"
)

type CreateCmd struct {
	DID         string `json:"did"`
	TemplateDID string `json:"template_did"`
	CreatedBy   string `json:"created_by"`
	HolderDID   string `json:"holder_did"`
	// Counterparty is the single peer DCS (a did:web) this contract is offered
	// to and negotiated with. It drives the PDF ship target and, together with
	// the origin, the party set the signature fields are seeded for (ADR-13).
	// Reviewer/approver/negotiator are internal RBAC roles, isolated per
	// instance — never peer DIDs.
	Counterparty string   `json:"counterparty"`
	Parties      []string `json:"parties"`
	// OriginatorRole is the contractual role the creating organization
	// declares for itself; it binds the origin DID to that role's party
	// node in the contract's ODRL rules. The counterpart role stays open
	// until the counterparty accepts by signing.
	OriginatorRole string             `json:"originator_role"`
	UserRoles      userrole.UserRoles `json:"user_roles"`
}

type Creator struct {
	DB          *sqlx.DB
	CRepo       db.ContractRepo
	CTRepo      db.ContractTemplateRepo
	RTRepo      db.ReviewTaskRepo
	ATRepo      db.ApprovalTaskRepo
	NTRepo      db.NegotiationTaskRepo
	DIDDocument identity.DIDDocument
}

type semanticBundleRefs struct {
	Context         string
	CanonicalShapes string
	Shapes          []string
	Profile         string
}

func effectiveBundleRefs(bundle semantichub.EffectiveBundle) (semanticBundleRefs, error) {
	if bundle.ContextVersion <= 0 || bundle.ProfileVersion <= 0 || len(bundle.Shapes) == 0 {
		return semanticBundleRefs{}, errors.New("complete versioned semantic bundle is required")
	}
	if bundle.Shapes[0].Name != semantichub.ShapesName || bundle.Shapes[0].Version <= 0 {
		return semanticBundleRefs{}, errors.New("canonical shapes must be the first versioned bundle entry")
	}
	shapeRefs := make([]string, 0, len(bundle.Shapes))
	for _, shape := range bundle.Shapes {
		if strings.TrimSpace(shape.Name) == "" || shape.Version <= 0 {
			return semanticBundleRefs{}, errors.New("every effective shape requires a name and version")
		}
		shapeRefs = append(shapeRefs, semantichub.AnchorURL("shapes", shape.Name, shape.Version))
	}
	return semanticBundleRefs{
		Context:         semantichub.AnchorURL("context", semantichub.ContextName, bundle.ContextVersion),
		CanonicalShapes: shapeRefs[0],
		Shapes:          shapeRefs,
		Profile:         semantichub.AnchorURL("profile", semantichub.ProfileName, bundle.ProfileVersion),
	}, nil
}

// createTasks opens this instance's own review, negotiation, and approval
// tasks (ADR-13): the responsible role lists hold local-RBAC holders only, so
// each DCS creates and owns its tasks; nothing crosses the boundary.
func createTasks(ctx context.Context, tx *sqlx.Tx, rtRepo db.ReviewTaskRepo, atRepo db.ApprovalTaskRepo, ntRepo db.NegotiationTaskRepo, did, createdBy string, resp db.Responsible) error {
	for _, reviewer := range resp.Reviewers {
		reviewTask := db.ReviewTaskData{
			DID:       did,
			Reviewer:  reviewer,
			State:     reviewtaskstate.Open.String(),
			CreatedBy: createdBy,
		}
		_, err := rtRepo.Create(ctx, tx, reviewTask)
		if err != nil {
			return fmt.Errorf("could not create review task: %w", err)
		}
	}

	for _, negotiator := range resp.Negotiators {
		negotiationTask := db.NegotiationTaskData{
			DID:        did,
			Negotiator: negotiator,
			State:      reviewtaskstate.Open.String(),
			CreatedBy:  createdBy,
		}
		_, err := ntRepo.Create(ctx, tx, negotiationTask)
		if err != nil {
			return fmt.Errorf("could not create negotiation task: %w", err)
		}
	}

	for _, approver := range resp.Approvers {
		data := db.ApprovalTaskData{
			DID:       did,
			CreatedBy: createdBy,
			Approver:  approver,
			State:     reviewtaskstate.Open.String(),
		}
		_, err := atRepo.Create(ctx, tx, data)
		if err != nil {
			return fmt.Errorf("could not create approval task: %w", err)
		}
	}

	return nil
}

// Handle has no entry in contractstate.Transitions: creation establishes the
// initial DRAFT state, it is not a transition from a prior state.
func (h *Creator) Handle(ctx context.Context, cmd CreateCmd) error {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	contractTemplate, err := h.CTRepo.ReadContractTemplateDataByID(ctx, tx, cmd.TemplateDID)
	if err != nil {
		return fmt.Errorf("could not read contract template data: %w", err)
	}

	contractDocument, err := contracttemplate.ConvertTemplateDataToContractData(contractTemplate.TemplateData, cmd.TemplateDID, contractTemplate.TemplateVersion)
	if err != nil {
		return fmt.Errorf("could not derive contract data from template: %w", err)
	}
	normalizedContractData, err := validation.NormalizeContractDataForPersistence(contractDocument, cmd.DID, false)
	if err != nil {
		return fmt.Errorf("contract data validation failed: %w", err)
	}
	bundle, err := semantichub.ResolveEffectiveBundle(ctx, tx)
	if err != nil {
		return fmt.Errorf("could not resolve effective Semantic Hub bundle: %w", err)
	}
	bundleRefs, err := effectiveBundleRefs(bundle)
	if err != nil {
		return fmt.Errorf("could not resolve effective Semantic Hub bundle references: %w", err)
	}
	normalizedContractData, err = validation.PinSemanticBundle(
		normalizedContractData,
		bundleRefs.Context,
		bundleRefs.CanonicalShapes,
		bundleRefs.Shapes,
		bundleRefs.Profile,
	)
	if err != nil {
		return fmt.Errorf("could not pin effective Semantic Hub bundle: %w", err)
	}
	// Parties are attached after normalization for the same reason renewal's
	// dcs:renewsContract is (see attachRenewsContractReference): the rebase
	// pass must not touch them. They gate party read-scoping in
	// query/contract/querybyid.go.
	if len(cmd.Parties) > 0 {
		normalizedContractData, err = attachContractParties(normalizedContractData, cmd.Parties)
		if err != nil {
			return fmt.Errorf("could not attach contract parties: %w", err)
		}
	}

	localPeer, err := h.DIDDocument.GetID()
	if err != nil {
		return fmt.Errorf("could not get DID: %w", err)
	}

	if cmd.OriginatorRole != "" {
		normalizedContractData, err = bindOriginatorParty(normalizedContractData, localPeer, cmd.OriginatorRole)
		if err != nil {
			return fmt.Errorf("could not bind originator party: %w", err)
		}
	}

	// Reviewer/approver/negotiator are this instance's own internal RBAC roles
	// (the origin's local users handle them); the counterparty is the single
	// peer the contract is offered to. Origin + Counterparty are the two
	// parties (ADR-13).
	resp := db.Responsible{
		Creator:      localPeer,
		Reviewers:    []string{localPeer},
		Approvers:    []string{localPeer},
		Negotiators:  []string{localPeer},
		Counterparty: cmd.Counterparty,
	}

	// Seed one AcroForm signature field per party (origin + counterparty) into the
	// genesis document, so the very first render carries the full signable
	// structure. A signature field can only be materialized by a fresh render;
	// seeding it here means every later render is a provenance-preserving amend of
	// the stored PDF (or a verbatim carry-over of an inbound one) rather than a
	// fresh render that would strip the C2PA chain and signatures (ADR-12/ADR-13).
	seeded, changed, err := seedSignatureFields(*normalizedContractData, resp.GetParties())
	if err != nil {
		return fmt.Errorf("could not seed signature fields: %w", err)
	}
	if changed {
		normalizedContractData = &seeded
	}

	data := db.Contract{
		DID:             cmd.DID,
		Origin:          localPeer,
		CreatedBy:       cmd.CreatedBy,
		State:           contractstate.Draft.String(),
		ContractData:    normalizedContractData,
		TemplateDID:     cmd.TemplateDID,
		TemplateVersion: contractTemplate.TemplateVersion,
		Name:            contractTemplate.Name,
		Description:     contractTemplate.Description,
		Responsible:     &resp,
	}
	err = h.CRepo.Create(ctx, tx, data)
	if err != nil {
		return fmt.Errorf("could not create contract: %w", err)
	}

	err = createTasks(ctx, tx, h.RTRepo, h.ATRepo, h.NTRepo, cmd.DID, cmd.CreatedBy, resp)
	if err != nil {
		return err
	}

	evt := contractevents.CreateEvent{
		DID:          cmd.DID,
		TemplateDID:  cmd.TemplateDID,
		CreatedBy:    cmd.CreatedBy,
		Name:         contractTemplate.Name,
		Description:  contractTemplate.Description,
		ContractData: normalizedContractData,
		OccurredAt:   data.CreatedAt,
		HolderDID:    cmd.HolderDID,
		UserRoles:    cmd.UserRoles,
		Responsible:  &resp,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}

// attachContractParties records the organizations authorized to read this
// contract as typed dcs:CompanyParty nodes under "dcs:parties". The legal
// name (the same value the OID4VP organization claim discloses) gates read
// access in query/contract/querybyid.go. Read authorization only: ODRL rule
// parties are bound from workflow evidence (bindOriginatorParty at
// creation, the counterparty when signing completes).
func attachContractParties(raw *datatype.JSON, parties []string) (*datatype.JSON, error) {
	var doc map[string]any
	if err := json.Unmarshal(*raw, &doc); err != nil {
		return nil, fmt.Errorf("could not decode contract data: %w", err)
	}
	nodes, _ := doc["dcs:parties"].([]any)
	for index, name := range parties {
		nodes = append(nodes, map[string]any{
			"@id":           fmt.Sprintf("%s#party-%d", doc["@id"], index),
			"@type":         "dcs:CompanyParty",
			"dcs:legalName": name,
		})
	}
	doc["dcs:parties"] = nodes
	encoded, err := datatype.NewJSON(doc)
	if err != nil {
		return nil, fmt.Errorf("could not encode contract data: %w", err)
	}
	return &encoded, nil
}

// bindOriginatorParty rewrites the role-derived placeholder party IRI for
// the role the creating organization declares for itself to the origin
// DID, so the contract's ODRL rules reference the originator as a real,
// resolvable identity from the moment the offer exists. If the rules do
// not reference the role, a party node is still recorded so the
// declaration is part of the document.
func bindOriginatorParty(raw *datatype.JSON, originDID, role string) (*datatype.JSON, error) {
	var doc map[string]any
	if err := json.Unmarshal(*raw, &doc); err != nil {
		return nil, fmt.Errorf("could not decode contract data: %w", err)
	}
	placeholder := partyPlaceholderIRI(doc, role)
	if placeholder != "" {
		replaceNodeIRI(doc, placeholder, originDID)
	} else {
		nodes, _ := doc["dcs:parties"].([]any)
		doc["dcs:parties"] = append(nodes, map[string]any{
			"@id":      originDID,
			"@type":    "dcs:CompanyParty",
			"dcs:role": role,
		})
	}
	encoded, err := datatype.NewJSON(doc)
	if err != nil {
		return nil, fmt.Errorf("could not encode contract data: %w", err)
	}
	return &encoded, nil
}

// partyPlaceholderIRI finds the dcs:parties node whose IRI carries the
// #party-<role> fragment and returns its IRI ("" when absent).
func partyPlaceholderIRI(doc map[string]any, role string) string {
	nodes, _ := doc["dcs:parties"].([]any)
	for _, rawNode := range nodes {
		node, ok := rawNode.(map[string]any)
		if !ok {
			continue
		}
		if iri, _ := node["@id"].(string); strings.HasSuffix(iri, "#party-"+role) {
			return iri
		}
	}
	return ""
}

// replaceNodeIRI rewrites every "@id" equal to old with new, recursively.
func replaceNodeIRI(current any, old, new string) {
	switch value := current.(type) {
	case map[string]any:
		if iri, _ := value["@id"].(string); iri == old {
			value["@id"] = new
		}
		for _, nested := range value {
			replaceNodeIRI(nested, old, new)
		}
	case []any:
		for _, nested := range value {
			replaceNodeIRI(nested, old, new)
		}
	}
}
