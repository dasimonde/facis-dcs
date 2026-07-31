package semantichub

import (
	"testing"
)

// gaiaXParticipantShapes is a Gaia-X Trust Framework-style excerpt: foreign
// namespace, sh:name/sh:description/sh:order metadata, a nested sh:node
// group, and an sh:in value set.
const gaiaXParticipantShapes = `@prefix gx: <https://w3id.org/gaia-x/development#> .
@prefix sh: <http://www.w3.org/ns/shacl#> .
@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .

gx:LegalParticipantShape
  a sh:NodeShape ;
  sh:targetClass gx:LegalParticipant ;
  rdfs:label "Gaia-X Legal Participant" ;
  sh:property [
    sh:path gx:legalName ;
    sh:name "Legal name" ;
    sh:description "The legal name of the participant." ;
    sh:order 1 ;
    sh:datatype xsd:string ;
    sh:minCount 1 ;
  ] ;
  sh:property [
    sh:path gx:headquarterAddress ;
    sh:name "Headquarter address" ;
    sh:order 2 ;
    sh:minCount 1 ;
    sh:node gx:AddressShape ;
  ] .

gx:AddressShape
  a sh:NodeShape ;
  sh:property [
    sh:path gx:countryCode ;
    sh:name "Country code" ;
    sh:datatype xsd:string ;
    sh:in ( "DE" "FR" "NL" ) ;
    sh:minCount 1 ;
  ] .
`

func TestParseClauseCatalogListsForeignNamespaceShapes(t *testing.T) {
	entries, err := ParseClauseCatalog(gaiaXParticipantShapes, map[string]string{
		"dcs": "https://w3id.org/facis/dcs/ontology/v1#",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly the target-classed participant shape, got %d entries: %+v", len(entries), entries)
	}
	entry := entries[0]
	if entry.Type != "https://w3id.org/gaia-x/development#LegalParticipant" {
		t.Fatalf("expected the full foreign-namespace IRI as type, got %q", entry.Type)
	}
	if entry.Label != "Gaia-X Legal Participant" {
		t.Fatalf("expected the rdfs:label, got %q", entry.Label)
	}
	if entry.Shape != "https://w3id.org/gaia-x/development#LegalParticipantShape" {
		t.Fatalf("expected the NodeShape IRI, got %q", entry.Shape)
	}
}

func TestParseClauseCatalogCompactsAgainstHubPrefixes(t *testing.T) {
	entries, err := ParseClauseCatalog(gaiaXParticipantShapes, map[string]string{
		"gx": "https://w3id.org/gaia-x/development#",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Type != "gx:LegalParticipant" {
		t.Fatalf("expected gx:LegalParticipant once the hub context declares the gx prefix, got %+v", entries)
	}
}
