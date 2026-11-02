package query

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"io"
	"net/http"
	"testing"
	"time"

	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/pgtest"
	tplpg "digital-contracting-service/internal/templaterepository/db/pg"
)

// UC-IF-34, UC-IF-35 (DCS-OR-C2PA-001, -003): template verify and export never
// append to the stored template PDF. The regenerator is the only writer, under
// its per-template lock, and records the event's reason; an on-demand append
// raced it and wrote a fixed reason. Verify reports the template's state over
// the stored PDF, export waits for the regenerator and serves what it stored.
func TestTemplateVerifyAndExportLeaveThePDFToTheRegenerator(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	store := artifactstore.New(&memoryObjects{blobs: map[string][]byte{}}, &artifactstore.PostgresCEKRepo{DB: database},
		ecdhAgreement{key}, "did:web:dcs-a.example", "did:web:dcs-a.example#agreement", &key.PublicKey)
	const did = "did:web:dcs-a.example:template:lagging"
	stored := []byte("%PDF " + did)
	cid, err := store.Put(ctx, artifactstore.TemplateScope(did), stored)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO contract_templates (did,created_by,state,template_type,party_type,name,template_data,pdf_ipfs_cid,pdf_c2pa_state,pdf_payload_hash)
		VALUES ($1,'did:web:dcs-a.example','REGISTERED','CONTRACT_TEMPLATE','SINGLE_PARTY','t','{}',$2,'draft',$3)`, did, cid, payloadHash([]byte("{}"))); err != nil {
		t.Fatal(err)
	}
	unchanged := func() {
		t.Helper()
		var after struct {
			CID   string `db:"pdf_ipfs_cid"`
			State string `db:"pdf_c2pa_state"`
		}
		if err := database.Get(&after, `SELECT pdf_ipfs_cid, pdf_c2pa_state FROM contract_templates WHERE did = $1`, did); err != nil {
			t.Fatal(err)
		}
		if after.CID != cid || after.State != "draft" {
			t.Fatalf("the stored template PDF state changed to %+v, want cid %s and draft", after, cid)
		}
	}

	verify := &VerifyTemplatePdfHandler{
		DB: database, TRepo: &tplpg.PostgresContractTemplateRepo{}, Artifacts: store,
		PDFCore: newStubPDFCore(t, http.StatusOK, map[string]any{"match": true, "c2pa_signature_valid": true}),
	}
	result, err := verify.Handle(ctx, VerifyTemplatePdfQry{DID: did})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if result.LifecycleStatus == nil || *result.LifecycleStatus != "active" {
		t.Fatalf("lifecycle_status = %v, want the template's state active", result.LifecycleStatus)
	}
	unchanged()

	export := &ExportTemplatePdfHandler{DB: database, TRepo: &tplpg.PostgresContractTemplateRepo{}, Artifacts: store}
	waiting, cancel := context.WithTimeout(ctx, 600*time.Millisecond)
	defer cancel()
	if _, err := export.Handle(waiting, ExportTemplatePdfQry{DID: did}); err == nil {
		t.Fatal("export served a PDF that lags the template's state")
	}
	unchanged()

	// The regenerator catches up.
	if _, err := database.Exec(`UPDATE contract_templates SET pdf_c2pa_state = 'active' WHERE did = $1`, did); err != nil {
		t.Fatal(err)
	}
	body, err := export.Handle(ctx, ExportTemplatePdfQry{DID: did})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	served, _ := io.ReadAll(body)
	if string(served) != string(stored) {
		t.Fatalf("export served %q, want the stored PDF", served)
	}
}
