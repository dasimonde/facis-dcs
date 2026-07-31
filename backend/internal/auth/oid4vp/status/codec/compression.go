package codec

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

const maxDecompressedBytes = 1 << 20

func IsZLIBWrapper(data []byte) bool {
	return len(data) >= 2 && data[0] == 0x78
}

func IsGZIPWrapper(data []byte) bool {
	return len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b
}

func DecodeMultibaseBase64URL(encoded string) ([]byte, error) {
	encoded = strings.TrimSpace(encoded)
	if !strings.HasPrefix(encoded, "u") {
		return nil, fmt.Errorf("multibase prefix must be u")
	}
	return base64.RawURLEncoding.DecodeString(strings.TrimPrefix(encoded, "u"))
}

func GZIPDecompressLimited(compressed []byte, limit int) ([]byte, error) {
	if limit <= 0 {
		limit = maxDecompressedBytes
	}
	if !IsGZIPWrapper(compressed) {
		return nil, fmt.Errorf("status list data is not gzip compressed")
	}
	r, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	return readLimited(r, limit)
}

func ZLIBDecompressLimited(compressed []byte, limit int) ([]byte, error) {
	if limit <= 0 {
		limit = maxDecompressedBytes
	}
	if !IsZLIBWrapper(compressed) {
		return nil, fmt.Errorf("status list data is not zlib compressed")
	}
	r, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	return readLimited(r, limit)
}

func readLimited(r io.Reader, limit int) ([]byte, error) {
	limited := io.LimitReader(r, int64(limit)+1)
	out, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(out) > limit {
		return nil, fmt.Errorf("decompressed payload exceeds %d bytes", limit)
	}
	return out, nil
}

func DecodeBase64URL(encoded string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimSpace(encoded))
}

func DecodeBase64Std(encoded string) ([]byte, error) {
	encoded = strings.TrimSpace(encoded)
	if raw, err := base64.StdEncoding.DecodeString(encoded); err == nil {
		return raw, nil
	}
	return base64.RawStdEncoding.DecodeString(encoded)
}

func DecodeBase64Flexible(encoded string) ([]byte, error) {
	encoded = strings.TrimSpace(encoded)
	if raw, err := base64.RawURLEncoding.DecodeString(encoded); err == nil {
		return raw, nil
	}
	if raw, err := base64.URLEncoding.DecodeString(encoded); err == nil {
		return raw, nil
	}
	return DecodeBase64Std(encoded)
}
