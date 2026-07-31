package validation

import (
	"errors"
	"testing"
)

func TestValidateContractHierarchyInvariants(t *testing.T) {
	tests := []struct {
		name    string
		data    documentData
		wantErr bool
	}{
		{
			name:    "no hierarchy properties is fine",
			data:    documentData{"@type": "dcs:Contract"},
			wantErr: false,
		},
		{
			name:    "single parent object is fine",
			data:    documentData{"dcs:parentContract": map[string]any{"@id": "did:example:parent"}},
			wantErr: false,
		},
		{
			name:    "single parent in a one-element array is fine",
			data:    documentData{"dcs:parentContract": []any{map[string]any{"@id": "did:example:parent"}}},
			wantErr: false,
		},
		{
			name: "two parent references are rejected",
			data: documentData{"dcs:parentContract": []any{
				map[string]any{"@id": "did:example:a"},
				map[string]any{"@id": "did:example:b"},
			}},
			wantErr: true,
		},
		{
			name:    "dcs:childContracts is rejected",
			data:    documentData{"dcs:childContracts": []any{map[string]any{"@id": "did:example:child"}}},
			wantErr: true,
		},
		{
			name:    "dcs:hasPart is rejected",
			data:    documentData{"dcs:hasPart": []any{map[string]any{"@id": "did:example:child"}}},
			wantErr: true,
		},
		{
			name:    "dcs:children (layout term) at top level is NOT a child-enumerating property",
			data:    documentData{"dcs:children": map[string]any{"@list": []any{}}},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateContractHierarchyInvariants(tt.data)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !errors.Is(err, ErrContractHierarchyInvalid) {
					t.Fatalf("expected error to wrap ErrContractHierarchyInvalid, got %v", err)
				}
			} else if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}
