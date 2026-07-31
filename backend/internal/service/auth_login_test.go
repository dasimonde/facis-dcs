package service

import "testing"

func TestExtractSinglePresentation(t *testing.T) {
	vp, err := extractSinglePresentation(`{"dcs_poa_credential":["a~b~c"]}`, "dcs_poa_credential")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vp != "a~b~c" {
		t.Fatalf("unexpected vp: %q", vp)
	}
}

func TestExtractSinglePresentationMissingQueryID(t *testing.T) {
	_, err := extractSinglePresentation(`{"other":["a~b~c"]}`, "dcs_poa_credential")
	if err == nil {
		t.Fatal("expected error for missing query id")
	}
}

func TestExtractSinglePresentationRejectsMultiplePresentations(t *testing.T) {
	_, err := extractSinglePresentation(`{"dcs_poa_credential":["a","b"]}`, "dcs_poa_credential")
	if err == nil {
		t.Fatal("expected error for multiple presentations")
	}
}

func TestExtractSinglePresentationRejectsNonObject(t *testing.T) {
	_, err := extractSinglePresentation(`"a~b~c"`, "dcs_poa_credential")
	if err == nil {
		t.Fatal("expected error for non-object vp_token")
	}
}

func TestCredentialQueryIDsFromDCQL(t *testing.T) {
	ids, err := credentialQueryIDsFromDCQL(map[string]any{
		"credentials": []any{
			map[string]any{"id": "query-1"},
			map[string]any{"id": "query-2"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 || ids[0] != "query-1" || ids[1] != "query-2" {
		t.Fatalf("unexpected query ids: %v", ids)
	}
}

// A wallet answers under the id of whichever alternative query it matched, so
// the answer must be found under any of them, not only the first.
func TestExtractSinglePresentationFindsAnAlternativeQueryID(t *testing.T) {
	vp, err := extractSinglePresentation(`{"dcs_poa_credential_vc_sd_jwt":["a~b~c"]}`, "dcs_poa_credential", "dcs_poa_credential_vc_sd_jwt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vp != "a~b~c" {
		t.Fatalf("unexpected vp: %q", vp)
	}
}
