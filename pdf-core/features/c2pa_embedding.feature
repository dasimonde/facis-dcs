Feature: C2PA embedding
  Scenario: Generated PDFs include core C2PA manifest-store structures
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:c2pa-coverage",
        "@type": "ContractTemplate",
        "documentTitle": "C2PA Coverage Ledger",
        "metadata": {
          "@type": "TemplateMetadata",
          "title": "C2PA Coverage Ledger"
        },
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {
              "@type": "LayoutNode",
              "isRoot": true,
              "children": ["urn:doc:c2pa-coverage#s1"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:c2pa-coverage#s1",
              "children": ["urn:doc:c2pa-coverage#c1"]
            }
          ],
          "blocks": [
            {
              "@type": "Section",
              "@id": "urn:doc:c2pa-coverage#s1",
              "title": "1. Provenance"
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:c2pa-coverage#c1",
              "content": [
                "This document records provenance through ",
                "prov:Entity",
                " relationships."
              ]
            }
          ]
        }
      }
      """
    When I compile the payload through /download
    Then the embedded C2PA attachment starts with a JUMBF superbox
    And the embedded C2PA attachment contains these markers:
      | marker          |
      | c2pa            |
      | c2pa.assertions |
      | c2pa.claim.v2   |
      | c2pa.signature  |
    And the embedded c2pa.hash.data assertion hash matches the compiled PDF bytes
    And the embedded c2pa.hash.data assertion payload contains the "pad" field
    And the embedded signing leaf certificate is x509 v3
    And the embedded signing leaf certificate includes emailProtection EKU
    And the embedded C2PA signature includes x5chain protected header key
    And all saved PDF artifacts are validated by c2patool and dockerized veraPDF CLI
