package envelope

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"math/big"

	"github.com/fxamacker/cbor/v2"
)

const (
	coseHeaderAlgorithm   = 1
	coseHeaderContentType = 3
	coseHeaderKID         = 4
	coseAlgES256          = -7
	coseSign1Context      = "Signature1"
)

func marshalCOSESign1(protected, payload, signature []byte, unprotected map[int64]any) ([]byte, error) {
	if unprotected == nil {
		unprotected = map[int64]any{}
	}
	inner, err := cbor.Marshal([]any{protected, unprotected, payload, signature})
	if err != nil {
		return nil, err
	}
	return cbor.Marshal(cbor.Tag{Number: 18, Content: cbor.RawMessage(inner)})
}

func unmarshalCOSESign1(raw []byte) (protected, payload, signature []byte, err error) {
	protected, _, payload, signature, err = unmarshalCOSESign1Full(raw)
	return protected, payload, signature, err
}

func unmarshalCOSESign1Full(raw []byte) (protected []byte, unprotected map[int64]any, payload, signature []byte, err error) {
	var tag cbor.Tag
	if err = cbor.Unmarshal(raw, &tag); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("invalid cose message: %w", err)
	}
	if tag.Number != 18 {
		return nil, nil, nil, nil, fmt.Errorf("expected COSE_Sign1 tag 18")
	}
	content, ok := tag.Content.(cbor.RawMessage)
	if !ok {
		contentBytes, err := cbor.Marshal(tag.Content)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		content = cbor.RawMessage(contentBytes)
	}
	var parts []cbor.RawMessage
	if err = cbor.Unmarshal(content, &parts); err != nil {
		return nil, nil, nil, nil, err
	}
	if len(parts) != 4 {
		return nil, nil, nil, nil, fmt.Errorf("invalid COSE_Sign1 part count %d", len(parts))
	}
	if protected, err = decodeCBORByteString(parts[0]); err != nil {
		return nil, nil, nil, nil, err
	}
	unprotected = map[int64]any{}
	if err = cbor.Unmarshal(parts[1], &unprotected); err != nil {
		return nil, nil, nil, nil, err
	}
	if payload, err = decodeCBORByteString(parts[2]); err != nil {
		return nil, nil, nil, nil, err
	}
	if signature, err = decodeCBORByteString(parts[3]); err != nil {
		return nil, nil, nil, nil, err
	}
	return protected, unprotected, payload, signature, nil
}

func padCOSEInt(v *big.Int, size int) []byte {
	out := make([]byte, size)
	b := v.Bytes()
	copy(out[size-len(b):], b)
	return out
}

func decodeCBORByteString(raw cbor.RawMessage) ([]byte, error) {
	var out []byte
	if err := cbor.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func signCOSE(protected, payload []byte, privateKey *ecdsa.PrivateKey) ([]byte, error) {
	return signCOSESign1(protected, map[int64]any{}, payload, privateKey)
}

func signCOSESign1(protected []byte, unprotected map[int64]any, payload []byte, privateKey *ecdsa.PrivateKey) ([]byte, error) {
	sigStructure, err := cbor.Marshal([]any{coseSign1Context, protected, []byte{}, payload})
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(sigStructure)
	r, s, err := ecdsa.Sign(rand.Reader, privateKey, digest[:])
	if err != nil {
		return nil, err
	}
	signature := append(padCOSEInt(r, 32), padCOSEInt(s, 32)...)
	return marshalCOSESign1(protected, payload, signature, unprotected)
}

func verifyCOSE(raw []byte, resolve func(issuer string) (*ecdsa.PublicKey, error)) (payload []byte, err error) {
	protected, payload, signature, err := unmarshalCOSESign1(raw)
	if err != nil {
		return nil, err
	}
	var protectedMap map[int64]any
	if err := cbor.Unmarshal(protected, &protectedMap); err != nil {
		return nil, err
	}
	if alg, _ := protectedMap[coseHeaderAlgorithm].(int64); alg != coseAlgES256 {
		return nil, fmt.Errorf("unsupported COSE algorithm %v", protectedMap[coseHeaderAlgorithm])
	}
	sigStructure, err := cbor.Marshal([]any{coseSign1Context, protected, []byte{}, payload})
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(sigStructure)
	if len(signature) != 64 {
		return nil, fmt.Errorf("invalid COSE signature length %d", len(signature))
	}
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:])

	var claims map[string]any
	if err := cbor.Unmarshal(payload, &claims); err != nil {
		return nil, err
	}
	issuer, err := issuerFromCOSEClaims(claims)
	if err != nil {
		return nil, err
	}
	pub, err := resolve(issuer)
	if err != nil {
		return nil, err
	}
	if !ecdsa.Verify(pub, digest[:], r, s) {
		return nil, fmt.Errorf("cose signature verification failed")
	}
	return payload, nil
}

func issuerFromCOSEClaims(claims map[string]any) (string, error) {
	if issuer, ok := claims["issuer"].(string); ok && issuer != "" {
		return issuer, nil
	}
	if iss, ok := claims["iss"].(string); ok && iss != "" {
		return iss, nil
	}
	return "", fmt.Errorf("cose payload missing issuer")
}

var _ = elliptic.P256
