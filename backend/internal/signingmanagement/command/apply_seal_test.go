package command

import (
	"encoding/json"
	"testing"

	"digital-contracting-service/internal/base/datatype"
	db "digital-contracting-service/internal/signingmanagement/db"

	"github.com/stretchr/testify/require"
)

// TestSealAgreementStampsPartyFunctions proves the seal tags both signatories
// with their ODRL party function (§4.3.7/4.3.8): the offeror/creator is the
// contractingParty, the accepting counterparty is the contractedParty.
func TestSealAgreementStampsPartyFunctions(t *testing.T) {
	doc := map[string]any{
		"@id":          "urn:contract:1",
		"dcs:policies": map[string]any{"@type": "odrl:Offer"},
		"dcs:parties": []any{
			map[string]any{"@id": "did:web:origin", "@type": "dcs:CompanyParty", "dcs:role": "assigner"},
			map[string]any{"@id": "urn:contract:1#party-assignee", "@type": "dcs:CompanyParty"},
		},
	}
	raw, err := datatype.NewJSON(doc)
	require.NoError(t, err)

	sealed, err := sealAgreementForSigning(raw, &db.Responsible{Creator: "did:web:origin"}, "did:web:signer")
	require.NoError(t, err)
	sealed, err = recordSignatory(sealed, &db.Responsible{Creator: "did:web:origin"}, "did:web:signer", "did:web:signer", "did:web:signer")
	require.NoError(t, err)

	var out map[string]any
	require.NoError(t, json.Unmarshal(sealed, &out))

	require.Equal(t, "odrl:Agreement", out["dcs:policies"].(map[string]any)["@type"])

	functions := map[string]string{}
	poa := map[string]string{}
	for _, rawNode := range out["dcs:parties"].([]any) {
		node := rawNode.(map[string]any)
		if fn, ok := node["odrl:function"].(map[string]any); ok {
			functions[node["@id"].(string)] = fn["@id"].(string)
		}
		if p, ok := node["dcs:hasPowerOfAttorney"].(map[string]any); ok {
			poa[node["@id"].(string)] = p["@id"].(string)
		}
	}
	require.Equal(t, "odrl:contractingParty", functions["did:web:origin"], "offeror is the contracting party")
	require.Equal(t, "odrl:contractedParty", functions["did:web:signer"], "counterparty is the contracted party")
	require.Equal(t, "did:web:signer", poa["did:web:signer"], "the signing party carries its Power of Attorney organization")
}

// When the ORIGINATOR signs first — which every two-instance scenario drives —
// the party that signs and the party the open placeholder resolves to are
// different. The signatory and its Power of Attorney belong on the signer's
// node; stamping them on the counterparty's asserts that the counterparty was
// signed for by someone else's signatory, and leaves the party the credential
// actually authorizes carrying no evidence at all.
func TestSealAgreementStampsTheSignatoryOnThePartyThatSigned(t *testing.T) {
	doc := map[string]any{
		"@id":          "urn:contract:1",
		"dcs:policies": map[string]any{"@type": "odrl:Offer"},
		"dcs:parties": []any{
			map[string]any{"@id": "did:web:a", "@type": "dcs:CompanyParty", "dcs:role": "assigner"},
			map[string]any{"@id": "urn:contract:1#party-assignee", "@type": "dcs:CompanyParty"},
		},
	}
	raw, err := datatype.NewJSON(doc)
	require.NoError(t, err)

	// A's own user signs A's field, while B is the contract's counterparty.
	responsible := &db.Responsible{Creator: "did:web:a", Counterparty: "did:web:b"}
	sealed, err := sealAgreementForSigning(raw, responsible, "did:jwk:aUser")
	require.NoError(t, err)
	sealed, err = recordSignatory(sealed, responsible, "did:jwk:aUser", "did:web:a", "did:web:a")
	require.NoError(t, err)

	var out map[string]any
	require.NoError(t, json.Unmarshal(sealed, &out))

	nodes := map[string]map[string]any{}
	for _, rawNode := range out["dcs:parties"].([]any) {
		node := rawNode.(map[string]any)
		nodes[node["@id"].(string)] = node
	}

	require.Contains(t, nodes, "did:web:b", "the open placeholder still resolves to the counterparty")

	signer := nodes["did:web:a"]
	require.Equal(t, map[string]any{"@id": "did:jwk:aUser"}, signer["dcs:hasSignatory"],
		"the signatory belongs on the party that signed")
	require.Equal(t, map[string]any{"@id": "did:web:a"}, signer["dcs:hasPowerOfAttorney"],
		"the Power of Attorney belongs with the signature it authorized")

	require.NotContains(t, nodes["did:web:b"], "dcs:hasSignatory",
		"the counterparty has not signed and must not be recorded as having a signatory")
	require.NotContains(t, nodes["did:web:b"], "dcs:hasPowerOfAttorney",
		"the counterparty's node must not carry the signer's Power of Attorney")
}

// Sealing is the acceptance act and happens once; attribution happens per
// signature. Recording the signatory only at the seal left every LATER
// signature's party carrying no authorization — while the credential behind it
// still shipped to the counterparty, which found nothing in the contract to
// check it against and refused the whole exchange.
func TestEverySignatureRecordsItsOwnAuthority(t *testing.T) {
	doc := map[string]any{
		"@id":          "urn:contract:1",
		"dcs:policies": map[string]any{"@type": "odrl:Offer"},
		"dcs:parties": []any{
			map[string]any{"@id": "did:web:a", "@type": "dcs:CompanyParty", "dcs:role": "assigner"},
			map[string]any{"@id": "urn:contract:1#party-assignee", "@type": "dcs:CompanyParty"},
		},
	}
	raw, err := datatype.NewJSON(doc)
	require.NoError(t, err)

	responsible := &db.Responsible{Creator: "did:web:a", Counterparty: "did:web:b"}

	// First signature: A signs A's field, and the agreement is sealed.
	sealed, err := sealAgreementForSigning(raw, responsible, "did:jwk:aUser")
	require.NoError(t, err)
	first, err := recordSignatory(sealed, responsible, "did:jwk:aUser", "did:web:a", "did:web:a")
	require.NoError(t, err)

	// Second signature: B countersigns its own field. No seal runs this time.
	second, err := recordSignatory(first, responsible, "did:jwk:bUser", "did:web:b", "did:web:b")
	require.NoError(t, err)

	var out map[string]any
	require.NoError(t, json.Unmarshal(second, &out))

	nodes := map[string]map[string]any{}
	for _, rawNode := range out["dcs:parties"].([]any) {
		node := rawNode.(map[string]any)
		nodes[node["@id"].(string)] = node
	}

	for party, signatory := range map[string]string{"did:web:a": "did:jwk:aUser", "did:web:b": "did:jwk:bUser"} {
		require.Equal(t, map[string]any{"@id": signatory}, nodes[party]["dcs:hasSignatory"],
			"party %s must record the signatory that signed for it", party)
		require.Equal(t, map[string]any{"@id": party}, nodes[party]["dcs:hasPowerOfAttorney"],
			"party %s must record the authority its signature was made under", party)
	}
}
