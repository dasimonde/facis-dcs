Feature: Payload ontology requirements
  Scenario: Missing metadata is rejected
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:req-missing-metadata",
        "@type": "ContractTemplate",
        "documentTitle": "Missing Metadata",
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:req-missing-metadata#s1"]},
            {"@type": "LayoutNode", "@id": "urn:doc:req-missing-metadata#s1", "children": ["urn:doc:req-missing-metadata#c1"]}
          ],
          "blocks": [
            {"@type": "Section", "@id": "urn:doc:req-missing-metadata#s1", "title": "1. Intro"},
            {"@type": "Clause", "@id": "urn:doc:req-missing-metadata#c1", "content": ["Clause text."]}
          ]
        }
      }
      """
    When I compile the payload through /download
    Then the response status is 400
    And the response body contains "payload failed SHACL validation"
    And the response body contains "ontology/v1#metadata"

  Scenario: Missing metadata title is rejected
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:req-missing-title",
        "@type": "ContractTemplate",
        "documentTitle": "Has Metadata But No Title",
        "metadata": {
          "@type": "TemplateMetadata"
        },
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:req-missing-title#s1"]},
            {"@type": "LayoutNode", "@id": "urn:doc:req-missing-title#s1", "children": ["urn:doc:req-missing-title#c1"]}
          ],
          "blocks": [
            {"@type": "Section", "@id": "urn:doc:req-missing-title#s1", "title": "1. Intro"},
            {"@type": "Clause", "@id": "urn:doc:req-missing-title#c1", "content": ["Clause text."]}
          ]
        }
      }
      """
    When I compile the payload through /download
    Then the response status is 400
    And the response body contains "payload failed SHACL validation"
    And the response body contains "ontology/v1#title"

  Scenario: Multiple metadata objects are rejected
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:req-multi-metadata",
        "@type": "ContractTemplate",
        "documentTitle": "Multiple Metadata",
        "metadata": [
          {"@type": "TemplateMetadata", "title": "Version A"},
          {"@type": "TemplateMetadata", "title": "Version B"}
        ],
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:req-multi-metadata#s1"]},
            {"@type": "LayoutNode", "@id": "urn:doc:req-multi-metadata#s1", "children": ["urn:doc:req-multi-metadata#c1"]}
          ],
          "blocks": [
            {"@type": "Section", "@id": "urn:doc:req-multi-metadata#s1", "title": "1. Intro"},
            {"@type": "Clause", "@id": "urn:doc:req-multi-metadata#c1", "content": ["Clause text."]}
          ]
        }
      }
      """
    When I compile the payload through /download
    Then the response status is 400
    And the response body contains "ontology/v1#metadata"
    And the response body contains "MaxCountConstraintComponent"

  Scenario: Equivalent JSON-LD flavors compile to identical output
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:req-flavors",
        "@type": "ContractTemplate",
        "documentTitle": "Flavor Equivalence",
        "metadata": {
          "@type": "TemplateMetadata",
          "title": "Flavor Equivalence"
        },
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:req-flavors#s1"]},
            {"@type": "LayoutNode", "@id": "urn:doc:req-flavors#s1", "children": ["urn:doc:req-flavors#c1"]}
          ],
          "blocks": [
            {"@type": "Section", "@id": "urn:doc:req-flavors#s1", "title": "1. Terms"},
            {"@type": "Clause", "@id": "urn:doc:req-flavors#c1", "content": ["Reference to dcs:Clause remains equivalent across JSON-LD flavors."]}
          ]
        }
      }
      """
    And an equivalent semantic payload flavor:
      """
      {
        "@context": {
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:req-flavors",
        "@type": "dcs:ContractTemplate",
        "dcs:documentTitle": "Flavor Equivalence",
        "dcs:metadata": {
          "@type": "dcs:TemplateMetadata",
          "dcs:title": "Flavor Equivalence"
        },
        "dcs:documentStructure": {
          "@type": "dcs:DocumentStructure",
          "dcs:layout": [
            {"@type": "dcs:LayoutNode", "dcs:isRoot": true, "dcs:children": ["urn:doc:req-flavors#s1"]},
            {"@type": "dcs:LayoutNode", "@id": "urn:doc:req-flavors#s1", "dcs:children": ["urn:doc:req-flavors#c1"]}
          ],
          "dcs:blocks": [
            {"@type": "dcs:Section", "@id": "urn:doc:req-flavors#s1", "dcs:title": "1. Terms"},
            {"@type": "dcs:Clause", "@id": "urn:doc:req-flavors#c1", "dcs:content": ["Reference to dcs:Clause remains equivalent across JSON-LD flavors."]}
          ]
        }
      }
      """
    When I compile both payload flavors through /download
    Then both compiled payload flavors are byte-for-byte identical
