package sdjwt

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

const defaultSDAlg = "sha-256"

// arrayElementPlaceholderKey is the single member RFC 9901 §4.2.4.2 gives an
// array element that stands in for an undisclosed value.
const arrayElementPlaceholderKey = "..."

// reservedDisclosureNames are claims a disclosure must never carry into the top
// level of the payload (RFC 9901 §5.2 forbids a disclosure overriding an
// existing claim).
//
// The registered claims are validated against the raw signed payload before the
// merge, but several are read again from the merged map afterwards: `iss`
// decides which issuer entry the organization entitlement is checked against,
// `cnf` is the holder binding, and `status` is where revocation is looked up. A
// disclosure named for one of those would move a check to a target the issuer
// never signed for. Nested objects are not read that way, so the restriction
// applies where the verifier actually looks.
var reservedDisclosureNames = map[string]bool{
	"iss": true, "sub": true, "aud": true, "exp": true, "nbf": true, "iat": true,
	"cnf": true, "vct": true, "status": true, "_sd": true, "_sd_alg": true,
}

// disclosure is one decoded RFC 9901 disclosure. A three-element disclosure
// names an object property; a two-element one is the value of a hidden array
// element and carries no name.
type disclosure struct {
	digest string
	name   string
	value  any
}

// MergeDisclosedClaims resolves the holder's disclosures into the issuer-signed
// payload and returns the claim set the verifier reads.
//
// RFC 9901 lets `_sd` digest arrays appear in any object at any depth and lets
// array elements be hidden behind {"...": digest} placeholders, so this is a
// recursive walk of the payload rather than a merge over one top-level array.
// A digest nobody disclosed stays hidden; a disclosure the walk never consumed
// is refused, because nothing the issuer signed asked for it.
func MergeDisclosedClaims(issuerClaims jwt.MapClaims, disclosures []string) (jwt.MapClaims, error) {
	sdAlg, _ := issuerClaims["_sd_alg"].(string)
	if strings.TrimSpace(sdAlg) == "" {
		sdAlg = defaultSDAlg
	}
	if sdAlg != defaultSDAlg {
		return nil, fmt.Errorf("unsupported _sd_alg %q", sdAlg)
	}

	byDigest := make(map[string]disclosure, len(disclosures))
	for _, encoded := range disclosures {
		decoded, err := decodeDisclosure(encoded)
		if err != nil {
			return nil, err
		}
		if _, dup := byDigest[decoded.digest]; dup {
			return nil, fmt.Errorf("duplicate disclosure digest")
		}
		byDigest[decoded.digest] = decoded
	}

	used := make(map[string]bool, len(byDigest))
	out, err := resolveObject(issuerClaims, byDigest, used, true)
	if err != nil {
		return nil, err
	}

	for digest := range byDigest {
		if !used[digest] {
			return nil, fmt.Errorf("disclosure digest is not listed in the credential")
		}
	}

	return out, nil
}

// resolveObject rebuilds one JSON object with its `_sd` digests replaced by the
// disclosures that hash to them. topLevel marks the payload object itself,
// where the registered-claim restriction applies.
func resolveObject(obj map[string]any, byDigest map[string]disclosure, used map[string]bool, topLevel bool) (map[string]any, error) {
	out := make(map[string]any, len(obj))

	for name, value := range obj {
		if name == "_sd" || name == "_sd_alg" {
			continue
		}
		resolved, err := resolveValue(value, byDigest, used)
		if err != nil {
			return nil, err
		}
		out[name] = resolved
	}

	digests, err := sdDigests(obj["_sd"])
	if err != nil {
		return nil, err
	}

	for _, digest := range digests {
		decoded, disclosed := byDigest[digest]
		if !disclosed {
			continue
		}
		if used[digest] {
			return nil, fmt.Errorf("duplicate disclosure digest")
		}
		if decoded.name == "" {
			return nil, fmt.Errorf("an array-element disclosure is listed in an _sd digest array")
		}
		if topLevel && reservedDisclosureNames[decoded.name] {
			return nil, fmt.Errorf("disclosure may not carry the registered claim %q", decoded.name)
		}
		if _, taken := out[decoded.name]; taken {
			return nil, fmt.Errorf("disclosure %q overrides a claim the issuer already signed", decoded.name)
		}
		used[digest] = true
		resolved, err := resolveValue(decoded.value, byDigest, used)
		if err != nil {
			return nil, err
		}
		out[decoded.name] = resolved
	}

	return out, nil
}

// resolveArray rebuilds a JSON array, replacing each {"...": digest} element
// with the disclosed value and dropping the ones the holder withheld.
func resolveArray(arr []any, byDigest map[string]disclosure, used map[string]bool) ([]any, error) {
	out := make([]any, 0, len(arr))

	for _, item := range arr {
		digest, isPlaceholder := arrayElementDigest(item)
		if !isPlaceholder {
			resolved, err := resolveValue(item, byDigest, used)
			if err != nil {
				return nil, err
			}
			out = append(out, resolved)
			continue
		}

		decoded, disclosed := byDigest[digest]
		if !disclosed {
			continue
		}
		if used[digest] {
			return nil, fmt.Errorf("duplicate disclosure digest")
		}
		if decoded.name != "" {
			return nil, fmt.Errorf("a property disclosure %q stands in for an array element", decoded.name)
		}
		used[digest] = true
		resolved, err := resolveValue(decoded.value, byDigest, used)
		if err != nil {
			return nil, err
		}
		out = append(out, resolved)
	}

	return out, nil
}

func resolveValue(value any, byDigest map[string]disclosure, used map[string]bool) (any, error) {
	switch typed := value.(type) {
	case map[string]any:
		return resolveObject(typed, byDigest, used, false)
	case jwt.MapClaims:
		return resolveObject(typed, byDigest, used, false)
	case []any:
		return resolveArray(typed, byDigest, used)
	default:
		return value, nil
	}
}

// arrayElementDigest reports the digest an array element hides, for an element
// that is exactly the one-member placeholder object and nothing else.
func arrayElementDigest(item any) (string, bool) {
	obj, ok := item.(map[string]any)
	if !ok || len(obj) != 1 {
		return "", false
	}
	digest, ok := obj[arrayElementPlaceholderKey].(string)
	if !ok || strings.TrimSpace(digest) == "" {
		return "", false
	}

	return digest, true
}

func sdDigests(raw any) ([]string, error) {
	if raw == nil {
		return nil, nil
	}

	return stringSliceFromAny(raw)
}

func disclosureDigest(encodedDisclosure string) string {
	return sha256Base64URL(encodedDisclosure)
}

func decodeDisclosure(encoded string) (disclosure, error) {
	raw, err := base64.RawURLEncoding.DecodeString(encoded)

	if err != nil {
		return disclosure{}, fmt.Errorf("decode disclosure: %w", err)
	}

	var arr []any
	err = json.Unmarshal(raw, &arr)

	if err != nil {
		return disclosure{}, fmt.Errorf("parse disclosure json: %w", err)
	}

	switch len(arr) {
	case 2:
		// [salt, value]: the value of an array element hidden behind a
		// placeholder. Any issuer of a selectively disclosable array-valued
		// claim produces these.
		return disclosure{digest: disclosureDigest(encoded), value: arr[1]}, nil
	case 3:
		// [salt, name, value]: an object property.
		name, ok := arr[1].(string)
		if !ok || strings.TrimSpace(name) == "" {
			return disclosure{}, fmt.Errorf("disclosure claim name must be a non-empty string")
		}
		return disclosure{digest: disclosureDigest(encoded), name: name, value: arr[2]}, nil
	default:
		return disclosure{}, fmt.Errorf("disclosure must be a two- or three-element array")
	}
}

func stringSliceFromAny(raw any) ([]string, error) {
	arr, ok := raw.([]any)

	if !ok {
		return nil, fmt.Errorf("expected json array")
	}
	out := make([]string, 0, len(arr))

	for _, item := range arr {
		s, ok := item.(string)
		if !ok || strings.TrimSpace(s) == "" {
			return nil, fmt.Errorf("expected string array")
		}
		out = append(out, s)
	}

	return out, nil
}
