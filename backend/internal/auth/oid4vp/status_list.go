package oid4vp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"goa.design/clue/log"
)

// Status list entry kinds: credentialStatus.type.
const (
	statusKindBitstringStatusList = "BitstringStatusListEntry"
	statusKindStatusList2021      = "StatusList2021Entry"
	statusKindJWTStatusList       = "JWTStatusListEntry"
	statusKindTokenStatusList     = "TokenStatusList"
)

const (
	defaultStatusSize         uint32 = 1
	maxSupportedStatusSize    uint32 = 32
	maxStatusListResponseSize        = 16 << 20 // 16 MiB compressed/JSON/JWT response cap.
)

var (
	defaultStatusListHTTPClient = &http.Client{Timeout: 10 * time.Second}
	statusListSkipIfUnavailable bool
)

// StatusListUnavailableError indicates the status list credential URL could not be fetched.
type StatusListUnavailableError struct {
	URI string
	Err error
}

func (e *StatusListUnavailableError) Error() string {
	return fmt.Sprintf("status list unavailable at %s: %v", e.URI, e.Err)
}

func (e *StatusListUnavailableError) Unwrap() error {
	return e.Err
}

// ConfigureStatusListSkipIfUnavailable allows OID4VP login to proceed when status list URLs are unreachable.
func ConfigureStatusListSkipIfUnavailable(skip bool) {
	statusListSkipIfUnavailable = skip
}

func isStatusListUnavailable(err error) bool {
	var unavailable *StatusListUnavailableError
	return errors.As(err, &unavailable)
}

type statusListReference struct {
	kind       string
	uri        string
	index      uint32
	purpose    string
	statusSize uint32
}

func parseStatusListIndex(raw any) (uint32, bool) {
	idx, ok := parseUint32(raw)
	return idx, ok
}

func parseUint32(raw any) (uint32, bool) {
	switch v := raw.(type) {
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return 0, false
		}
		n, err := strconv.ParseUint(s, 10, 32)
		if err != nil {
			return 0, false
		}

		return uint32(n), true
	case json.Number:
		n, err := strconv.ParseUint(v.String(), 10, 32)
		if err != nil {
			return 0, false
		}

		return uint32(n), true
	case float64:
		if v < 0 || v > math.MaxUint32 || math.Trunc(v) != v {
			return 0, false
		}

		return uint32(v), true
	case int:
		if v < 0 {
			return 0, false
		}

		return parseUint32(uint64(v))
	case int64:
		if v < 0 {
			return 0, false
		}

		return parseUint32(uint64(v))
	case uint:
		return parseUint32(uint64(v))
	case uint32:
		return v, true
	case uint64:
		if v > math.MaxUint32 {
			return 0, false
		}

		return uint32(v), true
	default:
		return 0, false
	}
}

func parseStatusSize(raw any) (uint32, bool) {
	if raw == nil {
		return defaultStatusSize, true
	}

	size, ok := parseUint32(raw)
	if !ok || size == 0 || size > maxSupportedStatusSize {
		return 0, false
	}

	return size, true
}

func parseStatusListReference(claims map[string]any) (statusListReference, bool) {
	refs, err := parseStatusListReferences(claims)

	if err != nil || len(refs) == 0 {
		return statusListReference{}, false
	}

	return refs[0], true
}

func parseStatusListReferences(claims map[string]any) ([]statusListReference, error) {
	if csRaw, exists := claims["credentialStatus"]; exists {
		refs, err := parseCredentialStatusReferences(csRaw)
		if err != nil {
			return nil, err
		}
		if len(refs) > 0 {
			return refs, nil
		}
	}

	ref, ok := parseTokenStatusListReference(claims["status"])
	if !ok {
		return nil, fmt.Errorf("credential missing supported credentialStatus or status.status_list")
	}

	return []statusListReference{ref}, nil
}

func parseCredentialStatusReferences(raw any) ([]statusListReference, error) {
	items := []any{raw}
	if arr, ok := raw.([]any); ok {
		items = arr
	}

	refs := make([]statusListReference, 0, len(items))
	for _, item := range items {
		cs, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("credentialStatus must be an object or array of objects")
		}

		typ, _ := cs["type"].(string)
		typ = strings.TrimSpace(typ)
		switch typ {
		case "", statusKindStatusList2021:
			if typ == "" {
				typ = statusKindStatusList2021
			}
		case statusKindBitstringStatusList, statusKindJWTStatusList:
		default:
			continue
		}

		uri, _ := cs["statusListCredential"].(string)
		uri = strings.TrimSpace(uri)
		index, ok := parseStatusListIndex(cs["statusListIndex"])
		if uri == "" || !ok {
			return nil, fmt.Errorf("credentialStatus has invalid statusListCredential or statusListIndex")
		}

		statusSize, ok := parseStatusSize(cs["statusSize"])
		if !ok {
			return nil, fmt.Errorf("credentialStatus has unsupported statusSize")
		}

		purpose, _ := cs["statusPurpose"].(string)
		refs = append(refs, statusListReference{
			kind:       typ,
			uri:        uri,
			index:      index,
			purpose:    strings.TrimSpace(purpose),
			statusSize: statusSize,
		})
	}

	return refs, nil
}

func parseTokenStatusListReference(raw any) (statusListReference, bool) {
	status, ok := raw.(map[string]any)
	if !ok {
		return statusListReference{}, false
	}

	sl, ok := status["status_list"].(map[string]any)
	if !ok {
		return statusListReference{}, false
	}

	uri, _ := sl["uri"].(string)
	uri = strings.TrimSpace(uri)
	index, ok := parseStatusListIndex(sl["idx"])

	if uri == "" || !ok {
		return statusListReference{}, false
	}

	return statusListReference{kind: statusKindTokenStatusList, uri: uri, index: index, statusSize: defaultStatusSize}, true
}

func validStatusListURI(uri string) error {
	parsed, err := url.Parse(strings.TrimSpace(uri))
	if err != nil {
		return fmt.Errorf("parse status list URI: %w", err)
	}

	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return fmt.Errorf("status list URI must use http or https")
	}

	if parsed.Host == "" {
		return fmt.Errorf("status list URI is missing a host")
	}

	if parsed.User != nil {
		return fmt.Errorf("status list URI must not contain userinfo")
	}

	return nil
}

// checkStatusList fetches statusListCredential URIs and checks revocation.
func checkStatusList(rawClaims json.RawMessage) error {
	ctx := context.Background()

	if len(rawClaims) == 0 {
		return fmt.Errorf("credential claims are empty")
	}

	dec := json.NewDecoder(strings.NewReader(string(rawClaims)))
	dec.UseNumber()
	var claims map[string]any

	if err := dec.Decode(&claims); err != nil {
		return fmt.Errorf("parse credential claims for status list check: %w", err)
	}

	refs, err := parseStatusListReferences(claims)
	if err != nil {
		return err
	}

	if len(refs) == 0 {
		return fmt.Errorf("credential missing supported credentialStatus or status.status_list")
	}

	for _, ref := range refs {
		if err := validStatusListURI(ref.uri); err != nil {
			return err
		}

		var err error
		switch ref.kind {
		case statusKindBitstringStatusList:
			if ref.purpose == "" && ref.statusSize == defaultStatusSize {
				err = verifyBitstringStatusList(ctx, ref.uri, ref.index)
			} else {
				err = verifyBitstringStatusListReference(ctx, ref)
			}
		case statusKindStatusList2021:
			if ref.purpose == "" && ref.statusSize == defaultStatusSize {
				err = verifyStatusList2021(ctx, ref.uri, ref.index)
			} else {
				err = verifyStatusList2021Reference(ctx, ref)
			}
		case statusKindJWTStatusList:
			err = verifyJWTStatusList(ctx, ref.uri, ref.index)
		case statusKindTokenStatusList:
			err = verifyTokenStatusList(ctx, ref.uri, ref.index)
		default:
			err = fmt.Errorf("unsupported status list kind %q", ref.kind)
		}
		if err != nil {
			if statusListSkipIfUnavailable && isStatusListUnavailable(err) {
				log.Printf(ctx, "oid4vp status list check skipped (unavailable): %v", err)
				continue
			}
			return err
		}
	}

	return nil
}

// verifyBitstringStatusList checks W3C VC 2.0 BitstringStatusListEntry with default options.
func verifyBitstringStatusList(ctx context.Context, uri string, index uint32) error {
	return verifyBitstringStatusListReference(ctx, statusListReference{
		kind: statusKindBitstringStatusList, uri: uri, index: index, statusSize: defaultStatusSize,
	})
}

func verifyBitstringStatusListReference(ctx context.Context, ref statusListReference) error {
	return verifyJSONBitstringStatusList(ctx, ref, "bitstring status list")
}

// verifyStatusList2021 checks legacy StatusList2021Entry-compatible JSON lists with default options.
func verifyStatusList2021(ctx context.Context, uri string, index uint32) error {
	return verifyStatusList2021Reference(ctx, statusListReference{
		kind: statusKindStatusList2021, uri: uri, index: index, statusSize: defaultStatusSize,
	})
}

func verifyStatusList2021Reference(ctx context.Context, ref statusListReference) error {
	return verifyJSONBitstringStatusList(ctx, ref, "status list 2021")
}

func verifyJSONBitstringStatusList(ctx context.Context, ref statusListReference, label string) error {
	body, err := fetchStatusListBody(ctx, ref.uri)
	if err != nil {
		return err
	}

	if isLikelyJWT(body) {
		return verifyJWTStatusListBodyWithContext(ctx, body, ref.index)
	}

	status, err := queryEntryStatusFromBodyWithOptions(body, ref.index, ref.statusSize, ref.purpose)
	if err != nil {
		return fmt.Errorf("query %s: %w", label, err)
	}

	return ensureActiveStatus(status, ref.index)
}

// verifyTokenStatusList checks IETF Token Status List (status.status_list reference).
func verifyTokenStatusList(ctx context.Context, uri string, index uint32) error {
	body, err := fetchStatusListBody(ctx, uri)
	if err != nil {
		return err
	}

	if isLikelyJWT(body) {
		return verifyJWTStatusListBodyWithContext(ctx, body, index)
	}

	return fmt.Errorf("token status list at %q is not a JWT status list token", uri)
}

func isLikelyJWT(body []byte) bool {
	trimmed := strings.TrimSpace(string(body))
	parts := strings.Split(trimmed, ".")

	if len(parts) != 3 {
		return false
	}
	_, err := decodeBase64URLOrStd(parts[0])

	return err == nil
}

func fetchStatusListBody(ctx context.Context, uri string) ([]byte, error) {
	return fetchStatusListBodyWithClient(ctx, defaultStatusListHTTPClient, uri)
}

func fetchStatusListBodyWithClient(ctx context.Context, client *http.Client, uri string) ([]byte, error) {
	if client == nil {
		client = defaultStatusListHTTPClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, fmt.Errorf("build status list request: %w", err)
	}

	req.Header.Set("Accept", "application/vc+ld+json, application/vc+jwt, application/status-list+jwt, application/json, text/plain;q=0.5")

	resp, err := client.Do(req)
	if err != nil {
		return nil, &StatusListUnavailableError{URI: uri, Err: fmt.Errorf("GET %s: %w", uri, err)}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, &StatusListUnavailableError{
			URI: uri,
			Err: fmt.Errorf("status list service returned %d", resp.StatusCode),
		}
	}

	limited := io.LimitReader(resp.Body, maxStatusListResponseSize+1)
	body, err := io.ReadAll(limited)

	if err != nil {
		return nil, &StatusListUnavailableError{URI: uri, Err: fmt.Errorf("read status list response: %w", err)}
	}

	if len(body) > maxStatusListResponseSize {
		return nil, &StatusListUnavailableError{
			URI: uri,
			Err: fmt.Errorf("status list response exceeds %d bytes", maxStatusListResponseSize),
		}
	}

	return body, nil
}
