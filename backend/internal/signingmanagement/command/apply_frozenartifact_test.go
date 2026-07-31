package command

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/jades"
	db "digital-contracting-service/internal/signingmanagement/db"
)

const (
	signerB     = "did:jwk:eyJrdHkiOiJFQyIsImIiOjF9"
	contractIRI = "did:web:dcs-a.localhost%3A18080:contract:" + contractID
)

// persistedCopy is what a database hands back for a document it stored: the same
// contract, re-serialized. Postgres normalizes JSONB on write, so the bytes a
// peer embedded in its PDF are never the bytes its counterparty reads back out
// of its own contract_data column — the JAdES payload is JCS-canonicalized
// (RFC 8785) precisely so the two still agree.
func persistedCopy(t *testing.T, raw datatype.JSON) datatype.JSON {
	t.Helper()
	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))
	stored, err := datatype.NewJSON(doc)
	require.NoError(t, err)
	return stored
}

func strptr(s string) *string { return &s }

// A countersignature cannot change the contract document: the artifact it signs
// is the originator's PAdES-signed PDF, which can never be re-rendered, so the
// document embedded in it is the last word on what the contract says.
//
// The whole federated exchange rests on that. The countersigner ships a JAdES
// built over its own contract_data, and the receiver rebuilds the payload it
// expects from the document embedded in the shipped PDF — so any byte the
// countersignature adds to contract_data and cannot add to the PDF makes the two
// disagree, and the ship is refused with "JAdES payload does not match the
// contract document embedded in the shipped PDF" on every retry, forever.
func TestACountersignatureShipsAJAdESOverTheDocumentEmbeddedInItsPDF(t *testing.T) {
	// The originator signs first: nothing is frozen yet, so the agreement is
	// sealed, the signature is attributed, and the result is re-embedded into the
	// PDF that is then shipped.
	originator := &db.Responsible{Creator: instanceA, Counterparty: instanceB}
	genesis := genesisContractDocument(t, instanceB)
	embedded, mutated, err := contractDocumentForSignature(genesis, originator, signerA, instanceA, instanceA, false)
	require.NoError(t, err)
	require.True(t, mutated, "the first signature seals the agreement and attributes itself")

	// The counterparty receives that PDF verbatim and rebuilds its own copy from
	// the payload embedded in it.
	countersigner := &db.Responsible{Creator: instanceA, Counterparty: instanceB}
	stored := persistedCopy(t, embedded)

	// It now countersigns. Its own database holds no signature row for the
	// originator's signature, so the signature count says "first"; only the
	// stored artifact says the document is frozen.
	countersigned, mutated, err := contractDocumentForSignature(stored, countersigner, signerB, instanceB, instanceB, true)
	require.NoError(t, err)

	// The ship: the JAdES is built over the countersigner's contract_data
	// (dcstodcs/synchronizer.go jadesForSignedContract) — and this is also the
	// payload pinned for the signatory's own JAdES at prepare.
	shipped, err := jades.BuildContractPayload(contractIRI, 7, countersigned)
	require.NoError(t, err)

	// The receipt: the peer rebuilds the expected payload from the document
	// embedded in the shipped PDF, at the version the JAdES itself claims
	// (service/dcs_to_dcs.go verifyShippedJades).
	expected, err := jades.BuildContractPayload(contractIRI, 7, embedded)
	require.NoError(t, err)

	require.Equal(t, string(expected), string(shipped),
		"the countersigner's shipped JAdES must attest the document embedded in the PDF it ships")
	require.False(t, mutated, "a countersignature must not rewrite a contract whose artifact is already signed")

	// What this replaces: attributing the countersignature into contract_data
	// leaves the shipped JAdES describing a document embedded nowhere, and the
	// peer refuses the exchange — which is what made a federated contract
	// undeployable, since the deploy gate requires that JAdES as its evidence
	// that the counterparty signed.
	rewritten, err := recordSignatory(stored, countersigner, signerB, instanceB, instanceB)
	require.NoError(t, err)
	drifted, err := jades.BuildContractPayload(contractIRI, 7, rewritten)
	require.NoError(t, err)
	require.NotEqual(t, string(expected), string(drifted),
		"rewriting the frozen document is exactly the divergence the peer rejects")
}

// The document a frozen artifact holds is returned untouched, byte for byte:
// anything else, however semantically equal, would change the JCS
// canonicalization only if it changed the document — but a caller that persists
// a "changed" document also bumps the contract version and re-hashes the
// payload, so the artifact and its record drift apart even when the JSON does not.
func TestAFrozenContractDocumentIsReturnedUnchanged(t *testing.T) {
	responsible := &db.Responsible{Creator: instanceA, Counterparty: instanceB}
	genesis := genesisContractDocument(t, instanceB)
	signed, _, err := contractDocumentForSignature(genesis, responsible, signerA, instanceA, instanceA, false)
	require.NoError(t, err)

	frozen, mutated, err := contractDocumentForSignature(signed, responsible, signerB, instanceB, instanceB, true)
	require.NoError(t, err)
	require.False(t, mutated)
	require.Equal(t, []byte(signed), []byte(frozen), "the frozen document is the same bytes, not an equivalent re-encoding")

	// The originator's own attribution, made while the document was still
	// mutable, survives the countersignature untouched.
	nodes := partyNodes(t, frozen)
	require.Equal(t, map[string]any{"@id": signerA}, nodes[instanceA]["dcs:hasSignatory"])
	require.Equal(t, map[string]any{"@id": instanceA}, nodes[instanceA]["dcs:hasPowerOfAttorney"])
}

// The first signature is unaffected: it still seals the offer into the
// odrl:Agreement and records who signed for which party under what authority,
// because that document is still re-embedded into the PDF the signature covers.
func TestTheFirstSignatureStillSealsAndAttributesTheContract(t *testing.T) {
	raw, err := datatype.NewJSON(map[string]any{
		"@id":          "urn:contract:1",
		"dcs:policies": map[string]any{"@type": "odrl:Offer"},
		"dcs:parties": []any{
			map[string]any{"@id": instanceA, "@type": "dcs:CompanyParty"},
			map[string]any{"@id": instanceB, "@type": "dcs:CompanyParty"},
		},
	})
	require.NoError(t, err)

	responsible := &db.Responsible{Creator: instanceA, Counterparty: instanceB}
	signed, mutated, err := contractDocumentForSignature(raw, responsible, signerA, instanceA, instanceA, false)
	require.NoError(t, err)
	require.True(t, mutated)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(signed, &doc))
	require.Equal(t, "odrl:Agreement", doc["dcs:policies"].(map[string]any)["@type"],
		"the offer is sealed into the agreement the signatures bind")

	nodes := partyNodes(t, signed)
	require.Equal(t, map[string]any{"@id": signerA}, nodes[instanceA]["dcs:hasSignatory"])
	require.Equal(t, map[string]any{"@id": instanceA}, nodes[instanceA]["dcs:hasPowerOfAttorney"])
}

// ceremonyAuthorities answers for the ceremonies a contract's signatures were
// made under.
type ceremonyAuthorities map[string]*db.SignatureCeremony

func (c ceremonyAuthorities) GetCeremonyByID(_ context.Context, _ *sqlx.Tx, id string) (*db.SignatureCeremony, error) {
	return c[id], nil
}

// A signature the frozen document cannot record is still judged: its authority
// is retained on the ceremony that produced it (and shipped to the counterparty
// in the signing summary issued from it), so the compliance viewer reads it from
// there rather than reporting nothing — which would be indistinguishable from a
// compliant signature (UC-14, FR-SM-03/-04, FR-SM-26).
func TestComplianceJudgesASignatureTheFrozenDocumentCannotRecord(t *testing.T) {
	responsible := &db.Responsible{Creator: instanceA, Counterparty: instanceB}
	genesis := genesisContractDocument(t, instanceB)
	document, _, err := contractDocumentForSignature(genesis, responsible, signerA, instanceA, instanceA, false)
	require.NoError(t, err)

	findings, attributed := poaComplianceFindings(document)
	require.Empty(t, findings)
	require.True(t, attributed[instanceA], "the originator's attribution is in the document")
	require.False(t, attributed[instanceB], "the countersignature's is not, and cannot be")

	signatures := []db.SignatureRecord{
		{Status: "SIGNED", SignerDID: signerA, FieldName: strptr(instanceA), CeremonyID: strptr("ceremony-a")},
		{Status: "SIGNED", SignerDID: signerB, FieldName: strptr(instanceB), CeremonyID: strptr("ceremony-b")},
	}
	authorities := ceremonyAuthorities{
		"ceremony-a": {ID: "ceremony-a", FieldName: instanceA, PoAOrganization: strptr(instanceA)},
		"ceremony-b": {ID: "ceremony-b", FieldName: instanceB, PoAOrganization: strptr(instanceB)},
	}

	judged, err := appliedSignatureFindings(context.Background(), nil, authorities, signatures, attributed)
	require.NoError(t, err)
	require.Empty(t, judged, "both signatures were made under a Power of Attorney authorizing their own party")

	// The same countersignature made under no Power of Attorney raises the
	// finding the viewer exists for, from the evidence alone.
	authorities["ceremony-b"] = &db.SignatureCeremony{ID: "ceremony-b", FieldName: instanceB}
	judged, err = appliedSignatureFindings(context.Background(), nil, authorities, signatures, attributed)
	require.NoError(t, err)
	require.Len(t, judged, 1)
	require.Contains(t, judged[0], instanceB)
	require.Contains(t, judged[0], signerB)

	// And a party the document already accounts for is never judged twice.
	authorities["ceremony-a"] = &db.SignatureCeremony{ID: "ceremony-a", FieldName: instanceA}
	judged, err = appliedSignatureFindings(context.Background(), nil, authorities, signatures, attributed)
	require.NoError(t, err)
	require.Len(t, judged, 1, "the document's own verdict on a party it records is the only one")
}
