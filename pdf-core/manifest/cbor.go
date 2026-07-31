package manifest

import "fmt"

// parseCBORTextMap decodes a CBOR map whose keys and values are all text
// strings. This matches the dcs.lifecycle assertion payload pdf-core emits
// (renderLifecycleAssertionCBOR, pdf-core/compiler/compiler_c2pa.go): a flat
// map of text keys to text values.
func parseCBORTextMap(data []byte) (map[string]string, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("empty CBOR")
	}
	if data[0]>>5 != 5 {
		return nil, fmt.Errorf("expected CBOR map (major type 5), got %d", data[0]>>5)
	}
	count, pos, err := cborLength(data)
	if err != nil {
		return nil, fmt.Errorf("map header: %w", err)
	}
	result := make(map[string]string, count)
	for i := 0; i < count; i++ {
		key, n, err := decodeCBORText(data[pos:])
		if err != nil {
			return nil, fmt.Errorf("key %d: %w", i, err)
		}
		pos += n
		val, n, err := decodeCBORText(data[pos:])
		if err != nil {
			return nil, fmt.Errorf("val %d: %w", i, err)
		}
		pos += n
		result[key] = val
	}
	return result, nil
}

// cborLength decodes the additional-info length of a CBOR head byte, returning
// the length and the number of header bytes consumed.
func cborLength(data []byte) (length, hdr int, err error) {
	add := int(data[0] & 0x1F)
	switch {
	case add <= 23:
		return add, 1, nil
	case add == 24:
		if len(data) < 2 {
			return 0, 0, fmt.Errorf("truncated length header")
		}
		return int(data[1]), 2, nil
	case add == 25:
		if len(data) < 3 {
			return 0, 0, fmt.Errorf("truncated length header")
		}
		return int(data[1])<<8 | int(data[2]), 3, nil
	default:
		return 0, 0, fmt.Errorf("unsupported length encoding (additional=%d)", add)
	}
}

// decodeCBORText decodes a single CBOR text string, returning the string,
// bytes consumed, and any error.
func decodeCBORText(data []byte) (string, int, error) {
	if len(data) < 1 {
		return "", 0, fmt.Errorf("empty")
	}
	if data[0]>>5 != 3 {
		return "", 0, fmt.Errorf("expected CBOR text (major type 3), got %d", data[0]>>5)
	}
	length, hdr, err := cborLength(data)
	if err != nil {
		return "", 0, err
	}
	end := hdr + length
	if len(data) < end {
		return "", 0, fmt.Errorf("truncated text: need %d, have %d", end, len(data))
	}
	return string(data[hdr:end]), end, nil
}
