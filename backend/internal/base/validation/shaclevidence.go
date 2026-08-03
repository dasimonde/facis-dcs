package validation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// SHACLEvidence runs the Semantic Hub SHACL validation pass for a document
// and returns the shapes version it validated against (the sh:shapesGraph
// pin when present, otherwise the active version) and a stable hash of the
// findings. Embedded into signed evidence: a verifier resolves
// sh:shapesGraph, re-runs validation, and compares hashes.
func SHACLEvidence(ctx context.Context, contractDocument any) (schemaVersion int, reportHash string, err error) {
	contract, err := normalizeObject(contractDocument)
	if err != nil {
		return 0, "", err
	}
	findings, version, err := validateAgainstHubShapes(ctx, contract)
	if err != nil {
		return 0, "", err
	}
	return version, ValidationReportHash(findings), nil
}

// RequireHubConformance blocks a document that violates the Semantic Hub
// shapes graphs it declares (the canonical shapes with the clause catalog,
// plus any registered library it names): error-severity findings
// fail it with every violation spelled out; warnings/info pass. The gate
// runs at contract submission and at signature application (DCS-FR-TR-20,
// DCS-FR-PACM-03: non-conformant contracts MUST NOT proceed toward
// execution or signing).
func RequireHubConformance(ctx context.Context, contractDocument any) error {
	contract, err := normalizeObject(contractDocument)
	if err != nil {
		return err
	}
	findings, version, err := validateAgainstHubShapes(ctx, contract)
	if err != nil {
		return err
	}
	var violations []string
	for _, f := range findings {
		if f.Severity == "error" {
			violations = append(violations, fmt.Sprintf("%s: %s", f.RuleID, f.Message))
		}
	}
	if len(violations) == 0 {
		return nil
	}
	sort.Strings(violations)
	return fmt.Errorf("document violates Semantic Hub shapes (version %d): %s", version, strings.Join(violations, "; "))
}

// ValidationReportHash computes a stable SHA-256 hash (hex) of a set of
// SHACL findings — deterministic regardless of the order goRDFlib produced
// them in, so the same document validated twice against the same hub
// version always hashes identically.
func ValidationReportHash(findings []PolicyFinding) string {
	type reportEntry struct {
		RuleID   string `json:"ruleId"`
		Severity string `json:"severity"`
		Path     string `json:"path"`
		Message  string `json:"message"`
	}
	entries := make([]reportEntry, 0, len(findings))
	for _, f := range findings {
		entries = append(entries, reportEntry{RuleID: f.RuleID, Severity: f.Severity, Path: f.Path, Message: f.Message})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].RuleID != entries[j].RuleID {
			return entries[i].RuleID < entries[j].RuleID
		}
		return entries[i].Path < entries[j].Path
	})
	// Errors are never expected to be raised by json.Marshal on this
	// concrete, non-cyclic struct slice.
	canonical, _ := json.Marshal(entries)
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:])
}
