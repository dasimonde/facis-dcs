package dcstodcs

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

func contractWithData(t *testing.T, jsonld string) *db.Contract {
	t.Helper()
	payload := datatype.JSON(jsonld)
	return &db.Contract{ContractData: &payload}
}

// regeneratorHash is the hash pdfgeneration/event.Subscriber.appendC2PA records
// against the render it just stored. The ship gate compares against it, so the
// two must be computed over the same bytes the same way.
func regeneratorHash(jsonld string) string {
	sum := sha256.Sum256([]byte(jsonld))
	return hex.EncodeToString(sum[:])
}

func TestShipDefersAPDFRenderedFromASupersededDocument(t *testing.T) {
	contract := contractWithData(t, `{"dcs:documentStructure":{},"dcs:contractFields":[{"dcs:value":"8"}]}`)
	pdfState := &db.ContractPDFState{
		IPFSCID:     "QmStale",
		C2PAState:   "draft",
		PayloadHash: regeneratorHash(`{"dcs:documentStructure":{},"dcs:contractFields":[{}]}`),
	}

	require.True(t, holdsSupersededPDF(pdfState, contract),
		"a PDF rendered before the fields were filled must not be shipped: the receiver adopts the "+
			"document carried in it as its own copy")
}

func TestShipProceedsWhenThePDFMatchesTheStoredDocument(t *testing.T) {
	jsonld := `{"dcs:documentStructure":{},"dcs:contractFields":[{"dcs:value":"8"}]}`
	pdfState := &db.ContractPDFState{
		IPFSCID:     "QmCurrent",
		C2PAState:   "draft",
		PayloadHash: regeneratorHash(jsonld),
	}

	require.False(t, holdsSupersededPDF(pdfState, contractWithData(t, jsonld)))
}

func TestShipProceedsForAFrozenArtifactWhoseHashCameFromSigning(t *testing.T) {
	// Signing records the hash of the document it signed and the artifact is
	// never re-rendered afterwards, so a mismatch here can never resolve —
	// deferring on it would strand every signed contract's ship.
	pdfState := &db.ContractPDFState{
		IPFSCID:     "QmSigned",
		C2PAState:   "active",
		PayloadHash: regeneratorHash(`{"signed":"under a different byte order"}`),
	}

	require.False(t, holdsSupersededPDF(pdfState, contractWithData(t, `{"dcs:documentStructure":{}}`)))
}

func TestShipProceedsWhenNoPayloadHashWasEverRecorded(t *testing.T) {
	pdfState := &db.ContractPDFState{IPFSCID: "QmLegacy", C2PAState: "draft"}

	require.False(t, holdsSupersededPDF(pdfState, contractWithData(t, `{"dcs:documentStructure":{}}`)))
}

func TestSupersededCheckHashesAContractWithoutDataLikeTheRegenerator(t *testing.T) {
	require.Equal(t, regeneratorHash(""), contractDataHash(&db.Contract{}))
}

// Deferring is only safe while something is guaranteed to re-render: the
// lifecycle event that asked for a regeneration is delivered at most once, so a
// regeneration lost to a transient pdf-core or artifact-store failure leaves a
// stored PDF that this gate refuses for good. The background regenerator's
// retry sweep is what makes it converge — its work-list query
// (PostgresContractRepo.ReadDIDsNeedingRegeneration) selects exactly the
// contracts this gate refuses, so the two must state the same condition. This
// pins the gate's side of it: every arm below is one the query also tests.
func TestEveryConditionTheGateDefersOnIsOneTheRetrySweepSelects(t *testing.T) {
	jsonld := `{"dcs:documentStructure":{}}`
	stale := regeneratorHash(`{"older":"document"}`)

	for _, tc := range []struct {
		name     string
		pdfState *db.ContractPDFState
		defers   bool
	}{
		// pdf_payload_hash <> encode(sha256(contract_data), 'hex')
		{"a render the document moved past", &db.ContractPDFState{IPFSCID: "Qm", C2PAState: "draft", PayloadHash: stale}, true},
		// the same predicate, satisfied
		{"a render of the current document", &db.ContractPDFState{IPFSCID: "Qm", C2PAState: "draft", PayloadHash: regeneratorHash(jsonld)}, false},
		// COALESCE(pdf_c2pa_state, '') IN ('', 'draft')
		{"a frozen artifact hashed at signing", &db.ContractPDFState{IPFSCID: "Qm", C2PAState: "active", PayloadHash: stale}, false},
		// COALESCE(pdf_payload_hash, '') <> ''
		{"a PDF predating payload-hash recording", &db.ContractPDFState{IPFSCID: "Qm", C2PAState: "draft"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.defers, holdsSupersededPDF(tc.pdfState, contractWithData(t, jsonld)))
		})
	}
}
