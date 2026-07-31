package envelope_test

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"testing"

	"digital-contracting-service/internal/auth/oid4vp/status/envelope"

	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

const draft21CWTExampleHex = "d2845820a2012610781a6170706c69636174696f6e2f7374617475736c6973742b63" +
	"7774a1044231325850a502782168747470733a2f2f6578616d706c652e636f6d2f73" +
	"74617475736c697374732f31061a648c5bea041a8898dfea19fffe19a8c019fffda2" +
	"646269747301636c73744a78dadbb918000217015d584093fa4d01032b18c35e2fe1" +
	"101b77fd6cc9440022caa4694450c4e4e9feab4e99d1fa6d9772ce2bf3a12e0323de" +
	"d7c982c5e101a5e67f0cbc1e2b6f57ce99c279"

func draft21ExampleClaims() map[string]any {
	lst, _ := hex.DecodeString("78dadbb918000217015d")
	return map[string]any{
		"sub": "https://example.com/statuslists/1",
		"iat": int64(1686920170),
		"exp": int64(2291720170),
		"ttl": int64(43200),
		"status_list": map[string]any{
			"bits": int64(1),
			"lst":  lst,
		},
	}
}

func TestStatusListCWT_WithoutIssResolvesByKID(t *testing.T) {
	privateKey := testECPrivateKey(t)
	otherKey := testECPrivateKey(t)
	kid := "statuslist-key-12"

	claims := draft21ExampleClaims()
	signed, err := envelope.SignStatusListCWT(claims, privateKey, kid)
	require.NoError(t, err)

	verified, err := envelope.VerifyStatusListCWT(signed, envelope.CWTVerifier{
		StatusListURI: claims["sub"].(string),
		ResolveECDSAByKID: func(_ string, resolvedKID string) (*ecdsa.PublicKey, error) {
			require.Equal(t, kid, resolvedKID)
			return &privateKey.PublicKey, nil
		},
	})
	require.NoError(t, err)
	require.Equal(t, claims["sub"], verified["sub"])
	require.Equal(t, claims["iat"], verified["iat"])
	require.NotContains(t, verified, "iss")

	_, err = envelope.VerifyStatusListCWT(signed, envelope.CWTVerifier{
		ResolveECDSAByKID: func(string, string) (*ecdsa.PublicKey, error) {
			return &otherKey.PublicKey, nil
		},
	})
	require.Error(t, err)
}

func TestStatusListCWT_KIDEncodedAsByteString(t *testing.T) {
	privateKey := testECPrivateKey(t)
	claims := draft21ExampleClaims()

	signed, err := envelope.SignStatusListCWT(claims, privateKey, "12")
	require.NoError(t, err)

	_, unprotected, _, _, err := envelope.UnmarshalCOSESign1FullForTest(signed)
	require.NoError(t, err)

	kid, ok := unprotected[int64(4)].([]byte)
	require.True(t, ok)
	require.Equal(t, []byte("12"), kid)
}

func TestStatusListCWT_RejectsByteStringSub(t *testing.T) {
	privateKey := testECPrivateKey(t)
	payload, err := cbor.Marshal(map[int64]any{
		int64(2): []byte("https://example.com/statuslists/1"),
		int64(6): int64(1686920170),
		int64(65533): map[string]any{
			"bits": uint64(1),
			"lst":  []byte{0x78, 0x01, 0x03, 0x00, 0x00},
		},
	})
	require.NoError(t, err)

	signed, err := signStatusListCWTForTest(t, payload, privateKey, "kid-1")
	require.NoError(t, err)

	_, err = envelope.VerifyStatusListCWT(signed, envelope.CWTVerifier{
		ResolveECDSAByKID: func(string, string) (*ecdsa.PublicKey, error) {
			return &privateKey.PublicKey, nil
		},
	})
	require.ErrorContains(t, err, "text string")
}

func TestStatusListCWT_RejectsTextKID(t *testing.T) {
	privateKey := testECPrivateKey(t)
	payload, err := cbor.Marshal(map[int64]any{
		int64(2): "https://example.com/statuslists/1",
		int64(6): int64(1686920170),
		int64(65533): map[string]any{
			"bits": uint64(1),
			"lst":  []byte{0x78, 0x01, 0x03, 0x00, 0x00},
		},
	})
	require.NoError(t, err)

	protected, err := cbor.Marshal(map[int64]any{
		int64(1):  int64(-7),
		int64(16): "application/statuslist+cwt",
	})
	require.NoError(t, err)

	signed, err := signCOSESign1ForTest(t, protected, map[int64]any{
		int64(4): "text-kid",
	}, payload, privateKey)
	require.NoError(t, err)

	_, err = envelope.VerifyStatusListCWT(signed, envelope.CWTVerifier{
		ResolveECDSAByKID: func(string, string) (*ecdsa.PublicKey, error) {
			return &privateKey.PublicKey, nil
		},
	})
	require.ErrorContains(t, err, "byte string")
}

func TestStatusListCWT_RejectsInvalidOptionalIss(t *testing.T) {
	privateKey := testECPrivateKey(t)
	payload, err := cbor.Marshal(map[int64]any{
		int64(1): []byte("not-text-iss"),
		int64(2): "https://example.com/statuslists/1",
		int64(6): int64(1686920170),
		int64(65533): map[string]any{
			"bits": uint64(1),
			"lst":  []byte{0x78, 0x01, 0x03, 0x00, 0x00},
		},
	})
	require.NoError(t, err)

	signed, err := signStatusListCWTForTest(t, payload, privateKey, "kid-1")
	require.NoError(t, err)

	_, err = envelope.VerifyStatusListCWT(signed, envelope.CWTVerifier{
		ResolveECDSAByKID: func(string, string) (*ecdsa.PublicKey, error) {
			return &privateKey.PublicKey, nil
		},
	})
	require.ErrorContains(t, err, "cwt claim 1 (iss)")
}

func TestStatusListCWT_RejectsInvalidNbf(t *testing.T) {
	privateKey := testECPrivateKey(t)
	payload, err := cbor.Marshal(map[int64]any{
		int64(2): "https://example.com/statuslists/1",
		int64(5): "not-a-number",
		int64(6): int64(1686920170),
		int64(65533): map[string]any{
			"bits": uint64(1),
			"lst":  []byte{0x78, 0x01, 0x03, 0x00, 0x00},
		},
	})
	require.NoError(t, err)

	signed, err := signStatusListCWTForTest(t, payload, privateKey, "kid-1")
	require.NoError(t, err)

	_, err = envelope.VerifyStatusListCWT(signed, envelope.CWTVerifier{
		ResolveECDSAByKID: func(string, string) (*ecdsa.PublicKey, error) {
			return &privateKey.PublicKey, nil
		},
	})
	require.ErrorContains(t, err, "nbf")
}

func TestStatusListCWT_RejectsDuplicateKIDInHeaders(t *testing.T) {
	privateKey := testECPrivateKey(t)
	payload, err := cbor.Marshal(map[int64]any{
		int64(2): "https://example.com/statuslists/1",
		int64(6): int64(1686920170),
		int64(65533): map[string]any{
			"bits": uint64(1),
			"lst":  []byte{0x78, 0x01, 0x03, 0x00, 0x00},
		},
	})
	require.NoError(t, err)

	protected, err := cbor.Marshal(map[int64]any{
		int64(1):  int64(-7),
		int64(16): "application/statuslist+cwt",
		int64(4):  []byte("protected-kid"),
	})
	require.NoError(t, err)

	signed, err := signCOSESign1ForTest(t, protected, map[int64]any{
		int64(4): []byte("unprotected-kid"),
	}, payload, privateKey)
	require.NoError(t, err)

	_, err = envelope.VerifyStatusListCWT(signed, envelope.CWTVerifier{
		ResolveECDSAByKID: func(string, string) (*ecdsa.PublicKey, error) {
			return &privateKey.PublicKey, nil
		},
	})
	require.ErrorContains(t, err, "must not appear in both protected and unprotected headers")
}

func TestStatusListCWT_OptionalIssFallback(t *testing.T) {
	privateKey := testECPrivateKey(t)
	claims := draft21ExampleClaims()
	claims["iss"] = "did:web:dev.example:issuer:poa"

	signed, err := envelope.SignStatusListCWT(claims, privateKey, "")
	require.NoError(t, err)

	verified, err := envelope.VerifyStatusListCWT(signed, envelope.CWTVerifier{
		ResolveECDSA: func(issuer string) (*ecdsa.PublicKey, error) {
			require.Equal(t, claims["iss"], issuer)
			return &privateKey.PublicKey, nil
		},
	})
	require.NoError(t, err)
	require.Equal(t, claims["iss"], verified["iss"])
}

func TestStatusListCWT_RejectsTextListBytes(t *testing.T) {
	privateKey := testECPrivateKey(t)
	payload, err := cbor.Marshal(map[int64]any{
		int64(2): "https://example.com/statuslists/1",
		int64(6): int64(1686920170),
		int64(65533): map[string]any{
			"bits": uint64(1),
			"lst":  "not-a-byte-string",
		},
	})
	require.NoError(t, err)

	signed, err := signStatusListCWTForTest(t, payload, privateKey, "kid-1")
	require.NoError(t, err)

	_, err = envelope.VerifyStatusListCWT(signed, envelope.CWTVerifier{
		ResolveECDSAByKID: func(string, string) (*ecdsa.PublicKey, error) {
			return &privateKey.PublicKey, nil
		},
	})
	require.ErrorContains(t, err, "byte string")
}

func TestStatusListCWT_RejectsTTLZero(t *testing.T) {
	privateKey := testECPrivateKey(t)
	payload, err := cbor.Marshal(map[int64]any{
		int64(2):     "https://example.com/statuslists/1",
		int64(6):     int64(1686920170),
		int64(65534): uint64(0),
		int64(65533): map[string]any{
			"bits": uint64(1),
			"lst":  []byte{0x78, 0x01, 0x03, 0x00, 0x00},
		},
	})
	require.NoError(t, err)

	signed, err := signStatusListCWTForTest(t, payload, privateKey, "kid-1")
	require.NoError(t, err)

	_, err = envelope.VerifyStatusListCWT(signed, envelope.CWTVerifier{
		ResolveECDSAByKID: func(string, string) (*ecdsa.PublicKey, error) {
			return &privateKey.PublicKey, nil
		},
	})
	require.ErrorContains(t, err, "ttl")
}

func TestStatusListCWT_Draft21ExampleStructure(t *testing.T) {
	privateKey := testECPrivateKey(t)
	claims := draft21ExampleClaims()

	signed, err := envelope.SignStatusListCWT(claims, privateKey, "12")
	require.NoError(t, err)

	verified, err := envelope.VerifyStatusListCWT(signed, envelope.CWTVerifier{
		StatusListURI: claims["sub"].(string),
		ResolveECDSAByKID: func(_ string, kid string) (*ecdsa.PublicKey, error) {
			require.Equal(t, "12", kid)
			return &privateKey.PublicKey, nil
		},
	})
	require.NoError(t, err)
	require.Equal(t, claims["sub"], verified["sub"])
	require.Equal(t, claims["ttl"], verified["ttl"])

	statusList := verified["status_list"].(map[string]any)
	require.Equal(t, int64(1), statusList["bits"])
	require.Equal(t, claims["status_list"].(map[string]any)["lst"], statusList["lst"])
}

func TestStatusListCWT_Draft21ExampleHexPayloadValidates(t *testing.T) {
	raw, err := hex.DecodeString(draft21CWTExampleHex)
	require.NoError(t, err)

	protected, unprotected, payload, _, err := envelope.UnmarshalCOSESign1FullForTest(raw)
	require.NoError(t, err)
	kid, err := envelope.COSEKIDFromHeadersForTest(protected, unprotected)
	require.NoError(t, err)
	require.Equal(t, "12", kid)

	claims, err := envelope.DecodeCWTClaimsSetForTest(payload)
	require.NoError(t, err)
	require.NoError(t, envelope.ValidateStatusListCWTClaimsForTest(claims))
	require.NoError(t, envelope.ValidateStatusListCWTProtectedForTest(protected))
}

func TestStatusListCWT_RejectsLegacyProtectedHeaderKey(t *testing.T) {
	privateKey := testECPrivateKey(t)
	payload, err := cbor.Marshal(map[int64]any{
		int64(2): "http://example.com/status",
		int64(6): int64(1719129600),
		int64(65533): map[string]any{
			"bits": uint64(1),
			"lst":  []byte{0x78, 0x01, 0x03, 0x00, 0x00},
		},
	})
	require.NoError(t, err)

	protected, err := cbor.Marshal(map[int64]any{
		int64(1): int64(-7),
		int64(4): "statuslist+cwt",
	})
	require.NoError(t, err)

	signed, err := signCOSESign1ForTest(t, protected, map[int64]any{int64(4): []byte("kid-1")}, payload, privateKey)
	require.NoError(t, err)

	_, err = envelope.VerifyStatusListCWT(signed, envelope.CWTVerifier{
		ResolveECDSAByKID: func(string, string) (*ecdsa.PublicKey, error) {
			return &privateKey.PublicKey, nil
		},
	})
	require.Error(t, err)
}

func signStatusListCWTForTest(t *testing.T, payload []byte, privateKey *ecdsa.PrivateKey, kid string) ([]byte, error) {
	t.Helper()
	protected, err := cbor.Marshal(map[int64]any{
		int64(1):  int64(-7),
		int64(16): "application/statuslist+cwt",
	})
	if err != nil {
		return nil, err
	}
	unprotected := map[int64]any{}
	if kid != "" {
		unprotected[int64(4)] = []byte(kid)
	}
	return signCOSESign1ForTest(t, protected, unprotected, payload, privateKey)
}

func signCOSESign1ForTest(t *testing.T, protected []byte, unprotected map[int64]any, payload []byte, privateKey *ecdsa.PrivateKey) ([]byte, error) {
	t.Helper()
	sigStructure, err := cbor.Marshal([]any{"Signature1", protected, []byte{}, payload})
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(sigStructure)
	r, s, err := ecdsa.Sign(rand.Reader, privateKey, digest[:])
	if err != nil {
		return nil, err
	}
	signature := append(padCOSEIntForTest(r, 32), padCOSEIntForTest(s, 32)...)
	inner, err := cbor.Marshal([]any{protected, unprotected, payload, signature})
	if err != nil {
		return nil, err
	}
	return cbor.Marshal(cbor.Tag{Number: 18, Content: cbor.RawMessage(inner)})
}

func padCOSEIntForTest(v *big.Int, size int) []byte {
	out := make([]byte, size)
	b := v.Bytes()
	copy(out[size-len(b):], b)
	return out
}
