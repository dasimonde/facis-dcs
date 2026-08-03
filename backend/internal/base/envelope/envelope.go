// Package envelope implements the content-encryption-key (CEK) envelope for
// artifacts at rest (DCS-NFR-SEC-14): a random 256-bit CEK encrypts artifact
// bytes with AES-256-GCM, and the CEK itself is wrapped to a recipient's
// P-256 keyAgreement public key via ECDH-ES+A256KW (RFC 7518 §4.6: ephemeral
// sender keypair → ECDH → Concat-KDF → RFC 3394 key wrap). Wrapping needs
// only the recipient's public key; unwrapping runs through the KeyAgreement
// interface, whose HSM implementation keeps the static private key in the
// token. Destroying every wrapped-CEK record shreds the CEK for good.
package envelope

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"digital-contracting-service/internal/base/hsm"
)

// Alg is the CEK wrap algorithm identifier (also the Concat-KDF AlgorithmID).
const Alg = "ECDH-ES+A256KW"

const (
	cekLen   = 32
	nonceLen = 12
	kwIV     = 0xA6A6A6A6A6A6A6A6
)

// KeyAgreement yields the raw ECDH shared secret between a held static
// private key and a peer public key.
type KeyAgreement interface {
	DeriveECDH(peerPub *ecdsa.PublicKey) ([]byte, error)
}

// HSMKeyAgreement returns a KeyAgreement backed by the token key with the
// given CKA_LABEL.
func HSMKeyAgreement(label string) KeyAgreement { return hsmKeyAgreement{label: label} }

type hsmKeyAgreement struct{ label string }

func (a hsmKeyAgreement) DeriveECDH(peerPub *ecdsa.PublicKey) ([]byte, error) {
	return hsm.DeriveECDH(a.label, peerPub)
}

// EphemeralPublicKey is the sender's ephemeral P-256 public key as a JWK.
type EphemeralPublicKey struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

// WrappedCEK is one wrapped-CEK record: the recipient unwraps it with the
// static private key matching the keyAgreement public key it was wrapped to.
//
// KID names that key — the recipient's verification method as its own DID
// document publishes it (RFC 7516 §4.1.6). Without it a recipient can only
// guess which of its keys was used, which is why the wrap says so.
type WrappedCEK struct {
	Alg     string             `json:"alg"`
	KID     string             `json:"kid"`
	EPK     EphemeralPublicKey `json:"epk"`
	Wrapped []byte             `json:"wrapped"`
}

// NewCEK returns a fresh random 256-bit content-encryption key.
func NewCEK() ([]byte, error) {
	cek := make([]byte, cekLen)
	if _, err := rand.Read(cek); err != nil {
		return nil, fmt.Errorf("generate cek: %w", err)
	}
	return cek, nil
}

// Wrap wraps a CEK to the recipient's P-256 keyAgreement public key
// (ECDH-ES+A256KW with an ephemeral sender keypair), naming that key by the
// verification method id the recipient publishes it under. Pure Go: only the
// recipient's public key is needed.
func Wrap(cek []byte, recipientMethodID string, recipient *ecdsa.PublicKey) (*WrappedCEK, error) {
	if len(cek) != cekLen {
		return nil, fmt.Errorf("cek must be %d bytes, got %d", cekLen, len(cek))
	}
	if strings.TrimSpace(recipientMethodID) == "" {
		return nil, errors.New("the recipient's key-agreement verification method must be named")
	}
	if recipient == nil || recipient.Curve != elliptic.P256() {
		return nil, errors.New("recipient public key must be ECDSA P-256")
	}
	recipientECDH, err := recipient.ECDH()
	if err != nil {
		return nil, fmt.Errorf("recipient public key: %w", err)
	}

	eph, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ephemeral key: %w", err)
	}
	z, err := eph.ECDH(recipientECDH)
	if err != nil {
		return nil, fmt.Errorf("ecdh: %w", err)
	}

	wrapped, err := keyWrap(concatKDF(z), cek)
	if err != nil {
		return nil, err
	}

	// Uncompressed point 0x04 || X || Y.
	point := eph.PublicKey().Bytes()
	return &WrappedCEK{
		Alg: Alg,
		KID: recipientMethodID,
		EPK: EphemeralPublicKey{
			Kty: "EC",
			Crv: "P-256",
			X:   base64.RawURLEncoding.EncodeToString(point[1:33]),
			Y:   base64.RawURLEncoding.EncodeToString(point[33:65]),
		},
		Wrapped: wrapped,
	}, nil
}

// Unwrap recovers the CEK from a wrapped record. The ECDH against the
// ephemeral public key runs through the KeyAgreement (the HSM in production).
func Unwrap(w *WrappedCEK, agreement KeyAgreement) ([]byte, error) {
	if w == nil {
		return nil, errors.New("wrapped cek is nil")
	}
	if w.Alg != Alg {
		return nil, fmt.Errorf("unsupported wrap algorithm %q", w.Alg)
	}
	epk, err := w.EPK.publicKey()
	if err != nil {
		return nil, fmt.Errorf("ephemeral public key: %w", err)
	}
	z, err := agreement.DeriveECDH(epk)
	if err != nil {
		return nil, fmt.Errorf("derive shared secret: %w", err)
	}
	cek, err := keyUnwrap(concatKDF(z), w.Wrapped)
	if err != nil {
		return nil, err
	}
	return cek, nil
}

// Encrypt encrypts plaintext with the CEK using AES-256-GCM, binding it to
// the scope via AAD. Output layout: nonce ∥ ciphertext.
func Encrypt(cek, plaintext []byte, scopeID string) ([]byte, error) {
	gcm, err := newGCM(cek)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, plaintext, []byte(scopeID)), nil
}

// Decrypt reverses Encrypt; it fails if the ciphertext was modified or the
// scope does not match the one it was encrypted under.
func Decrypt(cek, blob []byte, scopeID string) ([]byte, error) {
	gcm, err := newGCM(cek)
	if err != nil {
		return nil, err
	}
	if len(blob) < nonceLen+gcm.Overhead() {
		return nil, errors.New("ciphertext too short")
	}
	plaintext, err := gcm.Open(nil, blob[:nonceLen], blob[nonceLen:], []byte(scopeID))
	if err != nil {
		return nil, fmt.Errorf("decrypt content: %w", err)
	}
	return plaintext, nil
}

func newGCM(cek []byte) (cipher.AEAD, error) {
	if len(cek) != cekLen {
		return nil, fmt.Errorf("cek must be %d bytes, got %d", cekLen, len(cek))
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, fmt.Errorf("init content cipher: %w", err)
	}
	return cipher.NewGCM(block)
}

func (epk EphemeralPublicKey) publicKey() (*ecdsa.PublicKey, error) {
	if epk.Kty != "EC" || epk.Crv != "P-256" {
		return nil, fmt.Errorf("unsupported epk %s/%s (only EC P-256)", epk.Kty, epk.Crv)
	}
	xBytes, err := base64.RawURLEncoding.DecodeString(epk.X)
	if err != nil {
		return nil, fmt.Errorf("decoding x: %w", err)
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(epk.Y)
	if err != nil {
		return nil, fmt.Errorf("decoding y: %w", err)
	}
	pub := &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(xBytes),
		Y:     new(big.Int).SetBytes(yBytes),
	}
	if _, err := pub.ECDH(); err != nil {
		return nil, fmt.Errorf("epk not on P-256: %w", err)
	}
	return pub, nil
}

// concatKDF derives the 256-bit key-wrap key from the ECDH shared secret per
// RFC 7518 §4.6 (Concat KDF, SHA-256, single round for 256 output bits;
// AlgorithmID = Alg, empty PartyUInfo/PartyVInfo).
func concatKDF(z []byte) []byte {
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.BigEndian, uint32(1)) // round counter
	buf.Write(z)
	writeLenPrefixed(&buf, []byte(Alg))                        // AlgorithmID
	writeLenPrefixed(&buf, nil)                                // PartyUInfo
	writeLenPrefixed(&buf, nil)                                // PartyVInfo
	_ = binary.Write(&buf, binary.BigEndian, uint32(cekLen*8)) // SuppPubInfo
	sum := sha256.Sum256(buf.Bytes())
	return sum[:]
}

func writeLenPrefixed(buf *bytes.Buffer, data []byte) {
	_ = binary.Write(buf, binary.BigEndian, uint32(len(data)))
	buf.Write(data)
}

// keyWrap wraps the CEK under the KDF-derived key per RFC 3394.
func keyWrap(kek, cek []byte) ([]byte, error) {
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, fmt.Errorf("init wrap cipher: %w", err)
	}
	n := len(cek) / 8
	a := uint64(kwIV)
	r := make([][]byte, n)
	for i := range r {
		r[i] = bytes.Clone(cek[i*8 : (i+1)*8])
	}
	var b [16]byte
	for j := 0; j <= 5; j++ {
		for i := 1; i <= n; i++ {
			binary.BigEndian.PutUint64(b[:8], a)
			copy(b[8:], r[i-1])
			block.Encrypt(b[:], b[:])
			a = binary.BigEndian.Uint64(b[:8]) ^ uint64(n*j+i)
			copy(r[i-1], b[8:])
		}
	}
	out := make([]byte, 8+len(cek))
	binary.BigEndian.PutUint64(out[:8], a)
	for i, ri := range r {
		copy(out[8+i*8:], ri)
	}
	return out, nil
}

// keyUnwrap reverses keyWrap; the RFC 3394 integrity register detects a wrong
// key-agreement key as well as a modified wrapped blob.
func keyUnwrap(kek, wrapped []byte) ([]byte, error) {
	if len(wrapped) != 8+cekLen {
		return nil, fmt.Errorf("wrapped cek must be %d bytes, got %d", 8+cekLen, len(wrapped))
	}
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, fmt.Errorf("init wrap cipher: %w", err)
	}
	n := (len(wrapped) - 8) / 8
	a := binary.BigEndian.Uint64(wrapped[:8])
	r := make([][]byte, n)
	for i := range r {
		r[i] = bytes.Clone(wrapped[8+i*8 : 8+(i+1)*8])
	}
	var b [16]byte
	for j := 5; j >= 0; j-- {
		for i := n; i >= 1; i-- {
			binary.BigEndian.PutUint64(b[:8], a^uint64(n*j+i))
			copy(b[8:], r[i-1])
			block.Decrypt(b[:], b[:])
			a = binary.BigEndian.Uint64(b[:8])
			copy(r[i-1], b[8:])
		}
	}
	if a != kwIV {
		return nil, errors.New("unwrap cek: integrity check failed")
	}
	cek := make([]byte, n*8)
	for i, ri := range r {
		copy(cek[i*8:], ri)
	}
	return cek, nil
}
