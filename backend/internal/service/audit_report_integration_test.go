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
	"errors"
	"fmt"
	"testing"

	pac "digital-contracting-service/gen/process_audit_and_compliance"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/ipfs"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/middleware"
	"digital-contracting-service/internal/processauditandcompliance/auditexecutor"
)

func TestAuditReportPostgres(t *testing.T) {
	database := pgtest.Open(t)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	objects := &reportObjects{blobs: map[string][]byte{}}
	keys := &artifactstore.PostgresCEKRepo{DB: database}
	store := artifactstore.New(objects, keys, reportAgreement{key}, "did:example:instance", "did:example:instance#agreement", &key.PublicKey)
	service := &processAuditAndCompliancesrvc{DB: database, ATrailReader: base.AuditTrailReader{Artifacts: store}}
	// The operator's system client reads every frozen report of the instance.
	ctx := middleware.InjectAuthContext(context.Background(), []string{userrole.Auditor.String()}, "did:example:holder", "did:example:instance")
	fixture := reportSnapshotFixture(t)
	for _, replacement := range []struct{ old, value string }{{"audit-1", "12345678-1234-4234-8234-123456789abc"}, {"correlation-1", "12345678-1234-4234-8234-123456789def"}} {
		fixture.RequestBytes = bytes.ReplaceAll(fixture.RequestBytes, []byte(replacement.old), []byte(replacement.value))
		fixture.ResponseBytes = bytes.ReplaceAll(fixture.ResponseBytes, []byte(replacement.old), []byte(replacement.value))
	}
	var request auditexecutor.Request
	var response auditexecutor.Response
	if err := json.Unmarshal(fixture.RequestBytes, &request); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(fixture.ResponseBytes, &response); err != nil {
		t.Fatal(err)
	}
	request.Requester.Subject = "did:example:holder"
	request.Justification = "Frozen evidence"
	if err := service.persistAuditRun(ctx, request, response, fixture.ResponseBytes); err != nil {
		t.Fatal(err)
	}
	var stored []byte
	if err := database.Get(&stored, "SELECT snapshot_bytes FROM pac_audit_runs WHERE audit_id=$1", request.AuditID); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored, []byte("actor-a")) || bytes.Contains(stored, fixture.ResponseBytes) {
		t.Fatal("snapshot contains plaintext")
	}
	run, err := service.latestAuditRun(ctx, request.Scope, request.Resource.DID)
	if err != nil {
		t.Fatal(err)
	}
	expectedRequest, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(run.RequestBytes, expectedRequest) || !bytes.Equal(run.ResponseBytes, fixture.ResponseBytes) {
		t.Fatal("SQL snapshot changed executor bytes")
	}
	// Change the caller's evidence after persistence: exports must retain the frozen copy.
	request.Evidence = map[string]any{"contracts": []any{}}
	format := "json"
	payload := &pac.AuditReportPayload{Scope: &request.Scope, Did: &request.Resource.DID, Format: &format, Justification: "Export"}
	content, err := service.AuditReport(ctx, payload)
	if err != nil {
		t.Fatal(err)
	}
	var report auditReport
	if err := json.Unmarshal(content, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Events) != 3 || report.Events[0].Actor != "actor-a" || report.Events[0].Details["note"] != "snapshot" || len(report.Findings) != 1 || report.Findings[0].Result != "FAILED" || report.ResponseHash != hashBytes(fixture.ResponseBytes) {
		t.Fatalf("incomplete frozen report: %+v", report)
	}
	if len(objects.blobs) != 1 {
		t.Fatalf("expected one archived report, got %d", len(objects.blobs))
	}
	var reportCID string
	for cid, ciphertext := range objects.blobs {
		reportCID = cid
		if bytes.Contains(ciphertext, content) || bytes.Contains(ciphertext, []byte("actor-a")) {
			t.Fatal("archived report contains plaintext")
		}
	}
	envelope, err := store.Get(ctx, store.InstanceScope(), reportCID)
	if err != nil {
		t.Fatal(err)
	}
	archived, _, err := unsealPACSnapshot(ctx, store, report.ReportID, envelope)
	if err != nil || !bytes.Equal(archived, content) {
		t.Fatalf("archived report mismatch: %v", err)
	}
	var events int
	if err := database.Get(&events, "SELECT count(*) FROM outbox_events"); err != nil || events != 2 {
		t.Fatalf("audit/export outbox persistence: count=%d err=%v", events, err)
	}
	scope := artifactstore.ContractScope(request.Resource.DID)
	affected, err := keys.Shred(ctx, scope, "did:example:holder", "Erasure")
	if err != nil || affected != 1 {
		t.Fatalf("shred: affected=%d err=%v", affected, err)
	}
	store.Forget(scope)
	if _, err := service.latestAuditRun(ctx, request.Scope, request.Resource.DID); !artifactstore.IsShredded(err) {
		t.Fatalf("snapshot readable after shred: %v", err)
	}
	if _, _, err := unsealPACSnapshot(ctx, store, report.ReportID, envelope); !artifactstore.IsShredded(err) {
		t.Fatalf("archived report readable after shred: %v", err)
	}
	_, err = service.AuditReport(ctx, payload)
	var named interface{ ErrorName() string }
	if !errors.As(err, &named) || named.ErrorName() != "not_found" {
		t.Fatalf("export after shred: %v", err)
	}
}

// Only the external key provider and blob transport are replaced; CEKs and encryption are real.
type reportAgreement struct{ key *ecdsa.PrivateKey }

func (a reportAgreement) DeriveECDH(peer *ecdsa.PublicKey) ([]byte, error) {
	private, err := a.key.ECDH()
	if err != nil {
		return nil, err
	}
	public, err := peer.ECDH()
	if err != nil {
		return nil, err
	}
	return private.ECDH(public)
}

// reportKeys keeps the wrapped CEKs in memory for tests without Postgres.
type reportKeys struct{ records []artifactstore.CEKRecord }

func (k *reportKeys) Fetch(_ context.Context, scope artifactstore.Scope, recipient string) (*artifactstore.CEKRecord, error) {
	for i := range k.records {
		if r := &k.records[i]; r.ScopeKind == string(scope.Kind) && r.ScopeID == scope.ID && r.RecipientDID == recipient {
			return r, nil
		}
	}
	return nil, nil
}

func (k *reportKeys) List(_ context.Context, scope artifactstore.Scope) ([]artifactstore.CEKRecord, error) {
	var records []artifactstore.CEKRecord
	for _, r := range k.records {
		if r.ScopeKind == string(scope.Kind) && r.ScopeID == scope.ID {
			records = append(records, r)
		}
	}
	return records, nil
}

func (k *reportKeys) Insert(_ context.Context, scope artifactstore.Scope, recipient string, wrapped []byte) (bool, error) {
	k.records = append(k.records, artifactstore.CEKRecord{ScopeKind: string(scope.Kind), ScopeID: scope.ID, RecipientDID: recipient, WrappedCEK: wrapped})
	return true, nil
}

func (k *reportKeys) Shred(context.Context, artifactstore.Scope, string, string) (int64, error) {
	return 0, nil
}

type reportObjects struct{ blobs map[string][]byte }

func (o *reportObjects) CreateFile(_ context.Context, data any) (*ipfs.IPFSResult, error) {
	raw, ok := data.([]byte)
	if !ok {
		return nil, fmt.Errorf("expected bytes, got %T", data)
	}
	sum := sha256.Sum256(raw)
	cid := hex.EncodeToString(sum[:])
	o.blobs[cid] = bytes.Clone(raw)
	return o.FetchFile(cid)
}
func (o *reportObjects) FetchFile(cid string) (*ipfs.IPFSResult, error) {
	raw, ok := o.blobs[cid]
	if !ok {
		return nil, fmt.Errorf("missing object %s", cid)
	}
	result := &ipfs.IPFSResult{Data: bytes.Clone(raw)}
	result.Identifier.Format = "CID"
	result.Identifier.Value = cid
	return result, nil
}
