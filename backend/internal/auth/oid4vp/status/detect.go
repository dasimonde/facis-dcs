package status

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"digital-contracting-service/internal/auth/oid4vp/status/envelope"
	"digital-contracting-service/internal/auth/oid4vp/status/fetch"
)

const (
	IETFStatusListAccept  = "application/statuslist+jwt, application/statuslist+cwt"
	XFSCProbeContentType  = "application/json"
	XFSCSignedContentType = "statuslist+jwt"
)

func FetchStatusList(ctx context.Context, client *fetch.Client, uri string, opts fetch.RequestOpts) (fetch.Response, error) {
	if client == nil {
		client = fetch.NewClient()
	}
	return client.FetchWithRequest(ctx, uri, opts)
}

// IsXFSCStatusListJSON reports the XFSC unsigned envelope from
// GET with Content-Type: application/json ({tenantId, listId, list}).
func IsXFSCStatusListJSON(body []byte) bool {
	body = bytes.TrimSpace(body)
	if len(body) == 0 || !LooksLikeJSON(body) {
		return false
	}
	if strings.Contains(string(body), `"credentialSubject"`) || strings.Contains(string(body), `"status_list"`) {
		return false
	}

	var doc struct {
		TenantID string `json:"tenantId"`
		ListID   any    `json:"listId"`
		List     string `json:"list"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return false
	}
	return strings.TrimSpace(doc.TenantID) != "" && doc.ListID != nil && strings.TrimSpace(doc.List) != ""
}

// FetchOptsForReference returns the standard request headers for the reference
// mechanism before response-based handler selection.
func FetchOptsForReference(ref Reference) fetch.RequestOpts {
	if ref.Mechanism == MechanismIETFToken {
		return fetch.RequestOpts{Accept: IETFStatusListAccept}
	}
	return fetch.RequestOpts{}
}

// SelectMechanismFromResponse routes by the fetched response body and media type.
func SelectMechanismFromResponse(ref Reference, response fetch.Response) Mechanism {
	if IsXFSCStatusListJSON(response.Body) {
		return MechanismXFSC
	}
	if IsIETFStatusListResponse(response) {
		return MechanismIETFToken
	}
	if IsW3CStatusListResponse(response) {
		return MechanismW3CBitstring
	}
	return ref.Mechanism
}

func IsIETFStatusListResponse(response fetch.Response) bool {
	contentType := envelope.NormalizeContentType(response.ContentType)
	switch contentType {
	case "application/statuslist+jwt", "application/statuslist+cwt":
		return true
	default:
		return false
	}
}

func IsW3CStatusListResponse(response fetch.Response) bool {
	contentType := envelope.NormalizeContentType(response.ContentType)
	switch contentType {
	case "application/vc+jwt", "application/vc+cose", "application/vc", "application/ld+json":
		return true
	}

	body := response.Body
	if LooksLikeJSON(body) {
		text := string(body)
		if strings.Contains(text, `"credentialSubject"`) || strings.Contains(text, `"encodedList"`) {
			return true
		}
	}
	return IsLikelyJWT(body) && contentType != "application/statuslist+jwt"
}

func IsLikelyJWT(body []byte) bool {
	parts := strings.Split(strings.TrimSpace(string(body)), ".")
	return len(parts) == 3
}

func LooksLikeJSON(body []byte) bool {
	return strings.HasPrefix(strings.TrimSpace(string(body)), "{")
}

func ValidateIETFStatusListJWTHeader(header map[string]any) error {
	typ, _ := header["typ"].(string)
	typ = strings.TrimSpace(typ)
	if typ != "statuslist+jwt" {
		return fmt.Errorf("jwt typ must be statuslist+jwt, got %q", typ)
	}
	return nil
}

func IsXFSCStatusListJWTType(header map[string]any) bool {
	typ, _ := header["typ"].(string)
	typ = strings.TrimSpace(typ)
	return typ == "statuslist+jwt" || typ == "JWT"
}

func ParseTokenStatusBits(raw any) (uint, bool) {
	switch v := raw.(type) {
	case float64:
		if v != 1 && v != 2 && v != 4 && v != 8 {
			return 0, false
		}
		return uint(v), true
	case int:
		if v != 1 && v != 2 && v != 4 && v != 8 {
			return 0, false
		}
		return uint(v), true
	case int64:
		if v != 1 && v != 2 && v != 4 && v != 8 {
			return 0, false
		}
		return uint(v), true
	case uint64:
		if v != 1 && v != 2 && v != 4 && v != 8 {
			return 0, false
		}
		return uint(v), true
	default:
		return 0, false
	}
}
