package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/pgtest"
	syncpg "digital-contracting-service/internal/dcstodcs/db/pg"
	"digital-contracting-service/internal/middleware"
)

// CWE-6 (DCS-FR-CWE-07): the provenance of a received contract carries the
// whole contract document, so only a caller that may read the contract gets
// it. Any other caller gets the same not_found as for a contract without
// provenance.
func TestGetProvenanceHidesOtherOrganizationsContract(t *testing.T) {
	database := pgtest.Open(t)
	const did = "did:web:peer.example:contract:7"
	_, err := database.Exec(`INSERT INTO contracts (did, origin, created_by, state, template_did, template_version, contract_data)
		VALUES ($1, 'did:web:peer.example', 'Org Y', 'SIGNED', 'tpl', 1, '{"dcs:parties":[{"dcs:legalName":"Org Y"}]}')`, did)
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contract_sync_signatures (did, contract_version, from_peer_did, jades_signature)
		VALUES ($1, 1, 'did:web:peer.example', 'aGVhZGVy.c2VjcmV0IHRlcm1z.c2ln')`, did)
	require.NoError(t, err)
	svc := &dcsToDcssrvc{DB: database, SRepo: syncpg.PostgresSyncRepository{}, DIDDocument: *eraseTestDIDDocument(t, "local.example")}
	read := func(organization string) (*dcstodcs.DCSToDCSSyncProvenanceResponse, error) {
		ctx := middleware.InjectAuthContext(context.Background(), []string{userrole.ContractObserver.String()}, "did:key:holder", organization)
		return svc.GetProvenance(ctx, &dcstodcs.GetProvenancePayload{Did: did})
	}

	_, err = read("Org X")
	requireServiceError(t, err, "not_found")

	provenance, err := read("Org Y")
	require.NoError(t, err)
	require.Equal(t, did, provenance.Did)
}
