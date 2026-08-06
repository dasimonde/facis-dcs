package conf

import (
	"testing"
	"time"
)

func TestPACAuditEvidenceTimeoutDefault(t *testing.T) {
	t.Setenv("PAC_AUDIT_EVIDENCE_TIMEOUT", "")
	if got := PACAuditEvidenceTimeout(); got != 2*time.Minute {
		t.Fatalf("expected the 2-minute default, got %v", got)
	}
}

func TestPACAuditEvidenceTimeoutOverride(t *testing.T) {
	t.Setenv("PAC_AUDIT_EVIDENCE_TIMEOUT", "2m30s")
	if got := PACAuditEvidenceTimeout(); got != 2*time.Minute+30*time.Second {
		t.Fatalf("expected a 2m30s timeout, got %v", got)
	}
}

func TestPACAuditEvidenceTimeoutInvalidValuesKeepDefault(t *testing.T) {
	for _, value := range []string{"not-a-duration", "0", "-1s"} {
		t.Setenv("PAC_AUDIT_EVIDENCE_TIMEOUT", value)
		if got := PACAuditEvidenceTimeout(); got != 2*time.Minute {
			t.Fatalf("value %q: expected the 2-minute default, got %v", value, got)
		}
	}
}

func TestArchiveExpiringWindowDefault(t *testing.T) {
	t.Setenv("DCS_ARCHIVE_EXPIRING_WINDOW_DAYS", "")
	if got := ArchiveExpiringWindow(); got != 30*24*time.Hour {
		t.Fatalf("expected the 30-day default, got %v", got)
	}
}

func TestArchiveExpiringWindowOverride(t *testing.T) {
	t.Setenv("DCS_ARCHIVE_EXPIRING_WINDOW_DAYS", "45")
	if got := ArchiveExpiringWindow(); got != 45*24*time.Hour {
		t.Fatalf("expected a 45-day window, got %v", got)
	}
}

func TestArchiveExpiringWindowInvalidValuesKeepDefault(t *testing.T) {
	for _, value := range []string{"not-a-number", "0", "-7", "7.5"} {
		t.Setenv("DCS_ARCHIVE_EXPIRING_WINDOW_DAYS", value)
		if got := ArchiveExpiringWindow(); got != 30*24*time.Hour {
			t.Fatalf("value %q: expected the 30-day default, got %v", value, got)
		}
	}
}
