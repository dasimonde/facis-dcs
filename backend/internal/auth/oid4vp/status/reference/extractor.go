// Package reference extracts status-list pointers from decoded credential claims.
package reference

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"digital-contracting-service/internal/auth/oid4vp/status"
)

const (
	entryTypeBitstringStatusList = "BitstringStatusListEntry"
	defaultStatusSize            = 1
)

type Extractor interface {
	Supports(credential status.VerifiedCredential) bool
	Extract(credential status.VerifiedCredential) ([]status.Reference, error)
}

type Composite struct {
	extractors []Extractor
}

func NewComposite(extractors ...Extractor) *Composite {
	return &Composite{extractors: extractors}
}

func (c *Composite) Extract(credential status.VerifiedCredential) ([]status.Reference, error) {
	var refs []status.Reference
	for _, extractor := range c.extractors {
		if !extractor.Supports(credential) {
			continue
		}
		found, err := extractor.Extract(credential)
		if err != nil {
			return nil, err
		}
		refs = append(refs, found...)
	}
	if len(refs) == 0 {
		return nil, fmt.Errorf("no supported status reference found")
	}
	return refs, nil
}

func Extract(credential status.VerifiedCredential) ([]status.Reference, error) {
	hasW3C := credential.Claims["credentialStatus"] != nil
	hasIETF := hasIETFStatusReference(credential.Claims["status"])
	if hasW3C && hasIETF {
		return nil, fmt.Errorf("credential contains multiple incompatible status reference models")
	}

	extractor := NewComposite(
		W3C{},
		IETF{},
	)
	return extractor.Extract(credential)
}

func hasIETFStatusReference(raw any) bool {
	statusClaim, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	sl, ok := statusClaim["status_list"].(map[string]any)
	if !ok {
		return false
	}
	uri, _ := sl["uri"].(string)
	_, idxOK := parseStatusIndex(sl["idx"])
	return strings.TrimSpace(uri) != "" && idxOK
}

func parseStatusIndex(raw any) (uint64, bool) {
	switch v := raw.(type) {
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return 0, false
		}
		n, err := strconv.ParseUint(s, 10, 64)
		return n, err == nil
	case json.Number:
		n, err := strconv.ParseUint(v.String(), 10, 64)
		return n, err == nil
	case float64:
		if v < 0 || v > math.MaxUint64 || math.Trunc(v) != v {
			return 0, false
		}
		return uint64(v), true
	case int:
		if v < 0 {
			return 0, false
		}
		return uint64(v), true
	case int64:
		if v < 0 {
			return 0, false
		}
		return uint64(v), true
	case uint64:
		return v, true
	default:
		return 0, false
	}
}

func parseStatusSize(raw any) (uint, bool) {
	if raw == nil {
		return defaultStatusSize, true
	}
	size, ok := parseStatusIndex(raw)
	if !ok || size == 0 || size > 8 {
		return 0, false
	}
	return uint(size), true
}
