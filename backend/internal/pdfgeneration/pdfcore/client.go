package pdfcore

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	neturl "net/url"
	"strings"
	"time"
)

// RendererVersion is kept in sync with pdf-core/compiler/version.go.
// Bump both together when the pdf-core renderer produces different output for
// the same JSON-LD input, so that cached PDFs are invalidated.
const RendererVersion = "1.0.1"

// C2PASignFunc signs one COSE Sig_structure with the DCS dcs-c2pa key and returns
// the 64-byte ES256 r||s. pdf-core holds no key: it prepares the to-be-signed
// Sig_structures, the DCS signs them here, and pdf-core embeds the signatures.
type C2PASignFunc func(sigStructure []byte) ([]byte, error)

// Client is an HTTP client for the pdf-core microservice.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	// sign produces the dcs-c2pa signature for a prepared Sig_structure. The DCS
	// never lets pdf-core see key material, so every C2PA signature is produced
	// here and posted back to pdf-core's /c2pa/embed.
	sign C2PASignFunc
	// authority is this instance's DID, recorded as the asserting party in every
	// C2PA lifecycle assertion pdf-core writes for this client's renders. It is
	// sent per request because pdf-core is stateless and may be shared.
	authority string
	// x5chainPEM is the certificate chain naming the dcs-c2pa key `sign` uses,
	// leaf first. It travels with every request that renders a manifest, for the
	// same reason the key never leaves this process: pdf-core is shared, and a
	// chain it kept of its own would sign this instance's documents under
	// whichever instance configured it.
	x5chainPEM []byte
}

// lifecycleAuthorityHeader carries the asserting instance's DID to pdf-core.
const lifecycleAuthorityHeader = "X-DCS-Lifecycle-Authority"

// signingChainHeader carries this instance's C2PA x5chain to pdf-core as
// base64-encoded PEM. pdf-core holds no signing material and refuses a render
// that names none.
const signingChainHeader = "X-DCS-C2PA-X5Chain"

// New returns a Client pointed at baseURL. sign is the in-process dcs-c2pa
// signer the two-step render flow uses; it must be non-nil.
func New(baseURL string, sign C2PASignFunc) *Client {
	return NewWithAuthority(baseURL, sign, "")
}

// NewWithAuthority returns a Client that names issuerDID as the party asserting
// the lifecycle events of every render it requests.
func NewWithAuthority(baseURL string, sign C2PASignFunc, issuerDID string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
		sign:       sign,
		authority:  strings.TrimSpace(issuerDID),
	}
}

// WithSigningChain names the PEM certificate chain this client's renders are
// signed under — the chain issued for the same dcs-c2pa key `sign` uses.
func (c *Client) WithSigningChain(chainPEM []byte) *Client {
	c.x5chainPEM = append([]byte(nil), chainPEM...)
	return c
}

// setLifecycleAuthority tags a render request with this instance's DID, leaving
// the header off entirely when none is configured.
func (c *Client) setLifecycleAuthority(req *http.Request) {
	if c.authority != "" {
		req.Header.Set(lifecycleAuthorityHeader, c.authority)
	}
}

// setSigningChain names the identity this request signs under. An unset chain
// leaves the header off, which pdf-core refuses — a render that named no signer
// must fail at the boundary rather than proceed under an assumed one.
func (c *Client) setSigningChain(req *http.Request) {
	if len(c.x5chainPEM) > 0 {
		req.Header.Set(signingChainHeader, base64.StdEncoding.EncodeToString(c.x5chainPEM))
	}
}

// preparedC2PA is pdf-core's prepare response: the compiled PDF with zeroed COSE
// signature slots and the Sig_structures the DCS must sign (document order).
type preparedC2PA struct {
	PDFBase64         string   `json:"pdf_base64"`
	C2PASigStructures []string `json:"c2pa_sig_structures"`
}

// embedC2PA is pdf-core's /c2pa/embed request: the prepared PDF and the ES256
// signatures for its zeroed slots, in the order prepare returned them.
type embedC2PA struct {
	PDFBase64      string   `json:"pdf_base64"`
	C2PASignatures []string `json:"c2pa_signatures"`
}

// signAndEmbed signs each prepared Sig_structure with the dcs-c2pa key and posts
// the signatures to pdf-core's stateless /c2pa/embed, returning the finished PDF.
func (c *Client) signAndEmbed(ctx context.Context, prepared preparedC2PA) ([]byte, error) {
	if c.sign == nil {
		return nil, fmt.Errorf("pdf-core client has no C2PA signer configured")
	}
	signatures := make([]string, len(prepared.C2PASigStructures))
	for i, s := range prepared.C2PASigStructures {
		sigStructure, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("decode sig_structure %d: %w", i, err)
		}
		sig, err := c.sign(sigStructure)
		if err != nil {
			return nil, fmt.Errorf("sign c2pa sig_structure %d: %w", i, err)
		}
		signatures[i] = base64.StdEncoding.EncodeToString(sig)
	}
	body, err := json.Marshal(embedC2PA{PDFBase64: prepared.PDFBase64, C2PASignatures: signatures})
	if err != nil {
		return nil, fmt.Errorf("marshal c2pa embed request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/c2pa/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("pdf-core c2pa embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pdf-core c2pa embed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	pdf, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("pdf-core c2pa embed read: %w", err)
	}
	return pdf, nil
}

// Version fetches the renderer version string from pdf-core's GET /version.
func (c *Client) Version(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/version", nil)
	if err != nil {
		return "", fmt.Errorf("pdf-core version request: %w", err)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("pdf-core version: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkStatus(resp); err != nil {
		return "", err
	}
	var body struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("pdf-core version decode: %w", err)
	}
	return body.Version, nil
}

// Download posts jsonld to POST /render and returns the resulting PDF bytes
// plus the renderer version from the X-PDF-Core-Version response header.
func (c *Client) Download(ctx context.Context, jsonld []byte) (pdf []byte, version string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/render", bytes.NewReader(jsonld))
	if err != nil {
		return nil, "", fmt.Errorf("pdf-core download request: %w", err)
	}
	req.Header.Set("Content-Type", "application/ld+json")
	c.setLifecycleAuthority(req)
	c.setSigningChain(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("pdf-core download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkStatus(resp); err != nil {
		return nil, "", err
	}
	version = resp.Header.Get("X-PDF-Core-Version")
	var prepared preparedC2PA
	if err := json.NewDecoder(resp.Body).Decode(&prepared); err != nil {
		return nil, "", fmt.Errorf("pdf-core download decode prepared: %w", err)
	}
	pdf, err = c.signAndEmbed(ctx, prepared)
	if err != nil {
		return nil, "", fmt.Errorf("pdf-core download: %w", err)
	}
	return pdf, version, nil
}

// Update posts a multipart request to POST /render/amendment containing existingPDF as
// "pdf", jsonld as "payload", and optionally vcBytes as "vc". When vcBytes is
// non-nil the request proceeds even if the JSON-LD payload is unchanged.
// When manifestURL is non-empty it is sent as the "manifest_url" field so
// pdf-core embeds it as the C2PA claim's remote_manifests field
// (DCS-OR-C2PA-008). Returns the updated PDF bytes and the renderer version
// header.
func (c *Client) Update(ctx context.Context, existingPDF, jsonld, vcBytes []byte, manifestURL string) (pdf []byte, version string, err error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	if err := writeField(mw, "pdf", existingPDF); err != nil {
		return nil, "", fmt.Errorf("pdf-core update: write pdf field: %w", err)
	}
	if err := writeField(mw, "payload", jsonld); err != nil {
		return nil, "", fmt.Errorf("pdf-core update: write payload field: %w", err)
	}
	if len(vcBytes) > 0 {
		if err := writeField(mw, "vc", vcBytes); err != nil {
			return nil, "", fmt.Errorf("pdf-core update: write vc field: %w", err)
		}
	}
	if manifestURL != "" {
		if err := writeField(mw, "manifest_url", []byte(manifestURL)); err != nil {
			return nil, "", fmt.Errorf("pdf-core update: write manifest_url field: %w", err)
		}
	}
	if err := mw.Close(); err != nil {
		return nil, "", fmt.Errorf("pdf-core update: close multipart: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/render/amendment", &buf)
	if err != nil {
		return nil, "", fmt.Errorf("pdf-core update request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	c.setLifecycleAuthority(req)
	c.setSigningChain(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("pdf-core update: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkStatus(resp); err != nil {
		return nil, "", err
	}
	version = resp.Header.Get("X-PDF-Core-Version")
	var prepared preparedC2PA
	if err := json.NewDecoder(resp.Body).Decode(&prepared); err != nil {
		return nil, "", fmt.Errorf("pdf-core update decode prepared: %w", err)
	}
	pdf, err = c.signAndEmbed(ctx, prepared)
	if err != nil {
		return nil, "", fmt.Errorf("pdf-core update: %w", err)
	}
	return pdf, version, nil
}

// Reanchor appends a provenance-only C2PA manifest binding the signed PDF's
// current bytes (ADR-26), returning the re-anchored PDF. It changes no payload:
// the signature is applied after the lifecycle manifest so that it commits to
// the provenance, and this restores a whole-file binding over the result
// without touching the signature's byte range.
func (c *Client) Reanchor(ctx context.Context, pdf []byte, manifestURL string) ([]byte, error) {
	url := c.BaseURL + "/render/reanchor"
	if manifestURL != "" {
		url += "?manifest_url=" + neturl.QueryEscape(manifestURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(pdf))
	if err != nil {
		return nil, fmt.Errorf("pdf-core reanchor request: %w", err)
	}
	req.Header.Set("Content-Type", "application/pdf")
	c.setLifecycleAuthority(req)
	c.setSigningChain(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pdf-core reanchor: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	var prepared preparedC2PA
	if err := json.NewDecoder(resp.Body).Decode(&prepared); err != nil {
		return nil, fmt.Errorf("pdf-core reanchor decode prepared: %w", err)
	}
	return c.signAndEmbed(ctx, prepared)
}

// EmbedEvidence posts pdf + evidence to POST /evidence/embed and returns the
// PDF with the evidence attached but NOT signed — the attach-only step before
// an external PAdES signer (wallet/QTSP/DSS) produces the signature, so the
// signature's /ByteRange covers the evidence (embed-first-sign-second,
// DCS-FR-SM-08). pdf-core holds no key and never signs.
//
// Every call appends one more attachment under its own filename, including on a
// PDF that already carries a signature: the append is an incremental update, so
// the signatures already there stay valid (DCS-OR-C2PA-002).
func (c *Client) EmbedEvidence(ctx context.Context, pdf, evidence []byte) (embedded []byte, err error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := writeField(mw, "pdf", pdf); err != nil {
		return nil, fmt.Errorf("pdf-core embed: write pdf field: %w", err)
	}
	if err := writeField(mw, "evidence", evidence); err != nil {
		return nil, fmt.Errorf("pdf-core embed: write evidence field: %w", err)
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("pdf-core embed: close multipart: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/evidence/embed", &buf)
	if err != nil {
		return nil, fmt.Errorf("pdf-core embed request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pdf-core embed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	embedded, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("pdf-core embed read: %w", err)
	}
	return embedded, nil
}

// ExtractEvidence posts pdf to POST /evidence/extract and returns EVERY signing
// evidence attachment the PDF carries, oldest first — one per signing event, so
// a countersigned contract yields both parties' evidence. A PDF carrying none
// yields an empty slice.
func (c *Client) ExtractEvidence(ctx context.Context, pdf []byte) ([]json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/evidence/extract", bytes.NewReader(pdf))
	if err != nil {
		return nil, fmt.Errorf("pdf-core extract-evidence request: %w", err)
	}
	req.Header.Set("Content-Type", "application/pdf")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pdf-core extract-evidence: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("pdf-core extract-evidence read: %w", err)
	}
	var attachments []json.RawMessage
	if err := json.Unmarshal(body, &attachments); err != nil {
		return nil, fmt.Errorf("pdf-core extract-evidence decode: %w", err)
	}
	return attachments, nil
}

// VerifyResult is the structured response from pdf-core POST /verify.
type VerifyResult struct {
	// Match is true when the PDF was deterministically produced from its embedded payload.
	Match bool
	// C2PASignatureValid is the outcome of pdf-core's COSE claim-signature check:
	// every manifest's claim signature verified against its own x5chain leaf and
	// the assertions the signed claim commits to still hash to the recorded
	// values. C2PASignatureError carries the reason when it did not.
	C2PASignatureValid bool
	C2PASignatureError string
	// VCBytes are the raw contract-lifecycle-vc.json bytes from the PDF attachment,
	// present only when the PDF contains that attachment.
	VCBytes []byte
	// VCPresent says the PDF carries a lifecycle-credential attachment — all
	// pdf-core can say about it, since it holds no key material and resolves no
	// issuer. Whether the credential's proof VERIFIES is decided by the caller,
	// against the issuer's published assertion key (provenance.CredentialVerifier).
	VCPresent bool
	// JSONLDHash, BasePDFHash and StoredBasePDFHash are the SHA-256 digests (hex)
	// pdf-core reached its Match verdict on: the machine-readable payload embedded
	// in the PDF, the deterministic re-render produced from it, and the stored
	// bytes that re-render was compared against. The two PDF digests are equal
	// exactly when the document reproduces, so on a mismatch they name which side
	// diverged. Both are taken over pdf-core's COSE-zeroed normalization — the one
	// the comparison itself uses, since a fresh compile carries a fresh randomized
	// claim signature.
	//
	// pdf-core reports them on a 409 content mismatch too, so they are populated
	// alongside the returned error; they are empty only when it could not compute
	// them at all.
	JSONLDHash        string
	BasePDFHash       string
	StoredBasePDFHash string
}

// verifyBodyLimit caps the /verify response read. The body carries a
// base64-encoded witness PDF alongside the credential bytes, so it is sized for a
// document rather than for a status line.
const verifyBodyLimit = 64 << 20

// Verify posts pdf to POST /verify and returns the structured verification result.
// Returns an error on non-2xx (including 409 content-mismatch); on a mismatch the
// returned result still carries the digests pdf-core refused on.
func (c *Client) Verify(ctx context.Context, pdf []byte) (VerifyResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/verify", bytes.NewReader(pdf))
	if err != nil {
		return VerifyResult{}, fmt.Errorf("pdf-core verify request: %w", err)
	}
	req.Header.Set("Content-Type", "application/pdf")
	// The chain this instance signs under: pdf-core witnesses the result under it
	// and reports, per manifest, whether it is what signed the artifact.
	c.setSigningChain(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return VerifyResult{}, fmt.Errorf("pdf-core verify: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, verifyBodyLimit))
	if err != nil {
		return VerifyResult{}, fmt.Errorf("pdf-core verify read: %w", err)
	}

	var body struct {
		Match              bool   `json:"match"`
		C2PASignatureValid bool   `json:"c2pa_signature_valid"`
		C2PASignatureError string `json:"c2pa_signature_error"`
		VCBytes            string `json:"vc_bytes"`
		VCPresent          bool   `json:"vc_present"`
		JSONLDHash         string `json:"jsonld_hash"`
		BasePDFHash        string `json:"base_pdf_hash"`
		StoredBasePDFHash  string `json:"stored_base_pdf_hash"`
	}
	// A non-2xx body is pdf-core's Error schema carrying those same digest fields,
	// so it is decoded before the status is turned into an error. What decodes is
	// kept, what does not is absent, and the status stays the verdict either way.
	decodeErr := json.Unmarshal(raw, &body)

	result := VerifyResult{
		JSONLDHash:        body.JSONLDHash,
		BasePDFHash:       body.BasePDFHash,
		StoredBasePDFHash: body.StoredBasePDFHash,
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return result, fmt.Errorf("pdf-core %s: status %d: %s",
			resp.Request.URL.Path, resp.StatusCode, truncate(strings.TrimSpace(string(raw)), 512))
	}
	if decodeErr != nil {
		return VerifyResult{}, fmt.Errorf("pdf-core verify decode: %w", decodeErr)
	}

	result.Match = body.Match
	result.C2PASignatureValid = body.C2PASignatureValid
	result.C2PASignatureError = body.C2PASignatureError
	result.VCPresent = body.VCPresent
	if body.VCBytes != "" {
		decoded, err := base64.StdEncoding.DecodeString(body.VCBytes)
		if err != nil {
			return VerifyResult{}, fmt.Errorf("pdf-core verify: decode vc_bytes: %w", err)
		}
		result.VCBytes = decoded
	}
	return result, nil
}

// VerifyContent posts pdf to POST /verify/content and reports whether the PDF's
// human-readable page content is the deterministic re-render of its own embedded
// machine-readable payload. Unlike Verify (byte-prefix reproduction), this is
// tolerant of the C2PA, signature and amendment layers a peer may have appended,
// so a legitimately amended offered PDF still matches while tampered page content
// does not. On a mismatch it also returns a diagnostic detail (which page diverged
// and a snippet of both renders) so the caller can log WHERE they diverge. Returns
// an error on non-2xx.
func (c *Client) VerifyContent(ctx context.Context, pdf []byte) (bool, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/verify/content", bytes.NewReader(pdf))
	if err != nil {
		return false, "", fmt.Errorf("pdf-core verify/content request: %w", err)
	}
	req.Header.Set("Content-Type", "application/pdf")
	c.setSigningChain(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return false, "", fmt.Errorf("pdf-core verify/content: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkStatus(resp); err != nil {
		return false, "", err
	}
	var body struct {
		Match    bool   `json:"match"`
		Mismatch string `json:"mismatch"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return false, "", fmt.Errorf("pdf-core verify/content: decode: %w", err)
	}
	return body.Match, body.Mismatch, nil
}

// MatchContent posts submitted and reference to POST /verify/content-match and
// reports whether the submitted PDF's visible page content is still the
// reference PDF's, resolving the last definition of every object on both sides.
// Nothing is re-rendered, so the answer does not depend on render determinism:
// the reference is a document the caller already holds. On a mismatch it returns
// a diagnostic naming the page that diverged and a snippet of both sides.
func (c *Client) MatchContent(ctx context.Context, submitted, reference []byte) (bool, string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := writeField(mw, "pdf", submitted); err != nil {
		return false, "", fmt.Errorf("pdf-core verify/content-match: write pdf field: %w", err)
	}
	if err := writeField(mw, "reference", reference); err != nil {
		return false, "", fmt.Errorf("pdf-core verify/content-match: write reference field: %w", err)
	}
	if err := mw.Close(); err != nil {
		return false, "", fmt.Errorf("pdf-core verify/content-match: close multipart: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/verify/content-match", &buf)
	if err != nil {
		return false, "", fmt.Errorf("pdf-core verify/content-match request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	c.setSigningChain(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return false, "", fmt.Errorf("pdf-core verify/content-match: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkStatus(resp); err != nil {
		return false, "", err
	}
	var body struct {
		Match    bool   `json:"match"`
		Mismatch string `json:"mismatch"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return false, "", fmt.Errorf("pdf-core verify/content-match: decode: %w", err)
	}
	return body.Match, body.Mismatch, nil
}

// ExtractManifest posts pdf to POST /manifest/extract and returns the raw JUMBF
// C2PA manifest store bytes embedded in the PDF (DCS-OR-C2PA-008).
func (c *Client) ExtractManifest(ctx context.Context, pdf []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/manifest/extract", bytes.NewReader(pdf))
	if err != nil {
		return nil, fmt.Errorf("pdf-core extract-manifest request: %w", err)
	}
	req.Header.Set("Content-Type", "application/pdf")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pdf-core extract-manifest: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	manifest, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("pdf-core extract-manifest read: %w", err)
	}
	return manifest, nil
}

// ChainEntry is one manifest in a PDF's C2PA provenance chain: its JUMBF label
// and, when present, its parsed dcs.lifecycle assertion. pdf-core owns the
// JUMBF/CBOR parsing; the DCS consumes this structured form.
type ChainEntry struct {
	Label     string            `json:"label"`
	Lifecycle map[string]string `json:"lifecycle,omitempty"`
}

// ExtractManifestChain returns the parsed C2PA provenance chain embedded in a
// PDF (oldest manifest first).
func (c *Client) ExtractManifestChain(ctx context.Context, pdf []byte) ([]ChainEntry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/manifest/chain", bytes.NewReader(pdf))
	if err != nil {
		return nil, fmt.Errorf("pdf-core manifest-chain request: %w", err)
	}
	req.Header.Set("Content-Type", "application/pdf")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pdf-core manifest-chain: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	var chain []ChainEntry
	if err := json.NewDecoder(resp.Body).Decode(&chain); err != nil {
		return nil, fmt.Errorf("pdf-core manifest-chain decode: %w", err)
	}
	return chain, nil
}

// ExtractPayload returns the machine-readable JSON-LD contract payload embedded
// in a PDF. A peer that receives a contract PDF rebuilds its local copy from
// this, so the DCS never parses PDF bytes itself (ADR-13).
func (c *Client) ExtractPayload(ctx context.Context, pdf []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/payload/extract", bytes.NewReader(pdf))
	if err != nil {
		return nil, fmt.Errorf("pdf-core payload-extract request: %w", err)
	}
	req.Header.Set("Content-Type", "application/pdf")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pdf-core payload-extract: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("pdf-core payload-extract read: %w", err)
	}
	return payload, nil
}

// checkStatus returns an error for non-2xx responses, including the status code
// in the message. Hard-fail: callers must not silently swallow pdf-core errors.
func checkStatus(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("pdf-core %s: status %d: %s", resp.Request.URL.Path, resp.StatusCode, strings.TrimSpace(string(body)))
}

// truncate keeps an error message readable when the body it quotes is not.
func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit]
}

// writeField writes data as a plain multipart form field.
func writeField(mw *multipart.Writer, name string, data []byte) error {
	w, err := mw.CreateFormField(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}
