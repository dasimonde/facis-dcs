package tsa

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/digitorus/timestamp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIClient_Timestamp_Success(t *testing.T) {
	type input struct {
		Field string `json:"field"`
	}
	testData := input{Field: "test-value"}

	jsonData, err := json.Marshal(testData)
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "text/plain", r.Header.Get("Content-Type"))
		assert.Equal(t, "application/timestamp-reply", r.Header.Get("Accept"))
		expectedHash := sha256.Sum256(jsonData)
		assert.Equal(t, hex.EncodeToString(expectedHash[:]), strings.TrimPrefix(r.URL.Path, "/"))

		cert, key := mustTSACert(t)
		ts := &timestamp.Timestamp{
			HashAlgorithm: crypto.SHA256,
			HashedMessage: expectedHash[:],
			Time:          time.Now().UTC(),
			SerialNumber:  big.NewInt(1),
			Policy:        asn1.ObjectIdentifier{1, 2, 3, 4, 5},
		}
		resp, err := ts.CreateResponseWithOpts(cert, key, crypto.SHA256)
		require.NoError(t, err)

		w.Header().Set("Content-Type", "application/timestamp-reply")
		_, _ = w.Write(resp)
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL)
	require.NoError(t, err)

	result, err := client.Timestamp(context.Background(), testData)
	require.NoError(t, err)
	token, err := base64.StdEncoding.DecodeString(result)
	require.NoError(t, err)
	ts, err := timestamp.Parse(token)
	require.NoError(t, err)
	assert.NotEmpty(t, ts.HashedMessage)
	receipt, err := client.TimestampBytes(context.Background(), jsonData)
	require.NoError(t, err)
	assert.Equal(t, "base64", receipt.TokenEncoding)
	assert.NotEmpty(t, receipt.MessageImprint)
}

func TestAPIClient_Timestamp_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL + "/")
	require.NoError(t, err)

	_, err = client.Timestamp(context.Background(), map[string]string{"key": "value"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected TSA status")
}

func TestAPIClient_Timestamp_NetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	client, err := NewClient(srv.URL)
	require.NoError(t, err)

	_, err = client.Timestamp(context.Background(), map[string]string{"key": "value"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "call TSA endpoint")
}

func TestVerify_CertRejectedForNonFreeTSAKey(t *testing.T) {
	// Hash passes but cert check fails — full success requires a real FreeTSA TSR
	// (see TestVerify_FreeTSA_Integration).
	cert, key := mustTSACert(t)
	data := map[string]string{"field": "test-value"}
	tsr := makeTSR(t, cert, key, jsonHash(t, data))

	ok, err := Verify(tsr, data)
	require.Error(t, err)
	assert.False(t, ok)
	assert.Contains(t, err.Error(), "TSA certificate verification")
}

func TestAPIClient_VerifyLoadsTrustCertificateAfterPackageInitialization(t *testing.T) {
	cert, key := mustTSACert(t)
	certPath := t.TempDir() + "/tsa-cert.pem"
	require.NoError(t, os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	}), 0o600))
	t.Setenv("TSA_TRUST_CERT_FILE", certPath)

	client, err := NewClient("")
	require.NoError(t, err)
	data := map[string]string{"field": "loaded-after-package-init"}
	tsr := makeTSR(t, cert, key, jsonHash(t, data))

	ok, err := client.Verify(tsr, data)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestVerify_HashMismatch(t *testing.T) {
	cert, key := mustTSACert(t)
	original := map[string]string{"field": "original"}
	tsr := makeTSR(t, cert, key, jsonHash(t, original))

	ok, err := Verify(tsr, map[string]string{"field": "different"})
	require.Error(t, err)
	assert.False(t, ok)
	assert.Contains(t, err.Error(), "hash mismatch")
}

func TestVerify_InvalidBase64(t *testing.T) {
	ok, err := Verify("!!!not-base64!!!", map[string]string{"x": "y"})
	require.Error(t, err)
	assert.False(t, ok)
	assert.Contains(t, err.Error(), "decode TSR")
}

// jsonHash marshals v to JSON and returns its SHA-256 hash, mirroring what Verify does internally.
func jsonHash(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	h := sha256.Sum256(b)
	return h[:]
}

func makeTSR(t *testing.T, cert *x509.Certificate, key *ecdsa.PrivateKey, dataHash []byte) string {
	t.Helper()
	ts := &timestamp.Timestamp{
		HashAlgorithm:     crypto.SHA256,
		HashedMessage:     dataHash,
		Time:              time.Now().UTC(),
		SerialNumber:      big.NewInt(1),
		Policy:            asn1.ObjectIdentifier{1, 2, 3, 4, 5},
		AddTSACertificate: true,
	}
	resp, err := ts.CreateResponseWithOpts(cert, key, crypto.SHA256)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(resp)
}

// TestFreeTSACertFingerprint ensures the pinned cert still matches the known
// SHA-256 fingerprint. If FreeTSA rotates their cert, this test fails and
// signals that certs/tsa.crt must be updated.
func TestFreeTSACertFingerprint(t *testing.T) {
	const wantFingerprint = "a6379e7cecc05faa3cbf076013d745e327bbbaa38c0b9af22469d4701d18aabc"

	pemData, err := os.ReadFile("certs/tsa.crt")
	require.NoError(t, err)

	cert := mustParsePEM(t, pemData)
	got := sha256.Sum256(cert.Raw)
	assert.Equal(t, wantFingerprint, hex.EncodeToString(got[:]))
}

// TestVerify_FreeTSA_Integration calls the real FreeTSA API, receives a live TSR,
// and verifies it against the pinned TSA certificate.
// Run with: go test -run TestVerify_FreeTSA_Integration -tags integration
func TestVerify_FreeTSA_Integration(t *testing.T) {
	if os.Getenv("INTEGRATION") == "" {
		t.Skip("set INTEGRATION=1 to run live FreeTSA test")
	}

	testData := map[string]string{"field": "integration-test"}
	jsonBytes, err := json.Marshal(testData)
	require.NoError(t, err)
	hash := sha256.Sum256(jsonBytes)

	tsReq := &timestamp.Request{
		HashAlgorithm: crypto.SHA256,
		HashedMessage: hash[:],
		Certificates:  true,
	}
	reqBytes, err := tsReq.Marshal()
	require.NoError(t, err)

	resp, err := http.Post("https://freetsa.org/tsr", "application/timestamp-query", bytes.NewReader(reqBytes))
	require.NoError(t, err)
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("could not close response body: %v", err)
		}
	}()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	tsrBase64 := base64.StdEncoding.EncodeToString(body)

	ok, err := Verify(tsrBase64, testData)
	require.NoError(t, err)
	assert.True(t, ok)
}

func mustParsePEM(t *testing.T, pemData []byte) *x509.Certificate {
	t.Helper()
	// strip PEM header/footer and decode
	block := pemData
	const header = "-----BEGIN CERTIFICATE-----\n"
	const footer = "\n-----END CERTIFICATE-----\n"
	block = bytes.TrimPrefix(block, []byte(header))
	block = bytes.TrimSuffix(block, []byte(footer))
	der, err := base64.StdEncoding.DecodeString(
		string(bytes.ReplaceAll(block, []byte("\n"), nil)),
	)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return cert
}

func mustTSACert(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	now := time.Now().UTC()
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(42),
		Subject:               pkix.Name{CommonName: "test-tsa"},
		NotBefore:             now.Add(-1 * time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return cert, key
}
