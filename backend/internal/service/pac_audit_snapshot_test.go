package service

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	processauditandcompliance "digital-contracting-service/gen/process_audit_and_compliance"
	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/envelope"
	cweeventtype "digital-contracting-service/internal/contractworkflowengine/datatype/eventtype"
	"digital-contracting-service/internal/processauditandcompliance/auditexecutor"
)

type snapshotTestCipher struct{ shredded map[artifactstore.Scope]bool }

func (c snapshotTestCipher) InstanceScope() artifactstore.Scope {
	return artifactstore.Scope{Kind: artifactstore.KindInstance, ID: "did:example:instance"}
}
func (c snapshotTestCipher) Encrypt(_ context.Context, scope artifactstore.Scope, payload []byte) ([]byte, error) {
	if c.shredded[scope] {
		return nil, &artifactstore.ShreddedError{Scope: scope}
	}
	return envelope.Encrypt(bytes.Repeat([]byte{7}, 32), payload, scope.AAD())
}
func (c snapshotTestCipher) Decrypt(_ context.Context, scope artifactstore.Scope, payload []byte) ([]byte, error) {
	if c.shredded[scope] {
		return nil, &artifactstore.ShreddedError{Scope: scope}
	}
	return envelope.Decrypt(bytes.Repeat([]byte{7}, 32), payload, scope.AAD())
}

func TestPACSnapshotRequiresEveryResourceAndAuthenticatesEnvelope(t *testing.T) {
	ctx := context.Background()
	scopes := []artifactstore.Scope{artifactstore.ContractScope("did:example:a"), artifactstore.ContractScope("did:example:b")}
	cipher := snapshotTestCipher{shredded: map[artifactstore.Scope]bool{}}
	payload := []byte("original executor bytes with whitespace\n")
	sealed, err := sealPACSnapshot(ctx, cipher, "audit-1", scopes, payload)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := cipher.Encrypt(ctx, cipher.InstanceScope(), sealed)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored, payload) || bytes.Contains(stored, []byte("audit-1")) {
		t.Fatal("envelope or payload persisted in plaintext")
	}
	opened, err := cipher.Decrypt(ctx, cipher.InstanceScope(), stored)
	if err != nil {
		t.Fatal(err)
	}
	result, gotScopes, err := unsealPACSnapshot(ctx, cipher, "audit-1", opened)
	if err != nil || !bytes.Equal(payload, result) || !equalPACScopes(scopes, gotScopes) {
		t.Fatalf("roundtrip: %s %v", result, err)
	}
	if _, _, err := unsealPACSnapshot(ctx, cipher, "other-audit", opened); err == nil {
		t.Fatal("accepted wrong audit")
	}
	var invalid pacSnapshotEnvelope
	if err := json.Unmarshal(opened, &invalid); err != nil {
		t.Fatal(err)
	}
	invalid.Version++
	wrongVersion, err := json.Marshal(invalid)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := unsealPACSnapshot(ctx, cipher, "audit-1", wrongVersion); err == nil {
		t.Fatal("accepted unknown version")
	}
	tampered := bytes.Clone(stored)
	tampered[len(tampered)-1] ^= 1
	if _, err := cipher.Decrypt(ctx, cipher.InstanceScope(), tampered); err == nil {
		t.Fatal("accepted altered envelope")
	}
	for _, scope := range scopes {
		cipher.shredded[scope] = true
		if _, _, err := unsealPACSnapshot(ctx, cipher, "audit-1", opened); !artifactstore.IsShredded(err) {
			t.Fatalf("shred did not erase whole snapshot: %v", err)
		}
		if _, err := sealPACSnapshot(ctx, cipher, "audit-1", scopes, payload); !artifactstore.IsShredded(err) {
			t.Fatalf("accepted write after shred: %v", err)
		}
		delete(cipher.shredded, scope)
	}
}

func TestPACSnapshotScopeDependenciesFollowPrivateBodies(t *testing.T) {
	component := componenttype.ContractWorkflowEngine.String()
	cases := []struct {
		name     string
		data     map[string]any
		reason   string
		expected int
	}{
		{name: "header"},
		{name: "erased", data: map[string]any{"erased": true}},
		{name: "private", data: map[string]any{"created_by": "alice"}, expected: 1},
		{name: "mixed erased marker", data: map[string]any{"erased": true, "private": "value"}, expected: 1},
		{name: "synthetic reason", reason: "private validation evidence", expected: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resource := persistedAuditEvidenceResource{DID: "did:example:contract", Component: component, AuditTrail: []persistedAuditEvidenceEntry{{EventData: tc.data, Reason: tc.reason}}}
			request, err := json.Marshal(map[string]any{"scope": "contracts", "evidence": map[string]any{"contracts": []persistedAuditEvidenceResource{resource, resource}}})
			if err != nil {
				t.Fatal(err)
			}
			scopes, err := pacEvidenceScopes(request, snapshotTestCipher{}.InstanceScope())
			if err != nil || len(scopes) != tc.expected {
				t.Fatalf("scopes=%v err=%v", scopes, err)
			}
		})
	}
}

func TestPACSnapshotErasureRecordRemainsInstanceScoped(t *testing.T) {
	resource := persistedAuditEvidenceResource{DID: "did:example:erased", Component: componenttype.ContractWorkflowEngine.String(), AuditTrail: []persistedAuditEvidenceEntry{{EventType: cweeventtype.KeyShredded.String(), EventData: map[string]any{"shredded_by": "alice"}}}}
	request, err := json.Marshal(map[string]any{"scope": "contracts", "evidence": map[string]any{"contracts": []persistedAuditEvidenceResource{resource}}})
	if err != nil {
		t.Fatal(err)
	}
	scopes, err := pacEvidenceScopes(request, snapshotTestCipher{}.InstanceScope())
	if err != nil || len(scopes) != 0 {
		t.Fatalf("erasure record scopes=%v err=%v", scopes, err)
	}
}

func TestPACAuditOverShreddedResourceSealsWithoutItsContent(t *testing.T) {
	ctx := context.Background()
	erasedDID, liveDID := "did:example:erased", "did:example:live"
	archive := componenttype.ContractStorageArchive.String()
	checkKind, checkReason := "CHECK", "private integrity evidence"
	evidence := []*auditEvidenceResource{
		{Did: erasedDID, Component: archive, AuditTrail: []*processauditandcompliance.PACResourceAuditTrailEntry{
			{ID: 1, Component: archive, EventType: "ARCHIVE_ENTRY_AUDIT_SUMMARY", Did: &erasedDID, EventData: map[string]any{"storedBy": "did:example:private-actor"}},
			{ID: 2, Component: archive, EventType: "ARCHIVE_INTEGRITY_CHECK", Did: &erasedDID, Kind: &checkKind, Reason: &checkReason},
			{ID: 3, Component: archive, EventType: cweeventtype.DeleteArchived.String(), Did: &erasedDID, EventData: map[string]any{"deleted_by": "did:example:archive-manager"}},
		}},
		{Did: liveDID, Component: archive, AuditTrail: []*processauditandcompliance.PACResourceAuditTrailEntry{
			{ID: 4, Component: archive, EventType: "STORE_ARCHIVED_CONTRACT", Did: &liveDID, EventData: map[string]any{"stored_by": "did:example:storer"}},
		}},
	}
	cipher := snapshotTestCipher{shredded: map[artifactstore.Scope]bool{artifactstore.ContractScope(erasedDID): true}}

	redactErasedEvidence(ctx, cipher, evidence)
	request, err := json.Marshal(auditexecutor.Request{AuditID: "audit-1", Scope: "archive", Evidence: map[string]any{"archive": evidence}})
	if err != nil {
		t.Fatal(err)
	}
	scopes, err := pacEvidenceScopes(request, cipher.InstanceScope())
	if err != nil || !equalPACScopes(scopes, []artifactstore.Scope{artifactstore.ContractScope(liveDID)}) {
		t.Fatalf("scopes=%v err=%v", scopes, err)
	}
	sealed, err := sealPACSnapshot(ctx, cipher, "audit-1", scopes, request)
	if err != nil {
		t.Fatalf("audit over an erased resource cannot be sealed: %v", err)
	}
	if bytes.Contains(request, []byte("did:example:private-actor")) || bytes.Contains(request, []byte(checkReason)) {
		t.Fatalf("erased resource content reached the snapshot: %s", request)
	}
	erased := evidence[0].AuditTrail
	if len(erased) != 3 || erased[0].EventData.(map[string]any)["erased"] != true || erased[1].Reason != nil {
		t.Fatalf("erasure is not visible as such: %+v", erased)
	}
	if erased[2].EventData.(map[string]any)["deleted_by"] != "did:example:archive-manager" || evidence[1].AuditTrail[0].EventData.(map[string]any)["stored_by"] != "did:example:storer" {
		t.Fatal("redaction removed content that the shred does not cover")
	}

	cipher.shredded[artifactstore.ContractScope(liveDID)] = true
	if _, _, err := unsealPACSnapshot(ctx, cipher, "audit-1", sealed); !artifactstore.IsShredded(err) {
		t.Fatalf("run stayed readable after the shred of a resource it captured: %v", err)
	}
}

func TestPACSnapshotPayloadPreservesOriginalByteWhitespace(t *testing.T) {
	original := pacSnapshotPayload{RequestBytes: []byte("{ \"request\": true }\n"), ResponseBytes: []byte("{\n\"response\": true\n}\n")}
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded pacSnapshotPayload
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original.RequestBytes, decoded.RequestBytes) || !bytes.Equal(original.ResponseBytes, decoded.ResponseBytes) {
		t.Fatal("snapshot changed original bytes")
	}
}
