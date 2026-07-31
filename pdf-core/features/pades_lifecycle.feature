Feature: PAdES signature lifecycle

  Background:
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:pades-lifecycle",
        "@type": "ContractTemplate",
        "documentTitle": "PAdES Lifecycle Test Document",
        "metadata": {
          "@type": "TemplateMetadata",
          "title": "PAdES Lifecycle Test Document"
        },
        "signatureFields": [
          {"@type": "SignatureField", "@id": "urn:doc:pades-lifecycle#SignerOne", "signatoryName": "SignerOne"},
          {"@type": "SignatureField", "@id": "urn:doc:pades-lifecycle#SignerTwo", "signatoryName": "SignerTwo"}
        ],
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {
              "@type": "LayoutNode",
              "isRoot": true,
              "children": ["urn:doc:pades-lifecycle#s1"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:pades-lifecycle#s1",
              "children": ["urn:doc:pades-lifecycle#c1"]
            }
          ],
          "blocks": [
            {"@type": "Section", "@id": "urn:doc:pades-lifecycle#s1", "title": "1. Original Terms"},
            {"@type": "Clause", "@id": "urn:doc:pades-lifecycle#c1", "content": ["This is the original clause at version one."]}
          ]
        }
      }
      """

  Scenario: Compiled PDF includes visible signature fields
    Given I compile the payload through /download
    Then the PDF contains these markers:
      | marker    |
      | /AcroForm |
      | /FT /Sig  |
      | /T (SignerOne) |
      | /T (SignerTwo) |

  Scenario: Sign, amend, re-sign — original signature remains verifiable over its byte range
    Given I compile the payload through /download

    # Round 1 — first signer applies a PAdES signature to the freshly compiled PDF
    When I apply a PAdES signature to the compiled PDF at field "SignerOne"
    Then the signed PDF is longer than the compiled PDF
    And the signed PDF preserves the compiled PDF bytes as a prefix
    And the signed PDF contains a valid PAdES signature
    And the embedded c2pa.hash.data assertion hash matches the compiled document bytes

    # Amend — new clause added; incremental update appended after the signed bytes
    Given an amended semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:pades-lifecycle",
        "@type": "ContractTemplate",
        "documentTitle": "PAdES Lifecycle Test Document",
        "metadata": {
          "@type": "TemplateMetadata",
          "title": "PAdES Lifecycle Test Document"
        },
        "signatureFields": [
          {"@type": "SignatureField", "@id": "urn:doc:pades-lifecycle#SignerOne", "signatoryName": "SignerOne"},
          {"@type": "SignatureField", "@id": "urn:doc:pades-lifecycle#SignerTwo", "signatoryName": "SignerTwo"}
        ],
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {
              "@type": "LayoutNode",
              "isRoot": true,
              "children": [
                "urn:doc:pades-lifecycle#s1",
                "urn:doc:pades-lifecycle#s2"
              ]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:pades-lifecycle#s1",
              "children": ["urn:doc:pades-lifecycle#c1"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:pades-lifecycle#s2",
              "children": ["urn:doc:pades-lifecycle#c2"]
            }
          ],
          "blocks": [
            {"@type": "Section", "@id": "urn:doc:pades-lifecycle#s1", "title": "1. Original Terms"},
            {"@type": "Clause", "@id": "urn:doc:pades-lifecycle#c1", "content": ["This is the original clause at version one."]},
            {"@type": "Section", "@id": "urn:doc:pades-lifecycle#s2", "title": "2. Amendment"},
            {"@type": "Clause", "@id": "urn:doc:pades-lifecycle#c2", "content": ["Second clause added in amendment round one."]}
          ]
        }
      }
      """
    When I update the signed PDF with the amended payload through /update
    Then the response content type is "application/pdf"
    And the amended PDF is longer than the original
    And the amended PDF preserves the original bytes as a prefix
    And the amended PDF embeds the new JSON-LD payload
    And the amended PDF C2PA attachment contains 2 manifest boxes
    And the amended PDF C2PA preserves the compiled manifest as the parent chain node
    And the amended PDF C2PA ingredient references the compiled manifest with matching hash
    And the active manifest c2pa.hash.data hash matches the amended PDF bytes
    And the active manifest claim references c2pa.hash.data in the amended PDF

    # Round 2 — second signer countersigns the amended PDF
    When I apply a PAdES signature to the amended PDF at field "SignerTwo"
    Then the re-signed PDF is longer than the amended PDF
    And the re-signed PDF preserves the amended PDF bytes as a prefix
    And the re-signed PDF contains a valid PAdES signature
    And the C2PA signature box in the re-signed PDF is a valid COSE_Sign1 structure

    # Verify: first signature still intact over its original byte range
    And the first PAdES signature byte range is covered by the re-signed PDF bytes unchanged
    And the re-signed PDF contains 2 PAdES signatures

  Scenario: Amend twice then sign — single signature covers the full history
    Given I compile the payload through /download
    And an amended semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:pades-lifecycle",
        "@type": "ContractTemplate",
        "documentTitle": "PAdES Lifecycle Test Document",
        "metadata": {
          "@type": "TemplateMetadata",
          "title": "PAdES Lifecycle Test Document"
        },
        "signatureFields": [
          {"@type": "SignatureField", "@id": "urn:doc:pades-lifecycle#SignerOne", "signatoryName": "SignerOne"},
          {"@type": "SignatureField", "@id": "urn:doc:pades-lifecycle#SignerTwo", "signatoryName": "SignerTwo"}
        ],
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {
              "@type": "LayoutNode",
              "isRoot": true,
              "children": ["urn:doc:pades-lifecycle#s1"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:pades-lifecycle#s1",
              "children": ["urn:doc:pades-lifecycle#c1"]
            }
          ],
          "blocks": [
            {"@type": "Section", "@id": "urn:doc:pades-lifecycle#s1", "title": "1. Terms"},
            {"@type": "Clause", "@id": "urn:doc:pades-lifecycle#c1", "content": ["Clause one revised in first amendment."]}
          ]
        }
      }
      """
    When I update the compiled PDF with the amended payload through /update
    And an amended semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:pades-lifecycle",
        "@type": "ContractTemplate",
        "documentTitle": "PAdES Lifecycle Test Document",
        "metadata": {
          "@type": "TemplateMetadata",
          "title": "PAdES Lifecycle Test Document"
        },
        "signatureFields": [
          {"@type": "SignatureField", "@id": "urn:doc:pades-lifecycle#SignerOne", "signatoryName": "SignerOne"},
          {"@type": "SignatureField", "@id": "urn:doc:pades-lifecycle#SignerTwo", "signatoryName": "SignerTwo"}
        ],
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {
              "@type": "LayoutNode",
              "isRoot": true,
              "children": ["urn:doc:pades-lifecycle#s1"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:pades-lifecycle#s1",
              "children": ["urn:doc:pades-lifecycle#c1"]
            }
          ],
          "blocks": [
            {"@type": "Section", "@id": "urn:doc:pades-lifecycle#s1", "title": "1. Terms"},
            {"@type": "Clause", "@id": "urn:doc:pades-lifecycle#c1", "content": ["Clause one revised in second amendment."]}
          ]
        }
      }
      """
    When I update the amended PDF with the second amended payload through /update
    And I apply a PAdES signature to the twice-amended PDF at field "SignerOne"
    Then the re-signed PDF contains a valid PAdES signature
    And the re-signed PDF preserves the twice-amended PDF bytes as a prefix
    And the C2PA signature box in the re-signed PDF is a valid COSE_Sign1 structure
    And the twice-amended PDF C2PA attachment contains 3 manifest boxes
    And the active manifest c2pa.hash.data hash matches the twice-amended PDF bytes
    And the active manifest claim references c2pa.hash.data in the twice-amended PDF

  Scenario: Sign, update, countersign, SignerOne refreshes — three-signature lifecycle
    Given I compile the payload through /download

    # Round 1 — SignerOne signs the freshly compiled PDF
    When I apply a PAdES signature to the compiled PDF at field "SignerOne"
    Then the signed PDF is longer than the compiled PDF
    And the signed PDF preserves the compiled PDF bytes as a prefix
    And the signed PDF contains a valid PAdES signature

    # Amend — add a second clause after signing
    Given an amended semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:pades-lifecycle",
        "@type": "ContractTemplate",
        "documentTitle": "PAdES Lifecycle Test Document",
        "metadata": {
          "@type": "TemplateMetadata",
          "title": "PAdES Lifecycle Test Document"
        },
        "signatureFields": [
          {"@type": "SignatureField", "@id": "urn:doc:pades-lifecycle#SignerOne", "signatoryName": "SignerOne"},
          {"@type": "SignatureField", "@id": "urn:doc:pades-lifecycle#SignerTwo", "signatoryName": "SignerTwo"}
        ],
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {
              "@type": "LayoutNode",
              "isRoot": true,
              "children": [
                "urn:doc:pades-lifecycle#s1",
                "urn:doc:pades-lifecycle#s2"
              ]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:pades-lifecycle#s1",
              "children": ["urn:doc:pades-lifecycle#c1"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:pades-lifecycle#s2",
              "children": ["urn:doc:pades-lifecycle#c2"]
            }
          ],
          "blocks": [
            {"@type": "Section", "@id": "urn:doc:pades-lifecycle#s1", "title": "1. Original Terms"},
            {"@type": "Clause", "@id": "urn:doc:pades-lifecycle#c1", "content": ["This is the original clause at version one."]},
            {"@type": "Section", "@id": "urn:doc:pades-lifecycle#s2", "title": "2. Amendment"},
            {"@type": "Clause", "@id": "urn:doc:pades-lifecycle#c2", "content": ["Clause added after SignerOne signed."]}
          ]
        }
      }
      """
    When I update the signed PDF with the amended payload through /update
    Then the response content type is "application/pdf"
    And the amended PDF is longer than the original
    And the amended PDF preserves the original bytes as a prefix
    And the amended PDF embeds the new JSON-LD payload
    And the amended PDF C2PA attachment contains 2 manifest boxes
    And the amended PDF C2PA preserves the compiled manifest as the parent chain node
    And the amended PDF C2PA ingredient references the compiled manifest with matching hash
    And the active manifest c2pa.hash.data hash matches the amended PDF bytes
    And the active manifest claim references c2pa.hash.data in the amended PDF

    # SignerTwo countersigns the amended PDF
    When I apply a PAdES signature to the amended PDF at field "SignerTwo"
    Then the re-signed PDF is longer than the amended PDF
    And the re-signed PDF preserves the amended PDF bytes as a prefix
    And the re-signed PDF contains a valid PAdES signature
    And the C2PA signature box in the re-signed PDF is a valid COSE_Sign1 structure

    # SignerOne refreshes their signature to cover the full amended+countersigned state
    When I apply a PAdES signature to the re-signed PDF at field "SignerOne"
    Then the final PDF contains 3 PAdES signatures
    And all PAdES signatures in the final PDF are valid
    And the final PDF preserves the re-signed PDF bytes as a prefix
    And the C2PA signature box in the final PDF is a valid COSE_Sign1 structure
    And the C2PA content hash in the final PDF validates against the amended document
    And the active manifest claim references c2pa.hash.data in the final PDF
