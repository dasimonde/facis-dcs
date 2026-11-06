package service

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	goahttp "goa.design/goa/v3/http"

	archive "digital-contracting-service/gen/contract_storage_archive"
	archiveserver "digital-contracting-service/gen/http/contract_storage_archive/server"
	pdfserver "digital-contracting-service/gen/http/pdf_generation/server"
	smserver "digital-contracting-service/gen/http/signature_management/server"
	pdf "digital-contracting-service/gen/pdf_generation"
	sm "digital-contracting-service/gen/signature_management"
	"digital-contracting-service/internal/auth"
	authpg "digital-contracting-service/internal/auth/db/pq"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/userrole"
	basepg "digital-contracting-service/internal/base/db/pq"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/contractworkflowengine/command"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	smdb "digital-contracting-service/internal/signingmanagement/db"
	smpg "digital-contracting-service/internal/signingmanagement/db/pg"
)

func TestReadHTTPRegressionsPostgres(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	document := jadesTestDIDDocument(t, "local")
	peer, err := document.GetID()
	require.NoError(t, err)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	objects := &reportObjects{blobs: map[string][]byte{}}
	store := artifactstore.New(objects, &artifactstore.PostgresCEKRepo{DB: database}, reportAgreement{key}, peer, peer+"#agreement", &key.PublicKey)
	repo := &cwepg.PostgresContractRepo{}
	reader := base.AuditTrailReader{Artifacts: store, ARepo: &basepg.PostgresAuditTrailRepository{}}
	validator, token := signedTokens(t)
	authentication := auth.NewJWTAuthenticator(validator, database, &authpg.PostgresAccessAttemptRepo{}, &authpg.PostgresLockoutRepo{})
	for _, f := range []struct{ did, owner, data string }{
		{"own", "did:example:participant", `{}`},
		{"party", "other", `{"dcs:parties":[{"dcs:legalName":"did:example:participant"}]}`},
		{"foreign", "other", `{}`},
		{"root", "did:example:participant", `{}`},
		{"related", "other", `{"dcs:parentContract":{"@id":"root"}}`},
		{"child", "did:example:participant", `{"dcs:parentContract":{"@id":"foreign"}}`},
	} {
		_, err = database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,name,start_date,exp_date,responsible,contract_data) VALUES ($1,$2,$3,'EXPIRED','template',1,$1,'2026-01-01','2027-01-01','{"creator":"party-a","counterparty":"party-b"}',$4)`, f.did, peer, f.owner, f.data)
		require.NoError(t, err)
		snapshot, _ := json.Marshal(map[string]any{"did": f.did, "contract_version": 1, "contract_data": map[string]any{"frozen": f.did}})
		hash, err := command.HashArchiveSnapshot(datatype.JSON(snapshot))
		require.NoError(t, err)
		cid, err := store.Put(ctx, artifactstore.ContractScope(f.did), snapshot)
		require.NoError(t, err)
		_, err = database.Exec(`INSERT INTO contract_archive_entries (did,contract_version,stored_by,contract_snapshot,content_hash,snapshot_cid,evidence) VALUES ($1,1,'fixture',$2,$3,$4,'{"source":"test","snapshot_hash_algorithm":"SHA-256"}')`, f.did, snapshot, hash, cid)
		require.NoError(t, err)
	}
	archiveService := &contractStorageArchivesrvc{DB: database, DIDDocument: *document, CRepo: repo, ATrailReader: reader, JWTAuthenticator: authentication}
	archiveEndpoints := archive.NewEndpoints(archiveService)
	archiveEndpoints.Use(auth.AccessMetadataMiddleware)
	mux := goahttp.NewMuxer()
	archiveserver.Mount(mux, archiveserver.New(archiveEndpoints, mux, goahttp.RequestDecoder, goahttp.ResponseEncoder, nil, nil))
	ceremonyRepo := &smpg.PostgresCeremonyRepo{}
	tx, err := database.BeginTxx(ctx, nil)
	require.NoError(t, err)
	ids := map[string]string{"own": "10000000-0000-4000-8000-000000000001", "party": "10000000-0000-4000-8000-000000000002", "foreign": "10000000-0000-4000-8000-000000000003"}
	for did, id := range ids {
		require.NoError(t, ceremonyRepo.CreateCeremony(ctx, tx, smdb.SignatureCeremony{ID: id, ContractDID: did, FieldName: "confidential-field", RequestedBy: "fixture", Status: smdb.CeremonyPending, Nonce: id, ExpiresAt: time.Now().Add(time.Hour)}))
	}
	require.NoError(t, tx.Commit())
	signingService := &signatureManagementsrvc{DB: database, DIDDocument: *document, CeremonyRepo: ceremonyRepo, ATrailReader: reader, JWTAuthenticator: authentication}
	signingEndpoints := sm.NewEndpoints(signingService)
	signingEndpoints.Use(auth.AccessMetadataMiddleware)
	smserver.Mount(mux, smserver.New(signingEndpoints, mux, goahttp.RequestDecoder, goahttp.ResponseEncoder, nil, nil))
	get := func(path string, role userrole.UserRole, want int) []byte {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer "+token(role, time.Now().Add(time.Hour)))
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		require.Equal(t, want, response.Code, response.Body.String())
		return response.Body.Bytes()
	}
	for _, did := range []string{"own", "party"} {
		get("/signature/request/"+ids[did], userrole.ContractObserver, 200)
		get("/signature/audit?justification=review&did="+did, userrole.ComplianceOfficer, 200)
	}
	var before, after int
	require.NoError(t, database.Get(&before, "SELECT count(*) FROM outbox_events"))
	for _, path := range []string{"/signature/request/" + ids["foreign"], "/signature/audit?justification=review&did=foreign", "/signature/clause-report?did=foreign"} {
		role := userrole.ContractObserver
		if path != "/signature/request/"+ids["foreign"] {
			role = userrole.ComplianceOfficer
		}
		body := get(path, role, 403)
		require.NotContains(t, string(body), "confidential-field")
		var attempt struct {
			Outcome string
			Success bool
		}
		require.NoError(t, database.Get(&attempt, "SELECT outcome,success FROM access_attempts ORDER BY id DESC LIMIT 1"))
		require.Equal(t, "DENIED", attempt.Outcome)
		require.True(t, attempt.Success)
	}
	require.NoError(t, database.Get(&after, "SELECT count(*) FROM outbox_events"))
	require.Equal(t, before, after)
	// QA F03: a Sys.* or Auditor role of the caller's organization does not
	// read another organization's contract.
	get("/signature/request/"+ids["foreign"], userrole.SystemContractSigner, 403)
	get("/signature/audit?justification=review&did=foreign", userrole.Auditor, 403)
	get("/signature/request/10000000-0000-4000-8000-000000000099", userrole.ContractObserver, 404)
	get("/signature/audit?justification=review&did=missing", userrole.ComplianceOfficer, 404)
	body := get("/archive/detail?did=own&contract_version=1", userrole.ContractObserver, 200)
	require.Contains(t, string(body), `"frozen":"own"`)
	get("/archive/detail?did=foreign&contract_version=1", userrole.ContractObserver, 403)
	for _, f := range []struct {
		name, party, state, from, until, version string
		count                                    int
	}{
		{"all matching", "party-a", "EXPIRED", "2025-12-31T00:00:00Z", "2027-01-02T00:00:00Z", "1", 1},
		{"counterparty", "party-b", "EXPIRED", "2025-12-31T00:00:00Z", "2027-01-02T00:00:00Z", "1", 1},
		{"wrong party", "no-party", "EXPIRED", "2025-12-31T00:00:00Z", "2027-01-02T00:00:00Z", "1", 0},
		{"wrong state", "party-a", "DRAFT", "2025-12-31T00:00:00Z", "2027-01-02T00:00:00Z", "1", 0},
		{"outside date", "party-a", "EXPIRED", "2026-02-01T00:00:00Z", "2027-01-02T00:00:00Z", "1", 0},
		{"wrong version", "party-a", "EXPIRED", "2025-12-31T00:00:00Z", "2027-01-02T00:00:00Z", "2", 0},
	} {
		t.Run(f.name, func(t *testing.T) {
			values := url.Values{"did": {"own"}, "party": {f.party}, "state": {f.state}, "valid_from": {f.from}, "valid_until": {f.until}, "contract_version": {f.version}}
			body := get("/archive/search?"+values.Encode(), userrole.ContractObserver, 200)
			var rows []json.RawMessage
			require.NoError(t, json.Unmarshal(body, &rows))
			require.Len(t, rows, f.count)
		})
	}
	// The bundle traverses real SQL contracts; only PDF/manifest transport is synthetic.
	pdfCoreServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest/extract":
			_, _ = w.Write([]byte("test-manifest"))
		case "/manifest/chain":
			_, _ = w.Write([]byte(`[]`))
		default:
			t.Errorf("unexpected pdf-core path %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer pdfCoreServer.Close()
	for _, did := range []string{"root", "child"} {
		cid, err := store.Put(ctx, artifactstore.ContractScope(did), []byte("%PDF-1.7 fixture"))
		require.NoError(t, err)
		_, err = database.Exec("UPDATE contracts SET pdf_ipfs_cid=$2,pdf_c2pa_state='active' WHERE did=$1", did, cid)
		require.NoError(t, err)
	}
	pdfService := &pdfGenerationSrvc{DB: database, CRepo: repo, SignRepo: &smpg.PostgresContractRepo{}, Artifacts: store, PDFCore: &pdfcore.Client{BaseURL: pdfCoreServer.URL, HTTPClient: pdfCoreServer.Client()}, LocalPeer: peer, JWTAuthenticator: authentication}
	pdfEndpoints := pdf.NewEndpoints(pdfService)
	pdfEndpoints.Use(auth.AccessMetadataMiddleware)
	pdfserver.Mount(mux, pdfserver.New(pdfEndpoints, mux, goahttp.RequestDecoder, goahttp.ResponseEncoder, nil, nil))
	bundle := get("/contract/export/root", userrole.ContractObserver, 200)
	zipped, err := zip.NewReader(bytes.NewReader(bundle), int64(len(bundle)))
	require.NoError(t, err)
	for _, file := range zipped.File {
		require.NotContains(t, file.Name, "related")
		stream, err := file.Open()
		require.NoError(t, err)
		content, err := io.ReadAll(stream)
		require.NoError(t, err)
		require.NoError(t, stream.Close())
		require.NotContains(t, string(content), `"frozen":"related"`)
	}
	get("/contract/export/child", userrole.ContractObserver, 403)
}
