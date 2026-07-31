package compiler

import (
	"crypto/sha256"
	"encoding/base32"
	"strings"
)

// payloadCID returns the CIDv1 of the exact payload bytes: raw codec (0x55),
// sha2-256 multihash, multibase base32-lower ("b…") — the canonical IPFS
// content-address. It is a PURE function of the bytes (a plain raw block, no
// UnixFS/dag-pb chunking), so A's render, B's recompile, and any verifier compute
// the identical CID, and it resolves the machine-readable payload from IPFS.
func payloadCID(payload []byte) string {
	sum := sha256.Sum256(payload)
	// CIDv1 bytes: version(0x01) rawCodec(0x55) multihash[ sha2-256(0x12) len(0x20) digest(32) ]
	raw := make([]byte, 0, 4+len(sum))
	raw = append(raw, 0x01, 0x55, 0x12, 0x20)
	raw = append(raw, sum[:]...)
	enc := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
	return "b" + strings.ToLower(enc)
}
