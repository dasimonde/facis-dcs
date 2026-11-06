package envelope

import (
	"fmt"
	"math"
)

// Int64Claim reads a numeric claim from a JWT/CWT-style string-key map.
//
// Distinguishes three outcomes:
//   - absent: key missing → present=false, err=nil (optional claims stay optional)
//   - present and valid: present=true, err=nil, value in [0, MaxInt64]
//   - present but invalid: present=true, err!=nil (including an explicit null)
//
// Accepted encodings are the integer and whole-number float forms commonly
// produced by CBOR and JSON (jwt.MapClaims). Values outside the positive
// int64 range, NaN, ±Inf, and fractional floats are rejected.
func Int64Claim(claims map[string]any, key string) (value int64, present bool, err error) {
	if claims == nil {
		return 0, false, nil
	}
	raw, ok := claims[key]
	if !ok {
		return 0, false, nil
	}
	if raw == nil {
		return 0, true, fmt.Errorf("%s: numeric claim must not be null", key)
	}
	v, err := int64FromNumeric(raw)
	if err != nil {
		return 0, true, fmt.Errorf("%s: %w", key, err)
	}
	return v, true, nil
}

func int64FromNumeric(raw any) (int64, error) {
	switch v := raw.(type) {
	case int64:
		if v < 0 {
			return 0, fmt.Errorf("numeric claim must be non-negative")
		}
		return v, nil
	case int:
		if v < 0 {
			return 0, fmt.Errorf("numeric claim must be non-negative")
		}
		return int64(v), nil
	case int32:
		if v < 0 {
			return 0, fmt.Errorf("numeric claim must be non-negative")
		}
		return int64(v), nil
	case uint64:
		if v > math.MaxInt64 {
			return 0, fmt.Errorf("numeric claim exceeds int64 range")
		}
		return int64(v), nil
	case uint:
		if uint64(v) > math.MaxInt64 {
			return 0, fmt.Errorf("numeric claim exceeds int64 range")
		}
		return int64(v), nil
	case uint32:
		return int64(v), nil
	case float64:
		if err := rejectInvalidFloatClaim(v); err != nil {
			return 0, err
		}
		// float64(math.MaxInt64) rounds to 2^63, which is outside int64.
		// Reject anything >= 2^63 rather than comparing to float64(MaxInt64).
		if v >= 0x1p63 {
			return 0, fmt.Errorf("numeric claim exceeds int64 range")
		}
		return int64(v), nil
	case float32:
		return int64FromNumeric(float64(v))
	default:
		return 0, fmt.Errorf("numeric claim has unsupported type %T", raw)
	}
}

func rejectInvalidFloatClaim(v float64) error {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return fmt.Errorf("numeric claim must be a finite number")
	}
	if v < 0 {
		return fmt.Errorf("numeric claim must be non-negative")
	}
	if v != math.Trunc(v) {
		return fmt.Errorf("numeric claim must be a whole number")
	}
	return nil
}

func cwtUintAsInt64(raw any) (int64, error) {
	v, ok := cwtClaimUint(raw)
	if !ok {
		return 0, fmt.Errorf("must be a non-negative integer")
	}
	if v > math.MaxInt64 {
		return 0, fmt.Errorf("exceeds int64 range")
	}
	return int64(v), nil
}
