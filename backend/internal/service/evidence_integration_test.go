package service

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	cwe "digital-contracting-service/gen/contract_workflow_engine"
	cweserver "digital-contracting-service/gen/http/contract_workflow_engine/server"
	"digital-contracting-service/internal/auth"
	authpg "digital-contracting-service/internal/auth/db/pq"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/userrole"
	basepg "digital-contracting-service/internal/base/db/pq"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
	"digital-contracting-service/internal/middleware"
	"digital-contracting-service/internal/semantichub/hubtest"

	"github.com/stretchr/testify/require"
	goahttp "goa.design/goa/v3/http"
)

func TestContractEvidencePostgres(t *testing.T) {
	database := hubtest.Open(t)
	ctx := context.Background()
	document := jadesTestDIDDocument(t, "local")
	peer, err := document.GetID()
	require.NoError(t, err)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	objects := &reportObjects{blobs: map[string][]byte{}}
	keys := &artifactstore.PostgresCEKRepo{DB: database}
	store := artifactstore.New(objects, keys, reportAgreement{key}, peer, peer+"#agreement", &key.PublicKey)
	repo := &cwepg.PostgresContractRepo{}
	service := &contractWorkflowEnginesrvc{DB: database, DIDDocument: *document, CRepo: repo, ATrailReader: base.AuditTrailReader{Artifacts: store, ARepo: &basepg.PostgresAuditTrailRepository{}}}
	tx, err := database.BeginTxx(ctx, nil)
	require.NoError(t, err)
	now := time.Now().UTC().Truncate(time.Second)
	data := datatype.JSON(`{}`)
	for _, f := range []struct{ did, owner, state string }{{"evidence-contract", "did:example:participant", "APPROVED"}, {"other-contract", "did:example:participant", "APPROVED"}, {"foreign-contract", "foreign-org", "APPROVED"}, {"terminated-contract", "did:example:participant", "TERMINATED"}} {
		require.NoError(t, repo.Create(ctx, tx, cwedb.Contract{DID: f.did, Origin: peer, CreatedBy: f.owner, CreatedAt: now, State: f.state, ContractData: &data, TemplateDID: "template", TemplateVersion: 1}))
	}
	require.NoError(t, tx.Commit())
	userCtx := middleware.InjectAuthContext(ctx, []string{userrole.ContractManager.String()}, "holder", "did:example:participant")
	payload := &cwe.ContractStoreRequest{Did: "evidence-contract", UpdatedAt: now.Add(time.Second).Format(time.RFC3339), Filename: "private-evidence.bin", Description: "confidential description", Content: []byte{0, 255, 1, 2, 3, 4}}
	result, err := service.Store(userCtx, payload)
	require.NoError(t, err)
	require.NotEmpty(t, result.EvidenceID)
	retrieved, err := service.RetrieveEvidence(userCtx, &cwe.RetrieveEvidencePayload{Did: payload.Did, EvidenceID: result.EvidenceID})
	require.NoError(t, err)
	require.Equal(t, payload.Content, retrieved.Content)
	require.Equal(t, payload.Filename, retrieved.Filename)
	require.Equal(t, payload.Description, retrieved.Description)
	require.Equal(t, "did:example:participant", retrieved.RecordedBy)
	require.NotZero(t, retrieved.ContractVersion)
	_, err = time.Parse(time.RFC3339Nano, retrieved.RecordedAt)
	require.NoError(t, err)
	hash := sha256.Sum256(payload.Content)
	require.Equal(t, hex.EncodeToString(hash[:]), retrieved.Sha256)
	list, err := service.ListEvidence(userCtx, &cwe.ListEvidencePayload{Did: payload.Did})
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Nil(t, list[0].Content)
	var outbox string
	require.NoError(t, database.Get(&outbox, "SELECT row_to_json(e)::text FROM outbox_events e LIMIT 1"))
	require.Contains(t, outbox, result.EvidenceID)
	for _, secret := range []string{payload.Filename, payload.Description, retrieved.Sha256} {
		require.NotContains(t, outbox, secret)
	}
	for _, blob := range objects.blobs {
		require.NotContains(t, string(blob), payload.Description)
		require.NotContains(t, string(blob), payload.Filename)
	}
	var cid string
	require.NoError(t, database.Get(&cid, "SELECT artifact_cid FROM contract_evidence WHERE evidence_id=$1", result.EvidenceID))
	_, err = service.RetrieveEvidence(userCtx, &cwe.RetrieveEvidencePayload{Did: "other-contract", EvidenceID: result.EvidenceID})
	require.ErrorContains(t, err, "no rows")
	// The generated endpoints exercise signed JWT role checks before storage/read access.
	validator, token := signedTokens(t)
	service.JWTAuthenticator = auth.NewJWTAuthenticator(validator, database, &authpg.PostgresAccessAttemptRepo{}, &authpg.PostgresLockoutRepo{})
	endpoints := cwe.NewEndpoints(service)
	endpoints.Use(auth.AccessMetadataMiddleware)
	mux := goahttp.NewMuxer()
	server := cweserver.New(endpoints, mux, goahttp.RequestDecoder, goahttp.ResponseEncoder, nil, nil)
	cweserver.Mount(mux, server)

	httpEvidence := func(method, path string, payload any, want int) {
		t.Helper()
		raw, err := json.Marshal(payload)
		require.NoError(t, err)
		request := httptest.NewRequest(method, path, bytes.NewReader(raw))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+token(userrole.ContractManager, time.Now().Add(time.Hour)))
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		require.Equal(t, want, response.Code, response.Body.String())
	}
	httpEvidence(http.MethodGet, "/contract/other-contract/evidence/"+result.EvidenceID, nil, http.StatusNotFound)
	for _, f := range []struct {
		name   string
		status int
		change func(*cwe.ContractStoreRequest)
	}{
		{"empty file", 400, func(p *cwe.ContractStoreRequest) { p.Content = nil }},
		{"blank description", 400, func(p *cwe.ContractStoreRequest) { p.Description = "  " }},
		{"stale", 409, func(p *cwe.ContractStoreRequest) { p.UpdatedAt = now.Add(-time.Hour).Format(time.RFC3339) }},
		{"terminated", 400, func(p *cwe.ContractStoreRequest) { p.Did = "terminated-contract" }},
		{"foreign", 403, func(p *cwe.ContractStoreRequest) { p.Did = "foreign-contract" }},
		{"unknown", 404, func(p *cwe.ContractStoreRequest) { p.Did = "unknown" }},
		{"bad time", 400, func(p *cwe.ContractStoreRequest) { p.UpdatedAt = "invalid" }},
	} {
		t.Run(f.name, func(t *testing.T) {
			bad := *payload
			f.change(&bad)
			_, err := service.Store(userCtx, &bad)
			require.Error(t, err)
			httpEvidence(http.MethodPost, "/contract/store", map[string]any{"did": bad.Did, "updated_at": bad.UpdatedAt, "filename": bad.Filename, "description": bad.Description, "content": bad.Content}, f.status)
		})
	}
	var count int
	require.NoError(t, database.Get(&count, "SELECT count(*) FROM outbox_events"))
	require.Equal(t, 1, count)
	require.NoError(t, database.Get(&count, "SELECT count(*) FROM contract_evidence"))
	require.Equal(t, 1, count)
	t.Run("configured limit", func(t *testing.T) {
		t.Setenv("DCS_EVIDENCE_MAX_FILE_BYTES", "3")
		_, err := service.Store(userCtx, payload)
		require.ErrorContains(t, err, "configured limits")
	})
	for _, role := range []userrole.UserRole{userrole.ContractManager, userrole.SystemContractManager, userrole.Auditor, userrole.ComplianceOfficer, userrole.SystemAuditor, userrole.ContractObserver} {
		t.Run(role.String(), func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/contract/evidence-contract/evidence/"+result.EvidenceID, nil)
			request.Header.Set("Authorization", "Bearer "+token(role, time.Now().Add(time.Hour)))
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			allowed := role == userrole.ContractManager || role == userrole.SystemContractManager || role == userrole.Auditor || role == userrole.ComplianceOfficer
			if allowed {
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			} else {
				require.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
			}
			auditRequest := httptest.NewRequest(http.MethodPost, "/contract/audit", strings.NewReader(`{"did":"evidence-contract","justification":"evidence review"}`))
			auditRequest.Header.Set("Content-Type", "application/json")
			auditRequest.Header.Set("Authorization", "Bearer "+token(role, time.Now().Add(time.Hour)))
			auditResponse := httptest.NewRecorder()
			mux.ServeHTTP(auditResponse, auditRequest)
			// The audit trail is for the audit roles and the Contract Manager,
			// not for a machine role (DCS-NFR-BR-05).
			if allowed && role != userrole.SystemContractManager {
				require.Equal(t, http.StatusOK, auditResponse.Code, auditResponse.Body.String())
			} else {
				require.Equal(t, http.StatusForbidden, auditResponse.Code, auditResponse.Body.String())
			}
			raw, err := json.Marshal(map[string]any{"did": payload.Did, "updated_at": payload.UpdatedAt, "filename": payload.Filename, "description": payload.Description, "content": payload.Content})
			require.NoError(t, err)
			request = httptest.NewRequest(http.MethodPost, "/contract/store", bytes.NewReader(raw))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+token(role, time.Now().Add(time.Hour)))
			response = httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			if role == userrole.ContractManager || role == userrole.SystemContractManager {
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			} else {
				require.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
			}
		})
	}
	original := bytes.Clone(objects.blobs[cid])
	objects.blobs[cid][len(original)-1] ^= 1
	_, err = service.RetrieveEvidence(userCtx, &cwe.RetrieveEvidencePayload{Did: payload.Did, EvidenceID: result.EvidenceID})
	require.Error(t, err)
	httpEvidence(http.MethodGet, "/contract/"+payload.Did+"/evidence/"+result.EvidenceID, nil, http.StatusInternalServerError)
	objects.blobs[cid] = original
	scope := artifactstore.ContractScope(payload.Did)
	_, err = keys.Shred(ctx, scope, "holder", "Erasure")
	require.NoError(t, err)
	store.Forget(scope)
	_, err = service.RetrieveEvidence(userCtx, &cwe.RetrieveEvidencePayload{Did: payload.Did, EvidenceID: result.EvidenceID})
	require.ErrorContains(t, err, "shredded")
	_, err = service.ListEvidence(userCtx, &cwe.ListEvidencePayload{Did: payload.Did})
	require.ErrorContains(t, err, "shredded")
	httpEvidence(http.MethodGet, "/contract/"+payload.Did+"/evidence/"+result.EvidenceID, nil, http.StatusNotFound)
	httpEvidence(http.MethodGet, "/contract/"+payload.Did+"/evidence", nil, http.StatusNotFound)
}
