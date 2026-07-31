package provenance

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// StatusListPublisher publishes contract status to a status list service (DCS-OR-C2PA-005).
// This integrates with XFSC's OCM-W Status List Service to maintain a verifiable
// status list (Status List 2021/2023 format) with ≤ 5 minute update latency.
type StatusListPublisher interface {
	// PublishStatus updates the contract status in the status list.
	// Returns the entry the contract's credentials must advertise — the same
	// entry a revocation flips — and any error.
	PublishStatus(
		ctx context.Context,
		contractID string,
		status string, // "active", "suspended", "terminated", "expired", etc.
		reason string,
		effectiveAt time.Time,
	) (entry CredentialStatusRef, err error)

	// RevokeStatus marks a contract as revoked in the status list.
	RevokeStatus(ctx context.Context, contractID string) (entry CredentialStatusRef, err error)
}

// listSize is the number of entries in a standard 16 KB bitstring status list (2^17).
// It is the size the statuslist-service is deployed with; the bound an
// allocation is actually held to lives per list in status_list_cursors.
const listSize = 131072

// DefaultListID is the list contract revocation entries were allocated in
// before any rollover (1-indexed), and the one migration 20260734 registers.
const DefaultListID = 1

// statusListEntryType is the credentialStatus.type a contract VC advertises: a
// token status list, which is what the XFSC statuslist-service serves (see
// QueryStatusListStatus for the format and its LSB-first bit order).
const statusListEntryType = "TokenStatusList"

// OCMWStatusListPublisher is a client for the XFSC statuslist-service.
// It calls POST /v1/tenants/{tenantID}/status/{listID}/revoke/{index} to revoke entries.
// The status list is available at GET /v1/tenants/{tenantID}/status/{listID}.
//
// Which entry a contract owns is not derivable from the contract id: it is
// allocated once and read back from the allocator every time, so the entry a
// credential advertises and the entry a revocation flips cannot drift apart,
// and no two contracts can end up sharing one.
type OCMWStatusListPublisher struct {
	// ServiceURL is the statuslist-service root endpoint (e.g., http://statuslist:8080).
	ServiceURL string

	// IssuerDID is the issuer DID that owns the status list.
	IssuerDID string

	// TenantID is the tenant identifier in the statuslist-service path (default "default").
	TenantID string

	entries *StatusListAllocator

	client *http.Client
}

// NewOCMWStatusListPublisher creates a status list publisher that calls the
// XFSC statuslist-service HTTP API.  tenantID may be empty, defaulting to "default".
// entries must not be nil: without it a contract has no revocation entry, and
// guessing one is the defect this parameter exists to remove.
func NewOCMWStatusListPublisher(serviceURL, issuerDID, tenantID string, entries *StatusListAllocator) *OCMWStatusListPublisher {
	if tenantID == "" {
		tenantID = "default"
	}
	return &OCMWStatusListPublisher{
		ServiceURL: serviceURL,
		IssuerDID:  issuerDID,
		TenantID:   tenantID,
		entries:    entries,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

// statusListURI returns the URL at which the status list holding entry can be fetched.
func (p *OCMWStatusListPublisher) statusListURI(entry StatusListEntry) string {
	return fmt.Sprintf("%s/v1/tenants/%s/status/%d", p.ServiceURL, p.TenantID, entry.ListID)
}

// entryFor returns the contract's allocated status list entry as the reference
// a credential advertises.
func (p *OCMWStatusListPublisher) entryFor(ctx context.Context, contractID string) (StatusListEntry, CredentialStatusRef, error) {
	if p.entries == nil {
		return StatusListEntry{}, CredentialStatusRef{},
			fmt.Errorf("status list publisher has no entry allocator: required to place %s in the status list", contractID)
	}
	entry, err := p.entries.Allocate(ctx, contractID)
	if err != nil {
		return StatusListEntry{}, CredentialStatusRef{}, err
	}
	return entry, CredentialStatusRef{StatusListCredential: p.statusListURI(entry), Index: entry.Index}, nil
}

// revokeResponse is the JSON shape returned by the statuslist-service revoke endpoint.
type revokeResponse struct {
	TenantID string `json:"tenantId"`
	ListID   int    `json:"listId"`
	Index    int    `json:"index"`
	Status   string `json:"status"`
}

// setRevoked calls POST /{tenantID}/status/{listID}/revoke/{index} for the
// contract's allocated entry — the same entry its credentials advertise.
// ServiceURL must be non-empty; an empty URL is a hard failure (DCS hard-failure policy).
func (p *OCMWStatusListPublisher) setRevoked(ctx context.Context, contractID string, entry StatusListEntry) error {
	if p.ServiceURL == "" {
		return fmt.Errorf("status list ServiceURL must not be empty: required for revocation of %s", contractID)
	}
	url := fmt.Sprintf("%s/v1/tenants/%s/status/%d/revoke/%d", p.ServiceURL, p.TenantID, entry.ListID, entry.Index)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(nil))
	if err != nil {
		return fmt.Errorf("build revoke request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", url, err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Println("could not close response body:", err)
		}
	}(resp.Body)

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("statuslist-service revoke returned %d: %s", resp.StatusCode, body)
	}

	var r revokeResponse
	if err := json.Unmarshal(body, &r); err == nil {
		_ = r // parsed for logging; ignore unmarshal errors on unexpected shapes
	}
	return nil
}

// PublishStatus updates the contract status in the XFSC status list (DCS-OR-C2PA-005).
// Terminal states (terminated, expired, replaced, suspended) set the revocation bit.
// Comparison is case-insensitive so CWE UPPERCASE states (TERMINATED, EXPIRED, …)
// are handled correctly alongside the SRS lowercase vocabulary.
// Active/draft/amended states are the default (not revoked) and require no HTTP call.
func (p *OCMWStatusListPublisher) PublishStatus(
	ctx context.Context,
	contractID string,
	status string,
	reason string,
	effectiveAt time.Time,
) (CredentialStatusRef, error) {
	entry, ref, err := p.entryFor(ctx, contractID)
	if err != nil {
		return CredentialStatusRef{}, fmt.Errorf("publish status %s for %s: %w", status, contractID, err)
	}

	switch strings.ToLower(status) {
	case "terminated", "expired", "replaced", "suspended":
		if err := p.setRevoked(ctx, contractID, entry); err != nil {
			return CredentialStatusRef{}, fmt.Errorf("publish status %s for %s: %w", status, contractID, err)
		}
	}
	// active, draft, approved, amended — default state = not revoked, no action required.
	return ref, nil
}

// statusListResponse is the JSON shape actually returned by the deployed XFSC
// statuslist-service for GET /v1/tenants/{tenant}/status/{listId}:
//
//	{"list": "<base64, gzip-compressed bitstring>", "listId": 1, "tenantId": "default"}
//
// This is NOT a W3C VC (no credentialSubject wrapper).
type statusListResponse struct {
	List string `json:"list"`
}

// QueryStatusListStatus fetches the status list at statusListCredential and returns
// "revoked" if the entry at index is set, "active" otherwise (DCS-OR-C2PA-006).
//
// The XFSC statuslist-service (deployment/helm/charts/statuslist-service) returns a
// plain {"list": "...", "listId": ..., "tenantId": "..."} JSON object rather than a
// W3C VC; "list" is a base64-encoded, gzip-compressed bitstring. Bit packing
// follows the IETF Token Status List / XFSC convention (LSB-first), matching the
// parsing already established for status list checks in internal/auth/oid4vp.
func QueryStatusListStatus(ctx context.Context, client *http.Client, statusListCredential string, index uint32) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, statusListCredential, nil)
	if err != nil {
		return "", fmt.Errorf("build status list request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("GET %s: %w", statusListCredential, err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Println("could not close response body:", err)
		}
	}(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status list service returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read status list response: %w", err)
	}

	var sl statusListResponse
	if err := json.Unmarshal(body, &sl); err != nil {
		return "", fmt.Errorf("parse status list response: %w", err)
	}

	encoded := strings.TrimSpace(sl.List)
	if encoded == "" {
		return "", fmt.Errorf("status list response has no list field")
	}

	bitstring, err := decodeAndDecompressStatusList(encoded)
	if err != nil {
		return "", err
	}

	byteIdx := index / 8
	if int(byteIdx) >= len(bitstring) {
		return "", fmt.Errorf("index %d out of range for bitstring of %d bytes", index, len(bitstring))
	}
	// IETF Token Status List / XFSC statuslist-service convention: LSB-first —
	// bit N is at bit (N%8) of byte N/8.
	bitIdx := uint(index % 8)
	if bitstring[byteIdx]&(1<<bitIdx) != 0 {
		return "revoked", nil
	}
	return "active", nil
}

// decodeAndDecompressStatusList base64-decodes encoded (accepting both padded/
// unpadded and standard/url-safe alphabets, since deployments have been
// observed to disagree on this detail) and gzip-decompresses the result
// (the XFSC statuslist-service's only compression format).
func decodeAndDecompressStatusList(encoded string) ([]byte, error) {
	compressed, err := decodeStatusListBase64(encoded)
	if err != nil {
		return nil, fmt.Errorf("base64 decode status list: %w", err)
	}

	r, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("create gzip reader for bitstring: %w", err)
	}
	defer func(r io.ReadCloser) {
		if err := r.Close(); err != nil {
			log.Printf("close gzip reader for bitstring: %v", err)
		}
	}(r)
	out, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("decompress gzip bitstring: %w", err)
	}
	return out, nil
}

// decodeStatusListBase64 tries the base64 variants seen across StatusList2021
// (base64url, unpadded) and the XFSC statuslist-service (standard, padded).
func decodeStatusListBase64(s string) ([]byte, error) {
	var lastErr error
	for _, enc := range []*base64.Encoding{
		base64.RawURLEncoding,
		base64.StdEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
	} {
		if b, err := enc.DecodeString(s); err == nil {
			return b, nil
		} else {
			lastErr = err
		}
	}
	return nil, lastErr
}

// RevokeStatus marks the contract as revoked in the status list (DCS-OR-C2PA-005).
func (p *OCMWStatusListPublisher) RevokeStatus(ctx context.Context, contractID string) (CredentialStatusRef, error) {
	entry, ref, err := p.entryFor(ctx, contractID)
	if err != nil {
		return CredentialStatusRef{}, fmt.Errorf("revoke %s: %w", contractID, err)
	}
	if err := p.setRevoked(ctx, contractID, entry); err != nil {
		return CredentialStatusRef{}, fmt.Errorf("revoke %s: %w", contractID, err)
	}
	return ref, nil
}

// CredentialStatusRef locates one credential's entry in a status list.
type CredentialStatusRef struct {
	StatusListCredential string
	Index                uint32
}

// ExtractCredentialStatus reads the revocation entry a VC advertises.
//
// Three outcomes, and the difference between the last two is the whole point of
// the signature: a VC carrying no credentialStatus has nothing to check
// (present=false); a VC that advertises one this build cannot read is a VC whose
// revocation state is UNKNOWN (err), which the caller must report as a finding.
// One "not ok" for both made an unreadable entry indistinguishable from an
// absent one, so a caller skipped the revocation check silently — a fail-open on
// a malformed or unsupported entry, which is exactly the entry an attacker
// controls.
func ExtractCredentialStatus(vcBytes []byte) (ref CredentialStatusRef, present bool, err error) {
	var vcObj map[string]interface{}
	if err := json.Unmarshal(vcBytes, &vcObj); err != nil {
		return CredentialStatusRef{}, true, fmt.Errorf("credential is not readable JSON: %w", err)
	}
	csRaw, exists := vcObj["credentialStatus"]
	if !exists || csRaw == nil {
		return CredentialStatusRef{}, false, nil
	}
	cs, ok := csRaw.(map[string]interface{})
	if !ok {
		return CredentialStatusRef{}, true, fmt.Errorf("credentialStatus is not an object")
	}
	cred, _ := cs["statusListCredential"].(string)
	indexStr, _ := cs["statusListIndex"].(string)
	if strings.TrimSpace(cred) == "" || strings.TrimSpace(indexStr) == "" {
		return CredentialStatusRef{}, true, fmt.Errorf("credentialStatus names no statusListCredential and statusListIndex")
	}
	idx, parseErr := strconv.ParseUint(strings.TrimSpace(indexStr), 10, 32)
	if parseErr != nil {
		return CredentialStatusRef{}, true, fmt.Errorf("credentialStatus statusListIndex %q is not an index", indexStr)
	}
	return CredentialStatusRef{StatusListCredential: cred, Index: uint32(idx)}, true, nil
}

// ExtractStatusListURI extracts the credentialStatus.id from the VC JSON.
func ExtractStatusListURI(vcBytes []byte) string {
	var vcObj map[string]interface{}
	if err := json.Unmarshal(vcBytes, &vcObj); err != nil {
		return ""
	}
	credStatusRaw, ok := vcObj["credentialStatus"]
	if !ok {
		return ""
	}
	credStatusObj, ok := credStatusRaw.(map[string]interface{})
	if !ok {
		return ""
	}
	uri, _ := credStatusObj["id"].(string)
	return uri
}
