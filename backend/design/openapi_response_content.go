package design

import (
	"encoding/json"
	"fmt"
	"sort"

	"goa.design/goa/v3/codegen"
	"goa.design/goa/v3/eval"
	"goa.design/goa/v3/http/codegen/openapi"
	openapiv2 "goa.design/goa/v3/http/codegen/openapi/v2"
	openapiv3 "goa.design/goa/v3/http/codegen/openapi/v3"
)

const responseContentExtension = "x-dcs-response-content"

func init() {
	codegen.RegisterPlugin("dcs-response-content", "gen", nil, generateResponseContent)
}

// Goa supports one ContentType per response; the design extension declares alternatives.
func generateResponseContent(_ string, _ []eval.Root, files []*codegen.File) ([]*codegen.File, error) {
	for _, file := range files {
		for _, section := range file.SectionTemplates {
			switch spec := section.Data.(type) {
			case *openapiv3.OpenAPI:
				for _, path := range spec.Paths {
					for _, op := range []*openapiv3.Operation{path.Get, path.Put, path.Post, path.Delete, path.Options, path.Head, path.Patch, path.Trace, path.Connect} {
						if op == nil {
							continue
						}
						for _, ref := range op.Responses {
							if ref.Value == nil {
								continue
							}
							schemas, err := responseContentSchemas(ref.Value.Extensions)
							if err != nil {
								return nil, err
							}
							if schemas == nil {
								continue
							}
							ref.Value.Content = make(map[string]*openapiv3.MediaType, len(schemas))
							for mediaType, schema := range schemas {
								ref.Value.Content[mediaType] = &openapiv3.MediaType{Schema: schema}
							}
						}
					}
				}
			case *openapiv2.V2:
				for _, item := range spec.Paths {
					path, ok := item.(*openapiv2.Path)
					if !ok {
						continue
					}
					for _, op := range []*openapiv2.Operation{path.Get, path.Put, path.Post, path.Delete, path.Options, path.Head, path.Patch} {
						if op == nil {
							continue
						}
						for _, response := range op.Responses {
							schemas, err := responseContentSchemas(response.Extensions)
							if err != nil {
								return nil, err
							}
							if schemas == nil {
								continue
							}
							op.Produces = make([]string, 0, len(schemas))
							for mediaType := range schemas {
								op.Produces = append(op.Produces, mediaType)
							}
							sort.Strings(op.Produces)
							// OAS2 describes downloads as files; OAS3 carries per-format schemas.
							response.Schema = &openapi.Schema{Type: "file"}
						}
					}
				}
			}
		}
	}
	return files, nil
}

func responseContentSchemas(extensions map[string]any) (map[string]*openapi.Schema, error) {
	value, ok := extensions[responseContentExtension]
	if !ok {
		return nil, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", responseContentExtension, err)
	}
	var schemas map[string]*openapi.Schema
	if err := json.Unmarshal(raw, &schemas); err != nil {
		return nil, fmt.Errorf("decode %s: %w", responseContentExtension, err)
	}
	if len(schemas) == 0 {
		return nil, fmt.Errorf("%s must declare response media types", responseContentExtension)
	}
	for mediaType, schema := range schemas {
		if mediaType == "" || schema == nil {
			return nil, fmt.Errorf("invalid %s schema", responseContentExtension)
		}
	}
	return schemas, nil
}
