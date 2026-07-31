package manifest

import (
	"encoding/binary"
	"testing"
)

// bmffBox frames payload as a BMFF box: 4-byte big-endian size (incl. header),
// 4-byte type, payload.
func bmffBox(typ string, payload []byte) []byte {
	out := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(out[0:4], uint32(len(out)))
	copy(out[4:8], typ)
	copy(out[8:], payload)
	return out
}

// jumdBox builds a JUMBF description box carrying label (toggles=0x03 => label
// present), mirroring pdf-core's renderJUMBFDescriptionBox layout.
func jumdBox(label string) []byte {
	payload := make([]byte, 0, 16+1+len(label)+1)
	payload = append(payload, make([]byte, 16)...) // 16-byte UUID
	payload = append(payload, 0x03)                // toggles: label present
	payload = append(payload, []byte(label)...)
	payload = append(payload, 0x00) // null terminator
	return bmffBox("jumd", payload)
}

// jumbfSuperbox builds a "jumb" superbox with a jumd label box + content boxes.
func jumbfSuperbox(label string, children ...[]byte) []byte {
	payload := jumdBox(label)
	for _, c := range children {
		payload = append(payload, c...)
	}
	return bmffBox("jumb", payload)
}

// cborText encodes a short (<24 byte) CBOR text string.
func cborText(s string) []byte {
	return append([]byte{byte(3<<5) | byte(len(s))}, []byte(s)...)
}

// cborTextMap encodes a small CBOR map (<24 pairs) of text->text.
func cborTextMap(pairs ...string) []byte {
	out := []byte{byte(5<<5) | byte(len(pairs)/2)}
	for _, p := range pairs {
		out = append(out, cborText(p)...)
	}
	return out
}

func buildLifecycleManifest(label, contractID, status string) []byte {
	lifecycleCBOR := cborTextMap(
		"contract_id", contractID,
		"status", status,
		"file_hash", "abc123",
	)
	lifecycle := jumbfSuperbox("dcs.lifecycle", bmffBox("cbor", lifecycleCBOR))
	assertions := jumbfSuperbox("c2pa.assertions", lifecycle)
	claim := jumbfSuperbox("c2pa.claim.v2", bmffBox("cbor", []byte{0xA0}))
	return jumbfSuperbox(label, assertions, claim)
}

func TestParseChain_SingleManifestWithLifecycle(t *testing.T) {
	manifest := buildLifecycleManifest("urn:c2pa:manifest-1", "did:example:contract-1", "active")
	store := jumbfSuperbox("c2pa", manifest)

	entries, err := ParseChain(store)
	if err != nil {
		t.Fatalf("ParseChain: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 manifest entry, got %d", len(entries))
	}
	if entries[0].Label != "urn:c2pa:manifest-1" {
		t.Errorf("label = %q, want urn:c2pa:manifest-1", entries[0].Label)
	}
	if entries[0].Lifecycle == nil {
		t.Fatalf("expected lifecycle assertion to be parsed")
	}
	if got := entries[0].Lifecycle["status"]; got != "active" {
		t.Errorf("lifecycle status = %q, want active", got)
	}
	if got := entries[0].Lifecycle["contract_id"]; got != "did:example:contract-1" {
		t.Errorf("lifecycle contract_id = %q, want did:example:contract-1", got)
	}
}

func TestParseChain_MultipleManifests(t *testing.T) {
	m1 := buildLifecycleManifest("urn:c2pa:manifest-1", "did:example:c", "draft")
	m2 := buildLifecycleManifest("urn:c2pa:manifest-2", "did:example:c", "active")
	store := jumbfSuperbox("c2pa", m1, m2)

	entries, err := ParseChain(store)
	if err != nil {
		t.Fatalf("ParseChain: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 manifest entries, got %d", len(entries))
	}
	if entries[0].Label != "urn:c2pa:manifest-1" || entries[1].Label != "urn:c2pa:manifest-2" {
		t.Errorf("unexpected labels: %q, %q", entries[0].Label, entries[1].Label)
	}
	// At least one entry must carry a lifecycle assertion.
	found := false
	for _, e := range entries {
		if e.Lifecycle != nil {
			found = true
		}
	}
	if !found {
		t.Errorf("expected at least one lifecycle assertion across the chain")
	}
}

func TestParseChain_ManifestWithoutLifecycle(t *testing.T) {
	claim := jumbfSuperbox("c2pa.claim.v2", bmffBox("cbor", []byte{0xA0}))
	assertions := jumbfSuperbox("c2pa.assertions", jumbfSuperbox("c2pa.actions.v2", bmffBox("cbor", []byte{0xA0})))
	manifest := jumbfSuperbox("urn:c2pa:manifest-x", assertions, claim)
	store := jumbfSuperbox("c2pa", manifest)

	entries, err := ParseChain(store)
	if err != nil {
		t.Fatalf("ParseChain: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Label != "urn:c2pa:manifest-x" {
		t.Errorf("label = %q", entries[0].Label)
	}
	if entries[0].Lifecycle != nil {
		t.Errorf("expected no lifecycle assertion, got %v", entries[0].Lifecycle)
	}
}

func TestParseChain_RejectsNonJUMBFRoot(t *testing.T) {
	if _, err := ParseChain([]byte("not a jumbf store")); err == nil {
		t.Errorf("expected error for non-JUMBF root")
	}
}
