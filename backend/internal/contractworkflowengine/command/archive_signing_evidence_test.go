package command

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

func TestBuildArchiveEntryStoresHashedSigningEvidenceOnly(t *testing.T) {
	now := time.Now().UTC()
	entry, err := BuildArchiveEntry(&db.Contract{DID: "did:web:contract", ContractVersion: 1, State: contractstate.Signed.String(), CreatedAt: now, UpdatedAt: now}, "did:web:operator", ArchiveSigningEvidence{
		Signer: "did:web:signer", CredentialType: "PID", CeremonyID: "ceremony-1", Field: "signature-1", SignedAt: now,
		PDFCID: "bafy-pdf", PDFHash: strings.Repeat("a", 64), CredentialHashes: map[string]string{"presentation": "sha256:" + strings.Repeat("b", 64)},
	})
	if err != nil {
		t.Fatal(err)
	}
	metadata, _ := entry.SignatureMeta.MarshalJSON()
	var decoded map[string]any
	if err := json.Unmarshal(metadata, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"signer", "credential_type", "ceremony_id", "field", "signed_at", "pdf_cid", "pdf_hash"} {
		if decoded[key] == "" || decoded[key] == nil {
			t.Errorf("missing %s", key)
		}
	}
	hashes, _ := entry.CredentialHashes.MarshalJSON()
	if strings.Contains(strings.ToLower(string(hashes)), "vp_token") || !strings.Contains(string(hashes), "sha256:") {
		t.Fatalf("credential evidence is not hash-only: %s", hashes)
	}
}
