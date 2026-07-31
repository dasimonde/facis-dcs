package command

import (
	"errors"
	"go/token"
	"testing"

	"digital-contracting-service/internal/signingmanagement/db"
)

// A field can hold two verified unconsumed ceremonies at once — the wallet
// callback pins the ceremony from its callback URL while FindVerifiedCeremonyByField
// returns the newest — so two concurrent submits each consume their OWN ceremony
// and MarkCeremonyConsumed's guarded UPDATE stops neither. The only thing that
// stops the second is the already-signed read, and it only stops it if it runs
// after the writers are serialised: read before the DSS round trips, both submits
// see no SIGNED row, both write one, and the second SetSignedPDF drops the first
// signature from the stored artifact. This pins the re-read to the window between
// the lock and the first write.
func TestSubmitSignatureRechecksTheAlreadySignedGuardUnderTheLock(t *testing.T) {
	body := functionBody(t, "apply.go", "SubmitSignature")

	lock := lastCallPos(body, "acquireRegenerationLock")
	load := lastCallPos(body, "LoadSignatures")
	guard := lastCallPos(body, "assertFieldUnsigned")
	consume := lastCallPos(body, "MarkCeremonyConsumed")

	for name, pos := range map[string]token.Pos{
		"acquireRegenerationLock": lock, "LoadSignatures": load,
		"assertFieldUnsigned": guard, "MarkCeremonyConsumed": consume,
	} {
		if pos == token.NoPos {
			t.Fatalf("SubmitSignature no longer calls %s: this test no longer checks what it claims", name)
		}
	}
	if load < lock {
		t.Error("the signatures are read only before the regeneration lock: two concurrent submits for one field both read no signature and both write one")
	}
	if guard < lock {
		t.Error("the already-signed guard is evaluated only before the regeneration lock, where its answer is not yet settled")
	}
	if guard > consume {
		t.Error("the already-signed guard runs after the ceremony is consumed: the losing submit has already written")
	}
}

// The guard itself: only a SIGNED row for THIS field closes the field.
func TestAssertFieldUnsignedRejectsOnlyASignedRowForTheSameField(t *testing.T) {
	field := "signature-party-1"
	other := "signature-party-2"

	err := assertFieldUnsigned([]db.SignatureRecord{{Status: "SIGNED", FieldName: &field}}, field)
	if !errors.Is(err, ErrFieldAlreadySigned) {
		t.Errorf("a SIGNED row for the field must report ErrFieldAlreadySigned, got %v", err)
	}

	for name, records := range map[string][]db.SignatureRecord{
		"no signatures":        nil,
		"another field signed": {{Status: "SIGNED", FieldName: &other}},
		"revoked":              {{Status: "REVOKED", FieldName: &field}},
		"pending":              {{Status: "PENDING", FieldName: &field}},
		"no field recorded":    {{Status: "SIGNED"}},
	} {
		if err := assertFieldUnsigned(records, field); err != nil {
			t.Errorf("%s must leave the field signable, got %v", name, err)
		}
	}
}
