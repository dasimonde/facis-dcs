package command

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

func TestVerifyShippedAllowsIdempotentReshipOfFormalDocument(t *testing.T) {
	contracts := &taskContractRepoFake{stored: taskStoredContract()}
	negotiations := &negotiationRepoFake{}

	err := VerifyShippedAgainstLocalRound(
		context.Background(), nil, contracts, negotiations,
		taskContractDID, 1, []byte(*contracts.stored.ContractData),
	)
	require.NoError(t, err)
}

func TestVerifyShippedAllowsCounterofferWithoutLocalAccepts(t *testing.T) {
	contracts := &taskContractRepoFake{stored: taskStoredContract()}
	negotiations := &negotiationRepoFake{}
	shipped := datatype.JSON(`{
		"@type":"dcs:Contract",
		"dcs:documentStructure":{"dcs:blocks":{"@list":[]},"dcs:layout":[]},
		"dcs:metadata":{"@type":"dcs:ContractMetadata","dcs:title":"Counteroffer"}
	}`)

	err := VerifyShippedAgainstLocalRound(
		context.Background(), nil, contracts, negotiations,
		taskContractDID, 1, []byte(shipped),
	)
	require.NoError(t, err)
}

func TestVerifyShippedAcceptsReconstructedDescriptionOnlyMerge(t *testing.T) {
	base := taskStoredContract()
	name := "Formal Title"
	base.Name = &name
	stamped, _, err := stampSharedDetailsIntoContractData(base.ContractData, sharedContractDetails{Name: &name})
	require.NoError(t, err)
	base.ContractData = stamped

	newDesc := "Accepted description only"
	change, err := datatype.NewJSON(map[string]any{"description": newDesc})
	require.NoError(t, err)

	contracts := &taskContractRepoFake{stored: base}
	negotiations := &negotiationRepoFake{
		accepted: map[int][]db.NegotiationChangeData{
			1: {{ID: "n1", ChangeRequest: &change}},
		},
	}

	expected, err := reconstructExpectedShippedDocument(context.Background(), nil, contracts, negotiations, base, 1)
	require.NoError(t, err)

	err = VerifyShippedAgainstLocalRound(
		context.Background(), nil, contracts, negotiations,
		taskContractDID, 1, []byte(*expected),
	)
	require.NoError(t, err)

	parsed := parseShippedSharedDetails([]byte(*expected))
	require.NotNil(t, parsed.Description)
	require.Equal(t, newDesc, *parsed.Description)
	require.Equal(t, name, *parsed.Name)
}

func TestVerifyShippedRefusesConflictWithAcceptedRound(t *testing.T) {
	base := taskStoredContract()
	name := "Formal Title"
	base.Name = &name
	stamped, _, err := stampSharedDetailsIntoContractData(base.ContractData, sharedContractDetails{Name: &name})
	require.NoError(t, err)
	base.ContractData = stamped

	newDesc := "Accepted description"
	change, err := datatype.NewJSON(map[string]any{"description": newDesc})
	require.NoError(t, err)

	contracts := &taskContractRepoFake{stored: base}
	negotiations := &negotiationRepoFake{
		accepted: map[int][]db.NegotiationChangeData{
			1: {{ID: "n1", ChangeRequest: &change}},
		},
	}

	conflict := map[string]any{}
	require.NoError(t, json.Unmarshal([]byte(*base.ContractData), &conflict))
	meta := conflict["dcs:metadata"].(map[string]any)
	meta["dcs:description"] = "Not what was accepted"
	conflictRaw, err := json.Marshal(conflict)
	require.NoError(t, err)

	err = VerifyShippedAgainstLocalRound(
		context.Background(), nil, contracts, negotiations,
		taskContractDID, 1, conflictRaw,
	)
	require.ErrorIs(t, err, ErrShippedDisagreesWithLocalRound)
}

func TestAssertSharedDetailsConsistentRefusesRowMetadataMismatch(t *testing.T) {
	name := "Row Title"
	embedded := "Embedded Title"
	raw := datatype.JSON(`{"dcs:metadata":{"dcs:title":"` + embedded + `"}}`)

	err := AssertSharedDetailsConsistent(&name, nil, nil, nil, &raw)
	require.ErrorIs(t, err, ErrSharedDetailsInconsistent)
	require.ErrorContains(t, err, "name")
}

func TestAssertSharedDetailsConsistentAllowsMatchingFields(t *testing.T) {
	name := "Same"
	desc := "Also same"
	policy := "RENEWAL"
	notice := 30
	raw := datatype.JSON(`{
		"dcs:metadata":{
			"dcs:title":"Same",
			"dcs:description":"Also same",
			"dcs:expirationPolicy":"RENEWAL",
			"dcs:expirationNoticePeriod":30
		}
	}`)

	require.NoError(t, AssertSharedDetailsConsistent(&name, &desc, &policy, &notice, &raw))
}

func TestVerifyShippedAcceptsReconstructedExpirationOnlyMerge(t *testing.T) {
	base := taskStoredContract()
	name := "Formal Title"
	policy := "TERMINATION"
	notice := 7
	base.Name = &name
	base.ExpPolicy = &policy
	base.ExpNoticePeriod = &notice
	stamped, _, err := stampSharedDetailsIntoContractData(base.ContractData, sharedContractDetails{
		Name:            &name,
		ExpPolicy:       &policy,
		ExpNoticePeriod: &notice,
	})
	require.NoError(t, err)
	base.ContractData = stamped

	newPolicy := "RENEWAL"
	newNotice := 30
	change, err := datatype.NewJSON(map[string]any{
		"exp_policy":        newPolicy,
		"exp_notice_period": newNotice,
	})
	require.NoError(t, err)

	contracts := &taskContractRepoFake{stored: base}
	negotiations := &negotiationRepoFake{
		accepted: map[int][]db.NegotiationChangeData{
			1: {{ID: "n-exp", ChangeRequest: &change}},
		},
	}

	expected, err := reconstructExpectedShippedDocument(context.Background(), nil, contracts, negotiations, base, 1)
	require.NoError(t, err)

	require.NoError(t, VerifyShippedAgainstLocalRound(
		context.Background(), nil, contracts, negotiations,
		taskContractDID, 1, []byte(*expected),
	))

	parsed := parseShippedSharedDetails([]byte(*expected))
	require.Equal(t, newPolicy, *parsed.ExpPolicy)
	require.Equal(t, newNotice, *parsed.ExpNoticePeriod)
}

func TestFederationAgreementDigestNeutralizesAPIOrigins(t *testing.T) {
	a := []byte(`{"@id":"http://localhost:8991/api/contract/x","@context":"http://localhost:8991/api/semantic/context/facis-dcs?version=2","dcs:parties":[{"@id":"did:web:localhost%3A8991"}]}`)
	b := []byte(`{"@id":"http://localhost:8992/api/contract/x","@context":"http://localhost:8992/api/semantic/context/facis-dcs?version=2","dcs:parties":[{"@id":"did:web:localhost%3A8991"}]}`)
	da, err := federationAgreementDigest(a)
	require.NoError(t, err)
	db, err := federationAgreementDigest(b)
	require.NoError(t, err)
	require.Equal(t, da, db)

	bddA := []byte(`{"@id":"http://dcs-a.localhost:18080/digital-contracting-service/api/contract/x","@context":"http://dcs-a.localhost:18080/digital-contracting-service/api/semantic/context/facis-dcs?version=2","dcs:parties":[{"@id":"did:web:dcs-a.localhost%3A18080"}]}`)
	bddB := []byte(`{"@id":"http://dcs-b.localhost:18080/digital-contracting-service/api/contract/x","@context":"http://dcs-b.localhost:18080/digital-contracting-service/api/semantic/context/facis-dcs?version=2","dcs:parties":[{"@id":"did:web:dcs-a.localhost%3A18080"}]}`)
	daBDD, err := federationAgreementDigest(bddA)
	require.NoError(t, err)
	dbBDD, err := federationAgreementDigest(bddB)
	require.NoError(t, err)
	require.Equal(t, daBDD, dbBDD)

	// Party DIDs must remain distinct across parties — rewriting must not
	// collapse did:web hosts.
	other := []byte(`{"@id":"http://localhost:8992/api/contract/x","dcs:parties":[{"@id":"did:web:localhost%3A8992"}]}`)
	dOther, err := federationAgreementDigest(other)
	require.NoError(t, err)
	require.NotEqual(t, da, dOther)
}
