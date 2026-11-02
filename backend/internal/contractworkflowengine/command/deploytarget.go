package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/base/safehttp"
)

// ContractTargetClient dispatches a deployment payload to the configured
// Contract Target System (UC-05-01). Implementations are best-effort: the
// deploy command persists the deployment record and returns its response
// regardless of whether the outbound call below succeeds, since the target
// system's own acknowledgement (POST /contract/deployment/callback) is the
// authoritative signal of a successful deployment (DCS-FR-SM-12).
type ContractTargetClient interface {
	// DeployTo posts the payload to one registered target's endpoint. The URL is
	// a parameter rather than client state because a DCS may serve several
	// execution environments and each contract names its own (ADR-25).
	DeployTo(ctx context.Context, url string, payload map[string]any) error
}

// ErrInvalidTargetURL refuses a target URL that the signed contract may not
// be posted to.
var ErrInvalidTargetURL = errors.New("invalid target URL")

// ValidateTargetURL accepts an absolute https URL. A target receives the
// signed contract and its signatures (DCS-IR-CI-01), so plain http is
// accepted only for a host that did:web may be resolved from over http:
// loopback, or a host DCS_DIDWEB_INSECURE_HOSTS names (identity.DIDWebSchemes).
func ValidateTargetURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("%w: %q is not an absolute URL", ErrInvalidTargetURL, raw)
	}
	switch parsed.Scheme {
	case "https":
		return nil
	case "http":
		if slices.Contains(identity.DIDWebSchemes(parsed.Host), "http") {
			return nil
		}
		return fmt.Errorf("%w: %q uses plain http; a target receives the signed contract, so only loopback and the hosts DCS_DIDWEB_INSECURE_HOSTS names may use http",
			ErrInvalidTargetURL, raw)
	}
	return fmt.Errorf("%w: %q does not use https", ErrInvalidTargetURL, raw)
}

// HTTPContractTargetClient POSTs the deployment payload to a configured URL
// (deployment/helm/charts/orce/flows/contract-target-flow.json is the
// reference implementation of the receiving side). It follows no redirect and
// dials loopback only for a host ValidateTargetURL allows plain http for.
type HTTPContractTargetClient struct {
	strict   *http.Client
	loopback *http.Client
}

func NewHTTPContractTargetClient() *HTTPContractTargetClient {
	return &HTTPContractTargetClient{
		strict:   safehttp.Client(10*time.Second, safehttp.Policy{}),
		loopback: safehttp.Client(10*time.Second, safehttp.Policy{AllowLoopback: true}),
	}
}

func (c *HTTPContractTargetClient) DeployTo(ctx context.Context, target string, payload map[string]any) error {
	if err := ValidateTargetURL(target); err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal deployment payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSpace(target), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create deployment request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := c.strict
	if slices.Contains(identity.DIDWebSchemes(req.URL.Host), "http") {
		client = c.loopback
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("post deployment payload: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("contract target returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}
