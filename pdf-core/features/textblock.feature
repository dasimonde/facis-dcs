Feature: dcs:TextBlock rendering in human-readable PDF

  A dcs:TextBlock carries free-form prose via its dcs:text property. Every
  TextBlock referenced from the document layout must have its text rendered
  into the compiled PDF, whether it appears directly under the document root
  or nested inside a section. TextBlocks were previously dropped during layout
  traversal, leaving their text absent from the output.

  Background:
    Given the compiler service is running

  Scenario: TextBlocks render whether nested in a section or at the document root
    Given a semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:textblock-test",
        "@type": "ContractTemplate",
        "documentTitle": "TextBlock Rendering Test",
        "metadata": {
          "@type": "TemplateMetadata",
          "title": "TextBlock Rendering Test"
        },
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {
              "@type": "LayoutNode",
              "isRoot": true,
              "children": [
                "urn:doc:textblock-test#tb-root",
                "urn:doc:textblock-test#s1",
                "urn:doc:textblock-test#s2"
              ]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:textblock-test#s1",
              "children": [
                "urn:doc:textblock-test#c1",
                "urn:doc:textblock-test#tb-section"
              ]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:textblock-test#s2",
              "children": ["urn:doc:textblock-test#tb-nested"]
            }
          ],
          "blocks": [
            {
              "@type": "TextBlock",
              "@id": "urn:doc:textblock-test#tb-root",
              "text": "RootLevelTextBlockContent"
            },
            {
              "@type": "Section",
              "@id": "urn:doc:textblock-test#s1",
              "title": "1. Terms"
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:textblock-test#c1",
              "content": ["This is an ordinary clause."]
            },
            {
              "@type": "TextBlock",
              "@id": "urn:doc:textblock-test#tb-section",
              "text": "SectionTextBlockContent"
            },
            {
              "@type": "Section",
              "@id": "urn:doc:textblock-test#s2",
              "title": "2. Notes"
            },
            {
              "@type": "TextBlock",
              "@id": "urn:doc:textblock-test#tb-nested",
              "text": "NestedTextBlockContent"
            }
          ]
        }
      }
      """
    When I compile the payload through /download
    Then the response status is 200
    And the response content type is "application/pdf"
    And the PDF contains these markers:
      | marker                          |
      | (RootLevelTextBlockContent) Tj  |
      | (SectionTextBlockContent) Tj    |
      | (NestedTextBlockContent) Tj     |
