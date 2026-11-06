package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	goahttp "goa.design/goa/v3/http"

	archive "digital-contracting-service/gen/contract_storage_archive"
	cwe "digital-contracting-service/gen/contract_workflow_engine"
	cweserver "digital-contracting-service/gen/http/contract_workflow_engine/server"
	pdf "digital-contracting-service/gen/pdf_generation"
	sm "digital-contracting-service/gen/signature_management"
	"digital-contracting-service/internal/auth"
	authpg "digital-contracting-service/internal/auth/db/pq"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/contractreadpolicy"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/contractworkflowengine/command"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
	"digital-contracting-service/internal/middleware"
	"digital-contracting-service/internal/semantichub/hubtest"
	smpg "digital-contracting-service/internal/signingmanagement/db/pg"
)

func TestContractReadScopePostgres(t *testing.T) {
	database := hubtest.Open(t)
	ctx := context.Background()
	peer := "did:web:local"
	repo := &cwepg.PostgresContractRepo{}
	tx, err := database.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	fixtures := []struct {
		did, owner, origin, data string
		allowed                  bool
	}{
		{"owner", "Org A", peer, `{}`, true},
		{"party", "Org B", peer, `{"dcs:parties":[{"dcs:legalName":"Org A"}]}`, true},
		{"foreign", "Org B", peer, `{}`, false},
		{"remote", "did:web:remote", "did:web:remote", `{}`, false},
		{"remote-party", "did:web:remote", "did:web:remote", `{"dcs:parties":[{"dcs:legalName":"Org A"}]}`, true},
		{"null", "Org B", peer, `{"dcs:parties":null}`, false},
		{"object", "Org B", peer, `{"dcs:parties":{"dcs:legalName":"Org A"}}`, false},
		{"malformed", "Org B", peer, `{"dcs:parties":[null,123,"Org A",{"dcs:legalName":123}]}`, false},
	}
	scope := contractreadpolicy.Scope{Organization: "Org A", LocalPeer: peer}
	scoped := contractreadpolicy.WithScope(ctx, scope)
	for i, f := range fixtures {
		data := datatype.JSON(f.data)
		require.Equal(t, f.allowed, scope.MayRead(f.origin, f.owner, &data), f.did)
		require.NoError(t, repo.Create(ctx, tx, cwedb.Contract{DID: f.did, Origin: f.origin, CreatedBy: f.owner, CreatedAt: time.Now().Add(time.Duration(i) * time.Minute), State: "APPROVED", ContractData: &data, TemplateDID: "template", TemplateVersion: 1}))
		for _, table := range []string{"contract_review_task", "contract_approval_task", "contract_negotiation_task"} {
			actor := map[string]string{"contract_review_task": "reviewer", "contract_approval_task": "approver", "contract_negotiation_task": "negotiator"}[table]
			columns, values := "", ""
			if table == "contract_negotiation_task" {
				columns, values = ",contract_version", ",1"
			}
			_, err = tx.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (did,state,%s,created_by%s) VALUES ($1,'OPEN','peer','fixture'%s)", table, actor, columns, values), f.did)
			require.NoError(t, err)
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO contract_signatures (contract_did,signer_did) VALUES ($1,'peer')", f.did)
		require.NoError(t, err)
	}
	for _, f := range fixtures {
		_, err = tx.ExecContext(ctx, `INSERT INTO contract_archive_entries (did,contract_version,stored_by,contract_snapshot,content_hash,snapshot_cid,evidence) VALUES ($1,1,'fixture','{}',$2,'test-cid','{"source":"test","snapshot_hash_algorithm":"SHA-256"}')`, f.did, "sha256:"+fmt.Sprintf("%064d", 0))
		require.NoError(t, err)
	}
	archived, err := repo.ReadArchivedContracts(scoped, tx)
	require.NoError(t, err)
	require.Len(t, archived, 3)
	archived, err = repo.ReadArchivedContractsByFilter(scoped, tx, cwedb.SearchValues{DID: "foreign"})
	require.NoError(t, err)
	require.Empty(t, archived)

	rows, err := repo.ReadAllMetaData(scoped, tx, datatype.Pagination{Limit: 1, Offset: 1}, cwedb.ListOptions{})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "remote-party", rows[0].DID)
	rows, err = repo.ReadAllMetaData(scoped, tx, datatype.Pagination{Limit: 1, Offset: 2}, cwedb.ListOptions{})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "party", rows[0].DID)
	rows, err = repo.ReadAllMetaDataByFilter(scoped, tx, cwedb.SearchValues{}, datatype.Pagination{})
	require.NoError(t, err)
	require.Len(t, rows, 3)
	reviews, err := (&cwepg.PostgresReviewTaskRepo{}).ReadAllByReviewer(scoped, tx, "peer")
	require.NoError(t, err)
	require.Len(t, reviews, 3)
	approvals, err := (&cwepg.PostgresApprovalTaskRepo{}).ReadAllByApprover(scoped, tx, "peer")
	require.NoError(t, err)
	require.Len(t, approvals, 3)
	for _, task := range approvals {
		require.Equal(t, 1, task.ContractVersion)
	}
	negotiations, err := (&cwepg.PostgresNegotiationTaskRepo{}).ReadAllByNegotiator(scoped, tx, "peer")
	require.NoError(t, err)
	require.Len(t, negotiations, 3)
	signatures, err := (&smpg.PostgresContractRepo{}).ReadAllSigningTasks(scoped, tx)
	require.NoError(t, err)
	require.Len(t, signatures, 3)
	for _, task := range signatures {
		require.Equal(t, "PENDING", task.State)
		require.Equal(t, 1, task.ContractVersion)
	}
	signingRows, err := (&smpg.PostgresContractRepo{}).ReadAllMetaData(scoped, tx, datatype.Pagination{Limit: 1, Offset: 1})
	require.NoError(t, err)
	require.Len(t, signingRows, 1)
	require.Equal(t, "remote-party", signingRows[0].DID)
	// QA F03: an Auditor, System Administrator or Sys.* client of an unrelated
	// organization reads nothing; a system client acting for the instance reads all.
	unrelated := contractreadpolicy.WithScope(ctx, contractreadpolicy.Scope{Organization: "Org Unrelated", LocalPeer: peer})
	rows, err = repo.ReadAllMetaData(unrelated, tx, datatype.Pagination{}, cwedb.ListOptions{})
	require.NoError(t, err)
	require.Empty(t, rows)
	instance := contractreadpolicy.WithScope(ctx, contractreadpolicy.Scope{Organization: peer, LocalPeer: peer})
	rows, err = repo.ReadAllMetaData(instance, tx, datatype.Pagination{}, cwedb.ListOptions{})
	require.NoError(t, err)
	require.Len(t, rows, len(fixtures))
	rows, err = repo.ReadAllMetaData(ctx, tx, datatype.Pagination{}, cwedb.ListOptions{})
	require.NoError(t, err)
	require.Empty(t, rows)
	unscopedTasks, err := (&smpg.PostgresContractRepo{}).ReadAllSigningTasks(ctx, tx)
	require.NoError(t, err)
	require.Empty(t, unscopedTasks)
	rows, err = repo.ReadAllMetaData(contractreadpolicy.WithSystemScope(ctx), tx, datatype.Pagination{}, cwedb.ListOptions{})
	require.NoError(t, err)
	require.Len(t, rows, len(fixtures))
	require.NoError(t, tx.Commit())
	document := jadesTestDIDDocument(t, "local")
	userCtx := middleware.InjectAuthContext(ctx, []string{userrole.ContractObserver.String()}, "holder", "Org A")
	engine := &contractWorkflowEnginesrvc{DB: database, DIDDocument: *document, CRepo: repo}
	_, err = engine.RetrieveHistoryByID(userCtx, &cwe.ContractHistoryRetrieveByIDRequest{Did: "foreign"})
	require.ErrorContains(t, err, "not authorized")
	_, err = engine.Lifecycle(userCtx, &cwe.LifecyclePayload{Did: "foreign"})
	require.ErrorContains(t, err, "not authorized")
	_, err = engine.KpiObservations(userCtx, &cwe.ContractRetrieveByIDRequest{Did: "foreign"})
	require.ErrorContains(t, err, "not authorized")
	_, err = engine.RetrieveNegotiationDraft(userCtx, &cwe.ContractNegotiationDraftRetrieveRequest{Did: "foreign"})
	require.ErrorContains(t, err, "not authorized")
	var before, after int
	require.NoError(t, database.Get(&before, "SELECT count(*) FROM outbox_events"))
	_, err = engine.Review(userCtx, &cwe.ContractReviewRequest{Did: "foreign"})
	require.ErrorContains(t, err, "not authorized")
	require.NoError(t, database.Get(&after, "SELECT count(*) FROM outbox_events"))
	require.Equal(t, before, after)

	signing := &signatureManagementsrvc{DB: database, DIDDocument: *document}
	_, err = signing.RetrieveByID(userCtx, &sm.SMContractRetrieveByIDRequest{Did: "foreign"})
	require.ErrorContains(t, err, "not authorized")
	_, err = signing.View(userCtx, &sm.SMSignatureViewRequest{Did: "foreign"})
	require.ErrorContains(t, err, "not authorized")
	pdfService := &pdfGenerationSrvc{DB: database, LocalPeer: peer}
	_, _, bundleErr := pdfService.ExportContractBundle(userCtx, &pdf.ExportContractBundlePayload{Did: "foreign"})
	require.ErrorContains(t, bundleErr, "not authorized")
	_, err = pdfService.ExportContractPdf(userCtx, &pdf.ExportContractPdfPayload{Did: "foreign"})
	require.ErrorContains(t, err, "not authorized")
	// Real JWT -> generated HTTP handler -> authorization -> denied access row.
	validator, token := signedTokens(t)
	engine.JWTAuthenticator = auth.NewJWTAuthenticator(validator, database, &authpg.PostgresAccessAttemptRepo{}, &authpg.PostgresLockoutRepo{})
	endpoints := cwe.NewEndpoints(engine)
	endpoints.Use(auth.AccessMetadataMiddleware)
	mux := goahttp.NewMuxer()
	server := cweserver.New(endpoints, mux, goahttp.RequestDecoder, goahttp.ResponseEncoder, nil, nil)
	cweserver.Mount(mux, server)
	request := httptest.NewRequest(http.MethodGet, "/contract/history/foreign", nil)
	request.Header.Set("Authorization", "Bearer "+token(userrole.ContractObserver, time.Now().Add(time.Hour)))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	require.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
	var attempt struct {
		Outcome string
		Success bool
	}
	require.NoError(t, database.Get(&attempt, "SELECT outcome,success FROM access_attempts ORDER BY id DESC LIMIT 1"))
	require.Equal(t, "DENIED", attempt.Outcome)
	require.True(t, attempt.Success)
}

func TestArchiveDetailPostgres(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	document := jadesTestDIDDocument(t, "local")
	peer, err := document.GetID()
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,contract_data) VALUES ('archived',$1,'Org A','EXPIRED','template',1,'{"live":"changed"}')`, peer)
	require.NoError(t, err)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	objects := &reportObjects{blobs: map[string][]byte{}}
	keys := &artifactstore.PostgresCEKRepo{DB: database}
	store := artifactstore.New(objects, keys, reportAgreement{key}, peer, peer+"#agreement", &key.PublicKey)
	service := &contractStorageArchivesrvc{DB: database, DIDDocument: *document, ATrailReader: base.AuditTrailReader{Artifacts: store}}
	user := middleware.InjectAuthContext(ctx, []string{userrole.ArchiveManager.String()}, "holder", "Org A")
	raw := datatype.JSON(`{"did":"archived","contract_version":1,"name":"Frozen title","contract_data":{"snapshot":"original"}}`)
	hash, err := command.HashArchiveSnapshot(raw)
	require.NoError(t, err)
	cid, err := store.Put(ctx, artifactstore.ContractScope("archived"), raw)
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contract_archive_entries (did,contract_version,stored_by,contract_snapshot,content_hash,snapshot_cid,evidence) VALUES ('archived',1,'Org A',$1,$2,$3,'{"source":"test","snapshot_hash_algorithm":"SHA-256"}')`, raw, hash, cid)
	require.NoError(t, err)
	payload := &archive.DetailPayload{Did: "archived", ContractVersion: 1}
	result, err := service.Detail(user, payload)
	require.NoError(t, err)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	require.JSONEq(t, string(raw), string(encoded))
	// DCS-FR-CSA-18: the read names the contract in the archive audit log.
	retrieved := archiveEvents(t, database, "RETRIEVE_ARCHIVED_CONTRACTS")
	require.Len(t, retrieved, 1)
	require.Equal(t, []any{"archived"}, retrieved[0]["contract_dids"])
	_, err = service.Detail(user, &archive.DetailPayload{Did: "archived", ContractVersion: 2})
	require.Error(t, err)
	outsider := middleware.InjectAuthContext(ctx, []string{userrole.ArchiveManager.String()}, "holder", "Org B")
	_, err = service.Detail(outsider, payload)
	require.ErrorContains(t, err, "not authorized")
	failed := archiveEvents(t, database, "ARCHIVE_OPERATION_FAILED")
	require.Len(t, failed, 2, "a failed archive read must be audited")
	require.Equal(t, "detail", failed[1]["operation"])
	require.Equal(t, "Org B", failed[1]["actor"])
	originalBlob := append([]byte(nil), objects.blobs[cid]...)
	tamperedCID, err := store.Put(ctx, artifactstore.ContractScope("archived"), []byte(`{"did":"archived","contract_version":1,"contract_data":{"tampered":true}}`))
	require.NoError(t, err)
	objects.blobs[cid] = objects.blobs[tamperedCID]
	_, err = service.Detail(user, payload)
	require.ErrorContains(t, err, "integrity mismatch")
	delete(objects.blobs, cid)
	_, err = service.Detail(user, payload)
	require.ErrorContains(t, err, "unavailable")
	objects.blobs[cid] = originalBlob
	wrong := datatype.JSON(`{"did":"other","contract_version":2,"contract_data":{}}`)
	wrongHash, err := command.HashArchiveSnapshot(wrong)
	require.NoError(t, err)
	wrongCID, err := store.Put(ctx, artifactstore.ContractScope("archived"), wrong)
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contract_archive_entries (did,contract_version,stored_by,contract_snapshot,content_hash,snapshot_cid,evidence) VALUES ('archived',2,'Org A',$1,$2,$3,'{"source":"test","snapshot_hash_algorithm":"SHA-256"}')`, wrong, wrongHash, wrongCID)
	require.NoError(t, err)
	_, err = service.Detail(user, &archive.DetailPayload{Did: "archived", ContractVersion: 2})
	require.ErrorContains(t, err, "identity mismatch")
	_, err = database.Exec(`UPDATE contract_archive_entries SET archive_status='DELETED',deleted_at=NOW(),deleted_by='holder',deletion_reason='Test' WHERE contract_version=2`)
	require.NoError(t, err)
	_, err = service.Detail(user, &archive.DetailPayload{Did: "archived", ContractVersion: 2})
	require.ErrorContains(t, err, "no rows")

	_, err = keys.Shred(ctx, artifactstore.ContractScope("archived"), "holder", "Test")
	require.NoError(t, err)
	store.Forget(artifactstore.ContractScope("archived"))
	_, err = service.Detail(user, payload)
	require.ErrorContains(t, err, "unavailable")
}
