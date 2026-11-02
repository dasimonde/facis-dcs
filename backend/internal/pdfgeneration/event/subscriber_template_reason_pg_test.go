package event

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/pgtest"
	tplrepo "digital-contracting-service/internal/templaterepository/db/pg"
)

var emptyObjectHash = sha256.Sum256([]byte("{}"))

// UC-IF-35 (DCS-OR-C2PA-003): the lifecycle assertion the regenerator appends
// to a template PDF records the reason the template event names.
func TestTemplateLifecycleAssertionRecordsTheEventReason(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	store := artifactstore.New(&memoryObjects{blobs: map[string][]byte{}}, &artifactstore.PostgresCEKRepo{DB: database},
		ecdhAgreement{key}, "did:web:dcs-a.example", "did:web:dcs-a.example#agreement", &key.PublicKey)
	const did = "did:web:dcs-a.example:template:reasoned"
	cid, err := store.Put(ctx, artifactstore.TemplateScope(did), []byte("%PDF template"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO contract_templates (did,created_by,state,template_type,party_type,name,template_data,pdf_ipfs_cid,pdf_c2pa_state,pdf_payload_hash)
		VALUES ($1,'did:web:dcs-a.example','REGISTERED','CONTRACT_TEMPLATE','SINGLE_PARTY','t','{}',$2,'draft',$3)`, did, cid, hex.EncodeToString(emptyObjectHash[:])); err != nil {
		t.Fatal(err)
	}
	issuer := &recordingVCIssuer{}
	s := &Subscriber{
		DB: database, Artifacts: store, TRepo: &tplrepo.PostgresContractTemplateRepo{},
		PDFCore: appendingPDFCore(t, "{}"), IssuerDID: "did:web:dcs-a.example", VCIssuer: issuer,
	}

	if err := s.appendTemplateC2PA(ctx, minimalCWEEvent{DID: did, OccurredAt: time.Now().UTC(), Reason: "Approved after the legal review"}); err != nil {
		t.Fatal(err)
	}
	if len(issuer.issued) != 1 || issuer.issued[0].reason != "Approved after the legal review" {
		t.Fatalf("issued lifecycle credentials = %+v, want one with the event's reason", issuer.issued)
	}
}
