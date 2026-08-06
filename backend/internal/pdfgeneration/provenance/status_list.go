package provenance

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// StatusListPublisher publishes contract status to the status list this
// deployment serves (DCS-OR-C2PA-005).
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

// ListSize is the number of entries in a status list this deployment serves
// (2^17, a 16 KB bitstring). The bound an allocation is actually held to lives
// per list in status_list_cursors; this is the size list 1 is registered with.
const ListSize = 131072

// DefaultListID is the list contract revocation entries were allocated in
// before any rollover (1-indexed), and the one migration 20260734 registers.
const DefaultListID = 1

// statusListEntryType is the credentialStatus.type a contract VC advertises: a
// token status list, which is what the URI serves — a signed statuslist+jwt
// whose status_list.lst is a zlib-compressed, LSB-first bitstring.
const statusListEntryType = "TokenStatusList"

// StatusListRevocations is the persistence a served status list is built from:
// which of this deployment's allocated entries carry a set bit.
type StatusListRevocations interface {
	// Revoke sets subjectID's bit. Revoking an already-revoked subject keeps
	// the original moment: a status list is asked after the fact when a
	// credential stopped being valid, and re-stamping it on every republished
	// terminal state would move that answer.
	Revoke(ctx context.Context, subjectID string) error

	// RevokedIndices returns the entry indices of listID whose bit is set.
	RevokedIndices(ctx context.Context, listID int) ([]uint32, error)
}

// DCSStatusListPublisher places a contract in the status list this deployment
// issues, serves and signs (ADR-34).
//
// The DCS mints the contract lifecycle credential and the signing summary
// credential, so by the rule ADR-34 is named after it is the party that
// publishes their revocation status. Both the allocation and the bit live in
// this deployment's own database; nothing is POSTed to a separate service that
// would then have to be trusted to still hold the bit — and to say so under a
// signature — when a verifier asks.
//
// Which entry a contract owns is not derivable from the contract id: it is
// allocated once and read back from the allocator every time, so the entry a
// credential advertises and the entry a revocation flips cannot drift apart,
// and no two contracts can end up sharing one.
type DCSStatusListPublisher struct {
	// ListURI resolves a list id to the absolute URI credentials name and a
	// verifier fetches — the same URI the served token carries in `sub`, which
	// the verifier refuses if the two differ.
	ListURI func(listID int) string

	entries     *StatusListAllocator
	revocations StatusListRevocations
}

// NewDCSStatusListPublisher creates the publisher for the list this deployment
// serves itself. entries and revocations must not be nil: without the first a
// contract has no revocation entry, and without the second a terminal contract
// state would set no bit while every credential kept advertising one.
func NewDCSStatusListPublisher(
	listURI func(listID int) string,
	entries *StatusListAllocator,
	revocations StatusListRevocations,
) *DCSStatusListPublisher {
	return &DCSStatusListPublisher{ListURI: listURI, entries: entries, revocations: revocations}
}

// StatusListEntries hands out the status list entry a subject's credentials
// advertise, without asserting anything about the subject's lifecycle state.
//
// It exists for the credentials a contract accumulates besides the lifecycle
// one — the signing summary above all. They must name the SAME entry: the bit
// says whether the contract is in force, and a summary credential pointing
// somewhere else would keep reading valid after the contract it evidences was
// terminated. Issuing one must not, however, publish a lifecycle status, which
// is why this is not PublishStatus.
type StatusListEntries interface {
	EntryFor(ctx context.Context, subjectID string) (CredentialStatusRef, error)
}

// EntryFor returns the contract's allocated status list entry as the reference
// a credential advertises, allocating one the first time it is asked.
func (p *DCSStatusListPublisher) EntryFor(ctx context.Context, contractID string) (CredentialStatusRef, error) {
	return p.entryFor(ctx, contractID)
}

// entryFor returns the contract's allocated status list entry as the reference
// a credential advertises.
func (p *DCSStatusListPublisher) entryFor(ctx context.Context, contractID string) (CredentialStatusRef, error) {
	if p.entries == nil {
		return CredentialStatusRef{},
			fmt.Errorf("status list publisher has no entry allocator: required to place %s in the status list", contractID)
	}
	if p.ListURI == nil {
		return CredentialStatusRef{},
			fmt.Errorf("status list publisher has no list URI: %s would advertise an entry in a list nobody can fetch", contractID)
	}
	entry, err := p.entries.Allocate(ctx, contractID)
	if err != nil {
		return CredentialStatusRef{}, err
	}
	return CredentialStatusRef{StatusListCredential: p.ListURI(entry.ListID), Index: entry.Index}, nil
}

// PublishStatus records the contract status in the status list
// (DCS-OR-C2PA-005). Terminal states (terminated, expired, replaced, suspended)
// set the revocation bit. Comparison is case-insensitive so CWE UPPERCASE
// states (TERMINATED, EXPIRED, …) are handled correctly alongside the SRS
// lowercase vocabulary. Active/draft/approved/amended states are the default —
// not revoked — and set nothing.
func (p *DCSStatusListPublisher) PublishStatus(
	ctx context.Context,
	contractID string,
	status string,
	_ string,
	_ time.Time,
) (CredentialStatusRef, error) {
	ref, err := p.entryFor(ctx, contractID)
	if err != nil {
		return CredentialStatusRef{}, fmt.Errorf("publish status %s for %s: %w", status, contractID, err)
	}

	switch strings.ToLower(status) {
	case "terminated", "expired", "replaced", "suspended":
		if err := p.setRevoked(ctx, contractID); err != nil {
			return CredentialStatusRef{}, fmt.Errorf("publish status %s for %s: %w", status, contractID, err)
		}
	}
	return ref, nil
}

// RevokeStatus marks the contract as revoked in the status list (DCS-OR-C2PA-005).
func (p *DCSStatusListPublisher) RevokeStatus(ctx context.Context, contractID string) (CredentialStatusRef, error) {
	ref, err := p.entryFor(ctx, contractID)
	if err != nil {
		return CredentialStatusRef{}, fmt.Errorf("revoke %s: %w", contractID, err)
	}
	if err := p.setRevoked(ctx, contractID); err != nil {
		return CredentialStatusRef{}, fmt.Errorf("revoke %s: %w", contractID, err)
	}
	return ref, nil
}

func (p *DCSStatusListPublisher) setRevoked(ctx context.Context, contractID string) error {
	if p.revocations == nil {
		return fmt.Errorf("status list publisher has no revocation store: the bit for %s would never be set", contractID)
	}
	return p.revocations.Revoke(ctx, contractID)
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
