package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	archive "digital-contracting-service/gen/contract_storage_archive"
	pac "digital-contracting-service/gen/process_audit_and_compliance"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/datatype"
	basepg "digital-contracting-service/internal/base/db/pq"
	"digital-contracting-service/internal/base/pgtest"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
	erasepg "digital-contracting-service/internal/dcstodcs/db/pg"
	"digital-contracting-service/internal/middleware"
	"digital-contracting-service/internal/processauditandcompliance/auditexecutor"
)

type capturingExecutor struct{ requests []auditexecutor.Request }

func (e *capturingExecutor) Run(_ context.Context, request auditexecutor.Request) (auditexecutor.Response, []byte, error) {
	e.requests = append(e.requests, request)
	response := auditexecutor.Response{ContractVersion: request.ContractVersion, AuditID: request.AuditID, CorrelationID: request.CorrelationID, Scope: request.Scope, Resource: request.Resource, ExecutedAt: time.Now().UTC().Format(time.RFC3339)}
	raw, err := json.Marshal(response)
	return response, raw, err
}

func TestArchiveAccessBypassesPostgres(t *testing.T) {
	t.Setenv("ORCE_ARCHIVE_AUDIT_LOG_URL", "")
	database := pgtest.Open(t)
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
	reader := base.AuditTrailReader{Artifacts: store, ARepo: &basepg.PostgresAuditTrailRepository{}}
	archiveService := &contractStorageArchivesrvc{DB: database, DIDDocument: *document, CRepo: repo, ATrailReader: reader, Keys: keys, ERepo: &erasepg.PostgresEraseRepository{DB: database}}
	for _, fixture := range []struct{ did, owner string }{{"own", "Org A"}, {"foreign", "Org B"}} {
		_, err = database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,name,exp_date) VALUES ($1,$2,$3,'EXPIRED','template',1,$1,NOW()+INTERVAL '3 days')`, fixture.did, peer, fixture.owner)
		require.NoError(t, err)
		_, err = database.Exec(`INSERT INTO contract_archive_entries (did,contract_version,stored_by,contract_snapshot,content_hash,snapshot_cid,evidence) VALUES ($1,1,$2,'{}',$3,'test-cid','{"source":"test","snapshot_hash_algorithm":"SHA-256"}')`, fixture.did, fixture.owner, "sha256:"+fmt.Sprintf("%064d", 0))
		require.NoError(t, err)
	}
	manager := middleware.InjectAuthContext(ctx, []string{"Archive Manager"}, "holder", "Org A")
	// The operator's system client reads every organization; an Auditor of
	// another organization reads none of these contracts (QA F03).
	auditor := middleware.InjectAuthContext(ctx, []string{"Sys. Contract Manager", "Auditor"}, "holder", peer)
	unrelatedAuditor := middleware.InjectAuthContext(ctx, []string{"Auditor"}, "holder", "Audit Org")
	statistics, err := archiveService.Statistics(unrelatedAuditor, &archive.StatisticsPayload{})
	require.NoError(t, err)
	require.Equal(t, 0, statistics.ArchivedTotal)
	statistics, err = archiveService.Statistics(manager, &archive.StatisticsPayload{})
	require.NoError(t, err)
	require.Equal(t, 1, statistics.ArchivedTotal)
	require.Len(t, statistics.ExpiringContracts, 1)
	require.Equal(t, "own", statistics.ExpiringContracts[0].Did)
	statistics, err = archiveService.Statistics(auditor, &archive.StatisticsPayload{})
	require.NoError(t, err)
	require.Equal(t, 2, statistics.ArchivedTotal)
	own, foreign := "own", "foreign"
	_, err = archiveService.ErasureStatus(manager, &archive.ErasureStatusPayload{Did: foreign})
	require.ErrorContains(t, err, "not authorized")
	_, err = archiveService.ErasureStatus(manager, &archive.ErasureStatusPayload{Did: own})
	require.NoError(t, err)
	_, err = archiveService.Audit(manager, &archive.AuditPayload{Did: &foreign})
	require.ErrorContains(t, err, "not authorized")
	trail, err := archiveService.Audit(manager, &archive.AuditPayload{})
	require.NoError(t, err)
	require.Len(t, trail, 1)
	require.Equal(t, own, trail[0].Did)
	executor := &capturingExecutor{}
	auditService := &processAuditAndCompliancesrvc{DB: database, CRepo: repo, ATrailReader: reader, AuditExecutor: executor}
	_, err = auditService.Audit(manager, &pac.PACAuditRequest{Scope: "archive", Did: &foreign, Justification: "Test"})
	require.ErrorContains(t, err, "not authorized")
	require.Empty(t, executor.requests)
	_, err = auditService.Audit(manager, &pac.PACAuditRequest{Scope: "archive", Justification: "Test"})
	require.NoError(t, err)
	require.Len(t, executor.requests, 1)
	evidence := executor.requests[0].Evidence["archive"].([]*auditEvidenceResource)
	require.Len(t, evidence, 1)
	require.Equal(t, own, evidence[0].Did)
	scope := "archive"
	ownReport, err := auditService.AuditReport(manager, &pac.AuditReportPayload{Scope: &scope})
	require.NoError(t, err)
	require.NotEmpty(t, ownReport)
	_, err = auditService.Audit(manager, &pac.PACAuditRequest{Scope: "archive", Justification: "Latest own"})
	require.NoError(t, err)
	latestOwn := executor.requests[len(executor.requests)-1].AuditID
	_, err = auditService.Audit(auditor, &pac.PACAuditRequest{Scope: "archive", Justification: "Global"})
	require.NoError(t, err)
	latestGlobal := executor.requests[len(executor.requests)-1].AuditID
	database.SetMaxOpenConns(1)
	singleConnection, cancelSingle := context.WithTimeout(manager, 3*time.Second)
	selected, selectErr := auditService.latestAuditRun(singleConnection, scope, "")
	cancelSingle()
	database.SetMaxOpenConns(0)
	require.NoError(t, selectErr)
	var selectedRequest auditexecutor.Request
	require.NoError(t, json.Unmarshal(selected.RequestBytes, &selectedRequest))
	require.Equal(t, latestOwn, selectedRequest.AuditID)
	assertReport := func(caller context.Context, wantID string) {
		t.Helper()
		content, err := auditService.AuditReport(caller, &pac.AuditReportPayload{Scope: &scope})
		require.NoError(t, err)
		var report auditReport
		require.NoError(t, json.Unmarshal(content, &report))
		require.Equal(t, wantID, report.ReportID)
	}
	assertReport(manager, latestOwn)
	assertReport(middleware.InjectAuthContext(ctx, []string{"Archive Manager"}, "another-holder", "Org A"), latestOwn)
	assertReport(auditor, latestGlobal)
	require.Len(t, executor.requests, 3, "exports must not rerun the executor")
	noAccess := middleware.InjectAuthContext(ctx, []string{"Archive Manager"}, "holder-c", "Org C")
	report, err := auditService.AuditReport(noAccess, &pac.AuditReportPayload{Scope: &scope})
	var named interface{ ErrorName() string }
	require.True(t, errors.As(err, &named))
	require.Equal(t, "forbidden", named.ErrorName())
	require.Empty(t, report)
	// Current resource rights, not the identity that requested the audit, govern selection.
	_, err = database.Exec(`UPDATE contracts SET created_by='Org A' WHERE did='foreign'`)
	require.NoError(t, err)
	assertReport(manager, latestGlobal)
	_, err = database.Exec(`UPDATE contracts SET created_by='Org B' WHERE did='foreign'`)
	require.NoError(t, err)

	// Deliberately corrupt immutable rows only inside this test's disposable schema.
	_, err = database.Exec(`ALTER TABLE pac_audit_runs DISABLE TRIGGER pac_audit_runs_append_only`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := database.Exec(`ALTER TABLE pac_audit_runs ENABLE TRIGGER pac_audit_runs_append_only`)
		require.NoError(t, err)
	})
	// A timestamp tie must progress by audit_id instead of repeating a denied candidate.
	_, err = database.Exec(`UPDATE pac_audit_runs SET created_at='2026-01-01'`)
	require.NoError(t, err)
	wantOwn := latestOwn
	if executor.requests[0].AuditID > wantOwn {
		wantOwn = executor.requests[0].AuditID
	}
	assertReport(manager, wantOwn)
	_, err = auditService.AuditReport(noAccess, &pac.AuditReportPayload{Scope: &scope})
	require.True(t, errors.As(err, &named))
	require.Equal(t, "forbidden", named.ErrorName())
	_, err = database.Exec(`UPDATE pac_audit_runs SET created_at='2026-01-02' WHERE audit_id=$1`, latestGlobal)
	require.NoError(t, err)

	var originalSnapshot []byte
	var originalHash string
	require.NoError(t, database.QueryRow(`SELECT snapshot_bytes,response_hash FROM pac_audit_runs WHERE audit_id=$1`, latestGlobal).Scan(&originalSnapshot, &originalHash))
	envelope, err := store.Decrypt(ctx, store.InstanceScope(), originalSnapshot)
	require.NoError(t, err)
	payloadBytes, scopes, err := unsealPACSnapshot(ctx, store, latestGlobal, envelope)
	require.NoError(t, err)
	var brokenPayload pacSnapshotPayload
	require.NoError(t, json.Unmarshal(payloadBytes, &brokenPayload))
	brokenPayload.ResponseBytes = []byte(`{"auditId":`)
	brokenBytes, err := json.Marshal(brokenPayload)
	require.NoError(t, err)
	brokenEnvelope, err := sealPACSnapshot(ctx, store, latestGlobal, scopes, brokenBytes)
	require.NoError(t, err)
	brokenSnapshot, err := store.Encrypt(ctx, store.InstanceScope(), brokenEnvelope)
	require.NoError(t, err)
	assertTerminal := func(caller context.Context, want string) {
		t.Helper()
		var before, after int
		require.NoError(t, database.Get(&before, `SELECT count(*) FROM outbox_events`))
		blobCount := len(objects.blobs)
		content, err := auditService.AuditReport(caller, &pac.AuditReportPayload{Scope: &scope})
		require.Empty(t, content)
		var named interface{ ErrorName() string }
		require.True(t, errors.As(err, &named), "error: %v", err)
		require.Equal(t, want, named.ErrorName())
		require.NoError(t, database.Get(&after, `SELECT count(*) FROM outbox_events`))
		require.Equal(t, before, after, "failed export must not emit an event")
		require.Len(t, objects.blobs, blobCount)
	}
	for _, tc := range []struct {
		name     string
		snapshot []byte
		hash     string
	}{
		{"ciphertext", []byte("corrupt"), originalHash},
		{"hash", originalSnapshot, sha256String([]byte("wrong"))},
		{"response decode", brokenSnapshot, sha256String(brokenPayload.ResponseBytes)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := database.Exec(`UPDATE pac_audit_runs SET snapshot_bytes=$2,response_hash=$3 WHERE audit_id=$1`, latestGlobal, tc.snapshot, tc.hash)
			require.NoError(t, err)
			assertTerminal(manager, "internal_error")
		})
	}
	_, err = database.Exec(`UPDATE pac_audit_runs SET snapshot_bytes=$2,response_hash=$3 WHERE audit_id=$1`, latestGlobal, originalSnapshot, originalHash)
	require.NoError(t, err)
	cancelled, cancel := context.WithCancel(manager)
	cancel()
	assertTerminal(cancelled, "internal_error")
	require.Contains(t, scopes, artifactstore.ContractScope(foreign))
	_, err = keys.Shred(ctx, artifactstore.ContractScope(foreign), "holder", "Erasure")
	require.NoError(t, err)
	store.Forget(artifactstore.ContractScope(foreign))
	assertTerminal(manager, "not_found")

	// Authorization examines identities even when no payload needs a contract encryption key.
	for _, resource := range []persistedAuditEvidenceResource{
		{DID: foreign},
		{DID: foreign, AuditTrail: []persistedAuditEvidenceEntry{{EventType: "KEY_SHREDDED", EventData: map[string]any{"erased": true}}}},
		{DID: own, AuditTrail: []persistedAuditEvidenceEntry{{DID: foreign}}},
		{DID: ""},
		{DID: "unknown"},
	} {
		request, err := json.Marshal(map[string]any{"scope": "archive", "evidence": map[string]any{"archive": []persistedAuditEvidenceResource{resource}}})
		require.NoError(t, err)
		require.ErrorContains(t, auditService.authorizeAuditReport(manager, persistedAuditRun{RequestBytes: request, ResponseBytes: []byte(`{}`)}), "not authorized")
		require.NoError(t, auditService.authorizeAuditReport(auditor, persistedAuditRun{RequestBytes: request, ResponseBytes: []byte(`{}`)}))
	}
}

func TestArchiveRecentActionsFilterBeforeLimit(t *testing.T) {
	own, foreign := "own", "foreign"
	entries := []datatype.AuditLogEntry{{DID: &own, EventType: "OWN", CreatedAt: time.Now().Add(-time.Hour)}}
	for range archiveRecentActionLimit + 1 {
		entries = append(entries, datatype.AuditLogEntry{DID: &foreign, CreatedAt: time.Now()})
	}
	entries = append(entries, datatype.AuditLogEntry{CreatedAt: time.Now()})
	actions := recentArchiveActions(filterContractAuditChains([][]datatype.AuditLogEntry{entries}, map[string]bool{own: true}), archiveRecentActionLimit, true)
	require.Len(t, actions, 1)
	require.Equal(t, own, actions[0].Did)
}
