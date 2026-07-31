Feature: External JSON-LD claim binding
  # /claim lets a caller prove that a JSON-LD document produces the same
  # page content as a submitted PDF (which need not carry embedded metadata).
  # The service responds with the canonical compiled PDF — JSON-LD embedded,
  # C2PA manifest present — plus a verification witness, constituting machine-
  # issued evidence of the match.

  Scenario: Claim accepted for a PDF that has had its embedded JSON-LD stripped
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:claim-stripped",
        "@type": "ContractTemplate",
        "documentTitle": "Claim Test Document",
        "metadata": {
          "@type": "TemplateMetadata",
          "title": "Claim Test Document"
        },
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {
              "@type": "LayoutNode",
              "isRoot": true,
              "children": ["urn:doc:claim-stripped#s1"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:claim-stripped#s1",
              "children": ["urn:doc:claim-stripped#c1"]
            }
          ],
          "blocks": [
            {
              "@type": "Section",
              "@id": "urn:doc:claim-stripped#s1",
              "title": "1. Terms"
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:claim-stripped#c1",
              "content": ["This clause was rendered deterministically and its origin is being claimed."]
            }
          ]
        }
      }
      """
    And I compile the payload through /download
    And I strip the embedded JSON-LD from the compiled PDF
    When I claim the stripped PDF with its original payload through /claim
    Then the response content type is "application/pdf"
    And the claimed PDF is longer than the compiled PDF
    And the claimed PDF embeds the original JSON-LD payload
    And the claimed PDF contains a verification witness

  Scenario: Claim accepted for a PDF that still carries embedded metadata
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:claim-full",
        "@type": "ContractTemplate",
        "documentTitle": "Claim Test With Metadata",
        "metadata": {
          "@type": "TemplateMetadata",
          "title": "Claim Test With Metadata"
        },
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {
              "@type": "LayoutNode",
              "isRoot": true,
              "children": ["urn:doc:claim-full#s1"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:claim-full#s1",
              "children": ["urn:doc:claim-full#c1"]
            }
          ],
          "blocks": [
            {
              "@type": "Section",
              "@id": "urn:doc:claim-full#s1",
              "title": "1. Terms"
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:claim-full#c1",
              "content": ["This clause is in a PDF that still has its embedded JSON-LD present."]
            }
          ]
        }
      }
      """
    And I compile the payload through /download
    When I claim the compiled PDF with its original payload through /claim
    Then the response content type is "application/pdf"
    And the claimed PDF is longer than the compiled PDF
    And the claimed PDF embeds the original JSON-LD payload

  Scenario: Claim rejected when the payload does not match the PDF content
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:claim-mismatch",
        "@type": "ContractTemplate",
        "documentTitle": "Mismatch Source",
        "metadata": {
          "@type": "TemplateMetadata",
          "title": "Mismatch Source"
        },
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {
              "@type": "LayoutNode",
              "isRoot": true,
              "children": ["urn:doc:claim-mismatch#s1"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:claim-mismatch#s1",
              "children": ["urn:doc:claim-mismatch#c1"]
            }
          ],
          "blocks": [
            {
              "@type": "Section",
              "@id": "urn:doc:claim-mismatch#s1",
              "title": "1. Original"
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:claim-mismatch#c1",
              "content": ["Original clause content."]
            }
          ]
        }
      }
      """
    And I compile the payload through /download
    And an amended semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:claim-mismatch",
        "@type": "ContractTemplate",
        "documentTitle": "Mismatch Source",
        "metadata": {
          "@type": "TemplateMetadata",
          "title": "Mismatch Source"
        },
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {
              "@type": "LayoutNode",
              "isRoot": true,
              "children": ["urn:doc:claim-mismatch#s1"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:claim-mismatch#s1",
              "children": ["urn:doc:claim-mismatch#c1"]
            }
          ],
          "blocks": [
            {
              "@type": "Section",
              "@id": "urn:doc:claim-mismatch#s1",
              "title": "1. Original"
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:claim-mismatch#c1",
              "content": ["Completely different clause that renders differently."]
            }
          ]
        }
      }
      """
    When I claim the compiled PDF with the amended payload through /claim
    Then the response status is 409

  Scenario: Claim rejected for non-multipart content type
    Given the compiler service is running
    When I POST plain text to "/claim"
    Then the response status is 415
