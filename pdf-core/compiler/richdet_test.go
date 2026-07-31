package compiler

import (
	"bytes"
	"context"
	"testing"
)

const richDetPayload = `{
  "@context": {"dcs":"https://w3id.org/facis/dcs/ontology/v1#","xsd":"http://www.w3.org/2001/XMLSchema#"},
  "@id":"urn:doc:richdet","@type":"ContractTemplate",
  "dcs:metadata":{"@type":"TemplateMetadata","dcs:title":"Rich Determinism"},
  "dcs:documentStructure":{"@type":"DocumentStructure",
    "dcs:layout":[
      {"@type":"LayoutNode","dcs:isRoot":true,"dcs:children":["urn:doc:richdet#s1","urn:doc:richdet#s2","urn:doc:richdet#s3"]},
      {"@type":"LayoutNode","@id":"urn:doc:richdet#s1","dcs:children":["urn:doc:richdet#c1","urn:doc:richdet#c2"]},
      {"@type":"LayoutNode","@id":"urn:doc:richdet#s2","dcs:children":["urn:doc:richdet#c3"]},
      {"@type":"LayoutNode","@id":"urn:doc:richdet#s3","dcs:children":["urn:doc:richdet#c4","urn:doc:richdet#c5"]}
    ],
    "dcs:blocks":[
      {"@type":"Section","@id":"urn:doc:richdet#s1","dcs:title":"1. Grant"},
      {"@type":"Clause","@id":"urn:doc:richdet#c1","dcs:content":["The Supplier grants exclusive rights."]},
      {"@type":"Clause","@id":"urn:doc:richdet#c2","dcs:content":["The Territory is the UK and Ireland."]},
      {"@type":"Section","@id":"urn:doc:richdet#s2","dcs:title":"2. Obligations"},
      {"@type":"Clause","@id":"urn:doc:richdet#c3","dcs:content":["The Distributor purchases 1000 units per quarter."]},
      {"@type":"Section","@id":"urn:doc:richdet#s3","dcs:title":"3. Term"},
      {"@type":"Clause","@id":"urn:doc:richdet#c4","dcs:content":["This Agreement runs for three years."]},
      {"@type":"Clause","@id":"urn:doc:richdet#c5","dcs:content":["Renewal is by mutual consent."]}
    ]
  },
  "dcs:signatureFields":[
    {"@type":"SignatureField","@id":"urn:doc:richdet#sig-a","dcs:signatoryName":"did:web:dcs-a"},
    {"@type":"SignatureField","@id":"urn:doc:richdet#sig-b","dcs:signatoryName":"did:web:dcs-b"}
  ]
}`

func TestRichCanonicalizeDeterministic(t *testing.T) {
	a, err := CanonicalizePayload([]byte(richDetPayload))
	if err != nil { t.Fatal(err) }
	for i := 0; i < 8; i++ {
		b, err := CanonicalizePayload([]byte(richDetPayload))
		if err != nil { t.Fatal(err) }
		if !bytes.Equal(a, b) {
			t.Fatalf("CanonicalizePayload NON-DETERMINISTIC on run %d\nA=%s\n\nB=%s", i, a, b)
		}
	}
}

func TestRichDoubleCompilePageContent(t *testing.T) {
	ctx := context.Background()
	p1, err := CompilePDF(WithSigner(ctx, NewCapturingSigner()), []byte(richDetPayload), CanonicalCompiledAt)
	if err != nil { t.Fatal(err) }
	for i := 0; i < 5; i++ {
		p2, err := CompilePDF(WithSigner(ctx, NewCapturingSigner()), []byte(richDetPayload), CanonicalCompiledAt)
		if err != nil { t.Fatal(err) }
		if err := MatchPageContent(p1, p2); err != nil {
			t.Fatalf("rich double-compile page content differs on run %d: %v", i, err)
		}
	}
}
