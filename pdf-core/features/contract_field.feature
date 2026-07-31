Feature: dcs:ContractField rendering in human-readable PDF

  A clause references a top-level dcs:ContractField by a bare {"@id"} node.
  The DCS copies the placeholder's dcs:label and, once filled, its dcs:value onto
  that in-content reference. The compiler renders the filled value, or five
  underscores where a value has not yet been supplied — never the raw @id.

  Scenario: An unfilled placeholder reference renders as five underscores
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:placeholder-test",
        "@type": "ContractTemplate",
        "documentTitle": "Placeholder Rendering Test",
        "metadata": {
          "@type": "TemplateMetadata",
          "title": "Placeholder Rendering Test"
        },
        "contractFields": [
          {
            "@id": "urn:doc:placeholder-test#field-party-name",
            "@type": "Placeholder",
            "label": "Party Name",
            "datatype": "xsd:string",
            "required": true
          }
        ],
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {
              "@type": "LayoutNode",
              "isRoot": true,
              "children": ["urn:doc:placeholder-test#s1"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:placeholder-test#s1",
              "children": ["urn:doc:placeholder-test#c1"]
            }
          ],
          "blocks": [
            {
              "@type": "Section",
              "@id": "urn:doc:placeholder-test#s1",
              "title": "1. Parties"
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:placeholder-test#c1",
              "content": [
                "This agreement is entered into by ",
                {
                  "@id": "urn:doc:placeholder-test#field-party-name",
                  "label": "Party Name"
                },
                "."
              ]
            }
          ]
        }
      }
      """
    When I compile the payload through /download
    Then the response status is 200
    And the PDF contains these markers:
      | marker  |
      | _____   |

  Scenario: A filled placeholder reference renders its value, not its @id
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:placeholder-filled",
        "@type": "ContractTemplate",
        "documentTitle": "Filled Placeholder Test",
        "metadata": {
          "@type": "TemplateMetadata",
          "title": "Filled Placeholder Test"
        },
        "contractFields": [
          {
            "@id": "urn:doc:placeholder-filled#field-amount",
            "@type": "Placeholder",
            "label": "Payment Amount",
            "datatype": "xsd:decimal",
            "required": true,
            "value": 15000
          }
        ],
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {
              "@type": "LayoutNode",
              "isRoot": true,
              "children": ["urn:doc:placeholder-filled#s1"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:placeholder-filled#s1",
              "children": ["urn:doc:placeholder-filled#c1"]
            }
          ],
          "blocks": [
            {
              "@type": "Section",
              "@id": "urn:doc:placeholder-filled#s1",
              "title": "1. Payment"
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:placeholder-filled#c1",
              "content": [
                "The amount payable is ",
                {
                  "@id": "urn:doc:placeholder-filled#field-amount",
                  "label": "Payment Amount",
                  "value": 15000
                },
                " EUR."
              ]
            }
          ]
        }
      }
      """
    When I compile the payload through /download
    Then the response status is 200
    And the PDF contains these markers:
      | marker |
      | 15000  |
