// Package tsa provides a client for obtaining RFC 3161 timestamps and a
// verifier that checks timestamp responses (TSRs) against a trusted TSA
// certificate.
//
// This package does not communicate with an external TSA directly. All
// timestamp requests are forwarded to ORCE, which handles the actual RFC 3161
// flow with the upstream TSA provider. This package only verifies the returned
// TSR using the embedded CA certificate.
//
// # Switching TSA providers
//
// To use a different TSA, two changes are required:
//
//  1. Update the TSA flow in ORCE to point to the new provider.
//
//  2. Replace certs/tsa.crt with the CA certificate (PEM) of the new provider.
//     The file is embedded at compile time (see [tsaCertPEM]), so a rebuild of
//     this service is required after the certificate is replaced.
package tsa

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/digitorus/pkcs7"
	"github.com/digitorus/timestamp"
)

//go:embed certs/tsa.crt
var tsaCertPEM []byte

var embeddedTSACert = func() *x509.Certificate {
	block, _ := pem.Decode(tsaCertPEM)
	if block == nil {
		panic("tsa: failed to decode certs/tsa.crt")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		panic("tsa: failed to parse certs/tsa.crt: " + err.Error())
	}
	return cert
}()

// loadTrustedTSACertificate resolves the trust anchor after application
// configuration has been loaded. In local development main loads .env after
// Go package initialization, so reading TSA_TRUST_CERT_FILE in a package-level
// initializer would silently ignore the configured ORCE certificate.
func loadTrustedTSACertificate() (*x509.Certificate, error) {
	path := strings.TrimSpace(os.Getenv("TSA_TRUST_CERT_FILE"))
	if path == "" {
		return embeddedTSACert, nil
	}
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("TSA_TRUST_CERT_FILE %s is unreadable: %w", path, err)
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("TSA_TRUST_CERT_FILE does not contain a PEM certificate: %s", path)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse TSA_TRUST_CERT_FILE %s: %w", path, err)
	}
	return cert, nil
}

// APIClient sends timestamp requests to an RFC 3161 TSA HTTP endpoint.
type APIClient struct {
	// url is the base URL of the TSA endpoint. The hex-encoded hash of the
	// payload is appended to this URL for each request.
	url         string
	client      *http.Client
	trustedCert *x509.Certificate
}

type Receipt struct {
	Token          string    `json:"token"`
	TokenEncoding  string    `json:"token_encoding"`
	HashAlgorithm  string    `json:"hash_algorithm"`
	MessageImprint string    `json:"message_imprint"`
	GeneratedAt    time.Time `json:"generated_at"`
	Policy         string    `json:"policy,omitempty"`
	SerialNumber   string    `json:"serial_number,omitempty"`
}

// NewClient creates a new [APIClient] for the given TSA endpoint URL.
// url must end with a path separator so that the hash can be appended directly
func NewClient(url string) (*APIClient, error) {
	trustedCert, err := loadTrustedTSACertificate()
	if err != nil {
		return nil, fmt.Errorf("load TSA trust certificate: %w", err)
	}
	return &APIClient{
		url: strings.TrimSpace(url),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		trustedCert: trustedCert,
	}, nil
}

func (c *APIClient) Enabled() bool {
	return c != nil && c.url != ""
}

// Timestamp JSON-marshals data, computes its SHA-256 hash, and requests a
// timestamp token from the TSA. The returned string is the raw TSR
// (timestamp response) encoded in base64. Store this value alongside the
// original data and pass it to [Verify] later to prove the data existed at
// the returned point in time.
func (c *APIClient) Timestamp(ctx context.Context, data any) (string, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("marshal data: %w", err)
	}

	receipt, err := c.TimestampBytes(ctx, jsonData)
	if err != nil {
		return "", err
	}
	return receipt.Token, nil
}

func (c *APIClient) TimestampBytes(ctx context.Context, data []byte) (*Receipt, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("TSA URL is empty")
	}
	token, ts, err := requestTimestampToken(ctx, c.client, c.url, data)
	if err != nil {
		return nil, err
	}
	return receiptFromTimestamp(token, ts), nil
}

func VerifyReceipt(receipt Receipt, data []byte) (*timestamp.Timestamp, error) {
	if strings.TrimSpace(receipt.Token) == "" {
		return nil, fmt.Errorf("TSA token is empty")
	}
	if receipt.TokenEncoding != "" && receipt.TokenEncoding != "base64" {
		return nil, fmt.Errorf("unsupported TSA token encoding %q", receipt.TokenEncoding)
	}
	token, err := base64.StdEncoding.DecodeString(receipt.Token)
	if err != nil {
		return nil, fmt.Errorf("decode TSA token: %w", err)
	}
	ts, err := timestamp.Parse(token)
	if err != nil {
		return nil, fmt.Errorf("parse TSA token: %w", err)
	}
	if err := verifyTimestampForData(ts, data); err != nil {
		return nil, err
	}
	if receipt.MessageImprint != "" && receipt.MessageImprint != hex.EncodeToString(ts.HashedMessage) {
		return nil, fmt.Errorf("TSA receipt message imprint mismatch")
	}
	return ts, nil
}

// tsaRequestAttempts bounds the retries for one timestamp request. The
// upstream TSA (reached via ORCE) is an external service that intermittently
// drops or throttles requests; the request is an idempotent hash-keyed GET,
// so a short bounded retry absorbs transient failures without masking a real
// outage — after the last attempt the error still propagates.
const tsaRequestAttempts = 3

func requestTimestampToken(ctx context.Context, httpClient *http.Client, tsaURL string, data []byte) ([]byte, *timestamp.Timestamp, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	hash := sha256.Sum256(data)
	hashString := hex.EncodeToString(hash[:])

	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}

	body, err := fetchTimestampResponse(ctx, httpClient, timestampURL(tsaURL, hashString))
	if err != nil {
		return nil, nil, err
	}

	ts, err := timestamp.Parse(body)
	if err != nil {
		tsResp, respErr := timestamp.ParseResponse(body)
		if respErr != nil {
			return nil, nil, fmt.Errorf("parse TSA token: %w", err)
		}
		ts = tsResp
	}
	if err := verifyTimestampForData(ts, data); err != nil {
		return nil, nil, err
	}
	if len(ts.RawToken) == 0 {
		return nil, nil, fmt.Errorf("TSA response token is empty")
	}

	return ts.RawToken, ts, nil
}

func fetchTimestampResponse(ctx context.Context, httpClient *http.Client, url string) ([]byte, error) {
	var lastErr error
	for attempt := 1; attempt <= tsaRequestAttempts; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("call TSA endpoint: %w", ctx.Err())
			case <-time.After(time.Duration(attempt-1) * 2 * time.Second):
			}
		}
		body, retryable, err := doTimestampRequest(ctx, httpClient, url)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retryable || ctx.Err() != nil {
			break
		}
	}
	return nil, lastErr
}

func doTimestampRequest(ctx context.Context, httpClient *http.Client, url string) (body []byte, retryable bool, err error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, fmt.Errorf("create TSA HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "text/plain")
	httpReq.Header.Set("Accept", "application/timestamp-reply")

	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, true, fmt.Errorf("call TSA endpoint: %w", err)
	}
	defer func(Body io.ReadCloser) {
		if err := Body.Close(); err != nil {
			log.Println("could not close body")
		}
	}(httpResp.Body)

	body, err = io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, true, fmt.Errorf("read TSA response: %w", err)
	}
	if httpResp.StatusCode != http.StatusOK {
		return nil, httpResp.StatusCode >= http.StatusInternalServerError,
			fmt.Errorf("unexpected TSA status %d: %s", httpResp.StatusCode, string(body))
	}
	return body, false, nil
}

func timestampURL(baseURL, hash string) string {
	baseURL = strings.TrimSpace(baseURL)
	if strings.HasSuffix(baseURL, "/") {
		return baseURL + hash
	}
	return baseURL + "/" + hash
}

func verifyTimestampForData(ts *timestamp.Timestamp, data []byte) error {
	if ts == nil {
		return fmt.Errorf("TSA timestamp is empty")
	}
	expectedHash := sha256.Sum256(data)
	if !bytes.Equal(ts.HashedMessage, expectedHash[:]) {
		return fmt.Errorf("TSA hashed message mismatch")
	}
	return nil
}

func receiptFromTimestamp(token []byte, ts *timestamp.Timestamp) *Receipt {
	receipt := &Receipt{
		Token:          base64.StdEncoding.EncodeToString(token),
		TokenEncoding:  "base64",
		HashAlgorithm:  "SHA-256",
		MessageImprint: hex.EncodeToString(ts.HashedMessage),
		GeneratedAt:    ts.Time.UTC(),
	}
	if len(ts.Policy) > 0 {
		receipt.Policy = ts.Policy.String()
	}
	if ts.SerialNumber != nil {
		receipt.SerialNumber = ts.SerialNumber.String()
	}
	return receipt
}

// Verify checks that a base64-encoded TSR covers data and was signed by the
// embedded TSA certificate (certs/tsa.crt). It JSON-marshals data, computes
// its SHA-256 hash, and compares it against the hash inside the TSR.
// Returns (true, nil) on success, (false, err) on any failure.
func Verify(tsrBase64 string, data any) (bool, error) {
	trustedCert, err := loadTrustedTSACertificate()
	if err != nil {
		return false, fmt.Errorf("load TSA trust certificate: %w", err)
	}
	return verifyWithCertificate(tsrBase64, data, trustedCert)
}

// Verify checks a timestamp using the trust anchor captured when the client
// was created, after the application's environment configuration was loaded.
func (c *APIClient) Verify(tsrBase64 string, data any) (bool, error) {
	if c == nil || c.trustedCert == nil {
		return false, fmt.Errorf("TSA client has no trust certificate")
	}
	return verifyWithCertificate(tsrBase64, data, c.trustedCert)
}

func verifyWithCertificate(tsrBase64 string, data any, trustedCert *x509.Certificate) (bool, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return false, fmt.Errorf("marshal data: %w", err)
	}
	// Recompute the same hash that was sent to the TSA during Timestamp().
	hash := sha256.Sum256(jsonData)

	rawBytes, err := base64.StdEncoding.DecodeString(tsrBase64)
	if err != nil {
		return false, fmt.Errorf("decode TSR: %w", err)
	}

	// Try parsing as a full HTTP timestamp response first; fall back to the
	// bare DER-encoded TimeStampToken format used by some providers.
	var ts *timestamp.Timestamp
	ts, err = timestamp.ParseResponse(rawBytes)
	if err != nil {
		ts, err = timestamp.Parse(rawBytes)
		if err != nil {
			return false, fmt.Errorf("parse TSR: %w", err)
		}
	}

	// Ensure the TSR was issued for exactly the data we supplied.
	if !bytes.Equal(ts.HashedMessage, hash[:]) {
		return false, fmt.Errorf("TSR hash mismatch")
	}

	// Verify the TSA's cryptographic signature against the trusted TSA
	// certificate (embedded default or TSA_TRUST_CERT_FILE override).
	pool := x509.NewCertPool()
	pool.AddCert(trustedCert)
	p7, err := pkcs7.Parse(ts.RawToken)
	if err != nil {
		return false, fmt.Errorf("parse TSR token: %w", err)
	}
	if err := p7.VerifyWithChain(pool); err != nil {
		return false, fmt.Errorf("TSA certificate verification: %w", err)
	}

	return true, nil
}
