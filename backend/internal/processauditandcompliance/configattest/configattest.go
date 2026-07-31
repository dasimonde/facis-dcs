// Package configattest authenticates and integrity-protects the file-based
// configuration the DCS runs on (DCS-NFR-SEC-04): at startup every
// security-critical mounted config file is hashed, the hashes are recorded as
// a CONFIG_INTEGRITY_ATTESTATION audit event (the requirement's
// "configuration integrity verification log"), and operator-pinned expected
// hashes (DCS_CONFIG_SHA256_PINS) turn the attestation into an enforced
// authentication: a pinned file that is missing or does not match its pin
// aborts startup.
package configattest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"

	"github.com/jmoiron/sqlx"
)

// AttestationEvent is the audit record of one startup's config verification:
// which files were present and their SHA-256 hashes, plus whether pins were
// enforced.
type AttestationEvent struct {
	DID        string            `json:"did"`
	FileHashes map[string]string `json:"file_hashes"`
	PinnedKeys []string          `json:"pinned_keys,omitempty"`
	OccurredAt time.Time         `json:"occurred_at"`
}

func (e AttestationEvent) EventType() string { return "CONFIG_INTEGRITY_ATTESTATION" }
func (e AttestationEvent) GetDID() string    { return e.DID }

// HashFiles hashes every config file in files (name → path). Entries with an
// empty path (unset optional config) are skipped; a set path that cannot be
// read is a hard error — a mounted config file the process depends on must
// be readable, or the deployment is broken.
func HashFiles(files map[string]string) (map[string]string, error) {
	hashes := map[string]string{}
	for name, path := range files {
		if strings.TrimSpace(path) == "" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("config file %q (%s) is not readable: %w", name, path, err)
		}
		sum := sha256.Sum256(data)
		hashes[name] = hex.EncodeToString(sum[:])
	}
	return hashes, nil
}

// ParsePins parses DCS_CONFIG_SHA256_PINS ("name=hexhash,name=hexhash").
// Malformed input is a hard error: a deployment that tries to pin its config
// but gets the syntax wrong must not start with silently unenforced pins.
func ParsePins(raw string) (map[string]string, error) {
	pins := map[string]string{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return pins, nil
	}
	for _, part := range strings.Split(raw, ",") {
		name, hash, ok := strings.Cut(strings.TrimSpace(part), "=")
		name, hash = strings.TrimSpace(name), strings.ToLower(strings.TrimSpace(hash))
		if !ok || name == "" || len(hash) != 64 {
			return nil, fmt.Errorf("DCS_CONFIG_SHA256_PINS entry %q is not name=<64-hex-sha256>", part)
		}
		if _, err := hex.DecodeString(hash); err != nil {
			return nil, fmt.Errorf("DCS_CONFIG_SHA256_PINS entry %q carries a non-hex hash", part)
		}
		pins[name] = hash
	}
	return pins, nil
}

// VerifyPins checks every operator pin against the computed hashes. Every
// pinned name must have been hashed (the file must exist) and match exactly.
func VerifyPins(hashes, pins map[string]string) error {
	var problems []string
	for name, pinned := range pins {
		actual, ok := hashes[name]
		if !ok {
			problems = append(problems, fmt.Sprintf("pinned config %q is not present", name))
			continue
		}
		if actual != pinned {
			problems = append(problems, fmt.Sprintf("config %q hash %s does not match its pin %s", name, actual, pinned))
		}
	}
	if len(problems) > 0 {
		return errors.New("config integrity verification failed: " + strings.Join(problems, "; "))
	}
	return nil
}

// Attest runs the startup config verification: hash, enforce pins, and record
// the attestation in the transactional outbox (anchored to IPFS/TSA by the
// outbox processor like every other audit event).
func Attest(ctx context.Context, db *sqlx.DB, instanceDID string, files map[string]string, pinsEnv string) error {
	hashes, err := HashFiles(files)
	if err != nil {
		return err
	}
	pins, err := ParsePins(pinsEnv)
	if err != nil {
		return err
	}
	if err := VerifyPins(hashes, pins); err != nil {
		return err
	}

	pinnedKeys := make([]string, 0, len(pins))
	for name := range pins {
		pinnedKeys = append(pinnedKeys, name)
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := event.Create(ctx, tx, AttestationEvent{
		DID:        instanceDID,
		FileHashes: hashes,
		PinnedKeys: pinnedKeys,
		OccurredAt: time.Now().UTC(),
	}, componenttype.ProcessAuditAndCompliance); err != nil {
		return fmt.Errorf("could not record config attestation: %w", err)
	}
	return tx.Commit()
}
