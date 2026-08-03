package provenance

import (
	"fmt"
	"net/url"
	"strings"
)

// StatusListIssuerURL derives this deployment's issuer identifier — the `iss` of
// every status list it serves — from its public API URL (DCS_PUBLIC_URL).
//
// It is the origin alone, with the API path dropped, because the list is served
// at the origin root the way did.json is: a verifier that holds only a
// credential has the URL the credential names and nothing else, and reaching it
// must not depend on knowing this deployment's API prefix.
func StatusListIssuerURL(publicAPIURL string) (string, error) {
	raw := strings.TrimSpace(publicAPIURL)
	if raw == "" {
		return "", fmt.Errorf("no public URL configured: a status list has no issuer identifier to name")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("public URL %q is not a URL: %w", raw, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("public URL %q is not absolute, so it names no origin a verifier could reach", raw)
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

// StatusListURI is the absolute URI of one served list: what credentials name,
// what a verifier fetches, and what the token itself carries in `sub`. All three
// come from here so they cannot be written differently in three places — the
// verifier refuses the list outright when `sub` and the fetched URI differ.
func StatusListURI(issuerURL string, listID int) string {
	return fmt.Sprintf("%s%s%d", strings.TrimRight(issuerURL, "/"), StatusListPath, listID)
}
