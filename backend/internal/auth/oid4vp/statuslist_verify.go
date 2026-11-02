package oid4vp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"digital-contracting-service/internal/auth/oid4vp/sdjwt"
	"digital-contracting-service/internal/auth/oid4vp/status"
	"digital-contracting-service/internal/auth/oid4vp/status/handler"
)

var statusListVerifier *status.Verifier

// StatusListVerifier returns the verifier ConfigureStatusListVerification wired,
// for the paths outside OID4VP that also have to resolve a credential's status —
// the C2PA provenance verification, whose lifecycle credential names this
// deployment's own list (ADR-34). They share this one so an issuer trusted for a
// presented credential and an issuer trusted for a provenance credential cannot
// become two different sets. nil until configuration has run, which every caller
// must treat as "the state is unknown", never as "not revoked".
func StatusListVerifier() *status.Verifier { return statusListVerifier }

// ConfigureStatusListVerification wires the status-list verifier used by
// OID4VP, off the trust config already loaded rather than off a second read of
// the same file. The status-list path used to re-parse the trust document with
// its own struct, which had no purposes and no organizations fields — so the
// two paths could disagree about who is trusted without anything saying so.
func ConfigureStatusListVerification(trustCfg *TrustConfig) error {
	var trust *status.TrustConfig
	if trustCfg != nil {
		bundled := map[string]json.RawMessage{}
		for issuer, entry := range trustCfg.Issuers {
			bundled[issuer] = entry.JWKS
		}
		cfg, err := status.NewTrustConfig(bundled)
		if err != nil {
			return fmt.Errorf("status list trust config: %w", err)
		}
		// An issuer configured to publish its key by certificate chain bundles
		// no JWKS, so its status list is verified from the chain the token
		// carries. Without the anchors here that chain verifies against
		// nothing and the status list is refused, which is every status list
		// an x5c issuer signs.
		cfg.X5CRoots = trustCfg.unionX5CRoots()
		// Neither a bundled key nor an anchor means every status list this
		// deployment ever fetches resolves to no key and is refused. That is a
		// misconfiguration to report at startup, not one to discover at the
		// first login. Either alone is enough: our own issuers publish by
		// certificate and bundle no key at all (ADR-34).
		if len(cfg.Issuers) == 0 && cfg.X5CRoots == nil {
			return fmt.Errorf(
				"status list trust config: no issuer carries a bundled JWKS and no x5c trust anchors are configured, " +
					"so no status list could be verified")
		}
		trust = cfg
	}
	statusListVerifier = handler.NewVerifier(trust, handler.Options{})
	return nil
}

// statusReferenceClaims are the claims a status check reads: the credential's
// issuer, which the status list must match, and its status pointer.
var statusReferenceClaims = []string{"iss", "issuer", "status", "credentialStatus"}

// StatusReference keeps from verified credential claims only what a later
// status check of the same credential needs, so a session can carry it.
func StatusReference(rawClaims json.RawMessage) (json.RawMessage, error) {
	var claims map[string]json.RawMessage
	if err := json.Unmarshal(rawClaims, &claims); err != nil {
		return nil, fmt.Errorf("parse credential claims: %w", err)
	}
	ref := map[string]json.RawMessage{}
	for _, name := range statusReferenceClaims {
		if v, ok := claims[name]; ok {
			ref[name] = v
		}
	}
	return json.Marshal(ref)
}

// CheckStatus checks the current status of a credential again, from claims
// that StatusReference kept. A revoked or suspended credential is an error.
func CheckStatus(statusReference json.RawMessage) error {
	return checkStatusList(statusReference)
}

func checkStatusList(rawClaims json.RawMessage) error {
	result, err := resolveStatusList(context.Background(), rawClaims)
	if err != nil {
		return err
	}
	if !result.Accepted {
		return mapStatusListRejection(result)
	}
	return nil
}

func resolveStatusList(ctx context.Context, rawClaims json.RawMessage) (status.CredentialVerificationResult, error) {
	if statusListVerifier == nil {
		return status.CredentialVerificationResult{}, fmt.Errorf("status list verifier is not configured")
	}

	if len(rawClaims) == 0 {
		return status.CredentialVerificationResult{}, fmt.Errorf("credential claims are empty")
	}

	dec := json.NewDecoder(strings.NewReader(string(rawClaims)))
	dec.UseNumber()
	var claims map[string]any
	if err := dec.Decode(&claims); err != nil {
		return status.CredentialVerificationResult{}, fmt.Errorf("parse credential claims for status list check: %w", err)
	}

	result, err := statusListVerifier.VerifyStatus(ctx, status.VerifiedCredential{
		Format: "sd-jwt",
		Claims: claims,
	})
	if err != nil {
		return status.CredentialVerificationResult{}, fmt.Errorf("status list check: %w", err)
	}
	return result, nil
}

// PresentedCredentialState reads, live, the status-list entry of the credential
// in a presentation that a signing ceremony received and kept. The issuer
// signature and the issuer trust for the purpose are verified again first,
// because the entry points wherever the credential author chose. The KB-JWT is
// not verified: its nonce belongs to a ceremony that is complete.
func PresentedCredentialState(ctx context.Context, presentation string, trust *TrustConfig, purpose Purpose) (status.State, error) {
	if trust == nil {
		return "", fmt.Errorf("trust config is not configured")
	}
	parsed, err := sdjwt.ParsePresentation(strings.TrimSpace(presentation))
	if err != nil {
		return "", err
	}
	var chain []poaLink
	if purpose == PurposePID {
		// A PID names a natural person: no organization, no roles, no
		// delegation.
		claims, err := sdjwt.VerifyCredential(parsed.IssuerJWT, parsed.Disclosures, trust.For(purpose))
		if err != nil {
			return "", err
		}
		raw, err := json.Marshal(claims)
		if err != nil {
			return "", err
		}
		chain = []poaLink{{statusClaims: raw}}
	} else if chain, err = verifyPoAChain(parsed.IssuerJWT, parsed.Disclosures, trust, purpose); err != nil {
		return "", err
	}
	// A delegated credential is as revoked as the most severe state of any
	// link of its delegation chain.
	var results []status.Result
	var rejected *status.CredentialVerificationResult
	for _, link := range chain {
		result, err := resolveStatusList(ctx, link.statusClaims)
		if err != nil {
			return "", err
		}
		if len(result.StatusResults) == 0 {
			return "", fmt.Errorf("status list check: %s", result.Reason)
		}
		results = append(results, result.StatusResults...)
		if !result.Accepted && rejected == nil {
			rejected = &result
		}
	}
	if reference := mostSevereStatus(results); reference.State != status.StateValid {
		return reference.State, nil
	}
	if rejected != nil {
		return "", mapStatusListRejection(*rejected)
	}
	return status.StateValid, nil
}

// mostSevereStatus picks a revoked entry over a suspended one, a suspended one
// over any other non-valid one, and that over a valid one. On a tie the earlier
// entry wins. results must not be empty.
func mostSevereStatus(results []status.Result) status.Result {
	rank := func(state status.State) int {
		switch state {
		case status.StateInvalid:
			return 3
		case status.StateSuspended:
			return 2
		case status.StateValid:
			return 0
		}
		return 1
	}
	worst := results[0]
	for _, result := range results[1:] {
		if rank(result.State) > rank(worst.State) {
			worst = result
		}
	}
	return worst
}

func mapStatusListRejection(result status.CredentialVerificationResult) error {
	if len(result.StatusResults) > 0 {
		ref := mostSevereStatus(result.StatusResults)
		switch ref.State {
		case status.StateInvalid:
			return fmt.Errorf("credential status list index %d is revoked", ref.Index)
		case status.StateSuspended:
			return fmt.Errorf("credential status list index %d is suspended", ref.Index)
		}
	}
	if reason := strings.TrimSpace(result.Reason); reason != "" {
		return fmt.Errorf("status list check: %s", reason)
	}
	return fmt.Errorf("status list check: credential rejected")
}
