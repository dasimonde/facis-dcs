package conf

import (
	"testing"
	"time"
)

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
