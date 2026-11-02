package validation

import (
	"reflect"
	"testing"
	"time"
)

func TestSigningOrderReadsPositiveIntegerPositions(t *testing.T) {
	doc := []byte(`{
		"dcs:signatureFields": [
			{"dcs:signatoryName": "a", "dcs:signingOrder": 1},
			{"dcs:signatoryName": "b", "dcs:signingOrder": {"@value": "2", "@type": "xsd:integer"}},
			{"dcs:signatoryName": "c"},
			{"dcs:signatoryName": "d", "dcs:signingOrder": 0},
			{"dcs:signatoryName": "e", "dcs:signingOrder": 1.5}
		]
	}`)
	got := SigningOrder(doc)
	want := map[string]int{"a": 1, "b": 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SigningOrder = %v, want %v", got, want)
	}
}

func TestSigningDeadline(t *testing.T) {
	for name, tc := range map[string]struct {
		doc  string
		want time.Time
		ok   bool
	}{
		"dateTime":  {`{"dcs:signingDeadline": {"@value": "2026-11-30T12:00:00+01:00", "@type": "xsd:dateTime"}}`, time.Date(2026, 11, 30, 11, 0, 0, 0, time.UTC), true},
		"date":      {`{"dcs:signingDeadline": "2026-11-30"}`, time.Date(2026, 11, 30, 23, 59, 59, 999999999, time.UTC), true},
		"absent":    {`{}`, time.Time{}, false},
		"malformed": {`{"dcs:signingDeadline": "soon"}`, time.Time{}, false},
	} {
		got, ok := SigningDeadline([]byte(tc.doc))
		if ok != tc.ok || !got.Equal(tc.want) {
			t.Errorf("%s: SigningDeadline = %v, %v; want %v, %v", name, got, ok, tc.want, tc.ok)
		}
	}
}
