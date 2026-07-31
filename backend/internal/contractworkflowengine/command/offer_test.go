package command

import (
	"context"
	"errors"
	"testing"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/validation"
)

func TestValidateOfferReadyRejectsMissingContractData(t *testing.T) {
	err := validateOfferReady(context.Background(), nil)
	if !errors.Is(err, validation.ErrContractNotClosed) {
		t.Fatalf("expected ErrContractNotClosed for missing contract data, got: %v", err)
	}
}

func TestValidateOfferReadyRejectsNonCanonicalEnvelope(t *testing.T) {
	flat := datatype.JSON(`{"title":"not canonical"}`)
	err := validateOfferReady(context.Background(), &flat)
	if err == nil {
		t.Fatalf("expected a semantic validation error for a non-canonical envelope")
	}
}

func TestValidateOfferReadyRejectsUnresolvedProsePlaceholder(t *testing.T) {
	id := "did:web:facis.example:contract:offer-open"
	fieldID := id + "#field-payment-amount"
	data := map[string]any{
		"@context": map[string]any{
			"dcs": "https://w3id.org/facis/dcs/ontology/v1#",
			"xsd": "http://www.w3.org/2001/XMLSchema#",
		},
		"@id":   id,
		"@type": "dcs:Contract",
		"dcs:metadata": map[string]any{
			"@id":   id + "#metadata",
			"@type": "dcs:ContractMetadata",
		},
		"dcs:contractFields": []any{
			map[string]any{
				"@id":          fieldID,
				"@type":        "dcs:ContractField",
				"dcs:label":    "Payment Amount",
				"dcs:datatype": "xsd:decimal",
				"dcs:required": true,
			},
		},
		"dcs:documentStructure": map[string]any{
			"@id":   id + "#document-structure",
			"@type": "dcs:DocumentStructure",
			"dcs:blocks": map[string]any{"@list": []any{
				map[string]any{
					"@id":   id + "#clause-1",
					"@type": "dcs:Clause",
					"dcs:content": map[string]any{
						"@list": []any{
							"The provider invoices ",
							map[string]any{"@id": fieldID},
							" per period.",
						},
					},
				},
			}},
			"dcs:layout": map[string]any{"@list": []any{
				map[string]any{
					"@id":        id + "#root",
					"dcs:isRoot": true,
					"dcs:children": map[string]any{
						"@list": []any{
							map[string]any{"@id": id + "#clause-1"},
						},
					},
				},
				map[string]any{
					"@id": id + "#clause-1",
					"dcs:children": map[string]any{
						"@list": []any{},
					},
				},
			}},
		},
	}
	raw, err := datatype.NewJSON(data)
	if err != nil {
		t.Fatalf("could not create contract data: %v", err)
	}
	err = validateOfferReady(context.Background(), &raw)
	if !errors.Is(err, validation.ErrContractNotClosed) {
		t.Fatalf("expected ErrContractNotClosed for an unfilled prose placeholder, got: %v", err)
	}
}
