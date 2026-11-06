package service

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	signaturemanagement "digital-contracting-service/gen/signature_management"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/middleware"
	smpg "digital-contracting-service/internal/signingmanagement/db/pg"
)

type unusedRequestSigner struct{}

func (unusedRequestSigner) SignAuthorizationRequestJWT(jwt.MapClaims) (string, error) {
	return "", nil
}

const (
	ceremonyTestContract = "did:example:org-y-contract"
	ceremonyTestOrg      = "Org Y"
	ceremonyTestID       = "20000000-0000-4000-8000-000000000001"
	ceremonyTestSigner   = "did:jwk:org-y-signer"
)

// ceremonyTestService serves an Org Y contract with one verified ceremony
// that Org Y's signer published.
func ceremonyTestService(t *testing.T) (*sqlx.DB, *signatureManagementsrvc) {
	t.Helper()
	database := pgtest.Open(t)
	_, err := database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,contract_data) VALUES ($1,'',$2,'APPROVED','tpl',1,'{}')`, ceremonyTestContract, ceremonyTestOrg)
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO signature_ceremonies (id,contract_did,field_name,requested_by,requested_holder_did,status,nonce,signer_did,verified_at,expires_at,
		prepared_pdf,prepared_pdf_sha256,request_nonce,request_expires_at,published_by,published_holder_did)
		VALUES ($1,$2,$3,$3,'did:jwk:org-y-session','verified','n',$4,now(),now()+interval '1 hour',
		'%PDF-prepared',encode(sha256('%PDF-prepared'::bytea),'hex'),'request-nonce',now()+interval '10 minutes',$3,'did:jwk:org-y-session')`,
		ceremonyTestID, ceremonyTestContract, ceremonyTestOrg, ceremonyTestSigner)
	require.NoError(t, err)
	return database, &signatureManagementsrvc{
		DB: database, DIDDocument: *jadesTestDIDDocument(t, "local"), CeremonyRepo: &smpg.PostgresCeremonyRepo{},
		RequestSigner: unusedRequestSigner{}, OID4VPClientID: "x509_san_dns:dcs.example", PublicAPIBase: "https://dcs.example",
		PIDDCQLQuery: map[string]any{"credentials": []any{map[string]any{"id": "pid"}}},
		DCQLQuery:    map[string]any{"credentials": []any{map[string]any{"id": "poa"}}},
	}
}

// DCS-FR-SM-14, DCS-FR-SM-15, DCS-IR-SM-01: a Contract Signer of another
// organization can neither prepare, submit, start a ceremony for nor publish
// a signing request of a contract it may not read.
func TestSigningEndpointsRefuseAContractTheCallerMayNotRead(t *testing.T) {
	_, service := ceremonyTestService(t)
	ctx := middleware.InjectAuthContext(context.Background(), []string{"Contract Signer"}, "did:jwk:org-x-session", "Org X")
	field := ceremonyTestOrg

	_, err := service.PrepareSignature(ctx, &signaturemanagement.SMSignaturePrepareRequest{Did: ceremonyTestContract, SignerDid: ceremonyTestSigner, FieldName: &field})
	require.Equal(t, "forbidden", errorName(err))
	_, err = service.SubmitSignature(ctx, &signaturemanagement.SMSignatureSubmitRequest{Did: ceremonyTestContract, SignerDid: ceremonyTestSigner, FieldName: &field, SignedPdf: []byte("%PDF")})
	require.Equal(t, "forbidden", errorName(err))
	_, err = service.StartCeremony(ctx, &signaturemanagement.SMSignatureRequestStartRequest{ContractDid: ceremonyTestContract, FieldName: field})
	require.Equal(t, "forbidden", errorName(err))
	_, err = service.PublishSignatureRequest(ctx, &signaturemanagement.SMSignatureRequestPublishRequest{CeremonyID: ceremonyTestID})
	require.Equal(t, "forbidden", errorName(err))
}

// DCS-FR-SM-19, DCS-FR-UC-01-4: a refused ceremony presentation is audited
// with step PRESENTATION, a refused callback body or state is audited too,
// and the signer's status poll reports the last refusal (DCS-FR-SM-23).
func TestRefusedWalletResponsesAreAuditedAndReported(t *testing.T) {
	database, service := ceremonyTestService(t)
	ctx := context.Background()
	failures := func(step string) int {
		var n int
		require.NoError(t, database.Get(&n, `SELECT count(*) FROM outbox_events WHERE event_type = 'APPLY_SIGNATURE_FAILED' AND event_data->>'step' = $1`, step))
		return n
	}
	post := func(id, body string) error {
		_, err := service.SignatureRequestCallback(ctx, &signaturemanagement.SignatureRequestCallbackPayload{CeremonyID: id}, io.NopCloser(strings.NewReader(body)))
		return err
	}

	pending := "20000000-0000-4000-8000-000000000002"
	_, err := database.Exec(`INSERT INTO signature_ceremonies (id,contract_did,field_name,requested_by,status,nonce,expires_at) VALUES ($1,$2,$3,$3,'pending','n',now()+interval '1 hour')`, pending, ceremonyTestContract, ceremonyTestOrg)
	require.NoError(t, err)
	require.Error(t, post(pending, `vp_token={"pid":["eyJ.pid~"]}`))
	require.Equal(t, 1, failures("PRESENTATION"), "a presentation without a Power of Attorney is not audited")

	require.Error(t, post(ceremonyTestID, "state=another-ceremony&documentWithSignature=JVBERg=="))
	require.Equal(t, 1, failures("WALLET_CALLBACK"), "a callback for another state is not audited")

	authorized := middleware.InjectAuthContext(ctx, []string{"Contract Signer"}, "did:jwk:org-y-session", ceremonyTestOrg)
	status, err := service.CeremonyStatus(authorized, &signaturemanagement.SMSignatureRequestStatusRequest{CeremonyID: ceremonyTestID})
	require.NoError(t, err)
	require.Equal(t, "bad_request", *status.RefusalCode)
	require.Contains(t, *status.Refusal, "does not match ceremony")
	require.NotNil(t, status.RequestExpiresAt)
}

// DCS-FR-SM-15, DCS-FR-SM-19: the unauthenticated document endpoint audits a
// retrieval once per ceremony and document, not once per request.
func TestTheDocumentRetrievalIsAuditedOncePerDocument(t *testing.T) {
	database, service := ceremonyTestService(t)
	for range 3 {
		body, err := service.SignatureRequestDocument(context.Background(), &signaturemanagement.SignatureRequestDocumentPayload{CeremonyID: ceremonyTestID})
		require.NoError(t, err)
		_ = body.Close()
	}
	var retrievals int
	require.NoError(t, database.Get(&retrievals, `SELECT count(*) FROM outbox_events WHERE event_type = 'RETRIEVE_CONTRACT_FOR_SIGNING'`))
	require.Equal(t, 1, retrievals)
}
