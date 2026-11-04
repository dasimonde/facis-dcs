package semantichub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/tggo/goRDFlib/shacl"
)

const xsdNamespace = "http://www.w3.org/2001/XMLSchema#"

var numericValuePattern = regexp.MustCompile(`^[+-]?([0-9]+([.][0-9]*)?|[.][0-9]+)([eE][+-]?[0-9]+)?$`)

func textSearchType(iri string, catalog bool) bool {
	if catalog {
		return false
	}
	switch iri {
	case xsdNamespace + "string", xsdNamespace + "normalizedString", xsdNamespace + "token",
		xsdNamespace + "language", xsdNamespace + "Name", xsdNamespace + "NCName", xsdNamespace + "NMTOKEN":
		return true
	default:
		return false
	}
}

func numericType(iri string) (integer bool, min, max float64, ok bool) {
	if !strings.HasPrefix(iri, xsdNamespace) {
		return false, 0, 0, false
	}
	switch strings.TrimPrefix(iri, xsdNamespace) {
	case "decimal", "double", "float":
		return false, 0, 0, true
	case "integer":
		return true, math.Inf(-1), math.Inf(1), true
	case "long":
		return true, -9223372036854775808, 9223372036854775807, true
	case "int":
		return true, -2147483648, 2147483647, true
	case "short":
		return true, -32768, 32767, true
	case "byte":
		return true, -128, 127, true
	case "nonNegativeInteger":
		return true, 0, math.Inf(1), true
	case "positiveInteger":
		return true, 1, math.Inf(1), true
	case "nonPositiveInteger":
		return true, math.Inf(-1), 0, true
	case "negativeInteger":
		return true, math.Inf(-1), -1, true
	case "unsignedLong":
		return true, 0, 18446744073709551615, true
	case "unsignedInt":
		return true, 0, 4294967295, true
	case "unsignedShort":
		return true, 0, 65535, true
	case "unsignedByte":
		return true, 0, 255, true
	}
	return false, 0, 0, false
}

func validTemporalValue(datatype, value string) bool {
	validOffset := func(parsed time.Time) bool {
		_, offset := parsed.Zone()
		if offset < -14*3600 || offset > 14*3600 {
			return false
		}
		if strings.HasSuffix(value, "Z") {
			return true
		}
		minutes, err := strconv.Atoi(value[len(value)-2:])
		return err == nil && minutes < 60
	}
	switch datatype {
	case xsdNamespace + "date":
		_, err := time.Parse("2006-01-02", value)
		if err == nil {
			return true
		}
		parsed, err := time.Parse("2006-01-02Z07:00", value)
		return err == nil && validOffset(parsed)
	case xsdNamespace + "dateTime", xsdNamespace + "dateTimeStamp":
		if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
			return validOffset(parsed)
		}
		if datatype == xsdNamespace+"dateTime" {
			_, err := time.Parse("2006-01-02T15:04:05.999999999", value)
			return err == nil
		}
		return false
	}
	return false
}

func validTypedValue(datatype string, value any) bool {
	if datatype == xsdNamespace+"boolean" {
		_, ok := value.(bool)
		return ok
	}
	if datatype == xsdNamespace+"date" || datatype == xsdNamespace+"dateTime" || datatype == xsdNamespace+"dateTimeStamp" {
		text, ok := value.(string)
		return ok && validTemporalValue(datatype, text)
	}
	if integer, min, max, numeric := numericType(datatype); numeric {
		var text string
		switch number := value.(type) {
		case json.Number:
			text = number.String()
		case string:
			text = number
		case float64:
			text = fmt.Sprint(number)
		default:
			return false
		}
		if len(text) > 1024 || !numericValuePattern.MatchString(text) {
			return false
		}
		if i := strings.IndexAny(text, "eE"); i >= 0 {
			exponent, err := strconv.Atoi(text[i+1:])
			if err != nil || exponent < -1000 || exponent > 1000 {
				return false
			}
		}
		number, ok := new(big.Rat).SetString(text)
		if !ok || (integer && !number.IsInt()) {
			return false
		}
		if !integer {
			return true
		}
		minText, maxText := fmt.Sprintf("%.0f", min), fmt.Sprintf("%.0f", max)
		switch datatype {
		case xsdNamespace + "long":
			maxText = "9223372036854775807"
		case xsdNamespace + "unsignedLong":
			maxText = "18446744073709551615"
		}
		if !math.IsInf(min, -1) {
			bound, _ := new(big.Rat).SetString(minText)
			if number.Cmp(bound) < 0 {
				return false
			}
		}
		if !math.IsInf(max, 1) {
			bound, _ := new(big.Rat).SetString(maxText)
			if number.Cmp(bound) > 0 {
				return false
			}
		}
		return true
	}
	_, ok := value.(string)
	return ok
}

type ResolvedSearchFilter struct {
	ID       string `json:"filter_id"`
	FieldIRI string `json:"field_iri"`
	Value    any    `json:"value"`
	Text     bool   `json:"-"`
	Datatype string `json:"-"`
}

func ResolveSearchFilters(ctx context.Context, tx *sqlx.Tx, raw string) ([]ResolvedSearchFilter, error) {
	if raw == "" {
		return nil, nil
	}
	var requested []struct {
		ID    string `json:"filter_id"`
		Value any    `json:"value"`
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(&requested); err != nil || requested == nil {
		return nil, fmt.Errorf("%w: semantic_filters must be an array", ErrInvalidSearchFilter)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("%w: unexpected data after semantic_filters", ErrInvalidSearchFilter)
	}
	config, err := LoadSearchFilters(ctx, tx)
	if err != nil {
		return nil, err
	}
	contextSchema, err := (Repo{}).Get(ctx, tx, ContextName, "context", 0)
	if err != nil {
		return nil, err
	}
	var contextDoc struct {
		Context map[string]any `json:"@context"`
	}
	if err := json.Unmarshal([]byte(contextSchema.Content), &contextDoc); err != nil {
		return nil, err
	}
	dcs, ok := contextDoc.Context["dcs"].(string)
	if !ok || dcs == "" {
		return nil, fmt.Errorf("active context has no dcs namespace")
	}
	byID := make(map[string]SearchFilter, len(config.Filters))
	for _, filter := range config.Filters {
		byID[filter.ID] = filter
	}
	result := make([]ResolvedSearchFilter, 0, len(requested))
	seen := map[string]bool{}
	for _, item := range requested {
		filter, ok := byID[item.ID]
		if !ok || item.Value == nil || seen[item.ID] {
			return nil, fmt.Errorf("%w: unknown filter or missing value %q", ErrInvalidSearchFilter, item.ID)
		}
		seen[item.ID] = true
		switch value := item.Value.(type) {
		case string:
			if strings.TrimSpace(value) == "" {
				return nil, fmt.Errorf("%w: filter value is empty", ErrInvalidSearchFilter)
			}
		case json.Number, bool:
		default:
			return nil, fmt.Errorf("%w: filter value must be scalar", ErrInvalidSearchFilter)
		}
		schema, err := (Repo{}).Get(ctx, tx, filter.Source.Name, filter.Source.Kind, filter.Source.Version)
		if err != nil {
			return nil, err
		}
		graph, err := shacl.LoadTurtleString(schema.Content, "urn:dcs:search-filter:source")
		if err != nil {
			return nil, err
		}
		field := shacl.IRI(filter.FieldIRI)
		var rangeIRI string
		catalog := false
		if filter.Source.Kind == "ontology" {
			for _, value := range graph.Objects(field, shacl.IRI(shacl.RDFS+"range")) {
				rangeIRI = value.Value()
				break
			}
			for _, constraint := range graph.Objects(field, shacl.IRI(dcs+"hasValueConstraint")) {
				catalog = catalog || len(graph.Objects(constraint, shacl.IRI(dcs+"allowedValue"))) > 0 || len(graph.Objects(constraint, shacl.IRI(dcs+"valueCatalog"))) > 0 || len(graph.Objects(constraint, shacl.IRI(dcs+"allowedValuesRef"))) > 0
			}
		} else {
			for _, shape := range graph.Subjects(shacl.IRI(shacl.SH+"path"), field) {
				catalog = catalog || len(graph.Objects(shape, shacl.IRI(shacl.SH+"in"))) > 0
				for _, value := range graph.Objects(shape, shacl.IRI(shacl.SH+"datatype")) {
					rangeIRI = value.Value()
					break
				}
			}
		}
		if !validTypedValue(rangeIRI, item.Value) {
			return nil, fmt.Errorf("%w: filter %q requires a valid value of its declared type", ErrInvalidSearchFilter, item.ID)
		}
		result = append(result, ResolvedSearchFilter{ID: item.ID, FieldIRI: filter.FieldIRI, Value: item.Value, Text: textSearchType(rangeIRI, catalog), Datatype: rangeIRI})
	}
	return result, nil
}

// SearchFilterSQL returns conjunctive predicates for a JSONB projection column.
func SearchFilterSQL(filters []ResolvedSearchFilter, column string, next int) (string, []any) {
	var clauses []string
	var args []any
	for _, filter := range filters {
		key := fmt.Sprintf("$%d", next+len(args))
		value := fmt.Sprintf("$%d", next+len(args)+1)
		condition := "element.value #>> '{}' = " + value
		argument := filter.Value
		switch {
		case filter.Text:
			condition = "jsonb_typeof(element.value) = 'string' AND element.value #>> '{}' ILIKE " + value + ` ESCAPE '\'`
			text := filter.Value.(string)
			argument = "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(text) + "%"
		case strings.HasSuffix(filter.Datatype, "#boolean"):
			condition = "element.value #>> '{}' = " + value
			argument = fmt.Sprint(filter.Value)
		case func() bool { _, _, _, ok := numericType(filter.Datatype); return ok }():
			condition = `CASE WHEN element.value #>> '{}' ~ '^[+-]?([0-9]+([.][0-9]*)?|[.][0-9]+)([eE][+-]?[0-9]+)?$' AND pg_input_is_valid(element.value #>> '{}', 'numeric') THEN (element.value #>> '{}')::numeric = ` + value + `::numeric ELSE FALSE END`
		case filter.Datatype == xsdNamespace+"date":
			if len(filter.Value.(string)) > 10 {
				stored := "element.value #>> '{}'"
				instant := `substring(` + stored + ` from 1 for 10) || 'T00:00:00' || substring(` + stored + ` from 11)`
				requested := `substring(` + value + ` from 1 for 10) || 'T00:00:00' || substring(` + value + ` from 11)`
				condition = `CASE WHEN ` + stored + ` ~ '^\d{4}-\d{2}-\d{2}(Z|[+-]\d{2}:\d{2})$' AND pg_input_is_valid(` + instant + `, 'timestamp with time zone') THEN (` + instant + `)::timestamptz = (` + requested + `)::timestamptz ELSE FALSE END`
			} else {
				condition = `CASE WHEN element.value #>> '{}' ~ '^\d{4}-\d{2}-\d{2}$' AND pg_input_is_valid(element.value #>> '{}', 'date') THEN (element.value #>> '{}')::date = ` + value + `::date ELSE FALSE END`
			}
		case filter.Datatype == xsdNamespace+"dateTime", filter.Datatype == xsdNamespace+"dateTimeStamp":
			zoned := strings.HasSuffix(filter.Value.(string), "Z") || strings.ContainsAny(filter.Value.(string)[10:], "+-")
			stored := "element.value #>> '{}'"
			fraction := `('0' || COALESCE(substring(` + stored + ` from '\.\d+'), ''))::numeric = ('0' || COALESCE(substring(` + value + ` from '\.\d+'), ''))::numeric`
			if zoned {
				condition = `CASE WHEN ` + stored + ` ~ '^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$' AND pg_input_is_valid(` + stored + `, 'timestamp with time zone') THEN regexp_replace(` + stored + `, '\.\d+(Z|[+-]\d{2}:\d{2})$', '\1')::timestamptz = regexp_replace(` + value + `, '\.\d+(Z|[+-]\d{2}:\d{2})$', '\1')::timestamptz AND ` + fraction + ` ELSE FALSE END`
			} else {
				condition = `CASE WHEN ` + stored + ` ~ '^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?$' AND pg_input_is_valid(` + stored + `, 'timestamp without time zone') THEN regexp_replace(` + stored + `, '\.\d+$', '')::timestamp = regexp_replace(` + value + `, '\.\d+$', '')::timestamp AND ` + fraction + ` ELSE FALSE END`
			}
		}
		clauses = append(clauses, "("+column+" ? "+key+" AND EXISTS (SELECT 1 FROM jsonb_array_elements("+column+" -> "+key+") AS element(value) WHERE "+condition+"))")
		args = append(args, filter.FieldIRI, argument)
	}
	return strings.Join(clauses, " AND "), args
}
