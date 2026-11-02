package command

import (
	"errors"
	"testing"

	"digital-contracting-service/internal/signingmanagement/db"
)

func signedRecord(field string) db.SignatureRecord {
	return db.SignatureRecord{Status: "SIGNED", FieldName: &field}
}

// DCS-FR-CSA-08, DCS-FR-CWE-20: the archive entry is written once, by the
// signature that completes the signatures this instance holds, so it carries
// all of them.
func TestSigningCompletesOnlyWithTheLastLocalSignature(t *testing.T) {
	fields := []string{"signer-1", "signer-2"}

	if signingCompletes(fields, []db.SignatureRecord{signedRecord("signer-1")}, "signer-1") {
		t.Error("the first of two signatures must not complete signing")
	}
	if !signingCompletes(fields, []db.SignatureRecord{signedRecord("signer-1"), signedRecord("signer-2")}, "signer-2") {
		t.Error("the second of two signatures must complete signing")
	}
	revoked := signedRecord("signer-2")
	revoked.Status = "REVOKED"
	if signingCompletes(fields, []db.SignatureRecord{signedRecord("signer-1"), revoked}, "signer-1") {
		t.Error("a revoked signature does not count")
	}

	// A contract that declares no field completes with its first signature.
	if !signingCompletes(nil, []db.SignatureRecord{{Status: "SIGNED"}}, "") {
		t.Error("the first signature of a contract without fields must complete signing")
	}
	if signingCompletes(nil, []db.SignatureRecord{{Status: "SIGNED"}, {Status: "SIGNED"}}, "") {
		t.Error("a later signature of a contract without fields must not archive again")
	}
}

// The counterparty's slot is signed in the peer's deployment, so only the
// local slots decide when this instance's signing completes.
func TestLocalSignatureFieldsLeaveOutTheCounterpartySlot(t *testing.T) {
	responsible := &db.Responsible{Creator: instanceA, Counterparty: instanceB}
	local := localSignatureFields([]string{instanceA, instanceB}, responsible, instanceA)
	if len(local) != 1 || local[0] != instanceA {
		t.Fatalf("want only the own slot, got %v", local)
	}
	if !signingCompletes(local, []db.SignatureRecord{signedRecord(instanceA)}, instanceA) {
		t.Error("the own signature completes this instance's signing of a federated contract")
	}
}

// DCS-FR-SM-07, DCS-FR-SM-17: signatures are applied one at a time against the
// latest revision. A document prepared before another signature was applied
// does not extend the current signed revision and is refused.
func TestAssertLatestRevisionRefusesAStaleRevision(t *testing.T) {
	unsigned := []byte("%PDF-1.7\n1 0 obj\n<< >>\nendobj\n%%EOF\n")
	preparedFromUnsigned := append(append([]byte(nil), unsigned...), []byte("2 0 obj\n<< /evidence 1 >>\nendobj\n%%EOF\n")...)
	firstSigned := append(append([]byte(nil), unsigned...), []byte("3 0 obj\n<< /Type /Sig /ByteRange [0 1 2 3] >>\nendobj\n%%EOF\n")...)
	preparedFromSigned := append(append([]byte(nil), firstSigned...), []byte("4 0 obj\n<< /evidence 2 >>\nendobj\n%%EOF\n")...)

	if err := assertLatestRevision(firstSigned, preparedFromUnsigned); !errors.Is(err, ErrStaleRevision) || !errors.Is(err, ErrDocumentMismatch) {
		t.Errorf("a document prepared before the first signature must be refused, got %v", err)
	}
	if err := assertLatestRevision(firstSigned, preparedFromSigned); err != nil {
		t.Errorf("a document prepared from the latest signed revision must pass, got %v", err)
	}
	if err := assertLatestRevision(unsigned, preparedFromUnsigned); err != nil {
		t.Errorf("an unsigned current revision holds no signature to lose, got %v", err)
	}
	if err := assertLatestRevision(nil, preparedFromUnsigned); err != nil {
		t.Errorf("a contract without a stored PDF must pass, got %v", err)
	}
}
