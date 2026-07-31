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

func withCreationTimestamp(data db.Contract, evt contractevents.CreateEvent) (db.Contract, contractevents.CreateEvent) {
	occurredAt := time.Now().UTC()
	data.CreatedAt = occurredAt
	evt.OccurredAt = occurredAt
	return data, evt
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

	normalizedContractData, err = resolvePendingTargets(normalizedContractData)
	if err != nil {
		return fmt.Errorf("could not resolve the contract's own rule targets: %w", err)
	}

	localPeer, err := h.DIDDocument.GetID()
	if err != nil {
		return fmt.Errorf("could not get DID: %w", err)
	}

	if cmd.OriginatorRole != "" {
		normalizedContractData, err = bindOriginatorParty(normalizedContractData, localPeer, cmd.OriginatorRole, cmd.CreatedBy)
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

	// Declare the two parties (origin + counterparty, ADR-13) as party nodes of
	// the document itself and seed the signature field that names each of them,
	// so the genesis document carries the full signable structure. A signature
	// field can only be materialized by a fresh render; seeding it here means
	// every later render is a provenance-preserving amend of the stored PDF (or a
	// verbatim carry-over of an inbound one) rather than a fresh render that
	// would strip the C2PA chain and signatures (ADR-12/ADR-13).
	seeded, changed, err := SeedPartiesAndSignatureFields(*normalizedContractData, resp.GetParties())
	if err != nil {
		return fmt.Errorf("could not seed contract parties and signature fields: %w", err)
	}
	if changed {
		normalizedContractData = &seeded
	}

	data, evt := withCreationTimestamp(db.Contract{
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
	}, contractevents.CreateEvent{
		DID:          cmd.DID,
		TemplateDID:  cmd.TemplateDID,
		CreatedBy:    cmd.CreatedBy,
		Name:         contractTemplate.Name,
		Description:  contractTemplate.Description,
		ContractData: normalizedContractData,
		HolderDID:    cmd.HolderDID,
		UserRoles:    cmd.UserRoles,
		Responsible:  &resp,
	})
	err = h.CRepo.Create(ctx, tx, data)
	if err != nil {
		return fmt.Errorf("could not create contract: %w", err)
	}

	err = createTasks(ctx, tx, h.RTRepo, h.ATRepo, h.NTRepo, cmd.DID, cmd.CreatedBy, resp)
	if err != nil {
		return err
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

// resolvePendingTargets points every rule that targets "the contract" at this
// contract. The builder cannot know the contract IRI while a template is being
// authored, so it writes a pending-target placeholder; derivation rebases it
// onto the TEMPLATE, which is not what the rule means and resolves to nothing
// in the contract. Left alone, a deployed policy names an asset the target
// system cannot dereference — and the target system is the party that has to
// act on it (SRS §1.2).
func resolvePendingTargets(raw *datatype.JSON) (*datatype.JSON, error) {
	var doc map[string]any
	if err := json.Unmarshal(*raw, &doc); err != nil {
		return nil, fmt.Errorf("could not decode contract data: %w", err)
	}
	contractIRI, _ := doc["@id"].(string)
	if strings.TrimSpace(contractIRI) == "" {
		return raw, nil
	}
	policies, ok := doc["dcs:policies"].(map[string]any)
	if !ok {
		return raw, nil
	}
	rewritten := false
	for _, bucket := range []string{"odrl:permission", "odrl:prohibition", "odrl:obligation"} {
		rules, ok := policies[bucket].([]any)
		if !ok {
			continue
		}
		for _, rawRule := range rules {
			rule, ok := rawRule.(map[string]any)
			if !ok {
				continue
			}
			target, ok := rule["odrl:target"].(map[string]any)
			if !ok {
				continue
			}
			if iri, _ := target["@id"].(string); strings.HasSuffix(iri, "#pending-target") || iri == pendingTargetURN {
				target["@id"] = contractIRI
				rewritten = true
			}
		}
	}
	if !rewritten {
		return raw, nil
	}
	encoded, err := datatype.NewJSON(doc)
	if err != nil {
		return nil, fmt.Errorf("could not encode contract data: %w", err)
	}
	return &encoded, nil
}

// pendingTargetURN is what the template builder writes for "the contract"
// before a contract IRI exists (dcsDraftStore.ts contractTargetIri).
const pendingTargetURN = "urn:uuid:pending-target"

// bindOriginatorParty rewrites the role-derived placeholder party IRI for
// the role the creating organization declares for itself to the origin
// DID, so the contract's ODRL rules reference the originator as a real,
// resolvable identity from the moment the offer exists. If the rules do
// not reference the role, a party node is still recorded so the
// declaration is part of the document.
//
// The node it produces carries BOTH halves of a party's identity: "@id" is
// the did:web of the instance the party acts on, and dcs:legalName is the
// organization within that instance (the OID4VP organization claim, the
// value read back as the caller's identity). Two keys are needed because
// they answer different questions and neither implies the other — several
// organizations share one instance, which is what the party read-scoping
// scenarios in features/03_contract_creation exercise, while one
// organization is reachable on exactly one instance's did:web.
//
// Writing both onto one node is what lets mergePartyNodes fold the
// originator's attribution node together with its authorization node: the
// merge keys on "@id", so a legal-name-only node under a #party-N IRI could
// never fold into the DID node, and the read-ACL — which reads only
// dcs:legalName — could never see the originator at all.
func bindOriginatorParty(raw *datatype.JSON, originDID, role, legalName string) (*datatype.JSON, error) {
	var doc map[string]any
	if err := json.Unmarshal(*raw, &doc); err != nil {
		return nil, fmt.Errorf("could not decode contract data: %w", err)
	}
	placeholder := partyPlaceholderIRI(doc, role)
	if placeholder != "" {
		replaceNodeIRI(doc, placeholder, originDID)
		setPartyLegalName(doc, originDID, legalName)
	} else {
		node := map[string]any{
			"@id":      originDID,
			"@type":    "dcs:CompanyParty",
			"dcs:role": role,
		}
		if legalName != "" {
			node["dcs:legalName"] = legalName
		}
		nodes, _ := doc["dcs:parties"].([]any)
		doc["dcs:parties"] = append(nodes, node)
	}
	encoded, err := datatype.NewJSON(doc)
	if err != nil {
		return nil, fmt.Errorf("could not encode contract data: %w", err)
	}
	return &encoded, nil
}

// setPartyLegalName records the organization on the party node named by iri,
// without overwriting a name the document already carries for it.
func setPartyLegalName(doc map[string]any, iri, legalName string) {
	if legalName == "" {
		return
	}
	nodes, _ := doc["dcs:parties"].([]any)
	for _, rawNode := range nodes {
		node, ok := rawNode.(map[string]any)
		if !ok {
			continue
		}
		if nodeIRI, _ := node["@id"].(string); nodeIRI != iri {
			continue
		}
		if existing, _ := node["dcs:legalName"].(string); existing == "" {
			node["dcs:legalName"] = legalName
		}
		return
	}
}

// SeedPartiesAndSignatureFields seeds a contract's party nodes and the
// signature fields naming them in one step. The two are one structure and are
// seeded together so they cannot drift apart: a signature is attributed to the
// party node its signature field names (signingmanagement/command/apply.go
// recordSignatory), and the Power of Attorney behind it is recorded there for
// the counterparty to verify (ADR-31). A field whose party has no node
// attributes to nobody, silently — the contract still signs, and the evidence
// simply is not there.
//
// It is additive and idempotent, so it also RE-seeds: a client that rebuilds
// the contract document from its own editor state and posts it back drops
// whatever it does not model, and dcs:parties is the first casualty. Every
// party the document already declares is left exactly as it stands, including
// the dcs:hasSignatory / dcs:hasPowerOfAttorney an applied signature stamped on
// it, so re-seeding never overwrites recorded attribution.
func SeedPartiesAndSignatureFields(raw datatype.JSON, partyDIDs []string) (datatype.JSON, bool, error) {
	withParties, partiesChanged, err := SeedContractParties(raw, partyDIDs)
	if err != nil {
		return nil, false, err
	}
	seeded, fieldsChanged, err := SeedSignatureFields(withParties, partyDIDs)
	if err != nil {
		return nil, false, err
	}
	return seeded, partiesChanged || fieldsChanged, nil
}

// reseedPartiesAndSignatureFields re-applies SeedPartiesAndSignatureFields to
// the stored document and persists only a real addition. It is called from
// every command that has just replaced the document wholesale with one a client
// assembled (negotiate's redline, submit's submitted draft), which is where the
// party nodes go missing.
//
// The party set is the workflow's record of who the parties are
// (db.Responsible), so it is what must be present; the document is not pruned to
// match it. A party node may already carry an applied signature, and dropping
// one because the counterparty was re-assigned would erase that evidence.
func reseedPartiesAndSignatureFields(ctx context.Context, tx *sqlx.Tx, cRepo db.ContractRepo, did string) error {
	contract, err := cRepo.ReadDataByDID(ctx, tx, did)
	if err != nil {
		return fmt.Errorf("could not read contract for party and signature-field seeding: %w", err)
	}
	if contract.ContractData == nil || !contract.ContractData.IsNotNullValue() || contract.Responsible == nil {
		return nil
	}
	seeded, changed, err := SeedPartiesAndSignatureFields(*contract.ContractData, contract.Responsible.GetParties())
	if err != nil {
		return fmt.Errorf("could not seed contract parties and signature fields: %w", err)
	}
	if !changed {
		return nil
	}
	if err := cRepo.Update(ctx, tx, db.ContractUpdateData{DID: did, ContractData: &seeded}); err != nil {
		return fmt.Errorf("could not persist seeded contract parties and signature fields: %w", err)
	}
	return nil
}

// SeedContractParties records one typed dcs:CompanyParty node per contract
// party DID (origin + counterparty, the same set the signature fields are
// seeded for), skipping any party the document already declares — a role
// placeholder bound to the origin, or a read-authorization node.
//
// The IRI is the party's own DID, so the seeded signature field's
// dcs:signatoryName and the party node it attributes a signature to are the
// same identifier on both instances of a federated contract.
func SeedContractParties(raw datatype.JSON, partyDIDs []string) (datatype.JSON, bool, error) {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, false, fmt.Errorf("could not decode contract data: %w", err)
	}

	nodes, _ := doc["dcs:parties"].([]any)
	declared := map[string]bool{}
	for _, rawNode := range nodes {
		node, ok := rawNode.(map[string]any)
		if !ok {
			continue
		}
		if iri, _ := node["@id"].(string); iri != "" {
			declared[iri] = true
		}
	}

	changed := false
	for _, did := range partyDIDs {
		if did == "" || declared[did] {
			continue
		}
		nodes = append(nodes, map[string]any{
			"@id":   did,
			"@type": "dcs:CompanyParty",
		})
		declared[did] = true
		changed = true
	}
	if !changed {
		return raw, false, nil
	}

	doc["dcs:parties"] = nodes
	encoded, err := datatype.NewJSON(doc)
	if err != nil {
		return nil, false, fmt.Errorf("could not encode contract data: %w", err)
	}
	return encoded, true, nil
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
