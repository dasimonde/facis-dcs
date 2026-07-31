Feature: Verification witness
  Scenario: Verified PDFs are returned as append-only incremental updates
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:verify",
        "@type": "ContractTemplate",
        "documentTitle": "Verification Witness",
        "metadata": {
          "@type": "TemplateMetadata",
          "title": "Verification Witness"
        },
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {
              "@type": "LayoutNode",
              "isRoot": true,
              "children": ["urn:doc:verify#s1", "urn:doc:verify#s2"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:verify#s1",
              "children": ["urn:doc:verify#c1", "urn:doc:verify#c2"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:verify#s2",
              "children": ["urn:doc:verify#c3"]
            }
          ],
          "blocks": [
            {
              "@type": "Section",
              "@id": "urn:doc:verify#s1",
              "title": "1. Verification"
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:verify#c1",
              "content": [
                "Verification re-renders the ",
                "prov:Entity",
                " extracted from the embedded JSON-LD payload."
              ]
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:verify#c2",
              "content": [
                "The ",
                "prov:Activity",
                " sealing this witness was performed by a ",
                "prov:SoftwareAgent",
                " acting as the compiler runtime."
              ]
            },
            {
              "@type": "Section",
              "@id": "urn:doc:verify#s2",
              "title": "2. Integrity"
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:verify#c3",
              "content": [
                "Witness bytes are appended without disturbing the original prefix, preserving the ",
                "prov:Bundle",
                " integrity."
              ]
            }
          ]
        }
      }
      """
    And I compile the payload through /download
    When I verify the compiled PDF through /verify
    Then the response content type is "application/json"
    And the verified PDF is longer than the original
    And the verified PDF preserves the original bytes as a prefix
    And the verified PDF C2PA attachment contains two manifest boxes
    And the verified PDF preserves the original manifest bytes as the parent chain node
    And the verified PDF C2PA attachment contains the c2pa.ingredient assertion marker
    And the verified PDF ingredient references the parent manifest with matching hash
    And the verified PDF C2PA attachment includes an opened action with ingredient references
    And the active update manifest claim references c2pa.hash.data
    And the active update manifest c2pa.hash.data assertion hash matches the verified PDF bytes
    And the verified PDF contains these markers:
      | marker                  |
      | /Prev                   |
      | verification-witness    |
      | /Type /XRef             |
