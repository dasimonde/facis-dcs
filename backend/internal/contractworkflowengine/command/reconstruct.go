package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/jades"
	"digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/contractworkflowengine/negotiationmerging"
)

// apiOriginPattern matches an instance's public API base inside absolute IRIs
// (DCS_PUBLIC_URL), e.g. http://localhost:8991/api. Party did:web IRIs are left
// alone — they are identity, not document location.
var apiOriginPattern = regexp.MustCompile(`https?://[^/"\s?#]+(?:/[^/"\s?#]+)*/api`)

// ErrShippedDisagreesWithLocalRound refuses a peer PDF re-ship whose embedded
// document is not what this instance rebuilds from its formal base plus the
// confirmed accepted decisions for that round.
var ErrShippedDisagreesWithLocalRound = errors.New("shipped contract does not match the locally reconstructed negotiation result")

// VerifyShippedAgainstLocalRound rebuilds the expected post-Submit document from
// the local formal row and accepted change requests for contractVersion, then
// compares it to the peer-shipped JSON-LD. An identical re-ship of the current
// formal document is allowed (idempotent PDF retry).
//
// Reconstruct runs only when this instance already holds confirmed Accept rows
// for the round (post-Submit merge ships). A content change with no local
// Accepts is a proposal/counteroffer or signed-artifact re-ship (ADR-13 PDF
// exchange) — those must adopt without reconstruct. Negotiation rows travel on
// their own outbound channel; they are not a prerequisite for adopting a peer
// PDF that is not a Submit-merge ship.
func VerifyShippedAgainstLocalRound(
	ctx context.Context,
	tx *sqlx.Tx,
	cRepo db.ContractRepo,
	nRepo db.NegotiationRepo,
	did string,
	contractVersion int,
	shippedPayload []byte,
) error {
	contract, err := cRepo.ReadDataByDID(ctx, tx, did)
	if err != nil {
		return fmt.Errorf("could not read local contract for reconstruct: %w", err)
	}

	if contract.ContractData == nil || !contract.ContractData.IsNotNullValue() {
		return nil
	}

	currentDigest, err := jades.ContractDocumentDigest([]byte(*contract.ContractData))
	if err != nil {
		return fmt.Errorf("could not digest local formal contract: %w", err)
	}

	shippedDigest, err := jades.ContractDocumentDigest(shippedPayload)
	if err != nil {
		return fmt.Errorf("could not digest shipped contract: %w", err)
	}

	if currentDigest == shippedDigest {
		return nil
	}

	accepted, err := nRepo.ReadAllAcceptedByContractDIDAndVersion(ctx, tx, did, contractVersion)
	if err != nil {
		return fmt.Errorf("could not read accepted negotiations for reconstruct: %w", err)
	}

	if len(accepted) == 0 {
		return nil
	}

	expected, err := reconstructExpectedShippedDocument(ctx, tx, cRepo, nRepo, contract, contractVersion)
	if err != nil {
		return err
	}

	// Compare the folded negotiation result to the ship:
	//  - drop initiator-only shared-detail metadata the Submit stamp may add
	//    from its local row for peer adopt;
	//  - digest after rewriting each instance's DCS_PUBLIC_URL host in absolute
	//    IRIs to a neutral stand-in, so A's http://localhost:8991/api/... and
	//    B's reconstruct via http://localhost:8992/api/... agree.
	projected, err := projectShippedOntoAgreementMetadata(shippedPayload, []byte(*expected))
	if err != nil {
		return fmt.Errorf("could not project shipped metadata for agreement compare: %w", err)
	}

	want, err := federationAgreementDigest([]byte(*expected))
	if err != nil {
		return fmt.Errorf("could not digest reconstructed contract: %w", err)
	}
	got, err := federationAgreementDigest(projected)
	if err != nil {
		return fmt.Errorf("could not digest shipped contract for agreement compare: %w", err)
	}
	if want != got {
		return fmt.Errorf("%w: reconstructed agreement digest %s, shipped agreement digest %s",
			ErrShippedDisagreesWithLocalRound, want, got)
	}
	return nil
}

// federationAgreementDigest digests contract JSON-LD after rewriting every
// instance API origin (…/api) inside string IRIs to a stable stand-in. did:
// URIs are not rewritten. Without this, Submit-merge on the initiator and
// reconstruct-merge on the peer normalize the same redline to different
// DCS_PUBLIC_URL hosts and the gate refuses every federated post-Submit ship.
func federationAgreementDigest(payload []byte) (string, error) {
	canonical, err := rewriteAPIOrigins(payload)
	if err != nil {
		return "", err
	}
	return jades.ContractDocumentDigest(canonical)
}

func rewriteAPIOrigins(payload []byte) ([]byte, error) {
	var doc any
	if err := json.Unmarshal(payload, &doc); err != nil {
		return nil, err
	}
	rewriteAPIOriginsValue(doc)
	return json.Marshal(doc)
}

func rewriteAPIOriginsValue(v any) {
	switch x := v.(type) {
	case map[string]any:
		for key, child := range x {
			if s, ok := child.(string); ok {
				x[key] = rewriteAPIOriginString(s)
				continue
			}
			rewriteAPIOriginsValue(child)
		}
	case []any:
		for i, child := range x {
			if s, ok := child.(string); ok {
				x[i] = rewriteAPIOriginString(s)
				continue
			}
			rewriteAPIOriginsValue(child)
		}
	}
}

func rewriteAPIOriginString(s string) string {
	if strings.HasPrefix(s, "did:") {
		return s
	}
	return apiOriginPattern.ReplaceAllString(s, "http://dcs.invalid/api")
}

// projectShippedOntoAgreementMetadata keeps shipped dcs:metadata keys that the
// reconstructed document also carries (the round's agreed fields) and drops
// extras the initiator stamped from its own contracts row.
func projectShippedOntoAgreementMetadata(shipped, expected []byte) ([]byte, error) {
	var shippedDoc, expectedDoc map[string]any
	if err := json.Unmarshal(shipped, &shippedDoc); err != nil {
		return nil, err
	}

	if err := json.Unmarshal(expected, &expectedDoc); err != nil {
		return nil, err
	}

	shippedMeta, _ := shippedDoc["dcs:metadata"].(map[string]any)
	expectedMeta, _ := expectedDoc["dcs:metadata"].(map[string]any)

	if shippedMeta == nil {
		shippedMeta = map[string]any{}
	}

	filtered := map[string]any{}

	for key := range expectedMeta {
		if v, ok := shippedMeta[key]; ok {
			filtered[key] = v
		}
	}

	shippedDoc["dcs:metadata"] = filtered

	return json.Marshal(shippedDoc)
}

// reconstructExpectedShippedDocument folds accepted change requests the same
// way Submit does, reseeds parties/signature fields, and stamps only the
// Name/Description the round agreed — not local date/policy columns.
func reconstructExpectedShippedDocument(
	ctx context.Context,
	tx *sqlx.Tx,
	cRepo db.ContractRepo,
	nRepo db.NegotiationRepo,
	contract *db.Contract,
	contractVersion int,
) (*datatype.JSON, error) {
	updated, _, err := negotiationmerging.MergeChangeRequests(ctx, tx, cRepo, nRepo, contract.DID, contractVersion)

	if err != nil {
		return nil, fmt.Errorf("could not reconstruct expected negotiation result: %w", err)
	}

	payload := contract.ContractData

	if updated.ContractData != nil {
		payload = updated.ContractData
		// Match Submit: party/signature reseed runs only when the merge replaced
		// the document. Shared-details-only folds leave the formal bytes alone
		// aside from the metadata stamp below — reseeding here would disagree
		// with the initiating instance's post-merge PDF.
		if contract.Responsible != nil {
			seeded, _, err := SeedPartiesAndSignatureFields(*payload, contract.Responsible.GetParties())
			if err != nil {
				return nil, fmt.Errorf("could not seed reconstructed parties: %w", err)
			}
			payload = &seeded
		}
	}

	details := sharedContractDetails{
		Name:            firstNonNil(updated.Name, contract.Name),
		Description:     firstNonNil(updated.Description, contract.Description),
		StartDate:       updated.StartDate,
		ExpDate:         updated.ExpDate,
		ExpPolicy:       updated.ExpPolicy,
		ExpNoticePeriod: updated.ExpNoticePeriod,
		// Do not fall back to local row dates/policy: the initiating Submit may
		// stamp its own row into the PDF for peer adopt, and this instance's
		// columns can lag until that ship lands.
	}
	stamped, _, err := stampSharedDetailsIntoContractData(payload, details)

	if err != nil {
		return nil, fmt.Errorf("could not stamp reconstructed shared details: %w", err)
	}

	return stamped, nil
}

func firstNonNil[T any](preferred, fallback *T) *T {
	if preferred != nil {
		return preferred
	}

	return fallback
}
