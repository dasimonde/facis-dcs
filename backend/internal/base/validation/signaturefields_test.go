package validation

import (
	"reflect"
	"testing"
)

func TestRequiredSignatureFields(t *testing.T) {
	doc := []byte(`{
		"@type": "dcs:Contract",
		"signatureFields": [
			{"@type": "SignatureField", "@id": "urn:doc:x#SignerOne", "signatoryName": "SignerOne"},
			{"@type": "SignatureField", "@id": "urn:doc:x#SignerTwo", "signatoryName": "SignerTwo"},
			{"@type": "SignatureField", "@id": "urn:doc:x#dup", "signatoryName": "SignerOne"},
			{"@type": "SignatureField", "@id": "urn:doc:x#blank", "signatoryName": "  "}
		]
	}`)
	got := RequiredSignatureFields(doc)
	want := []string{"SignerOne", "SignerTwo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}

	if got := RequiredSignatureFields([]byte(`{"@type":"dcs:Contract"}`)); len(got) != 0 {
		t.Fatalf("expected no fields for a contract without signatureFields, got %v", got)
	}
	if got := RequiredSignatureFields([]byte(`not json`)); got != nil {
		t.Fatalf("expected nil for unparseable data, got %v", got)
	}
}

func TestRequiredCredentialType(t *testing.T) {
	contract := []byte(`{
		"dcs:signatureFields": [
			{"dcs:signatoryName": "did:web:qes-party", "dcs:requiredCredentialType": "QES"},
			{"dcs:signatoryName": "did:web:aes-party", "dcs:requiredCredentialType": "AES"},
			{"dcs:signatoryName": "did:web:unspecified-party"}
		]
	}`)

	if got := RequiredCredentialType(contract, "did:web:qes-party"); got != "QES" {
		t.Fatalf("expected QES, got %q", got)
	}
	if got := RequiredCredentialType(contract, "did:web:aes-party"); got != "AES" {
		t.Fatalf("expected AES, got %q", got)
	}
	if got := RequiredCredentialType(contract, "did:web:unspecified-party"); got != "AES" {
		t.Fatalf("expected the AES default for a field with no explicit requirement, got %q", got)
	}
	if got := RequiredCredentialType(contract, "did:web:undeclared-field"); got != "AES" {
		t.Fatalf("expected the AES default for an undeclared field, got %q", got)
	}
	if got := RequiredCredentialType([]byte(`not json`), "did:web:x"); got != "AES" {
		t.Fatalf("expected the AES default for malformed contract data, got %q", got)
	}
}
