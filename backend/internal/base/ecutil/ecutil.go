// Package ecutil encodes and compares ECDSA public keys without reading the
// deprecated PublicKey.X / PublicKey.Y fields (removed from the safe API in
// Go 1.26; use PublicKey.Bytes instead).
package ecutil

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/base64"
	"fmt"
)

// FromXY builds a public key from raw X and Y coordinate bytes using the
// SEC 1 uncompressed encoding (avoids writing deprecated PublicKey.X/Y).
func FromXY(curve elliptic.Curve, x, y []byte) (*ecdsa.PublicKey, error) {
	if curve == nil {
		return nil, fmt.Errorf("ecutil: nil curve")
	}
	byteLen := (curve.Params().BitSize + 7) / 8
	xb, err := fixedCoord(x, byteLen)
	if err != nil {
		return nil, fmt.Errorf("ecutil: x: %w", err)
	}
	yb, err := fixedCoord(y, byteLen)
	if err != nil {
		return nil, fmt.Errorf("ecutil: y: %w", err)
	}
	raw := make([]byte, 1+2*byteLen)
	raw[0] = 0x04
	copy(raw[1:1+byteLen], xb)
	copy(raw[1+byteLen:], yb)
	return ecdsa.ParseUncompressedPublicKey(curve, raw)
}

// FromJWKCoords builds a P-256 public key from base64url-encoded JWK x/y.
func FromJWKCoords(xB64, yB64 string) (*ecdsa.PublicKey, error) {
	x, err := base64.RawURLEncoding.DecodeString(xB64)
	if err != nil {
		return nil, fmt.Errorf("ecutil: decoding x: %w", err)
	}
	y, err := base64.RawURLEncoding.DecodeString(yB64)
	if err != nil {
		return nil, fmt.Errorf("ecutil: decoding y: %w", err)
	}
	return FromXY(elliptic.P256(), x, y)
}

func fixedCoord(b []byte, size int) ([]byte, error) {
	for len(b) > size && len(b) > 0 && b[0] == 0 {
		b = b[1:]
	}
	if len(b) > size {
		return nil, fmt.Errorf("coordinate longer than %d bytes", size)
	}
	if len(b) == size {
		return b, nil
	}
	out := make([]byte, size)
	copy(out[size-len(b):], b)
	return out, nil
}

// Equal reports whether a and b are the same NIST-curve public point.
func Equal(a, b *ecdsa.PublicKey) bool {
	if a == nil || b == nil {
		return a == b
	}
	ab, err1 := a.Bytes()
	bb, err2 := b.Bytes()
	if err1 != nil || err2 != nil {
		return false
	}
	return bytes.Equal(ab, bb)
}

// UncompressedXY returns the X and Y coordinates of pub from its SEC 1
// uncompressed encoding (0x04 || X || Y).
func UncompressedXY(pub *ecdsa.PublicKey) (x, y []byte, err error) {
	if pub == nil {
		return nil, nil, fmt.Errorf("ecutil: nil public key")
	}
	raw, err := pub.Bytes()
	if err != nil {
		return nil, nil, err
	}
	if len(raw) < 3 || raw[0] != 0x04 {
		return nil, nil, fmt.Errorf("ecutil: unexpected public-key encoding")
	}
	n := (len(raw) - 1) / 2
	x = append([]byte(nil), raw[1:1+n]...)
	y = append([]byte(nil), raw[1+n:]...)
	return x, y, nil
}

// JWKCoordStrings returns the base64url-encoded X and Y coordinates for a JWK.
func JWKCoordStrings(pub *ecdsa.PublicKey) (x, y string, err error) {
	xb, yb, err := UncompressedXY(pub)
	if err != nil {
		return "", "", err
	}
	return base64.RawURLEncoding.EncodeToString(xb), base64.RawURLEncoding.EncodeToString(yb), nil
}

// MustJWKCoordStrings is for tests and fixtures; it panics if encoding fails.
func MustJWKCoordStrings(pub *ecdsa.PublicKey) (x, y string) {
	x, y, err := JWKCoordStrings(pub)
	if err != nil {
		panic(err)
	}
	return x, y
}

// MustJWKX / MustJWKY are drop-in replacements for deprecated PublicKey.X/Y
// base64 encodings in tests and fixtures.
func MustJWKX(pub *ecdsa.PublicKey) string {
	x, _, err := JWKCoordStrings(pub)
	if err != nil {
		panic(err)
	}
	return x
}

func MustJWKY(pub *ecdsa.PublicKey) string {
	_, y, err := JWKCoordStrings(pub)
	if err != nil {
		panic(err)
	}
	return y
}

// AsPublic returns an *ecdsa.PublicKey for a PublicKey or PrivateKey value/pointer.
func AsPublic(v any) *ecdsa.PublicKey {
	switch k := v.(type) {
	case *ecdsa.PublicKey:
		return k
	case ecdsa.PublicKey:
		return &k
	case *ecdsa.PrivateKey:
		return &k.PublicKey
	case ecdsa.PrivateKey:
		return &k.PublicKey
	default:
		panic("ecutil.AsPublic: unsupported key type")
	}
}
