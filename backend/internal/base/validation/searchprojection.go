package validation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// SearchProjectionJSON resolves the installed context source and projects a document.
func SearchProjectionJSON(ctx context.Context, raw []byte) ([]byte, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return []byte("{}"), nil
	}
	var document map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("invalid JSON after search document")
	}
	var normalizeContextVersion func(any)
	normalizeContextVersion = func(value any) {
		switch node := value.(type) {
		case map[string]any:
			for key, child := range node {
				if key == "@version" {
					if number, ok := child.(json.Number); ok {
						if version, err := number.Float64(); err == nil {
							node[key] = version
						}
					}
				}
				normalizeContextVersion(child)
			}
		case []any:
			for _, child := range node {
				normalizeContextVersion(child)
			}
		}
	}
	normalizeContextVersion(document)
	source, err := requireShapeSource()
	if err != nil {
		return nil, err
	}
	projection, err := SearchProjection(ctx, document, source)
	if err != nil {
		return nil, err
	}
	return json.Marshal(projection)
}

// SearchProjection extracts filled semantic values using the document's pinned context.
func SearchProjection(ctx context.Context, document map[string]any, source ShapeSource) (map[string][]any, error) {
	expanded, err := expandDocument(ctx, document, source)
	if err != nil {
		return nil, err
	}
	return expandedSearchProjection(map[string]any{"@graph": expanded}), nil
}

func expandedSearchProjection(root map[string]any) map[string][]any {
	result := map[string][]any{}
	nodes := map[string]map[string]any{}
	fields := map[string]map[string]any{}
	skipProperty := func(property string) bool {
		return strings.HasPrefix(property, odrlIRI) || property == dcsNamespace()+"policies" || property == dcsNamespace()+"valueConstraint"
	}
	skipNode := func(node map[string]any) bool {
		for _, typ := range expandedValues(node, "@type") {
			if iri, ok := typ.(string); ok && strings.HasPrefix(iri, odrlIRI) {
				return true
			}
		}
		return false
	}
	var index func(any)
	index = func(value any) {
		switch node := value.(type) {
		case []any:
			for _, child := range node {
				index(child)
			}
		case map[string]any:
			if skipNode(node) {
				return
			}
			if id, ok := node["@id"].(string); ok {
				if nodes[id] == nil {
					nodes[id] = map[string]any{}
				}
				for property, raw := range node {
					if values, ok := raw.([]any); ok {
						existing, _ := nodes[id][property].([]any)
						nodes[id][property] = append(existing, values...)
					}
				}
			}
			for property, child := range node {
				if !skipProperty(property) {
					index(child)
				}
			}
		}
	}
	index(root)
	for id, node := range nodes {
		isField := len(expandedValues(node, dcsNamespace()+"shape")) != 0
		for _, typ := range expandedValues(node, "@type") {
			if iri, ok := typ.(string); ok && iri == dcsNamespace()+"ContractField" {
				isField = true
			}
		}
		if isField {
			fields[id] = node
		}
	}
	var walk func(any)
	var add func(string, any)
	add = func(iri string, value any) {
		switch v := value.(type) {
		case []any:
			for _, item := range v {
				add(iri, item)
			}
		case string:
			if strings.TrimSpace(v) != "" {
				result[iri] = append(result[iri], v)
			}
		case bool, float64, int, int64, json.Number:
			result[iri] = append(result[iri], v)
		}
	}
	for _, field := range fields {
		for _, shape := range expandedValues(field, dcsNamespace()+"shape") {
			if iri := expandedID(shape); iri != "" {
				for _, v := range expandedValues(field, dcsNamespace()+"value") {
					add(iri, expandedLiteral(v))
				}
			}
		}
	}
	var addPropertyValue func(string, any)
	addPropertyValue = func(property string, value any) {
		scalar, ok := value.(map[string]any)
		if !ok {
			return
		}
		if v, exists := scalar["@value"]; exists {
			add(property, v)
		} else if list, ok := scalar["@list"].([]any); ok {
			for _, item := range list {
				addPropertyValue(property, item)
			}
		} else {
			if id, exists := scalar["@id"].(string); exists {
				if field, found := fields[id]; found {
					for _, v := range expandedValues(field, dcsNamespace()+"value") {
						add(property, expandedLiteral(v))
					}
				} else {
					add(property, id)
				}
			}
		}
	}
	walk = func(value any) {
		switch node := value.(type) {
		case []any:
			for _, child := range node {
				walk(child)
			}
		case map[string]any:
			if skipNode(node) {
				return
			}
			if _, hasID := node["@id"]; !hasID {
				for _, shape := range expandedValues(node, dcsNamespace()+"shape") {
					if iri := expandedID(shape); iri != "" {
						for _, v := range expandedValues(node, dcsNamespace()+"value") {
							add(iri, expandedLiteral(v))
						}
					}
				}
			}
			for property, children := range node {
				if skipProperty(property) {
					continue
				}
				if !strings.HasPrefix(property, "@") {
					for _, child := range expandedValues(node, property) {
						addPropertyValue(property, child)
					}
				}
				walk(children)
			}
		}
	}
	walk(root)
	return result
}
