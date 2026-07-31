package hsm

import "testing"

func TestVersionedLabel(t *testing.T) {
	cases := []struct {
		base    string
		version int
		want    string
	}{
		{"dcs-c2pa", 0, "dcs-c2pa"},
		{"dcs-c2pa", 1, "dcs-c2pa"},
		{"dcs-c2pa", 2, "dcs-c2pa-v2"},
		{"dcs-c2pa", 5, "dcs-c2pa-v5"},
	}
	for _, c := range cases {
		if got := VersionedLabel(c.base, c.version); got != c.want {
			t.Errorf("VersionedLabel(%q, %d) = %q, want %q", c.base, c.version, got, c.want)
		}
	}
}
