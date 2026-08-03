package compiler

import (
	"encoding/json"
)

// ContractTemplate is the Go representation of a dcs:ContractTemplate node in
// its canonical compact JSON-LD form. Field names match the compact keys
// produced by CanonicalizePayload so json.Unmarshal works directly.
type ContractTemplate struct {
	Context           json.RawMessage    `json:"@context,omitempty"`
	ID                string             `json:"@id,omitempty"`
	Type              string             `json:"@type,omitempty"`
	Metadata          *TemplateMetadata  `json:"metadata,omitempty"`
	DocumentStructure *DocumentStructure `json:"documentStructure,omitempty"`
	SignatureFields   []SignatureField   `json:"signatureFields,omitempty"`
	ContractFields    []ContractField    `json:"contractFields,omitempty"`
}

// ContractField is one fillable declaration. Value is raw so a bare scalar,
// a JSON number, or a typed {"@value":…} literal all decode.
type ContractField struct {
	ID    string          `json:"@id,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
}

// TemplateMetadata holds the dcs:TemplateMetadata node.
type TemplateMetadata struct {
	Type  string `json:"@type,omitempty"`
	Title string `json:"title"`
}

// DocumentStructure holds the dcs:DocumentStructure node.
// Layout and Blocks are ordered lists; with @container:@list in the stable
// context they always deserialize as JSON arrays, never as scalars.
type DocumentStructure struct {
	Type   string       `json:"@type,omitempty"`
	Layout []LayoutNode `json:"layout"`
	Blocks []Block      `json:"blocks"`
}

// LayoutNode describes one node in the document tree. Children are IRI strings
// referencing blocks (or other layout nodes) by @id.
type LayoutNode struct {
	ID       string   `json:"@id,omitempty"`
	Type     string   `json:"@type,omitempty"`
	IsRoot   bool     `json:"isRoot,omitempty"`
	Children []string `json:"children,omitempty"`
}

// Block holds any dcs:Section, dcs:Clause, or dcs:TextBlock node. All three
// share the same compact keys, so one struct covers all types; the Type field
// discriminates (values: "Section", "Clause", "TextBlock").
type Block struct {
	ID      string            `json:"@id,omitempty"`
	Type    string            `json:"@type"`
	Title   string            `json:"title,omitempty"`
	Text    string            `json:"text,omitempty"`
	Content []json.RawMessage `json:"content,omitempty"`
}

// SignatureField holds a dcs:SignatureField node.
type SignatureField struct {
	ID            string `json:"@id"`
	Type          string `json:"@type,omitempty"`
	SignatoryName string `json:"signatoryName"`
	Title         string `json:"title,omitempty"`
}

// ContentItem represents one element in a Clause/TextBlock content list.
// The compact canonical form can be:
//   - a plain string   → Value is set, ID and Datatype are empty
//   - a typed literal  → Value and Datatype are set  (e.g. xsd:decimal)
//   - an IRI reference → ID is set  (dcs/odrl/external term)
//   - an external link → ID is set (schema:url carried on same node)
type ContentItem struct {
	Value    string          `json:"@value,omitempty"`
	Datatype string          `json:"@type,omitempty"`
	ID       string          `json:"@id,omitempty"`
	Raw      json.RawMessage `json:"-"`
}

// UnmarshalJSON handles the two surface forms a content item can take in the
// canonical compact form: a plain JSON string or a JSON object.
func (c *ContentItem) UnmarshalJSON(b []byte) error {
	c.Raw = append(json.RawMessage(nil), b...)
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		c.Value = s
		return nil
	}
	type plain ContentItem
	return json.Unmarshal(b, (*plain)(c))
}
