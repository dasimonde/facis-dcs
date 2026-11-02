package command

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/event/outboxtest"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/signingmanagement/db"
	"digital-contracting-service/internal/signingmanagement/dss"
)

const orderContractDID = "did:web:dcs-a.localhost:contract:ordered"

// orderedContract declares two signature fields of two parties: "first"
// signs before "second". deadline is the contract's signing deadline, or empty.
func orderedContract(t *testing.T, deadline string) datatype.JSON {
	t.Helper()
	doc := map[string]any{
		"@id": orderContractDID,
		"dcs:parties": []any{
			map[string]any{"@type": "dcs:CompanyParty", "dcs:legalName": "first"},
			map[string]any{"@type": "dcs:CompanyParty", "dcs:legalName": "second"},
		},
		"dcs:signatureFields": []any{
			map[string]any{"@type": "dcs:SignatureField", "dcs:signatoryName": "second", "dcs:signingOrder": 2},
			map[string]any{"@type": "dcs:SignatureField", "dcs:signatoryName": "first", "dcs:signingOrder": 1},
		},
	}
	if deadline != "" {
		doc["dcs:signingDeadline"] = map[string]any{"@value": deadline, "@type": "xsd:dateTime"}
	}
	raw, err := json.Marshal(doc)
	require.NoError(t, err)
	return raw
}

var errPastTheGate = errors.New("past the signing workflow gate")

// orderRepo serves an APPROVED contract, its signatures and the peer
// signatures recorded at receipt. The PDF read after the gate stops prepare.
type orderRepo struct {
	db.ContractRepo
	data    datatype.JSON
	records []db.SignatureRecord
	peers   json.RawMessage
}

func (r orderRepo) ReadDataByDID(context.Context, *sqlx.Tx, string) (*db.Contract, error) {
	return &db.Contract{DID: orderContractDID, State: "APPROVED", ContractData: &r.data}, nil
}

func (r orderRepo) LoadSignatures(context.Context, *sqlx.Tx, string) ([]db.SignatureRecord, error) {
	return r.records, nil
}

func (r orderRepo) ReadPeerSignatureEvidence(context.Context, *sqlx.Tx, string) (json.RawMessage, error) {
	return r.peers, nil
}

func (r orderRepo) FetchContractPDFBytes(context.Context, *sqlx.Tx, string) ([]byte, error) {
	return nil, errPastTheGate
}

// orderCeremonies holds a verified, prepared ceremony for every field.
type orderCeremonies struct{ db.CeremonyRepo }

func orderCeremony(field string) *db.SignatureCeremony {
	signer, credential, hash, renderer := "did:jwk:signer", "AES", "hash", "v1"
	count, version := 0, 1
	verifiedAt := time.Now().UTC()
	return &db.SignatureCeremony{
		ID: "ceremony-" + field, ContractDID: orderContractDID, FieldName: field, Status: db.CeremonyVerified, SignerDID: &signer,
		VerifiedAt:  &verifiedAt,
		PreparedPDF: []byte("%PDF-prepared"), PinnedContentHash: &hash, PinnedRendererVersion: &renderer,
		PinnedSignedCount: &count, PinnedContractVersion: &version, RequiredCredentialType: &credential,
	}
}

func (orderCeremonies) GetCeremonyByID(_ context.Context, _ *sqlx.Tx, id string) (*db.SignatureCeremony, error) {
	return orderCeremony(id[len("ceremony-"):]), nil
}

func (orderCeremonies) FindVerifiedCeremonyByField(_ context.Context, _ *sqlx.Tx, _, field string) (*db.SignatureCeremony, error) {
	return orderCeremony(field), nil
}

type unusedValidator struct{}

func (unusedValidator) ValidatePDF(context.Context, []byte, string) (*dss.Report, error) {
	return nil, errPastTheGate
}

func orderSignedRecord(field string) db.SignatureRecord {
	return db.SignatureRecord{SignerDID: "did:jwk:signer", Status: "SIGNED", FieldName: &field}
}

// DCS-FR-SM-07, DCS-FR-SM-13: prepare and submit refuse a field whose
// position in the signing order comes before a field still unsigned, refuse
// any field after the signing deadline, and audit the refusal (DCS-FR-SM-19).
func TestSigningWorkflowGateOnPrepareAndSubmit(t *testing.T) {
	future := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	steps := map[string]func(*Applier, string) error{
		"PREPARE": func(h *Applier, field string) error {
			_, err := h.Prepare(context.Background(), ApplyCmd{DID: orderContractDID, SignerDID: "did:jwk:signer", CeremonyID: "ceremony-" + field, CredentialType: "AES"})
			return err
		},
		"SUBMIT": func(h *Applier, field string) error {
			return h.SubmitSignature(context.Background(), SubmitSignatureCmd{
				ApplyCmd:  ApplyCmd{DID: orderContractDID, SignerDID: "did:jwk:signer", CeremonyID: "ceremony-" + field, CredentialType: "AES"},
				SignedPDF: []byte("%PDF-signed"),
			})
		},
	}
	cases := []struct {
		name    string
		field   string
		repo    orderRepo
		refusal error
	}{
		{name: "second before first", field: "second", repo: orderRepo{data: orderedContract(t, future)}, refusal: ErrSigningOutOfOrder},
		{name: "first", field: "first", repo: orderRepo{data: orderedContract(t, future)}},
		{name: "second after first", field: "second", repo: orderRepo{data: orderedContract(t, future), records: []db.SignatureRecord{orderSignedRecord("first")}}},
		{name: "second after the peer signed first", field: "second", repo: orderRepo{data: orderedContract(t, ""), peers: json.RawMessage(`[{"party":"first","signatory":"did:jwk:peer"}]`)}},
		{name: "after the deadline", field: "first", repo: orderRepo{data: orderedContract(t, past)}, refusal: ErrSigningDeadlinePassed},
	}
	for step, attempt := range steps {
		for _, tc := range cases {
			t.Run(step+" "+tc.name, func(t *testing.T) {
				sqlDB, rec := outboxtest.Open(t)
				h := &Applier{DB: sqlDB, CRepo: tc.repo, CeremonyRepo: orderCeremonies{}, Validator: unusedValidator{}, PDFCore: &pdfcore.Client{}}
				err := attempt(h, tc.field)
				if tc.refusal == nil {
					// The step past the gate: prepare reads the PDF, submit
					// compares the submitted bytes with the prepared ones.
					passed := map[string]error{"PREPARE": errPastTheGate, "SUBMIT": ErrDocumentMismatch}[step]
					require.ErrorIs(t, err, passed)
					return
				}
				require.ErrorIs(t, err, tc.refusal)
				evt := onlyEvent(t, rec)
				require.Equal(t, "APPLY_SIGNATURE_FAILED", evt.Type)
				require.Equal(t, step, evt.Data["step"])
				require.Equal(t, err.Error(), evt.Data["error"])
			})
		}
	}
}

// DCS-FR-SM-07: the signature field decides whose Power of Attorney
// authorizes a signature, so prepare refuses a declared field that names no
// party of the contract.
func TestPrepareRefusesAFieldThatNamesNoParty(t *testing.T) {
	raw, err := json.Marshal(map[string]any{
		"@id":                 orderContractDID,
		"dcs:parties":         []any{map[string]any{"@type": "dcs:CompanyParty", "dcs:legalName": "second"}},
		"dcs:signatureFields": []any{map[string]any{"@type": "dcs:SignatureField", "dcs:signatoryName": "first"}},
	})
	require.NoError(t, err)
	sqlDB, _ := outboxtest.Open(t)
	h := &Applier{DB: sqlDB, CRepo: orderRepo{data: raw}, CeremonyRepo: orderCeremonies{}, Validator: unusedValidator{}, PDFCore: &pdfcore.Client{}}
	_, err = h.Prepare(context.Background(), ApplyCmd{DID: orderContractDID, SignerDID: "did:jwk:signer", CeremonyID: "ceremony-first", CredentialType: "AES"})
	require.ErrorIs(t, err, ErrUnknownSignatureField)
	require.ErrorContains(t, err, "names no party")

	local := "did:web:dcs-a.example"
	require.True(t, fieldNamesAParty(local, raw, "Org A", local, nil))
	require.True(t, fieldNamesAParty("did:web:dcs-b.example", raw, "Org A", local, &db.Responsible{Counterparty: "did:web:dcs-b.example"}))
	require.True(t, fieldNamesAParty("Org A", raw, "Org A", local, nil))
	require.True(t, fieldNamesAParty("second", raw, "Org A", local, nil))
	require.False(t, fieldNamesAParty("first", raw, "Org A", local, nil))
}

func TestSigningOrderNamesTheFieldsStillToSign(t *testing.T) {
	err := assertSigningOrder(orderedContract(t, ""), "second", map[string]bool{})
	require.ErrorIs(t, err, ErrSigningOutOfOrder)
	require.Contains(t, err.Error(), "waits for first")

	require.NoError(t, assertSigningOrder(orderedContract(t, ""), "unordered", map[string]bool{}))
}
