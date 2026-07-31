package status

import (
	"net/url"
	"strings"
)

func SubjectMatchesURI(subject, refURI string) bool {
	normalizedSubject, okSubject := normalizeStatusListURI(subject)
	normalizedRef, okRef := normalizeStatusListURI(refURI)
	if !okSubject || !okRef {
		return strings.TrimSpace(subject) == strings.TrimSpace(refURI)
	}
	return normalizedSubject == normalizedRef
}

func normalizeStatusListURI(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", false
	}

	if parsed.Path != "" {
		if decoded, err := url.PathUnescape(parsed.Path); err == nil {
			parsed.Path = decoded
		}
	}
	parsed.RawPath = ""
	parsed.Fragment = ""

	return parsed.String(), true
}
