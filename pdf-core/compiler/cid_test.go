package compiler

import "testing"

func TestPayloadCIDFormat(t *testing.T) {
	// CIDv1 raw + sha256 + base32-lower always starts "bafkrei" and is 59 chars.
	cid := payloadCID([]byte(`{"@type":"dcs:Contract"}`))
	if len(cid) != 59 {
		t.Fatalf("CID length %d, want 59: %s", len(cid), cid)
	}
	if cid[:7] != "bafkrei" {
		t.Fatalf("CID prefix %q, want bafkrei: %s", cid[:7], cid)
	}
	if payloadCID([]byte("x")) == payloadCID([]byte("y")) {
		t.Fatal("distinct payloads must yield distinct CIDs")
	}
	if payloadCID([]byte("same")) != payloadCID([]byte("same")) {
		t.Fatal("CID must be a pure function of the bytes")
	}
}
