package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"

	compiler "example.com/m/V2/compiler"
	"example.com/m/V2/manifest"

	"github.com/google/uuid"
)

// setPDFCoreVersionHeader emits the renderer version on every PDF-producing
// response so clients can cache-bust when the renderer is upgraded.
func setPDFCoreVersionHeader(w http.ResponseWriter) {
	w.Header().Set("X-PDF-Core-Version", compiler.RendererVersion)
}

// preparedResponse is the JSON body the render endpoints (/render,
// /render/amendment) return: the compiled PDF with zeroed 64-byte C2PA COSE
// signature slots, and the C2PA COSE Sig_structures the DCS backend must sign (in
// document order). These are the provenance-manifest signatures, NOT the PAdES
// signature — the DCS signs each with its dcs-c2pa key and posts them to
// /c2pa/embed; pdf-core holds no key material.
type preparedResponse struct {
	PDFBase64         string   `json:"pdf_base64"`
	C2PASigStructures []string `json:"c2pa_sig_structures"`
}

// embedRequest is the JSON body /c2pa/embed accepts: a prepared PDF and the C2PA
// ES256 signatures for its zeroed slots, in the same order /render returned the
// Sig_structures.
type embedRequest struct {
	PDFBase64      string   `json:"pdf_base64"`
	C2PASignatures []string `json:"c2pa_signatures"`
}

// writePrepared writes the prepare JSON envelope. A render stabilises its byte
// layout over several passes and signs each, but only the last pass survives into
// pdf; the surviving zeroed slots are filled by exactly the final pass's
// signings, which are the last len(slots) captures in document order.
func writePrepared(w http.ResponseWriter, pdf []byte, captured [][]byte) {
	slots := compiler.CountZeroedCOSESignatureSlots(pdf)
	if slots > len(captured) {
		writeError(w, errBadRequest(fmt.Errorf("compiled PDF has %d empty signature slots but only %d were captured", slots, len(captured))))
		return
	}
	surviving := captured[len(captured)-slots:]
	resp := preparedResponse{
		PDFBase64:         base64.StdEncoding.EncodeToString(pdf),
		C2PASigStructures: make([]string, len(surviving)),
	}
	for i, sig := range surviving {
		resp.C2PASigStructures[i] = base64.StdEncoding.EncodeToString(sig)
	}
	setPDFCoreVersionHeader(w)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resp)
}

// service implements all HTTP handlers for the DCS-PDF-CORE API.
type service struct{}

// httpError carries enough information to write a structured error response.
type httpError struct {
	status  int
	name    string
	message string
	// digests are merged into the error body when the refusal is a verification
	// verdict, so a caller sees the evidence the refusal rests on rather than a
	// message it can only quote.
	digests verifyDigests
}

func (e *httpError) Error() string { return e.message }

// withDigests attaches the digests a verification verdict was reached on.
func (e *httpError) withDigests(d verifyDigests) *httpError {
	e.digests = d
	return e
}

func prettyLog(m map[string]interface{}) {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		log.Println("error:", err)
		return
	}
	log.Println("\n" + string(b))
}

// writeError writes a JSON error body matching the OpenAPI Error schema.
func writeError(w http.ResponseWriter, err error) {
	var he *httpError
	if !errors.As(err, &he) {
		he = &httpError{
			status:  http.StatusInternalServerError,
			name:    "internal_error",
			message: err.Error(),
		}
	}
	body := map[string]interface{}{
		"name":      he.name,
		"id":        uuid.New().String(),
		"message":   he.message,
		"temporary": false,
		"timeout":   false,
		"fault":     false,
	}
	for key, digest := range map[string]string{
		"jsonld_hash":          he.digests.JSONLDHash,
		"base_pdf_hash":        he.digests.BasePDFHash,
		"stored_base_pdf_hash": he.digests.StoredBasePDFHash,
	} {
		// A digest that could not be computed is left out entirely: an empty
		// string under a hash key reads as "the hash is blank", which is a claim
		// about the content rather than about the check.
		if digest != "" {
			body[key] = digest
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(he.status)
	_ = json.NewEncoder(w).Encode(body)

	prettyLog(body)
}

func errBadRequest(err error) *httpError {
	return &httpError{status: http.StatusBadRequest, name: "bad_request", message: err.Error()}
}

func errUnsupportedMediaType(err error) *httpError {
	return &httpError{status: http.StatusUnsupportedMediaType, name: "unsupported_media_type", message: err.Error()}
}

func errConflict(err error) *httpError {
	return &httpError{status: http.StatusConflict, name: "conflict", message: err.Error()}
}

func errUnprocessableEntity(err error) *httpError {
	return &httpError{status: http.StatusUnprocessableEntity, name: "unprocessable_entity", message: err.Error()}
}

// checkMediaType returns an unsupported_media_type error if the Content-Type
// header is not in the allowed set. Allowed values are matched on the bare
// media type (parameters such as charset are ignored).
func checkMediaType(contentType string, allowed ...string) error {
	if contentType == "" {
		return errUnsupportedMediaType(errors.New("missing content type"))
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return errUnsupportedMediaType(fmt.Errorf("invalid content type: %w", err))
	}
	for _, a := range allowed {
		if mediaType == a {
			return nil
		}
	}
	return errUnsupportedMediaType(fmt.Errorf("unsupported content type %q", mediaType))
}

// limitRead reads up to limit bytes and errors if the body is empty.
func limitRead(r io.ReadCloser, limit int64) ([]byte, error) {
	defer r.Close()
	b, err := io.ReadAll(io.LimitReader(r, limit))
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return nil, errors.New("request body is empty")
	}
	return b, nil
}

func (s *service) version(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]string{"version": compiler.RendererVersion})
}

// lifecycleAuthorityHeader names the DID of the DCS instance asserting the
// lifecycle events of a render. It travels per request because pdf-core is a
// stateless renderer several instances may share, and the value is recorded in
// every dcs.lifecycle assertion the render writes.
const lifecycleAuthorityHeader = "X-DCS-Lifecycle-Authority"

// signingChainHeader carries the RFC 9360 x5chain the caller signs under,
// base64-encoded PEM (leaf first). pdf-core stores no chain of its own: it
// already holds no private key, and holding the certificate that names one was
// the last piece of signing material left behind — the piece that made a shared
// pdf-core sign one instance's documents under another's identity.
const signingChainHeader = "X-DCS-C2PA-X5Chain"

// callerSigningChain decodes the chain the request signs under. Absence is a
// refusal, not a fallback: a default chain is exactly how manifests came to be
// signed under an identity nobody named.
func callerSigningChain(r *http.Request) ([][]byte, error) {
	encoded := strings.TrimSpace(r.Header.Get(signingChainHeader))
	if encoded == "" {
		return nil, errBadRequest(fmt.Errorf("%s is required: pdf-core signs under the caller's x5chain and holds none of its own", signingChainHeader))
	}
	pemBytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errBadRequest(fmt.Errorf("%s must be base64-encoded PEM: %w", signingChainHeader, err))
	}
	chain, err := compiler.ParseSigningChainPEM(pemBytes)
	if err != nil {
		return nil, errBadRequest(fmt.Errorf("%s: %w", signingChainHeader, err))
	}
	return chain, nil
}

// renderContext carries the request's signer, asserting authority and signing
// chain into the compiler.
func renderContext(r *http.Request, signer compiler.Signer) (context.Context, error) {
	chain, err := callerSigningChain(r)
	if err != nil {
		return nil, err
	}
	ctx := compiler.WithSigner(r.Context(), signer)
	ctx = compiler.WithLifecycleAuthority(ctx, strings.TrimSpace(r.Header.Get(lifecycleAuthorityHeader)))
	return compiler.WithSigningChain(ctx, chain), nil
}

func (s *service) render(w http.ResponseWriter, r *http.Request) {
	if err := checkMediaType(r.Header.Get("Content-Type"), "application/ld+json", "application/json"); err != nil {
		writeError(w, err)
		return
	}
	raw, err := limitRead(r.Body, 8<<20)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	canonical, err := compiler.CanonicalizePayload(raw)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	if err := compiler.ValidatePayloadSHACL(canonical); err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	signer := compiler.NewCapturingSigner()
	renderCtx, err := renderContext(r, signer)
	if err != nil {
		writeError(w, err)
		return
	}
	// Pass the verbatim raw payload: CompilePDF canonicalizes internally for the
	// render model + graph hash, but embeds these exact bytes as the attachment.
	pdf, err := compiler.CompilePDF(renderCtx, raw, compiler.CanonicalCompiledAt)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	writePrepared(w, pdf, signer.Captured())
}

// verifyDigests are the SHA-256 digests (hex, the digest every other DCS
// component records) that the /verify match verdict is reached on. They are the
// evidence for the verdict rather than a restatement of it: on the reproduction
// check BasePDFHash and StoredBasePDFHash are equal exactly when the submitted
// document re-renders to its stored bytes.
//
// Both PDF digests are taken over compiler.ZeroCOSESignatures output, the same
// normalization the comparison itself uses. A fresh compile produces a fresh
// randomized ECDSA COSE claim signature, so digesting the raw bytes would report
// two different hashes for a document the verdict calls intact.
//
// Absent digests are omitted rather than sent empty — an empty hash field states
// something about the content, not about the check.
type verifyDigests struct {
	// JSONLDHash is the SHA-256 of the machine-readable payload embedded in the
	// submitted PDF — the latest one on an amended document, i.e. the payload
	// that governs its current visible state.
	JSONLDHash string `json:"jsonld_hash,omitempty"`
	// BasePDFHash is the SHA-256 of the deterministic re-render produced from
	// JSONLDHash's payload: a bare recompile for a plain document, the replay of
	// the last amendment hop for an incrementally updated one.
	BasePDFHash string `json:"base_pdf_hash,omitempty"`
	// StoredBasePDFHash is the SHA-256 of the stored bytes that re-render was
	// compared against — the leading span of the submitted PDF it must reproduce,
	// excluding the append-only PAdES signature layers that legitimately follow.
	StoredBasePDFHash string `json:"stored_base_pdf_hash,omitempty"`
}

// reproductionDigests digests the two sides of the reproduction check plus the
// payload it was driven from. reproduced may be nil when the check failed before
// producing one, and payload nil when none could be extracted; the corresponding
// digests are then left unset rather than reporting the digest of nothing.
func reproductionDigests(payload, stored, reproduced []byte) verifyDigests {
	var d verifyDigests
	if payload != nil {
		d.JSONLDHash = sha256Hex(payload)
	}
	if reproduced == nil {
		return d
	}
	fresh := compiler.ZeroCOSESignatures(reproduced)
	storedBase := compiler.ZeroCOSESignatures(stored)
	if len(storedBase) > len(fresh) {
		storedBase = storedBase[:len(fresh)]
	}
	d.BasePDFHash = sha256Hex(fresh)
	d.StoredBasePDFHash = sha256Hex(storedBase)
	return d
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// verifyResponse is the JSON body returned by POST /verify.
type verifyResponse struct {
	verifyDigests
	Match bool `json:"match"`
	// C2PASignatureValid is the outcome of compiler.VerifyC2PAClaimSignatures:
	// every manifest's COSE_Sign1 claim signature verified against its own
	// x5chain leaf and every assertion the signed claim commits to still hashes
	// to the recorded value. C2PASignatureError carries the reason it did not, so
	// a consumer reports the finding rather than a bare false.
	//
	// On a PAdES-signed contract the last manifest's whole-file hard binding does
	// NOT cover the appended signature revisions — the signature is applied after
	// the manifest so that it commits to the provenance (ADR-26) — so PAdESSigned
	// states how far the binding reaches rather than leaving a consumer to assume
	// it covers everything.
	C2PASignatureValid bool   `json:"c2pa_signature_valid"`
	C2PASignatureError string `json:"c2pa_signature_error,omitempty"`
	PAdESSigned        bool   `json:"pades_signed"`
	VCBytes            string `json:"vc_bytes,omitempty"` // base64-encoded VC JSON
	// VCPresent says only that the PDF carries a lifecycle-credential attachment,
	// which is the whole of what pdf-core can say about it: verifying the
	// credential's proof means resolving its issuer to a key it publishes for
	// assertions, and pdf-core holds no key material and resolves no DID. The
	// caller gets VCBytes and verifies them against the issuer itself.
	VCPresent bool   `json:"vc_present"`
	Artifact  string `json:"artifact"` // base64-encoded verification-witness PDF
	// Signers names the certificate each manifest was signed under, in chain
	// order. Match says the stored bytes reproduce from their embedded payloads
	// under the certificates the artifact itself carries — self-consistency, which
	// is silent about whose certificates those are. Signers answers the second
	// question: each entry reports whether it is the chain the caller named, so a
	// verdict is "reproduces, signed by us" or "reproduces, signed by someone
	// else" rather than an unqualified match. A federated contract legitimately
	// carries the peer's leaf on the hops the peer wrote, so a foreign signer is
	// reported, not refused — deciding which peers are trusted is the caller's.
	Signers []manifestSigner `json:"signers"`
}

// manifestSigner identifies the leaf one manifest's claim signature was made
// under. The leaf is fingerprinted rather than inlined: a caller comparing
// identities needs to tell them apart, not to re-parse the certificate.
type manifestSigner struct {
	Manifest   string `json:"manifest"`
	Subject    string `json:"subject"`
	LeafSHA256 string `json:"leaf_sha256"`
	// Expected is true when this manifest was signed under the chain the request
	// supplied — for a DCS verifying its own work, its own leaf.
	Expected bool `json:"expected"`
}

// describeSigners fingerprints the leaf behind every manifest and marks the ones
// signed under expectedLeaf.
func describeSigners(pdf []byte, expectedLeaf []byte) ([]manifestSigner, error) {
	found, err := compiler.ManifestSigners(pdf)
	if err != nil {
		return nil, err
	}
	out := make([]manifestSigner, 0, len(found))
	for _, signer := range found {
		entry := manifestSigner{
			Manifest:   signer.Manifest,
			LeafSHA256: sha256Hex(signer.LeafDER),
			Expected:   bytes.Equal(signer.LeafDER, expectedLeaf),
		}
		if leaf, parseErr := x509.ParseCertificate(signer.LeafDER); parseErr == nil {
			entry.Subject = leaf.Subject.String()
		}
		out = append(out, entry)
	}
	return out, nil
}

// renderReanchor appends a provenance-only C2PA manifest binding the submitted
// PDF's current bytes (ADR-26). The payload is unchanged, so this is not an
// amendment: it exists because a PAdES signature is applied after the lifecycle
// manifest, leaving that manifest's whole-file binding short of the signed file.
func (s *service) renderReanchor(w http.ResponseWriter, r *http.Request) {
	if err := checkMediaType(r.Header.Get("Content-Type"), "application/pdf"); err != nil {
		writeError(w, err)
		return
	}
	raw, err := limitRead(r.Body, 32<<20)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	signer := compiler.NewCapturingSigner()
	renderCtx, err := renderContext(r, signer)
	if err != nil {
		writeError(w, err)
		return
	}
	updated, err := compiler.ReanchorProvenance(renderCtx, raw,
		strings.TrimSpace(r.URL.Query().Get("manifest_url")), compiler.CanonicalCompiledAt)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	writePrepared(w, updated, signer.Captured())
}

func (s *service) verify(w http.ResponseWriter, r *http.Request) {
	if err := checkMediaType(r.Header.Get("Content-Type"), "application/pdf"); err != nil {
		writeError(w, err)
		return
	}
	raw, err := limitRead(r.Body, 32<<20)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	// The caller's own chain, for the two things a verify does under its own
	// identity: witnessing the result, and naming the signer it expected. The
	// replay below signs under the chain each stored manifest carries instead —
	// those are two different questions and pdf-core answers both.
	callerChain, err := callerSigningChain(r)
	if err != nil {
		writeError(w, err)
		return
	}

	var payload []byte
	var digests verifyDigests

	if _, ok := compiler.SplitAtIncrementalUpdate(raw); ok {
		reproduced, chainErr := compiler.VerifyIncrementalUpdate(
			compiler.WithSigner(r.Context(), compiler.NewCapturingSigner()), raw)
		// The payload is read whatever the chain verdict, so a rejected document
		// still names which machine-readable content was on the table. A payload
		// that cannot be read is reported only when the chain itself held —
		// otherwise the chain failure is the finding, and stays the status.
		payload, err = compiler.ExtractLatestEmbeddedJSONLD(raw)
		if chainErr != nil {
			writeError(w, errConflict(chainErr).withDigests(reproductionDigests(payload, raw, reproduced)))
			return
		}
		if err != nil {
			writeError(w, errBadRequest(err))
			return
		}
		digests = reproductionDigests(payload, raw, reproduced)
	} else {
		payload, err = compiler.ExtractEmbeddedJSONLD(raw)
		if err != nil {
			writeError(w, errBadRequest(err))
			return
		}
		compiledAt, err := compiler.ExtractLifecycleEffectiveAt(raw)
		if err != nil {
			writeError(w, errBadRequest(fmt.Errorf("extract lifecycle timestamp: %w", err)))
			return
		}
		authority, err := compiler.ExtractLifecycleAuthority(raw)
		if err != nil {
			writeError(w, errBadRequest(fmt.Errorf("extract lifecycle authority: %w", err)))
			return
		}
		chain, err := compiler.ExtractSigningChain(raw)
		if err != nil {
			writeError(w, errBadRequest(fmt.Errorf("extract signing chain: %w", err)))
			return
		}
		verifyCtx := compiler.WithSigningChain(
			compiler.WithLifecycleAuthority(compiler.WithSigner(r.Context(), compiler.NewCapturingSigner()), authority),
			chain)
		recompiled, err := compiler.CompilePDF(verifyCtx, payload, compiledAt)
		if err != nil {
			writeError(w, errUnprocessableEntity(err).withDigests(verifyDigests{JSONLDHash: sha256Hex(payload)}))
			return
		}
		digests = reproductionDigests(payload, raw, recompiled)
		// The compiled base must reproduce the leading bytes of the submitted
		// PDF. A PAdES signature (and the signing evidence embedded before it)
		// is an append-only incremental update written after the base's %%EOF —
		// it leaves the base bytes untouched and is itself covered by its own
		// /ByteRange — so the base is a byte-prefix of a signed PDF rather than
		// byte-equal to it (DCS-OR-C2PA-010).
		if !bytes.HasPrefix(compiler.ZeroCOSESignatures(raw), compiler.ZeroCOSESignatures(recompiled)) {
			writeError(w, errConflict(errors.New("embedded payload does not reproduce the submitted PDF")).withDigests(digests))
			return
		}
		// An unsigned base must be byte-EQUAL to its deterministic recompile. The
		// only legitimate bytes appended after a compiled base are a PAdES
		// signature layer (which makes the PDF signed) or a dcs incremental update
		// (handled by the branch above via its marker). Trailing content on an
		// otherwise-unsigned PDF is an offline amendment made outside /update —
		// reject it (DCS-OR-C2PA-002 tamper-evidence).
		//
		// The digests agree here — the divergence is in the trailing bytes, not in
		// the base the digests cover — so they are reported as the record of what
		// was checked, and the message names what actually failed.
		if !compiler.IsPAdESSigned(raw) &&
			len(compiler.ZeroCOSESignatures(raw)) != len(compiler.ZeroCOSESignatures(recompiled)) {
			writeError(w, errConflict(errors.New("submitted PDF was amended offline, outside the /update workflow")).withDigests(digests))
			return
		}
	}

	// Extract VC attachment if present — returned to the caller so it can verify
	// its proof and check the status list without parsing PDF bytes itself.
	vcBytes, vcFound, _ := compiler.ExtractEmbeddedVC(raw)

	c2paSignatureError := ""
	if err := compiler.VerifyC2PAClaimSignatures(raw); err != nil {
		c2paSignatureError = err.Error()
	}

	// Append a verification witness and embed the resulting PDF as artifact. The
	// witness is this verifier's own assertion about the document, so it is signed
	// under the caller's chain rather than the artifact's.
	witnessCtx := compiler.WithSigningChain(compiler.WithSigner(r.Context(), compiler.NewCapturingSigner()), callerChain)
	witness, err := compiler.AppendVerificationWitness(witnessCtx, raw, payload)
	if err != nil {
		writeError(w, errBadRequest(fmt.Errorf("append verification witness: %w", err)))
		return
	}

	signers, err := describeSigners(raw, callerChain[0])
	if err != nil {
		writeError(w, errBadRequest(fmt.Errorf("identify manifest signers: %w", err)))
		return
	}

	resp := verifyResponse{
		verifyDigests:      digests,
		Match:              true,
		Signers:            signers,
		C2PASignatureValid: c2paSignatureError == "",
		C2PASignatureError: c2paSignatureError,
		PAdESSigned:        compiler.IsPAdESSigned(raw),
		VCPresent:          vcFound && len(vcBytes) > 0,
		Artifact:           base64.StdEncoding.EncodeToString(witness),
	}
	if vcFound && len(vcBytes) > 0 {
		resp.VCBytes = base64.StdEncoding.EncodeToString(vcBytes)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resp)
}

// verifyContent checks ONLY that the submitted PDF's visible page content is the
// deterministic render of its own embedded machine-readable payload, tolerant of
// C2PA / signature / amendment incremental-update layers appended after the base
// (which /verify's byte-prefix reproduction check rejects). This is the legal
// human↔machine guarantee a DCS applies to a peer-received PDF that may already
// carry appended manifests: tampering of the visible document fails, an
// already-C2PA-amended offer passes.
func (s *service) verifyContent(w http.ResponseWriter, r *http.Request) {
	if err := checkMediaType(r.Header.Get("Content-Type"), "application/pdf"); err != nil {
		writeError(w, err)
		return
	}
	raw, err := limitRead(r.Body, 32<<20)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	payload, err := compiler.ExtractLatestEmbeddedJSONLD(raw)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	compiledAt, err := compiler.ExtractLifecycleEffectiveAt(raw)
	if err != nil {
		writeError(w, errBadRequest(fmt.Errorf("extract lifecycle timestamp: %w", err)))
		return
	}
	chain, err := callerSigningChain(r)
	if err != nil {
		writeError(w, err)
		return
	}
	recompiled, err := compiler.CompilePDF(
		compiler.WithSigningChain(compiler.WithSigner(r.Context(), compiler.NewCapturingSigner()), chain), payload, compiledAt)
	if err != nil {
		writeError(w, errUnprocessableEntity(err))
		return
	}
	var mismatch string
	if err := compiler.MatchPageContent(raw, recompiled); err != nil {
		mismatch = err.Error()
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(struct {
		Match    bool   `json:"match"`
		Mismatch string `json:"mismatch,omitempty"`
	}{Match: mismatch == "", Mismatch: mismatch})
}

// verifyContentMatch compares a submitted PDF's visible page content against a
// REFERENCE PDF's, resolving the LAST definition of every object on both sides.
// Neither side is re-rendered — the reference is the exact document the caller
// committed to — so this is free of the render determinism /verify/content
// depends on, and it answers a different question: not "does this PDF render its
// own embedded payload" (an attacker who supersedes both the pages and the
// attachment passes that) but "does this PDF still render the document we
// prepared". An appended revision that supersedes a page content stream diverges
// here; append-only signature, C2PA, evidence and timestamp layers do not.
func (s *service) verifyContentMatch(w http.ResponseWriter, r *http.Request) {
	ct := r.Header.Get("Content-Type")
	if err := checkMediaType(ct, "multipart/form-data"); err != nil {
		writeError(w, err)
		return
	}
	defer r.Body.Close()

	_, params, err := mime.ParseMediaType(ct)
	if err != nil {
		writeError(w, errBadRequest(fmt.Errorf("invalid multipart content type: %w", err)))
		return
	}
	boundary, ok := params["boundary"]
	if !ok {
		writeError(w, errBadRequest(errors.New("multipart boundary missing from Content-Type")))
		return
	}
	parts, err := readMultipartParts(r.Body, boundary)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	submitted, ok := parts["pdf"]
	if !ok || len(submitted) == 0 {
		writeError(w, errBadRequest(errors.New("pdf field required")))
		return
	}
	reference, ok := parts["reference"]
	if !ok || len(reference) == 0 {
		writeError(w, errBadRequest(errors.New("reference field required")))
		return
	}

	var mismatch string
	if err := compiler.MatchPageContent(submitted, reference); err != nil {
		mismatch = err.Error()
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(struct {
		Match    bool   `json:"match"`
		Mismatch string `json:"mismatch,omitempty"`
	}{Match: mismatch == "", Mismatch: mismatch})
}

func (s *service) renderAmendment(w http.ResponseWriter, r *http.Request) {
	ct := r.Header.Get("Content-Type")
	if err := checkMediaType(ct, "multipart/form-data"); err != nil {
		writeError(w, err)
		return
	}
	defer r.Body.Close()

	_, params, err := mime.ParseMediaType(ct)
	if err != nil {
		writeError(w, errBadRequest(fmt.Errorf("invalid multipart content type: %w", err)))
		return
	}
	boundary, ok := params["boundary"]
	if !ok {
		writeError(w, errBadRequest(errors.New("multipart boundary missing from Content-Type")))
		return
	}

	parts, err := readMultipartParts(r.Body, boundary)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}

	oldPDF, ok := parts["pdf"]
	if !ok || len(oldPDF) == 0 {
		writeError(w, errBadRequest(errors.New("pdf field required")))
		return
	}
	newPayload, ok := parts["payload"]
	if !ok || len(newPayload) == 0 {
		writeError(w, errBadRequest(errors.New("payload field required")))
		return
	}
	canonical, err := compiler.CanonicalizePayload(newPayload)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	if err := compiler.ValidatePayloadSHACL(canonical); err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	vcBytes := parts["vc"] // optional
	// manifest_url (DCS-OR-C2PA-008 AC3): when the DCS hosting layer supplies a
	// public manifest URL, it is embedded via a dcs.remote_manifests C2PA
	// assertion (not a claim field — c2pa-rs 0.85.1 hard-rejects an
	// unrecognized "remote_manifests" claim field, see the comment in
	// renderVerificationClaimCBOR) plus an XMP dcterms:provenance link.
	// Absent (the default) => neither is emitted.
	manifestURL := strings.TrimSpace(string(parts["manifest_url"]))

	signer := compiler.NewCapturingSigner()
	renderCtx, err := renderContext(r, signer)
	if err != nil {
		writeError(w, err)
		return
	}
	// Verbatim: the amended attachment is embedded exactly as submitted.
	updated, err := compiler.UpdatePDFWithOptions(renderCtx, oldPDF, newPayload, vcBytes, manifestURL, compiler.CanonicalCompiledAt)
	if err != nil {
		if errors.Is(err, compiler.ErrNoChanges) {
			writeError(w, errConflict(err))
			return
		}
		writeError(w, errBadRequest(err))
		return
	}
	writePrepared(w, updated, signer.Captured())
}

// embedC2PASignatures is the stateless "embed" step: it splices the DCS backend's
// ES256 signatures into the zeroed COSE slots a prepare left behind. It parses no
// domain content and holds no key — it only fills byte runs pdf-core itself
// reserved, so any replica can serve it.
func (s *service) embedC2PASignatures(w http.ResponseWriter, r *http.Request) {
	if err := checkMediaType(r.Header.Get("Content-Type"), "application/json"); err != nil {
		writeError(w, err)
		return
	}
	raw, err := limitRead(r.Body, 32<<20)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	var req embedRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		writeError(w, errBadRequest(fmt.Errorf("decode embed request: %w", err)))
		return
	}
	pdf, err := base64.StdEncoding.DecodeString(req.PDFBase64)
	if err != nil {
		writeError(w, errBadRequest(fmt.Errorf("decode prepared pdf: %w", err)))
		return
	}
	signatures := make([][]byte, len(req.C2PASignatures))
	for i, s := range req.C2PASignatures {
		sig, decErr := base64.StdEncoding.DecodeString(s)
		if decErr != nil {
			writeError(w, errBadRequest(fmt.Errorf("decode signature %d: %w", i, decErr)))
			return
		}
		signatures[i] = sig
	}
	embedded, err := compiler.InjectCOSESignatures(pdf, signatures)
	if err != nil {
		writeError(w, errUnprocessableEntity(err))
		return
	}
	setPDFCoreVersionHeader(w)
	w.Header().Set("Content-Type", "application/pdf")
	_, _ = w.Write(embedded)
}

// embedEvidence attaches the posted signing evidence to the PDF without signing
// it. It is the attach-only half of the embed-first-sign-second contract that a
// remote signer (EU DSS) needs: the DSS signDocument call does not attach, so
// the evidence must already be embedded — and therefore covered by the PAdES
// /ByteRange — before the DSS produces the signature (DCS-FR-SM-08).
func (s *service) embedEvidence(w http.ResponseWriter, r *http.Request) {
	ct := r.Header.Get("Content-Type")
	if err := checkMediaType(ct, "multipart/form-data"); err != nil {
		writeError(w, err)
		return
	}
	defer r.Body.Close()

	_, params, err := mime.ParseMediaType(ct)
	if err != nil {
		writeError(w, errBadRequest(fmt.Errorf("invalid multipart content type: %w", err)))
		return
	}
	boundary, ok := params["boundary"]
	if !ok {
		writeError(w, errBadRequest(errors.New("multipart boundary missing from Content-Type")))
		return
	}

	parts, err := readMultipartParts(r.Body, boundary)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}

	pdfBytes, ok := parts["pdf"]
	if !ok || len(pdfBytes) == 0 {
		writeError(w, errBadRequest(errors.New("pdf field required")))
		return
	}
	evidence, ok := parts["evidence"]
	if !ok || len(evidence) == 0 {
		writeError(w, errBadRequest(errors.New("evidence field required")))
		return
	}

	embedded, err := compiler.EmbedSigningEvidence(pdfBytes, evidence)
	if err != nil {
		writeError(w, errBadRequest(fmt.Errorf("embed signing evidence: %w", err)))
		return
	}

	setPDFCoreVersionHeader(w)
	w.Header().Set("Content-Type", "application/pdf")
	_, _ = w.Write(embedded)
}

func (s *service) extractEvidence(w http.ResponseWriter, r *http.Request) {
	if err := checkMediaType(r.Header.Get("Content-Type"), "application/pdf"); err != nil {
		writeError(w, err)
		return
	}
	raw, err := limitRead(r.Body, 32<<20)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	evidence, found, err := compiler.ExtractSigningEvidence(raw)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	if !found {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(evidence)
}

func (s *service) claim(w http.ResponseWriter, r *http.Request) {
	ct := r.Header.Get("Content-Type")
	if err := checkMediaType(ct, "multipart/form-data"); err != nil {
		writeError(w, err)
		return
	}
	defer r.Body.Close()

	_, params, err := mime.ParseMediaType(ct)
	if err != nil {
		writeError(w, errBadRequest(fmt.Errorf("invalid multipart content type: %w", err)))
		return
	}
	boundary, ok := params["boundary"]
	if !ok {
		writeError(w, errBadRequest(errors.New("multipart boundary missing from Content-Type")))
		return
	}

	parts, err := readMultipartParts(r.Body, boundary)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}

	submittedPDF, ok := parts["pdf"]
	if !ok || len(submittedPDF) == 0 {
		writeError(w, errBadRequest(errors.New("pdf field required")))
		return
	}
	payloadBytes, ok := parts["payload"]
	if !ok || len(payloadBytes) == 0 {
		writeError(w, errBadRequest(errors.New("payload field required")))
		return
	}
	canonical, err := compiler.CanonicalizePayload(payloadBytes)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	if err := compiler.ValidatePayloadSHACL(canonical); err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	chain, err := callerSigningChain(r)
	if err != nil {
		writeError(w, err)
		return
	}
	matchCtx := compiler.WithSigningChain(compiler.WithSigner(r.Context(), compiler.NewCapturingSigner()), chain)
	referencePDF, err := compiler.CompilePDF(matchCtx, payloadBytes, compiler.CanonicalCompiledAt)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	if err := compiler.MatchPageContent(submittedPDF, referencePDF); err != nil {
		writeError(w, errConflict(err))
		return
	}
	result, err := compiler.AppendVerificationWitness(matchCtx, referencePDF, payloadBytes)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	_, _ = w.Write(result)
}

func (s *service) extractManifest(w http.ResponseWriter, r *http.Request) {
	if err := checkMediaType(r.Header.Get("Content-Type"), "application/pdf"); err != nil {
		writeError(w, err)
		return
	}
	raw, err := limitRead(r.Body, 32<<20)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	manifestStore, err := compiler.ExtractManifestStore(raw)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(manifestStore)
}

// extractPayload returns the machine-readable JSON-LD contract payload embedded
// in the PDF (the latest version). A peer that receives a contract PDF rebuilds
// its local copy from this; the DCS never parses PDF bytes itself (ADR-13).
func (s *service) extractPayload(w http.ResponseWriter, r *http.Request) {
	if err := checkMediaType(r.Header.Get("Content-Type"), "application/pdf"); err != nil {
		writeError(w, err)
		return
	}
	raw, err := limitRead(r.Body, 32<<20)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	payload, err := compiler.ExtractLatestEmbeddedJSONLD(raw)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	w.Header().Set("Content-Type", "application/ld+json")
	_, _ = w.Write(payload)
}

// manifestChain extracts the embedded C2PA manifest store and returns its
// parsed manifest chain (one entry per manifest, oldest first, with each
// manifest's dcs.lifecycle assertion). All JUMBF/CBOR byte parsing lives here
// in pdf-core; the DCS consumes the structured chain.
func (s *service) manifestChain(w http.ResponseWriter, r *http.Request) {
	if err := checkMediaType(r.Header.Get("Content-Type"), "application/pdf"); err != nil {
		writeError(w, err)
		return
	}
	raw, err := limitRead(r.Body, 32<<20)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	store, err := compiler.ExtractManifestStore(raw)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	chain, err := manifest.ParseChain(store)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(chain)
}

func (s *service) ontologyContext(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/ld+json")
	_, _ = w.Write(ontologyContext)
}

// readMultipartParts reads all parts from a multipart body into a map keyed by form field name.
func readMultipartParts(body io.Reader, boundary string) (map[string][]byte, error) {
	parts := make(map[string][]byte)
	mr := multipart.NewReader(body, boundary)
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read multipart: %w", err)
		}
		name := part.FormName()
		var limit int64 = 8 << 20
		if name == "pdf" || name == "reference" {
			limit = 32 << 20
		}
		data, err := io.ReadAll(io.LimitReader(part, limit))
		part.Close()
		if err != nil {
			return nil, fmt.Errorf("read part %q: %w", name, err)
		}
		parts[name] = data
	}
	return parts, nil
}
