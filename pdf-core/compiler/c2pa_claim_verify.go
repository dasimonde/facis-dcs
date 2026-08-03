package compiler

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"fmt"
	"math/big"
	"strings"
)

// VerifyC2PAClaimSignatures verifies the C2PA provenance of a PDF: for every
// manifest in its embedded manifest store it rebuilds the COSE Sig_structure
// over the claim and checks the ES256 signature against the public key of the
// x5chain leaf in the COSE protected headers, then recomputes the SHA-256 of
// every assertion box the claim commits to through its created_assertions
// hashed-URIs.
//
// Together those two make the claim signature mean something: the signature
// covers only the claim, so an unchecked assertion store can be swapped wholesale
// underneath a claim signature that still verifies.
//
// What it does NOT do is decide whether the signing certificate is one to trust —
// the leaf travels inside the manifest it authenticates. pdf-core is shared by
// several DCS instances that each sign with their own dcs-c2pa key, so no single
// chain can be pinned here; anchoring the leaf is the calling verifier's policy.
func VerifyC2PAClaimSignatures(pdf []byte) error {
	store, err := ExtractManifestStore(pdf)
	if err != nil {
		return fmt.Errorf("extract C2PA manifest store: %w", err)
	}
	manifests, err := extractTopLevelManifestBoxes(store)
	if err != nil {
		return fmt.Errorf("read C2PA manifest store: %w", err)
	}
	if len(manifests) == 0 {
		return fmt.Errorf("C2PA manifest store carries no manifests")
	}
	for i, manifest := range manifests {
		label, err := extractJUMBFLabel(manifest)
		if err != nil {
			return fmt.Errorf("manifest %d: %w", i, err)
		}
		if err := verifyManifestClaim(manifest); err != nil {
			return fmt.Errorf("manifest %s: %w", label, err)
		}
	}
	return nil
}

// verifyManifestClaim checks one manifest's claim signature and the assertion
// hashes its claim commits to.
func verifyManifestClaim(manifest []byte) error {
	claimBox, err := extractLabeledChildJUMBFBox(manifest, "c2pa.claim.v2")
	if err != nil {
		return fmt.Errorf("find claim: %w", err)
	}
	claimPayload, err := jumbfCBORPayload(claimBox)
	if err != nil {
		return fmt.Errorf("read claim: %w", err)
	}
	signatureBox, err := extractLabeledChildJUMBFBox(manifest, "c2pa.signature")
	if err != nil {
		return fmt.Errorf("find claim signature: %w", err)
	}
	coseSign1, err := jumbfCBORPayload(signatureBox)
	if err != nil {
		return fmt.Errorf("read claim signature: %w", err)
	}
	protected, signature, err := decodeCOSESign1(coseSign1)
	if err != nil {
		return fmt.Errorf("decode COSE_Sign1: %w", err)
	}
	key, err := coseX5ChainLeafKey(protected)
	if err != nil {
		return err
	}
	if len(signature) != 64 {
		return fmt.Errorf("claim signature is %d bytes, want a 64-byte ES256 r||s", len(signature))
	}
	sigStructure := cborArray(
		cborText("Signature1"),
		cborBytes(protected),
		cborBytes([]byte{}),
		cborBytes(claimPayload),
	)
	digest := sha256.Sum256(sigStructure)
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:])
	if !ecdsa.Verify(key, digest[:], r, s) {
		return fmt.Errorf("COSE_Sign1 claim signature does not verify against the x5chain leaf key")
	}
	return verifyClaimAssertionHashes(manifest, claimPayload)
}

// verifyClaimAssertionHashes recomputes the SHA-256 of every assertion the claim
// references through created_assertions and compares it with the hash the signed
// claim recorded. An assertion hash is taken over its JUMBF superbox payload (the
// box without its 8-byte BMFF header), which is what the render side hashes.
func verifyClaimAssertionHashes(manifest, claimPayload []byte) error {
	claim, _, err := decodeCBOR(claimPayload)
	if err != nil {
		return fmt.Errorf("decode claim CBOR: %w", err)
	}
	assertions, ok := claim.entry("created_assertions")
	if !ok || assertions.major != cborMajorArray {
		return fmt.Errorf("claim declares no created_assertions array")
	}
	assertionStore, err := extractLabeledChildJUMBFBox(manifest, "c2pa.assertions")
	if err != nil {
		return fmt.Errorf("find assertion store: %w", err)
	}
	for _, ref := range assertions.array {
		url, urlOK := ref.entry("url")
		hash, hashOK := ref.entry("hash")
		if !urlOK || !hashOK || url.major != cborMajorText || hash.major != cborMajorBytes {
			return fmt.Errorf("created_assertions carries an entry that is not a hashed-URI")
		}
		label := assertionLabelFromURI(url.text)
		if label == "" {
			return fmt.Errorf("created_assertions entry %q names no assertion", url.text)
		}
		box, err := extractLabeledChildJUMBFBox(assertionStore, label)
		if err != nil {
			return fmt.Errorf("claim references assertion %q, which the manifest does not carry: %w", label, err)
		}
		if len(box) < 8 {
			return fmt.Errorf("assertion %q box is truncated", label)
		}
		actual := sha256.Sum256(box[8:])
		if string(actual[:]) != string(hash.bytes) {
			return fmt.Errorf("assertion %q does not match the hash the signed claim commits to", label)
		}
	}
	return nil
}

// assertionLabelFromURI reads the assertion label out of a claim hashed-URI. The
// reference is written relative ("self#jumbf=c2pa.assertions/dcs.lifecycle") or
// manifest-absolute ("self#jumbf=/c2pa/<manifest>/c2pa.assertions/dcs.lifecycle");
// both end in the label inside the manifest's own assertion store.
func assertionLabelFromURI(uri string) string {
	const marker = "c2pa.assertions/"
	idx := strings.LastIndex(uri, marker)
	if idx < 0 {
		return ""
	}
	return uri[idx+len(marker):]
}

// jumbfCBORPayload returns the payload of the "cbor" content box inside a JUMBF
// superbox.
func jumbfCBORPayload(jumbBox []byte) ([]byte, error) {
	outer, err := parseBMFFBoxes(jumbBox)
	if err != nil {
		return nil, err
	}
	if len(outer) != 1 || outer[0].Type != "jumb" {
		return nil, fmt.Errorf("JUMBF superbox expected")
	}
	children, err := parseBMFFBoxes(outer[0].Payload)
	if err != nil {
		return nil, err
	}
	for _, child := range children {
		if child.Type == "cbor" {
			return child.Payload, nil
		}
	}
	return nil, fmt.Errorf("no cbor content box")
}

// decodeCOSESign1 unwraps a tagged COSE_Sign1 with a detached payload and
// returns its protected-header bytes and signature.
func decodeCOSESign1(data []byte) (protected, signature []byte, err error) {
	if len(data) > 0 && data[0] == 0xD2 {
		data = data[1:]
	}
	envelope, _, err := decodeCBOR(data)
	if err != nil {
		return nil, nil, err
	}
	if envelope.major != cborMajorArray || len(envelope.array) != 4 {
		return nil, nil, fmt.Errorf("COSE_Sign1 must be a 4-element array")
	}
	if envelope.array[0].major != cborMajorBytes {
		return nil, nil, fmt.Errorf("COSE_Sign1 protected headers must be a byte string")
	}
	if envelope.array[3].major != cborMajorBytes {
		return nil, nil, fmt.Errorf("COSE_Sign1 signature must be a byte string")
	}
	// The claim is the detached payload: it lives in its own JUMBF box and the
	// Sig_structure puts it back in, so a COSE_Sign1 carrying one inline would be
	// signing bytes nobody reads.
	if envelope.array[2].major != cborMajorSimple {
		return nil, nil, fmt.Errorf("COSE_Sign1 payload must be detached (null)")
	}
	return envelope.array[0].bytes, envelope.array[3].bytes, nil
}

// coseX5Chain returns the DER certificates of the x5chain (RFC 9360 header 33)
// the protected headers carry, leaf first, provided the headers declare ES256.
func coseX5Chain(protected []byte) ([][]byte, error) {
	headers, _, err := decodeCBOR(protected)
	if err != nil {
		return nil, fmt.Errorf("decode COSE protected headers: %w", err)
	}
	if headers.major != cborMajorMap {
		return nil, fmt.Errorf("COSE protected headers must be a map")
	}
	alg, ok := headers.intEntry(1)
	if !ok || alg.major != cborMajorNegInt || alg.signedValue() != -7 {
		return nil, fmt.Errorf("COSE protected headers do not declare alg=ES256 (-7)")
	}
	chain, ok := headers.intEntry(33)
	if !ok {
		return nil, fmt.Errorf("COSE protected headers carry no x5chain (header 33)")
	}
	switch chain.major {
	case cborMajorBytes:
		return [][]byte{chain.bytes}, nil
	case cborMajorArray:
		if len(chain.array) == 0 {
			return nil, fmt.Errorf("x5chain is empty")
		}
		out := make([][]byte, 0, len(chain.array))
		for _, cert := range chain.array {
			if cert.major != cborMajorBytes {
				return nil, fmt.Errorf("x5chain carries an entry that is not a certificate")
			}
			out = append(out, cert.bytes)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("x5chain must be a certificate or an array of certificates")
	}
}

// coseX5ChainLeafKey returns the P-256 public key of the x5chain leaf (RFC 9360
// header 33) the protected headers carry, provided the headers declare ES256.
func coseX5ChainLeafKey(protected []byte) (*ecdsa.PublicKey, error) {
	chain, err := coseX5Chain(protected)
	if err != nil {
		return nil, err
	}
	leafDER := chain[0]
	leaf, err := x509.ParseCertificate(leafDER)
	if err != nil {
		return nil, fmt.Errorf("parse x5chain leaf certificate: %w", err)
	}
	key, ok := leaf.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("x5chain leaf %q does not hold an ECDSA key", leaf.Subject.CommonName)
	}
	return key, nil
}

// CBOR major types, as used by the C2PA structures pdf-core reads back.
const (
	cborMajorUint   byte = 0
	cborMajorNegInt byte = 1
	cborMajorBytes  byte = 2
	cborMajorText   byte = 3
	cborMajorArray  byte = 4
	cborMajorMap    byte = 5
	cborMajorSimple byte = 7
)

// cborItem is one decoded CBOR data item. Only the fields belonging to its major
// type are populated.
type cborItem struct {
	major byte
	arg   uint64
	bytes []byte
	text  string
	array []cborItem
	keys  []cborItem
	pairs []cborItem
}

// signedValue is the integer a major-type-0/1 item denotes.
func (i cborItem) signedValue() int64 {
	if i.major == cborMajorNegInt {
		return -1 - int64(i.arg)
	}
	return int64(i.arg)
}

// entry looks up a map value by text key.
func (i cborItem) entry(key string) (cborItem, bool) {
	for n, k := range i.keys {
		if k.major == cborMajorText && k.text == key {
			return i.pairs[n], true
		}
	}
	return cborItem{}, false
}

// intEntry looks up a map value by unsigned-integer key, the encoding COSE uses
// for its registered header labels.
func (i cborItem) intEntry(key uint64) (cborItem, bool) {
	for n, k := range i.keys {
		if k.major == cborMajorUint && k.arg == key {
			return i.pairs[n], true
		}
	}
	return cborItem{}, false
}

// decodeCBOR decodes one data item and returns it with the number of bytes it
// consumed. Indefinite-length items are refused rather than guessed at: nothing
// pdf-core or the C2PA structures it reads emits them.
func decodeCBOR(data []byte) (cborItem, int, error) {
	if len(data) == 0 {
		return cborItem{}, 0, fmt.Errorf("truncated CBOR")
	}
	major := data[0] >> 5
	arg, header, err := cborArgument(data)
	if err != nil {
		return cborItem{}, 0, err
	}
	item := cborItem{major: major, arg: arg}
	switch major {
	case cborMajorUint, cborMajorNegInt, cborMajorSimple:
		return item, header, nil
	case cborMajorBytes, cborMajorText:
		end := header + int(arg)
		if end < header || end > len(data) {
			return cborItem{}, 0, fmt.Errorf("truncated CBOR string")
		}
		if major == cborMajorBytes {
			item.bytes = data[header:end]
		} else {
			item.text = string(data[header:end])
		}
		return item, end, nil
	case cborMajorArray:
		pos := header
		for n := uint64(0); n < arg; n++ {
			element, size, err := decodeCBOR(data[pos:])
			if err != nil {
				return cborItem{}, 0, err
			}
			item.array = append(item.array, element)
			pos += size
		}
		return item, pos, nil
	case cborMajorMap:
		pos := header
		for n := uint64(0); n < arg; n++ {
			key, size, err := decodeCBOR(data[pos:])
			if err != nil {
				return cborItem{}, 0, err
			}
			pos += size
			value, size, err := decodeCBOR(data[pos:])
			if err != nil {
				return cborItem{}, 0, err
			}
			pos += size
			item.keys = append(item.keys, key)
			item.pairs = append(item.pairs, value)
		}
		return item, pos, nil
	default:
		return cborItem{}, 0, fmt.Errorf("unsupported CBOR major type %d", major)
	}
}

// cborArgument decodes a head byte's argument and returns it with the header
// length.
func cborArgument(data []byte) (uint64, int, error) {
	switch add := data[0] & 0x1F; {
	case add <= 23:
		return uint64(add), 1, nil
	case add == 24:
		if len(data) < 2 {
			return 0, 0, fmt.Errorf("truncated CBOR head")
		}
		return uint64(data[1]), 2, nil
	case add == 25:
		if len(data) < 3 {
			return 0, 0, fmt.Errorf("truncated CBOR head")
		}
		return uint64(binary.BigEndian.Uint16(data[1:3])), 3, nil
	case add == 26:
		if len(data) < 5 {
			return 0, 0, fmt.Errorf("truncated CBOR head")
		}
		return uint64(binary.BigEndian.Uint32(data[1:5])), 5, nil
	case add == 27:
		if len(data) < 9 {
			return 0, 0, fmt.Errorf("truncated CBOR head")
		}
		return binary.BigEndian.Uint64(data[1:9]), 9, nil
	default:
		return 0, 0, fmt.Errorf("unsupported CBOR additional information %d", add)
	}
}
