package oid4vp

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	ConfigureStatusListJWTVerification(nil, true)
	os.Exit(m.Run())
}

func makeXFSCListBody(bitstring []byte) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, _ = w.Write(bitstring)
	_ = w.Close()
	body, _ := json.Marshal(map[string]any{
		"tenantId": "default",
		"listId":   1,
		"list":     base64.RawStdEncoding.EncodeToString(buf.Bytes()),
	})
	return body
}

func makeW3CBitstringListBody(t *testing.T, bitstring []byte, purpose string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, err := w.Write(bitstring)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	body, err := json.Marshal(map[string]any{
		"type": []string{"VerifiableCredential", "BitstringStatusListCredential"},
		"credentialSubject": map[string]any{
			"type":          "BitstringStatusList",
			"statusPurpose": purpose,
			"encodedList":   "u" + base64.RawURLEncoding.EncodeToString(buf.Bytes()),
		},
	})
	require.NoError(t, err)
	return body
}

func makeJWTStatusListToken(t *testing.T, bitstring []byte, bits int) string {
	t.Helper()
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	_, err := w.Write(bitstring)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	lst := base64.RawURLEncoding.EncodeToString(buf.Bytes())

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"statuslist+jwt"}`))
	payload, err := json.Marshal(map[string]any{
		"sub": "https://example.com/statuslists/1",
		"iat": time.Now().Add(-time.Minute).Unix(),
		"exp": time.Now().Add(time.Hour).Unix(),
		"status_list": map[string]any{
			"bits": bits,
			"lst":  lst,
		},
	})
	require.NoError(t, err)
	payloadEnc := base64.RawURLEncoding.EncodeToString(payload)
	return header + "." + payloadEnc + "."
}

func TestQueryEntryStatusFromEncodedList_MSB(t *testing.T) {
	bitstring := make([]byte, 16)
	idx := uint32(5)
	byteIdx := idx / 8
	bitIdx := uint(7 - (idx % 8)) // W3C left-most bit ordering.
	bitstring[byteIdx] |= 1 << bitIdx

	body := makeW3CBitstringListBody(t, bitstring, "revocation")
	encoded, err := encodedListFromBodyForPurpose(body, "revocation")
	require.NoError(t, err)

	status, err := queryEntryStatusFromEncodedListWithPacking(encoded, idx, 1, bitPackingMSB)
	require.NoError(t, err)
	assert.Equal(t, "revoked", status)

	status, err = queryEntryStatusFromEncodedListWithPacking(encoded, idx+1, 1, bitPackingMSB)
	require.NoError(t, err)
	assert.Equal(t, "active", status)
}

func TestQueryEntryStatusFromBody_LegacyZlib_MSB(t *testing.T) {
	bitstring := make([]byte, 16)
	idx := uint32(5)
	byteIdx := idx / 8
	bitIdx := uint(7 - (idx % 8))
	bitstring[byteIdx] |= 1 << bitIdx

	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	_, err := w.Write(bitstring)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	encoded := base64.RawURLEncoding.EncodeToString(buf.Bytes())
	status, err := queryEntryStatusFromEncodedListWithPacking(encoded, idx, 1, bitPackingMSB)
	require.NoError(t, err)
	assert.Equal(t, "revoked", status)
}

func TestStatusListEntryBitPacking(t *testing.T) {
	assert.Equal(t, bitPackingLSB, statusListEntryBitPacking)
}

func TestQueryEntryStatusFromBody_XFSCGzip(t *testing.T) {
	bitstring := make([]byte, 125000)
	idx := uint32(42)

	body := makeXFSCListBody(bitstring)

	status, err := queryEntryStatusFromBody(body, idx)
	require.NoError(t, err)
	assert.Equal(t, "active", status)
}

func TestQueryEntryStatusFromBody_XFSCGzip_LSBRevoked(t *testing.T) {
	idx := uint32(38021)
	bitstring := make([]byte, 131072)
	byteIdx := idx / 8
	bitstring[byteIdx] |= 1 << (idx % 8) // XFSC: LSB-first within byte

	body := makeXFSCListBody(bitstring)

	status, err := queryEntryStatusFromBodyWithOptions(body, idx, 1, "revocation")
	require.NoError(t, err)
	assert.Equal(t, "revoked", status)
}

func TestBitSetAt_LSBvsMSB(t *testing.T) {
	data := []byte{0x20} // bit 5 set (LSB), not bit 2 (MSB for index 5)

	idx := uint32(5)
	lsb, err := bitSetAt(data, idx, true)
	require.NoError(t, err)
	assert.True(t, lsb)

	msb, err := bitSetAt(data, idx, false)
	require.NoError(t, err)
	assert.False(t, msb)
}

func TestTokenStatusListJWT_UsesLSBPacking(t *testing.T) {
	idx := uint32(1)
	bitstring := make([]byte, 16)
	bitstring[0] |= 1 << idx // IETF TSL uses least-significant-bit packing.
	token := makeJWTStatusListToken(t, bitstring, 1)

	err := verifyJWTStatusListBody([]byte(token), idx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "revoked")
}

func TestVerifyJWTStatusListBody_CryptoProviderBitsField(t *testing.T) {
	bitstring := make([]byte, 16384)
	idx := uint32(0)

	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.DefaultCompression)
	require.NoError(t, err)
	_, err = w.Write(bitstring)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"statuslist+jwt"}`))
	payload, err := json.Marshal(map[string]any{
		"iat": time.Now().Add(-time.Minute).Unix(),
		"exp": time.Now().Add(time.Hour).Unix(),
		"status_list": map[string]any{
			"bits":    base64.RawURLEncoding.EncodeToString(buf.Bytes()),
			"purpose": "revocation",
		},
	})
	require.NoError(t, err)
	payloadEnc := base64.RawURLEncoding.EncodeToString(payload)
	token := header + "." + payloadEnc + "."

	require.NoError(t, verifyJWTStatusListBody([]byte(token), idx))
}

func TestVerifyJWTStatusListBody_RejectsUnsignedWithoutSkip(t *testing.T) {
	ConfigureStatusListJWTVerification(nil, false)
	t.Cleanup(func() { ConfigureStatusListJWTVerification(nil, true) })

	token := makeJWTStatusListToken(t, make([]byte, 16), 1)
	err := verifyJWTStatusListBody([]byte(token), 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trust config")
}

func TestTokenStatusListJWT_MultiBitLSB(t *testing.T) {
	bitstring := []byte{0b00001000} // bits=2, index=1 => status value 2, bits 2..3
	status, err := queryTokenStatusFromEncodedList(compressZlibBase64URL(t, bitstring), 1, 2)
	require.NoError(t, err)
	assert.Equal(t, "revoked", status)
}

func compressZlibBase64URL(t *testing.T, bitstring []byte) string {
	t.Helper()
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	_, err := w.Write(bitstring)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return base64.RawURLEncoding.EncodeToString(buf.Bytes())
}

func TestValidStatusListURI(t *testing.T) {
	require.NoError(t, validStatusListURI("https://issuer.example/v1/tenants/acme/status/1"))
	require.NoError(t, validStatusListURI("http://localhost:30821/v1/tenants/default/status/1"))

	require.Error(t, validStatusListURI("file:///etc/passwd"))
	require.Error(t, validStatusListURI("not-a-url"))
	require.Error(t, validStatusListURI("https://user:pass@example.com/status"))
}

func TestParseStatusListIndex_Strict(t *testing.T) {
	idx, ok := parseStatusListIndex("123")
	require.True(t, ok)
	assert.Equal(t, uint32(123), idx)

	_, ok = parseStatusListIndex("123abc")
	assert.False(t, ok)
	_, ok = parseStatusListIndex(12.5)
	assert.False(t, ok)
	_, ok = parseStatusListIndex("4294967296")
	assert.False(t, ok)
}

func TestParseStatusListReference_Array(t *testing.T) {
	refs, err := parseStatusListReferences(map[string]any{
		"credentialStatus": []any{
			map[string]any{
				"type":                 statusKindBitstringStatusList,
				"statusPurpose":        "revocation",
				"statusListCredential": "https://issuer.example/status/1",
				"statusListIndex":      "42",
			},
			map[string]any{
				"type":                 statusKindBitstringStatusList,
				"statusPurpose":        "suspension",
				"statusListCredential": "https://issuer.example/status/2",
				"statusListIndex":      "43",
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, refs, 2)
	assert.Equal(t, "revocation", refs[0].purpose)
	assert.Equal(t, uint32(43), refs[1].index)
}

func TestParseStatusListReference_Single(t *testing.T) {
	ref, ok := parseStatusListReference(map[string]any{
		"credentialStatus": map[string]any{
			"type":                 statusKindStatusList2021,
			"statusListCredential": "https://issuer.example/status/1",
			"statusListIndex":      "7",
		},
	})
	require.True(t, ok)
	assert.Equal(t, statusKindStatusList2021, ref.kind)
	assert.Equal(t, uint32(7), ref.index)
}

func TestCheckStatusList_StatusList2021_Active(t *testing.T) {
	bitstring := make([]byte, 125000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(makeXFSCListBody(bitstring))
	}))
	defer srv.Close()

	claims, err := json.Marshal(map[string]any{
		"credentialStatus": map[string]any{
			"type":                 statusKindStatusList2021,
			"statusListCredential": srv.URL,
			"statusListIndex":      "62073",
		},
	})
	require.NoError(t, err)
	require.NoError(t, checkStatusList(claims))
}

func TestCheckStatusList_StatusList2021_Revoked(t *testing.T) {
	idx := uint32(3)
	bitstring := make([]byte, 16)
	byteIdx := idx / 8
	bitstring[byteIdx] |= 1 << (idx % 8) // XFSC body: LSB-first

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(makeXFSCListBody(bitstring))
	}))
	defer srv.Close()

	claims, err := json.Marshal(map[string]any{
		"credentialStatus": map[string]any{
			"type":                 statusKindStatusList2021,
			"statusListCredential": srv.URL,
			"statusListIndex":      "3",
		},
	})
	require.NoError(t, err)

	err = checkStatusList(claims)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "revoked")
}

func TestCheckStatusList_BitstringStatusList_XFSC_LSBRevoked(t *testing.T) {
	idx := uint32(38021)
	bitstring := make([]byte, 131072)
	bitstring[idx/8] |= 1 << (idx % 8)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(makeXFSCListBody(bitstring))
	}))
	defer srv.Close()

	claims, err := json.Marshal(map[string]any{
		"credentialStatus": map[string]any{
			"type":                 statusKindBitstringStatusList,
			"statusPurpose":        "revocation",
			"statusListCredential": srv.URL,
			"statusListIndex":      "38021",
		},
	})
	require.NoError(t, err)

	err = checkStatusList(claims)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "revoked")
}

func TestCheckStatusList_TokenStatusList_JWT(t *testing.T) {
	bitstring := make([]byte, 16)
	token := makeJWTStatusListToken(t, bitstring, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(token))
	}))
	defer srv.Close()

	claims, err := json.Marshal(map[string]any{
		"status": map[string]any{
			"status_list": map[string]any{
				"uri": srv.URL,
				"idx": 2,
			},
		},
	})
	require.NoError(t, err)
	require.NoError(t, checkStatusList(claims))
}

func TestCheckStatusList_FollowsCredentialURI(t *testing.T) {
	bitstring := make([]byte, 125000)
	externalURI := "https://external-issuer.example/v1/tenants/prod/status/9"

	claims, err := json.Marshal(map[string]any{
		"credentialStatus": map[string]any{
			"type":                 statusKindStatusList2021,
			"statusListCredential": externalURI,
			"statusListIndex":      "1",
		},
	})
	require.NoError(t, err)

	origTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = origTransport })
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, externalURI, req.URL.String())
		rec := httptest.NewRecorder()
		rec.WriteHeader(http.StatusOK)
		_, _ = rec.Write(makeXFSCListBody(bitstring))
		return rec.Result(), nil
	})

	require.NoError(t, checkStatusList(claims))
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
