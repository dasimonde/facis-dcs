package conf

import (
	"os"
	"strconv"
)

// EvidenceFileLimit bounds decoded files; DCS_EVIDENCE_MAX_FILE_BYTES overrides the 10 MiB default.
func EvidenceFileLimit() int { return evidenceLimit("DCS_EVIDENCE_MAX_FILE_BYTES", 10*1024*1024) }
func EvidenceTextLimit() int { return evidenceLimit("DCS_EVIDENCE_MAX_TEXT_BYTES", 4096) }
func EvidenceRequestLimit() int64 {
	return int64(EvidenceFileLimit())*2 + int64(EvidenceTextLimit())*12 + 8192
}
func evidenceLimit(name string, fallback int) int {
	n, err := strconv.Atoi(os.Getenv(name))
	if err != nil || n <= 0 || n > 100*1024*1024 {
		return fallback
	}
	return n
}
