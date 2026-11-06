package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	signaturemanagement "digital-contracting-service/gen/signature_management"
	"digital-contracting-service/internal/base/event/outboxtest"
	db "digital-contracting-service/internal/signingmanagement/db"
)

// DCS-FR-SM-15: the wallet's retrieval of the document to be signed is logged
// with the contract, the signer, the time and the digest the signed request
// object names, against which the wallet validates the document.
func TestSignatureRequestDocumentLogsTheRetrieval(t *testing.T) {
	pdf := []byte("%PDF-1.7 prepared")
	sum := sha256.Sum256(pdf)
	digest := hex.EncodeToString(sum[:])
	signer, nonce := "did:jwk:signer", "nonce"
	expires := time.Now().Add(time.Hour)
	ceremony := &db.SignatureCeremony{
		ID: "ceremony-1", ContractDID: "did:web:example.test:contracts:1", FieldName: "signature-1",
		SignerDID: &signer, PreparedPDF: pdf, PreparedPDFSHA256: &digest, RequestNonce: &nonce, RequestExpiresAt: &expires,
	}
	sqlDB, rec := outboxtest.Open(t)
	service := &signatureManagementsrvc{DB: sqlDB, CeremonyRepo: callbackCeremonyRepo{ceremony: ceremony}}

	if _, err := service.SignatureRequestDocument(context.Background(), &signaturemanagement.SignatureRequestDocumentPayload{CeremonyID: ceremony.ID}); err != nil {
		t.Fatal(err)
	}

	events := rec.Committed()
	if len(events) != 1 || events[0].Type != "RETRIEVE_CONTRACT_FOR_SIGNING" {
		t.Fatalf("committed events = %+v, want one RETRIEVE_CONTRACT_FOR_SIGNING", events)
	}
	data := events[0].Data
	if data["did"] != ceremony.ContractDID || data["signer_did"] != signer || data["document_sha256"] != digest || data["occurred_at"] == nil {
		t.Fatalf("event data = %#v, want contract, signer, digest and time", data)
	}
}
