package service

import (
	"testing"

	"digital-contracting-service/internal/auth/oid4vp"
)

// The ceremony needs a PID AND a PoA, while each is offered under either
// SD-JWT VC format identifier — so the merged query must ask for one of each,
// not for all four credentials, and must keep the current-format pair as the
// first option a wallet reads.
func TestMergeSigningCeremonyDCQLCrossesTheFormatAlternatives(t *testing.T) {
	merged, err := mergeSigningCeremonyDCQL(oid4vp.DefaultPIDDCQLQuery(), oid4vp.DefaultDCQLQuery())
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	query, ok := merged.(map[string]any)
	if !ok {
		t.Fatalf("merged query is not an object: %#v", merged)
	}
	if credentials, _ := query["credentials"].([]any); len(credentials) != 4 {
		t.Fatalf("expected four credential queries, got %d", len(credentials))
	}

	sets, _ := query["credential_sets"].([]any)
	if len(sets) != 1 {
		t.Fatalf("expected exactly one credential set, got %d", len(sets))
	}
	set, _ := sets[0].(map[string]any)
	options, _ := set["options"].([]any)
	if len(options) != 4 {
		t.Fatalf("expected four options, got %d: %#v", len(options), options)
	}

	first, _ := options[0].([]any)
	if len(first) != 2 || first[0] != oid4vp.PIDCredentialQueryID || first[1] != oid4vp.PoACredentialQueryID {
		t.Fatalf("the current-format pair is not the first option: %#v", first)
	}

	for _, rawOption := range options {
		option, _ := rawOption.([]any)
		if len(option) != 2 {
			t.Fatalf("an option does not name one credential per part: %#v", option)
		}
	}
}
