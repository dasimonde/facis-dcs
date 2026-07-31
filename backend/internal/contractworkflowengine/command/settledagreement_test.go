package command

import (
	"context"
	"encoding/json"
	"testing"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/contractworkflowengine/negotiationmerging"
	"digital-contracting-service/internal/pdfgeneration/provenance"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

type settledContractRepoFake struct {
	db.ContractRepo
	pdfState db.ContractPDFState
}

func (r *settledContractRepoFake) ReadPDFState(context.Context, *sqlx.Tx, string) (*db.ContractPDFState, error) {
	state := r.pdfState
	return &state, nil
}

// The defect this closes: the counterparty's copy stays in OFFERED when the
// originator ships a contract it has already signed (only a revocation may
// override intrinsic state), so the transition table still offers it its
// designated way out of an offer — and renegotiating there moves contract_data
// off the signed PDF that copy must never re-render, wedging it out of the
// federation for good.
func TestRenegotiationIsRefusedOnceTheArtifactCarriesThePeersSignature(t *testing.T) {
	repo := &settledContractRepoFake{pdfState: db.ContractPDFState{IPFSCID: "cid", C2PAState: "active"}}

	err := requireUnsettledAgreement(context.Background(), nil, repo, "did:web:example:contract:1")

	require.ErrorIs(t, err, ErrAgreementSettled)
}

// The headline flow: the counteroffer ping-pong before anyone has signed. The
// stored artifact is a plain draft render, so nothing here is settled.
func TestRenegotiationIsPermittedBeforeAnySignature(t *testing.T) {
	repo := &settledContractRepoFake{pdfState: db.ContractPDFState{IPFSCID: "cid", C2PAState: "draft"}}

	require.NoError(t, requireUnsettledAgreement(context.Background(), nil, repo, "did:web:example:contract:1"))
}

// A contract with no stored artifact yet (genesis, before the first render) is
// not settled either.
func TestRenegotiationIsPermittedWithNoStoredArtifact(t *testing.T) {
	repo := &settledContractRepoFake{}

	require.NoError(t, requireUnsettledAgreement(context.Background(), nil, repo, "did:web:example:contract:1"))
}

// The gate reads the artifact, not this instance's workflow: a received copy
// sitting in OFFERED holds a signed artifact and is settled, while every
// pre-signing local state over a draft artifact is not.
func TestSettlementIsReadFromTheArtifactNotTheLocalWorkflowState(t *testing.T) {
	for _, state := range []string{
		contractstate.Offered.String(),
		contractstate.Negotiation.String(),
		contractstate.Submitted.String(),
		contractstate.Reviewed.String(),
		contractstate.Approved.String(),
	} {
		draft, err := provenance.ArtifactC2PAState(state, []byte("%PDF-1.7\n1 0 obj\n<< >>\nendobj\n"))
		require.NoError(t, err)
		require.False(t, provenance.ArtifactCarriesSignature(draft), "state %s over an unsigned artifact", state)

		signed, err := provenance.ArtifactC2PAState(state, []byte("<< /Type /Sig /ByteRange [0 1 2 3] >>"))
		require.NoError(t, err)
		require.True(t, provenance.ArtifactCarriesSignature(signed), "state %s over a PAdES-signed artifact", state)
	}
}

// negotiate applies a structured redline to contract_data straight away and
// re-ships the result, so that is the change the settled-agreement gate has to
// stand in front of.
func TestStructuredRedlineRewritesTheDocument(t *testing.T) {
	redline, err := datatype.NewJSON(map[string]any{
		"contract_data": map[string]any{"dcs:documentStructure": map[string]any{}},
	})
	require.NoError(t, err)

	var change negotiationmerging.ChangeRequest
	require.NoError(t, json.Unmarshal(redline, &change))
	require.NotNil(t, change.ContractData)
}

// A free-text negotiation note leaves the document alone, so it stays legal on a
// settled agreement: it is what carries a copy still in OFFERED into NEGOTIATION
// and on towards its own countersignature.
func TestFreeTextNegotiationLeavesTheDocumentAlone(t *testing.T) {
	note, err := datatype.NewJSON("please confirm the delivery window")
	require.NoError(t, err)

	var change negotiationmerging.ChangeRequest
	if err := json.Unmarshal(note, &change); err == nil {
		require.Nil(t, change.ContractData)
	}
}
