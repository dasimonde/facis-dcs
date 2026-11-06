package service

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	signaturemanagement "digital-contracting-service/gen/signature_management"
	"digital-contracting-service/internal/base/event/outboxtest"
	db "digital-contracting-service/internal/signingmanagement/db"
)

type callbackCeremonyRepo struct {
	db.CeremonyRepo
	ceremony *db.SignatureCeremony
}

func (r callbackCeremonyRepo) GetCeremonyByID(context.Context, *sqlx.Tx, string) (*db.SignatureCeremony, error) {
	return r.ceremony, nil
}

func (r callbackCeremonyRepo) RecordRefusal(context.Context, *sqlx.Tx, string, string, string, time.Duration) (bool, error) {
	return true, nil
}

func (r callbackCeremonyRepo) MarkDocumentRetrieved(context.Context, *sqlx.Tx, string, string) (bool, error) {
	return true, nil
}

// DCS-FR-SM-19: a wallet callback refused before the submit is audited against
// the contract of its ceremony, with the signer and the outcome.
func TestSignatureRequestCallbackAuditsARefusedCallback(t *testing.T) {
	signer := "did:jwk:signer"
	ceremony := &db.SignatureCeremony{ID: "ceremony-1", ContractDID: "did:web:example.test:contracts:1", FieldName: "signature-1", Status: db.CeremonyVerified, SignerDID: &signer}
	for name, body := range map[string]string{
		"wallet error":      "error=access_denied",
		"no signed request": "documentWithSignature=JVBERg==",
	} {
		t.Run(name, func(t *testing.T) {
			sqlDB, rec := outboxtest.Open(t)
			service := &signatureManagementsrvc{DB: sqlDB, CeremonyRepo: callbackCeremonyRepo{ceremony: ceremony}}

			_, err := service.SignatureRequestCallback(context.Background(), &signaturemanagement.SignatureRequestCallbackPayload{CeremonyID: ceremony.ID}, io.NopCloser(strings.NewReader(body)))
			if err == nil {
				t.Fatal("the callback must be refused")
			}

			events := rec.Committed()
			if len(events) != 1 || events[0].Type != "APPLY_SIGNATURE_FAILED" {
				t.Fatalf("committed events = %+v, want one APPLY_SIGNATURE_FAILED", events)
			}
			data := events[0].Data
			if data["did"] != ceremony.ContractDID || data["signer_did"] != signer || data["step"] != "WALLET_CALLBACK" || data["outcome"] != "FAILED" || data["error"] == "" {
				t.Fatalf("event data = %#v, want contract, signer, step WALLET_CALLBACK and outcome FAILED", data)
			}
		})
	}
}
