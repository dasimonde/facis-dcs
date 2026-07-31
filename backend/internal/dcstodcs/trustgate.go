package dcstodcs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/federation"
	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/base/safehttp"
	"digital-contracting-service/internal/pdfgeneration/provenance"
	qry "digital-contracting-service/internal/processauditandcompliance/query"
)

// Direction names which side of an interaction the trust gate is being
// consulted for, carried verbatim in the policy-endpoint request body
// (ADR-19).
type Direction string

const (
	Inbound  Direction = "inbound"
	Outbound Direction = "outbound"
)

// GateFailureKind distinguishes the agreement-credential check (layer 3a,
// whose failure is retried via sync_fails on the outbound path) from the
// policy-endpoint check (layer 3b, whose failure is always terminal — never
// retried), per ADR-19's architect decision.
type GateFailureKind int

const (
	AgreementFailure GateFailureKind = iota
	PolicyFailure
	// PoAFailure is a counterparty whose shipped Power-of-Attorney evidence
	// does not verify (ADR-31). Inbound only, and terminal: the peer must ship
	// evidence that verifies, which no retry of the same ship will produce.
	PoAFailure
)

// GateError is returned by TrustGate.Check on any rejection, naming which of
// the two ADR-19 layers rejected the interaction and the peer involved (so a
// caller can anchor an incident report without threading the peer DID
// through separately).
type GateError struct {
	Kind    GateFailureKind
	PeerDID string
	Err     error
}

func (e *GateError) Error() string { return e.Err.Error() }
func (e *GateError) Unwrap() error { return e.Err }

// pdpTimeout bounds every HTTP call the trust gate makes (policy-endpoint
// consult and agreement-credential/did.json fetch): http.DefaultClient has no
// timeout, so a policy endpoint or peer that accepts the connection and then
// never responds — a silently wedged policy endpoint, or a wedged peer —
// would otherwise hang the ship/receive attempt indefinitely instead of
// denying, and ADR-19's fail-closed gate requires eventually failing.
const pdpTimeout = 10 * time.Second

// TrustGate implements ADR-19's federation trust gate — the third and final
// trust layer: layer 3a (the peer's self-signed agreement credential must
// verify against its own did.json's dedicated VC key and name this instance's
// own embedded federation rules hash) and layer 3b (this instance's own local
// policy
// endpoint, DCS_TRUST_PDP_URL, must answer 2xx). Consulted on both the
// inbound (PostPdf) and outbound (shipContractPDF) paths. Fail-closed: an
// unset, unreachable, or non-responding policy endpoint denies exactly like
// an explicit deny.
type TrustGate struct {
	PDPURL     string
	HTTPClient *http.Client
}

func (g *TrustGate) httpClient() *http.Client {
	if g.HTTPClient != nil {
		return g.HTTPClient
	}
	return &http.Client{Timeout: pdpTimeout}
}

// Check verifies the peer's agreement credential (layer 3a) then consults
// the local policy endpoint (layer 3b) for one interaction.
func (g *TrustGate) Check(ctx context.Context, peerDID string, direction Direction, contractDID, targetState string) error {
	credential, err := g.verifyAgreementCredential(peerDID)
	if err != nil {
		return &GateError{Kind: AgreementFailure, PeerDID: peerDID, Err: err}
	}
	if err := g.consultPolicyEndpoint(ctx, peerDID, credential, direction, contractDID, targetState); err != nil {
		return &GateError{Kind: PolicyFailure, PeerDID: peerDID, Err: err}
	}
	return nil
}

// verifyAgreementCredential fetches the peer's agreement credential, checks
// its signature against the verification method its own proof names — which the
// peer must publish as an assertionMethod — confirms the credential is
// self-signed by that same peer (its issuer resolves to the same hostname the
// credential was fetched from), and compares its termsOfUse.hash against this
// instance's own embedded federation rules hash.
func (g *TrustGate) verifyAgreementCredential(peerDID string) (json.RawMessage, error) {
	peerDIDDocument, err := identity.FetchDIDDocument(peerDID)
	if err != nil {
		return nil, fmt.Errorf("agreement credential: fetch peer did.json: %w", err)
	}
	// Resolution returns whatever the URL served; nothing in the fetch itself
	// binds that document to the identifier that was asked for. Everything after
	// this point is derived from the document's own id — which verification
	// method may assert, whose keys they are — so a document declaring itself to
	// be somebody else would have those checks run against that other identity
	// while this instance believes it is talking to peerDID.
	if err := requireDocumentIsFor(peerDIDDocument, peerDID); err != nil {
		return nil, fmt.Errorf("agreement credential: %w", err)
	}

	credential, err := fetchAgreementCredential(peerDID)
	if err != nil {
		return nil, fmt.Errorf("agreement credential: %w", err)
	}

	var parsed struct {
		Issuer json.RawMessage `json:"issuer"`
		Proof  struct {
			VerificationMethod string `json:"verificationMethod"`
			ProofPurpose       string `json:"proofPurpose"`
		} `json:"proof"`
		TermsOfUse json.RawMessage `json:"termsOfUse"`
	}
	if err := json.Unmarshal(credential, &parsed); err != nil {
		return nil, fmt.Errorf("agreement credential: decode: %w", err)
	}

	// The credential must be self-signed by the SAME peer it was fetched
	// from: its issuer is not required to equal peerDID verbatim (a peer
	// identifier may legitimately carry a path/fragment suffix beyond the
	// host component), but the issuer's did:web HOSTNAME (the same
	// resolution identity.DIDWebToHostname applies to peerDID, and the same
	// challenge-response check above it in PostPdf/shipToPeers uses) must
	// match the hostname the credential was actually fetched from — a
	// credential issued by some other party can't be presented on this
	// peer's behalf even if it happens to verify against a key published
	// here.
	issuer, err := issuerIdentifier(parsed.Issuer)
	if err != nil {
		return nil, fmt.Errorf("agreement credential: %w", err)
	}
	// Compared as full resolution targets, not hostnames: with path-segmented
	// identifiers several instances share one host, so a hostname match would
	// accept a credential issued by a different instance on the same host.
	issuerTarget, err := didWebTarget(issuer)
	if err != nil {
		return nil, fmt.Errorf("agreement credential: issuer: %w", err)
	}
	peerTarget, err := didWebTarget(peerDID)
	if err != nil {
		return nil, fmt.Errorf("agreement credential: peer: %w", err)
	}
	if issuerTarget != peerTarget {
		return nil, fmt.Errorf("agreement credential: issuer %q resolves to %q, not the peer's own %q",
			issuer, issuerTarget, peerTarget)
	}
	// A credential is an assertion, so its proof must have been made for that
	// purpose; a proof made to authenticate or to agree a key establishes no
	// claim (W3C VC Data Integrity §2.1). proofPurpose is mandatory there, so an
	// omitted one is a malformed proof, not a permissive default — and the
	// authorization test below is meaningless without knowing which relationship
	// to test the key against.
	if purpose := strings.TrimSpace(parsed.Proof.ProofPurpose); purpose != string(identity.PurposeAssertion) {
		return nil, fmt.Errorf("agreement credential: proof.proofPurpose is %q, not %q",
			purpose, identity.PurposeAssertion)
	}
	// The key comes from the credential's own proof, not from this instance's
	// key label: `#dcs-vc` is what OUR gendid names its credential key, and a
	// peer publishing `#key-2` signs an equally valid credential. What the peer
	// must state is that the named key may assert — DIDDocument.AssertionKey.
	vcPub, err := peerDIDDocument.AssertionKey(parsed.Proof.VerificationMethod)
	if err != nil {
		return nil, fmt.Errorf("agreement credential: proof.verificationMethod %q: %w", parsed.Proof.VerificationMethod, err)
	}
	if err := provenance.VerifyDataIntegrityProof(credential, vcPub); err != nil {
		return nil, fmt.Errorf("agreement credential: signature does not verify: %w", err)
	}

	rulesHash, err := termsOfUseHash(parsed.TermsOfUse)
	if err != nil {
		return nil, fmt.Errorf("agreement credential: %w", err)
	}
	if rulesHash != federation.Hash() {
		return nil, fmt.Errorf("agreement credential: federation rules hash %q does not match this instance's own embedded hash %q",
			rulesHash, federation.Hash())
	}

	// Last, because the checks above establish whose credential this is and what
	// it agrees to, and this one establishes only that it is still current. Until
	// it was here the gate accepted a credential of any age, so a peer removed
	// from the federation kept a layer-3a credential that passed forever.
	if err := requireCredentialInForce(credential, time.Now()); err != nil {
		return nil, fmt.Errorf("agreement credential: %w", err)
	}

	return credential, nil
}

// consultPolicyEndpoint is layer 3b: the sole peer-authorization authority.
// Any non-2xx response, an unset PDPURL, or an unreachable endpoint all deny
// (fail-closed) — none is distinguished from another.
func (g *TrustGate) consultPolicyEndpoint(ctx context.Context, peerDID string, credential json.RawMessage, direction Direction, contractDID, targetState string) error {
	pdpURL := strings.TrimSpace(g.PDPURL)
	if pdpURL == "" {
		return fmt.Errorf("policy endpoint: DCS_TRUST_PDP_URL is not configured")
	}

	body, err := json.Marshal(map[string]interface{}{
		"peerDID":             peerDID,
		"agreementCredential": credential,
		"direction":           string(direction),
		"contractDID":         contractDID,
		"targetState":         targetState,
	})
	if err != nil {
		return fmt.Errorf("policy endpoint: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pdpURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("policy endpoint: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("policy endpoint: unreachable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("policy endpoint: denied (HTTP %d): %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// RecordDenialIncident persists one trust-gate rejection into the audit
// trail (ADR-19: "raises an incident in the audit trail"), regardless of
// which of the two layers (agreement credential or policy endpoint) denied
// the interaction — used by both the inbound (PostPdf) and outbound
// (shipContractPDF) call sites.
func RecordDenialIncident(ctx context.Context, db *sqlx.DB, contractDID string, direction Direction, gateErr *GateError) error {
	reporter := qry.TrustGateDenialReporter{DB: db}
	return reporter.Handle(ctx, denialQry(contractDID, direction, gateErr))
}

// RecordDenialIncidentTxDeduped records a trust-gate rejection in the
// caller's transaction, skipping the insert if an incident for the exact same
// (contract DID, peer DID, direction) was already recorded — a terminal
// policy-endpoint denial
// (PolicyFailure) can be reached more than once for the same underlying
// interaction (Offer and PDF_REGENERATED both firing a ship attempt for the
// same offer, milliseconds apart), and only the first must raise an incident.
func RecordDenialIncidentTxDeduped(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, contractDID string, direction Direction, gateErr *GateError) error {
	reporter := qry.TrustGateDenialReporter{DB: db}
	return reporter.HandleTxDeduped(ctx, tx, denialQry(contractDID, direction, gateErr))
}

func denialQry(contractDID string, direction Direction, gateErr *GateError) qry.TrustGateDenialQry {
	peerDID, reason := "", ""
	if gateErr != nil {
		peerDID = gateErr.PeerDID
		reason = gateErr.Error()
	}
	return qry.TrustGateDenialQry{
		DID:       contractDID,
		PeerDID:   peerDID,
		Direction: string(direction),
		Reason:    reason,
	}
}

// fetchAgreementCredential fetches a peer's agreement credential via
// /.well-known/dcs-agreement-credential.json under the peer's own did:web base,
// first over https, then falling back to http — the same resolution order
// identity.FetchDIDDocument uses for did.json.
func fetchAgreementCredential(peerDID string) (json.RawMessage, error) {
	host, segments, err := identity.DIDWebPath(peerDID)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, scheme := range identity.DIDWebSchemes(host) {
		url := identity.DIDWebBaseURL(scheme, host, segments) + "/.well-known/dcs-agreement-credential.json"
		body, err := fetchAgreementCredentialFromURL(host, url)
		if err == nil {
			return body, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("fetching agreement credential for %s failed: %w", peerDID, lastErr)
}

// didWebTarget is a did:web identifier reduced to what it actually resolves to,
// so two identifiers can be compared without being byte-identical strings.
func didWebTarget(did string) (string, error) {
	host, segments, err := identity.DIDWebPath(did)
	if err != nil {
		return "", err
	}
	return identity.DIDWebBaseURL("https", host, segments), nil
}

// The two clients the agreement-credential fetch uses. Its URL is built from a
// peer identifier this instance did not choose, so it is the same server-side
// request primitive the did.json fetch beside it is: safehttp refuses redirects
// (a peer must serve its credential at the address its identifier names, not
// point at one) and refuses to dial addresses no published peer lives on. The
// timeout matches the policy-endpoint POST (pdpTimeout) — an unresponsive peer
// must fail this out, not hang it.
var (
	agreementFetchClientStrict   = safehttp.Client(pdpTimeout, safehttp.Policy{})
	agreementFetchClientLoopback = safehttp.Client(pdpTimeout, safehttp.Policy{AllowLoopback: true})
)

// agreementFetchClient picks between them by re-reading the decision
// identity.DIDWebSchemes has already made for this host: it offers a second,
// plaintext scheme exactly for the loopback and explicitly named insecure hosts
// the dev and CI stacks resolve over http. Reading that back keeps one rule
// instead of a second one here that can drift away from it.
func agreementFetchClient(host string) *http.Client {
	if len(identity.DIDWebSchemes(host)) > 1 {
		return agreementFetchClientLoopback
	}
	return agreementFetchClientStrict
}

func fetchAgreementCredentialFromURL(host, url string) (json.RawMessage, error) {
	resp, err := agreementFetchClient(host).Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s from %s", resp.Status, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading agreement credential: %w", err)
	}
	return body, nil
}

// issuerIdentifier extracts the issuer DID, per W3C VC Data Model 2.0 §4.4
// either a bare string or an object carrying an "id".
func issuerIdentifier(raw json.RawMessage) (string, error) {
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString, nil
	}
	var asObject struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &asObject); err == nil && asObject.ID != "" {
		return asObject.ID, nil
	}
	return "", fmt.Errorf("credential carries no issuer")
}

// termsOfUseHash extracts the TrustFrameworkPolicy hash, per W3C VC Data
// Model 2.0 §5.5 termsOfUse may be a single object or an array of objects.
func termsOfUseHash(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", fmt.Errorf("credential carries no termsOfUse")
	}

	var single struct {
		Type string `json:"type"`
		Hash string `json:"hash"`
	}
	if err := json.Unmarshal(raw, &single); err == nil && single.Hash != "" {
		return single.Hash, nil
	}

	var list []struct {
		Type string `json:"type"`
		Hash string `json:"hash"`
	}
	if err := json.Unmarshal(raw, &list); err == nil {
		for _, entry := range list {
			if entry.Type == "TrustFrameworkPolicy" && entry.Hash != "" {
				return entry.Hash, nil
			}
		}
	}

	return "", fmt.Errorf("credential's termsOfUse carries no TrustFrameworkPolicy hash")
}
