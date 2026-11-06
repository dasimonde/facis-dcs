package query

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"testing"

	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/ipfs"
	"digital-contracting-service/internal/base/pgtest"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
)

type memoryObjects struct{ blobs map[string][]byte }

func (o *memoryObjects) CreateFile(_ context.Context, data any) (*ipfs.IPFSResult, error) {
	raw, ok := data.([]byte)
	if !ok {
		return nil, fmt.Errorf("expected bytes, got %T", data)
	}
	sum := sha256.Sum256(raw)
	cid := hex.EncodeToString(sum[:])
	o.blobs[cid] = bytes.Clone(raw)
	return o.FetchFile(cid)
}

func (o *memoryObjects) FetchFile(cid string) (*ipfs.IPFSResult, error) {
	raw, ok := o.blobs[cid]
	if !ok {
		return nil, fmt.Errorf("missing object %s", cid)
	}
	result := &ipfs.IPFSResult{Data: bytes.Clone(raw)}
	result.Identifier.Format = "CID"
	result.Identifier.Value = cid
	return result, nil
}

type ecdhAgreement struct{ key *ecdsa.PrivateKey }

func (a ecdhAgreement) DeriveECDH(peer *ecdsa.PublicKey) ([]byte, error) {
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

// DCS-OR-C2PA-006, DCS-FR-UC-06-2: verifying is read-only. It verifies the PDF
// as it is stored and changes neither the PDF nor its bookkeeping, whether the
// PDF is frozen by a signature or lags a state change the regenerator has not
// rendered yet. A verify that rewrote pdf_c2pa_state made a later renewal drop
// dcs:signedDocument, and a verify that appended a manifest raced the
// regenerator without its lock.
func TestVerifyOfFrozenPDFLeavesC2PAStateUntouched(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	store := artifactstore.New(&memoryObjects{blobs: map[string][]byte{}}, &artifactstore.PostgresCEKRepo{DB: database},
		ecdhAgreement{key}, "did:web:dcs-a.example", "did:web:dcs-a.example#agreement", &key.PublicKey)
	handler := &VerifyContractPdfHandler{
		DB:        database,
		CRepo:     &cwepg.PostgresContractRepo{},
		Artifacts: store,
		PDFCore:   newStubPDFCore(t, http.StatusOK, map[string]any{"match": true, "c2pa_signature_valid": true}),
	}

	for _, tc := range []struct {
		did, contractState, storedState string
		signed                          bool
	}{
		{"did:web:dcs-a.example:contract:terminated-signed", "TERMINATED", "active", true},
		{"did:web:dcs-a.example:contract:approved-lagging", "TERMINATED", "draft", false},
	} {
		cid, err := store.Put(ctx, artifactstore.ContractScope(tc.did), []byte("%PDF "+tc.did))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,contract_data,pdf_ipfs_cid,pdf_c2pa_state,pdf_payload_hash,pdf_signed)
			VALUES ($1,'did:web:dcs-a.example','Org A',$2,'tpl',1,'{}',$3,$4,'hash',$5)`, tc.did, tc.contractState, cid, tc.storedState, tc.signed); err != nil {
			t.Fatal(err)
		}

		result, err := handler.Handle(ctx, VerifyContractPdfQry{DID: tc.did})
		if err != nil {
			t.Fatalf("%s: verify: %v", tc.did, err)
		}
		if result.LifecycleStatus == nil || *result.LifecycleStatus != "terminated" {
			t.Fatalf("%s: lifecycle_status = %v, want the contract's state terminated", tc.did, result.LifecycleStatus)
		}
		var after struct {
			CID   string `db:"pdf_ipfs_cid"`
			State string `db:"pdf_c2pa_state"`
		}
		if err := database.Get(&after, `SELECT pdf_ipfs_cid, pdf_c2pa_state FROM contracts WHERE did = $1`, tc.did); err != nil {
			t.Fatal(err)
		}
		if after.CID != cid || after.State != tc.storedState {
			t.Fatalf("%s: verify changed the stored PDF state to %+v, want cid %s and %q", tc.did, after, cid, tc.storedState)
		}
	}
}
