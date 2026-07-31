Feature: Structural markers
  Scenario: Generated PDFs expose the expected semantic markers
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:markers",
        "@type": "ContractTemplate",
        "documentTitle": "Semantic Marker Ledger",
        "metadata": {
          "@type": "TemplateMetadata",
          "title": "Semantic Marker Ledger"
        },
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {
              "@type": "LayoutNode",
              "isRoot": true,
              "children": ["urn:doc:markers#s1", "urn:doc:markers#s2"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:markers#s1",
              "children": ["urn:doc:markers#c1", "urn:doc:markers#c2"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:markers#s2",
              "children": ["urn:doc:markers#c3"]
            }
          ],
          "blocks": [
            {
              "@type": "Section",
              "@id": "urn:doc:markers#s1",
              "title": "1. Observations"
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:markers#c1",
              "content": [
                "Each ",
                "sosa:Observation",
                " in this collection was produced by a calibrated ",
                "sosa:Sensor",
                "."
              ]
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:markers#c2",
              "content": [
                "The ",
                "sosa:ObservableProperty",
                " is linked to the originating ",
                "sosa:Sensor",
                " via the ",
                "ssn:implements",
                " relation."
              ]
            },
            {
              "@type": "Section",
              "@id": "urn:doc:markers#s2",
              "title": "2. Provenance"
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:markers#c3",
              "content": [
                "A semantic glossary is appended after the body clauses by resolving the SOSA ontology at runtime."
              ]
            }
          ]
        }
      }
      """
    When I compile the payload through /download
    Then the PDF contains these markers:
      | marker                    |
      | /Type /Catalog            |
      | /MarkInfo << /Marked true |
      | /StructTreeRoot           |
      | /OutputIntents            |
      | /AF [                     |
      | /AFRelationship /Source   |
      | /EmbeddedFile             |
      | /Subtype /application#2Fld+json |
      | /Annots [                 |
      | /XYZ 54.00                |
      | /Type /StructTreeRoot     |
      | /S /Document              |
      | /S /Sect                  |
      | /S /H1                    |
      | /Outlines                 |
    And the PDF exposes positive non-overlapping text coordinates
