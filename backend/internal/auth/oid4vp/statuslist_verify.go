package oid4vp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"digital-contracting-service/internal/auth/oid4vp/status"
	"digital-contracting-service/internal/auth/oid4vp/status/handler"
)

var statusListVerifier *status.Verifier

// ConfigureStatusListVerification wires the status-list verifier used by
// OID4VP, off the trust config already loaded rather than off a second read of
// the same file. The status-list path used to re-parse the trust document with
// its own struct, which had no purposes and no organizations fields — so the
// two paths could disagree about who is trusted without anything saying so.
func ConfigureStatusListVerification(
	trustCfg *TrustConfig,
	xfscAllowUnsignedFallback bool,
) error {
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
		trust = cfg
	}

	statusListVerifier = handler.NewVerifier(trust, handler.Options{
		XFSCAllowUnsignedFallback: xfscAllowUnsignedFallback,
	})
	return nil
}

func checkStatusList(rawClaims json.RawMessage) error {
	if statusListVerifier == nil {
		return fmt.Errorf("status list verifier is not configured")
	}

	if len(rawClaims) == 0 {
		return fmt.Errorf("credential claims are empty")
	}

	dec := json.NewDecoder(strings.NewReader(string(rawClaims)))
	dec.UseNumber()
	var claims map[string]any
	if err := dec.Decode(&claims); err != nil {
		return fmt.Errorf("parse credential claims for status list check: %w", err)
	}

	result, err := statusListVerifier.VerifyStatus(context.Background(), status.VerifiedCredential{
		Format: "sd-jwt",
		Claims: claims,
	})
	if err != nil {
		return fmt.Errorf("status list check: %w", err)
	}
	if !result.Accepted {
		return mapStatusListRejection(result)
	}
	return nil
}

func mapStatusListRejection(result status.CredentialVerificationResult) error {
	if len(result.StatusResults) > 0 {
		ref := result.StatusResults[0]
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
