package validation

import (
	"sync"
	"testing"
)

// A hub activation (RefreshValidationAnchors) rewrites all four process-wide
// anchors from a request handler while other request goroutines normalize and
// validate documents against them. Run under -race, this fails on any anchor
// left unguarded.
func TestAnchorsSurviveAnActivationConcurrentWithNormalization(t *testing.T) {
	t.Cleanup(func() {
		SetSchemaAnchorRefs(SchemaJSONLDContextV1, SchemaSHACLShapesV1)
		SetCanonicalOntologyIRIs(nil)
		SetShapeLibraryAnchors(nil)
	})

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			SetShapeLibraryAnchors(map[string]ShapeLibraryAnchor{
				"dcs:Contract": {Name: "gaia-x", URL: "https://dcs.example/api/semantic/shapes/gaia-x?version=1"},
			})
			SetCanonicalOntologyIRIs(map[string]string{"dcs": "https://w3id.org/facis/dcs/ontology/v1#"})
			SetSchemaAnchorRefs(
				"https://dcs.example/api/semantic/context/facis-dcs?version=1",
				"https://dcs.example/api/semantic/shapes/facis-dcs?version=1",
			)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			data := documentData{"@context": map[string]any{"dcs": "https://w3id.org/facis/dcs/ontology/v1#"}}
			if err := enforceCanonicalOntologyIRIs(data); err != nil {
				t.Errorf("the canonical prefix must not be reported as a redefinition: %v", err)
				return
			}
			normalizeCanonicalContext(data)
			_ = dcsNamespace()
			_ = isHubContextAnchor("https://dcs.example/api/semantic/context/facis-dcs?version=1")
		}
	}()

	wg.Wait()
}
