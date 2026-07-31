Feature: Subsection rendering, accessibility anchoring, and amendment lifecycle

  Sections nest to arbitrary depth via the LayoutNode children hierarchy.
  Each subsection level must:
    - appear in the PDF bookmark outline, nested under its parent
    - be indented further right than its parent
    - carry an /H2, /H3, or /H4 struct tag matching its depth
    - have its /Sect struct element properly nested inside the parent /Sect
      (not as a direct child of /Document) — this is the PDF/A-3a accessibility
      anchoring requirement
  The re-rendering guarantee applies to subsection content equally: amending a
  subsection clause and recompiling from the embedded JSON-LD must produce
  identical page content to a fresh compile of the amended payload.

  Background:
    Given the compiler service is running

  # ── Scenario 1 ──────────────────────────────────────────────────────────────
  # Three-level nesting: headings and struct tags must reflect depth.
  Scenario: Three-level subsection document compiles with correct PDF/A-3a structure
    Given a semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:subsection-structure",
        "@type": "ContractTemplate",
        "documentTitle": "Nested Obligations Document",
        "metadata": {
          "@type": "TemplateMetadata",
          "title": "Nested Obligations Document"
        },
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {
              "@type": "LayoutNode",
              "isRoot": true,
              "children": [
                "urn:doc:subsection-structure#s1",
                "urn:doc:subsection-structure#s2"
              ]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:subsection-structure#s1",
              "children": [
                "urn:doc:subsection-structure#c1",
                "urn:doc:subsection-structure#s1-1",
                "urn:doc:subsection-structure#s1-2"
              ]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:subsection-structure#s1-1",
              "children": [
                "urn:doc:subsection-structure#c2",
                "urn:doc:subsection-structure#s1-1-1"
              ]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:subsection-structure#s1-1-1",
              "children": [
                "urn:doc:subsection-structure#c3",
                "urn:doc:subsection-structure#c4"
              ]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:subsection-structure#s1-2",
              "children": ["urn:doc:subsection-structure#c5"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:subsection-structure#s2",
              "children": [
                "urn:doc:subsection-structure#c6",
                "urn:doc:subsection-structure#c7",
                "urn:doc:subsection-structure#s2-1"
              ]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:subsection-structure#s2-1",
              "children": ["urn:doc:subsection-structure#c8"]
            }
          ],
          "blocks": [
            {"@type": "Section", "@id": "urn:doc:subsection-structure#s1", "title": "1. General Obligations"},
            {"@type": "Clause", "@id": "urn:doc:subsection-structure#c1", "content": ["The parties shall comply with all applicable regulations."]},
            {"@type": "Section", "@id": "urn:doc:subsection-structure#s1-1", "title": "1.1 Data Protection"},
            {"@type": "Clause", "@id": "urn:doc:subsection-structure#c2", "content": ["Each party shall act as an independent data controller."]},
            {"@type": "Section", "@id": "urn:doc:subsection-structure#s1-1-1", "title": "1.1.1 Breach Notification"},
            {"@type": "Clause", "@id": "urn:doc:subsection-structure#c3", "content": ["A party suffering a data breach shall notify the other within 72 hours."]},
            {"@type": "Clause", "@id": "urn:doc:subsection-structure#c4", "content": ["Notification shall include the nature of the breach and remediation steps taken."]},
            {"@type": "Section", "@id": "urn:doc:subsection-structure#s1-2", "title": "1.2 Health and Safety"},
            {"@type": "Clause", "@id": "urn:doc:subsection-structure#c5", "content": ["Each party shall maintain a safe working environment at all times."]},
            {"@type": "Section", "@id": "urn:doc:subsection-structure#s2", "title": "2. Dispute Resolution"},
            {"@type": "Clause", "@id": "urn:doc:subsection-structure#c6", "content": ["Disputes shall first be referred to a senior representative of each party."]},
            {"@type": "Clause", "@id": "urn:doc:subsection-structure#c7", "content": ["Unresolved disputes shall be submitted to binding arbitration."]},
            {"@type": "Section", "@id": "urn:doc:subsection-structure#s2-1", "title": "2.1 Arbitration Rules"},
            {"@type": "Clause", "@id": "urn:doc:subsection-structure#c8", "content": ["Arbitration shall be conducted under the ICC Rules of Arbitration."]}
          ]
        }
      }
      """
    And I compile the payload through /download
    Then the response status is 200
    And the response content type is "application/pdf"
    And the PDF contains these markers:
      | marker                          |
      | /H2                             |
      | /H3                             |
      | /S /H2                          |
      | /S /H3                          |
      | /Title (1.1 Data Protection)    |
      | /Title (1.1.1 Breach Notification) |
      | /Title (1.2 Health and Safety)  |
      | /Title (2.1 Arbitration Rules)  |
    And all saved PDF artifacts are validated by c2patool and dockerized veraPDF CLI

  # ── Scenario 2 ──────────────────────────────────────────────────────────────
  # Re-rendering from extracted JSON-LD must include all subsection content.
  Scenario: Re-rendering from extracted JSON-LD preserves all subsection headings
    Given a semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:subsection-rerender",
        "@type": "ContractTemplate",
        "documentTitle": "Environmental Compliance Policy",
        "metadata": {
          "@type": "TemplateMetadata",
          "title": "Environmental Compliance Policy"
        },
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {
              "@type": "LayoutNode",
              "isRoot": true,
              "children": [
                "urn:doc:subsection-rerender#s1",
                "urn:doc:subsection-rerender#s2"
              ]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:subsection-rerender#s1",
              "children": [
                "urn:doc:subsection-rerender#c1",
                "urn:doc:subsection-rerender#s1-1",
                "urn:doc:subsection-rerender#s1-2"
              ]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:subsection-rerender#s1-1",
              "children": ["urn:doc:subsection-rerender#c2"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:subsection-rerender#s1-2",
              "children": ["urn:doc:subsection-rerender#c3"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:subsection-rerender#s2",
              "children": [
                "urn:doc:subsection-rerender#c4",
                "urn:doc:subsection-rerender#s2-1"
              ]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:subsection-rerender#s2-1",
              "children": [
                "urn:doc:subsection-rerender#c5",
                "urn:doc:subsection-rerender#c6"
              ]
            }
          ],
          "blocks": [
            {"@type": "Section", "@id": "urn:doc:subsection-rerender#s1", "title": "1. Scope"},
            {"@type": "Clause", "@id": "urn:doc:subsection-rerender#c1", "content": ["This policy applies to all operations of the Organisation."]},
            {"@type": "Section", "@id": "urn:doc:subsection-rerender#s1-1", "title": "1.1 Definitions"},
            {"@type": "Clause", "@id": "urn:doc:subsection-rerender#c2", "content": ["\"Operations\" includes manufacturing, logistics, and office activities."]},
            {"@type": "Section", "@id": "urn:doc:subsection-rerender#s1-2", "title": "1.2 Exclusions"},
            {"@type": "Clause", "@id": "urn:doc:subsection-rerender#c3", "content": ["Research and development activities are governed by a separate policy."]},
            {"@type": "Section", "@id": "urn:doc:subsection-rerender#s2", "title": "2. Reporting"},
            {"@type": "Clause", "@id": "urn:doc:subsection-rerender#c4", "content": ["Annual environmental reports shall be submitted to the Board."]},
            {"@type": "Section", "@id": "urn:doc:subsection-rerender#s2-1", "title": "2.1 Metrics"},
            {"@type": "Clause", "@id": "urn:doc:subsection-rerender#c5", "content": ["Reports shall include carbon footprint, water usage, and waste volumes."]},
            {"@type": "Clause", "@id": "urn:doc:subsection-rerender#c6", "content": ["Metrics shall be independently verified by a registered environmental auditor."]}
          ]
        }
      }
      """
    And I compile the payload through /download
    When I extract and recompile the embedded JSON-LD from the "compiled" PDF
    Then the recompiled PDF page content matches a fresh compile of the original payload

  # ── Scenario 3 ──────────────────────────────────────────────────────────────
  # Amend a subsection clause; re-rendering from every lifecycle stage must
  # match a fresh compile of that version's payload.
  Scenario: Amending a subsection clause preserves re-rendering across sign and re-verify
    Given a semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:subsection-amendment",
        "@type": "ContractTemplate",
        "documentTitle": "Procurement Policy v1",
        "metadata": {
          "@type": "TemplateMetadata",
          "title": "Procurement Policy v1"
        },
        "signatureFields": [
          {"@type": "SignatureField", "@id": "urn:doc:subsection-structure#sig-cpo", "signatoryName": "sig-cpo"}
        ],
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {
              "@type": "LayoutNode",
              "isRoot": true,
              "children": [
                "urn:doc:subsection-amendment#s1",
                "urn:doc:subsection-amendment#s2"
              ]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:subsection-amendment#s1",
              "children": [
                "urn:doc:subsection-amendment#c1",
                "urn:doc:subsection-amendment#s1-1",
                "urn:doc:subsection-amendment#s1-2"
              ]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:subsection-amendment#s1-1",
              "children": [
                "urn:doc:subsection-amendment#c2",
                "urn:doc:subsection-amendment#c3"
              ]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:subsection-amendment#s1-2",
              "children": ["urn:doc:subsection-amendment#c4"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:subsection-amendment#s2",
              "children": [
                "urn:doc:subsection-amendment#c5",
                "urn:doc:subsection-amendment#c6"
              ]
            }
          ],
          "blocks": [
            {"@type": "Section", "@id": "urn:doc:subsection-amendment#s1", "title": "1. Supplier Selection"},
            {"@type": "Clause", "@id": "urn:doc:subsection-amendment#c1", "content": ["Suppliers shall be assessed against cost, quality, and delivery criteria."]},
            {"@type": "Section", "@id": "urn:doc:subsection-amendment#s1-1", "title": "1.1 Tendering Threshold"},
            {"@type": "Clause", "@id": "urn:doc:subsection-amendment#c2", "content": ["Contracts exceeding ten thousand (10,000) GBP require a formal tender process."]},
            {"@type": "Clause", "@id": "urn:doc:subsection-amendment#c3", "content": ["The Procurement Committee shall evaluate all tenders."]},
            {"@type": "Section", "@id": "urn:doc:subsection-amendment#s1-2", "title": "1.2 Preferred Suppliers"},
            {"@type": "Clause", "@id": "urn:doc:subsection-amendment#c4", "content": ["The preferred supplier list shall be reviewed annually."]},
            {"@type": "Section", "@id": "urn:doc:subsection-amendment#s2", "title": "2. Contract Management"},
            {"@type": "Clause", "@id": "urn:doc:subsection-amendment#c5", "content": ["All contracts shall be managed by a designated Contract Owner."]},
            {"@type": "Clause", "@id": "urn:doc:subsection-amendment#c6", "content": ["Contract renewals require approval from the Finance Director."]}
          ]
        }
      }
      """
    And I compile the payload through /download

    # Baseline: re-render from compiled document.
    When I extract and recompile the embedded JSON-LD from the "compiled" PDF
    Then the recompiled PDF page content matches a fresh compile of the original payload

    # CPO signs the policy.
    When I apply a PAdES signature to the compiled PDF at field "sig-cpo"
    Then the signed PDF contains a valid PAdES signature
    And the signed PDF has no extra AcroForm signature fields
    When I extract and recompile the embedded JSON-LD from the "signed" PDF
    Then the recompiled PDF page content matches a fresh compile of the original payload

    # Board updates the tendering threshold and adds a new emergency procurement subsection.
    When an amended semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:subsection-amendment",
        "@type": "ContractTemplate",
        "documentTitle": "Procurement Policy v2 (Amended)",
        "metadata": {
          "@type": "TemplateMetadata",
          "title": "Procurement Policy v2 (Amended)"
        },
        "signatureFields": [
          {"@type": "SignatureField", "@id": "urn:doc:subsection-structure#sig-cpo", "signatoryName": "sig-cpo"}
        ],
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {
              "@type": "LayoutNode",
              "isRoot": true,
              "children": [
                "urn:doc:subsection-amendment#s1",
                "urn:doc:subsection-amendment#s2"
              ]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:subsection-amendment#s1",
              "children": [
                "urn:doc:subsection-amendment#c1",
                "urn:doc:subsection-amendment#s1-1",
                "urn:doc:subsection-amendment#s1-2",
                "urn:doc:subsection-amendment#s1-3"
              ]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:subsection-amendment#s1-1",
              "children": [
                "urn:doc:subsection-amendment#c2",
                "urn:doc:subsection-amendment#c3",
                "urn:doc:subsection-amendment#c4"
              ]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:subsection-amendment#s1-2",
              "children": ["urn:doc:subsection-amendment#c5"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:subsection-amendment#s1-3",
              "children": [
                "urn:doc:subsection-amendment#c6",
                "urn:doc:subsection-amendment#c7"
              ]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:subsection-amendment#s2",
              "children": [
                "urn:doc:subsection-amendment#c8",
                "urn:doc:subsection-amendment#c9"
              ]
            }
          ],
          "blocks": [
            {"@type": "Section", "@id": "urn:doc:subsection-amendment#s1", "title": "1. Supplier Selection"},
            {"@type": "Clause", "@id": "urn:doc:subsection-amendment#c1", "content": ["Suppliers shall be assessed against cost, quality, and delivery criteria."]},
            {"@type": "Section", "@id": "urn:doc:subsection-amendment#s1-1", "title": "1.1 Tendering Threshold"},
            {"@type": "Clause", "@id": "urn:doc:subsection-amendment#c2", "content": ["Contracts exceeding twenty-five thousand (25,000) GBP require a formal tender process."]},
            {"@type": "Clause", "@id": "urn:doc:subsection-amendment#c3", "content": ["The Procurement Committee shall evaluate all tenders."]},
            {"@type": "Clause", "@id": "urn:doc:subsection-amendment#c4", "content": ["Sole-source justification must be documented for single-supplier awards."]},
            {"@type": "Section", "@id": "urn:doc:subsection-amendment#s1-2", "title": "1.2 Preferred Suppliers"},
            {"@type": "Clause", "@id": "urn:doc:subsection-amendment#c5", "content": ["The preferred supplier list shall be reviewed annually."]},
            {"@type": "Section", "@id": "urn:doc:subsection-amendment#s1-3", "title": "1.3 Emergency Procurement"},
            {"@type": "Clause", "@id": "urn:doc:subsection-amendment#c6", "content": ["In circumstances of operational emergency, the CEO may waive tender requirements."]},
            {"@type": "Clause", "@id": "urn:doc:subsection-amendment#c7", "content": ["Emergency procurement shall be ratified by the Board within thirty (30) days."]},
            {"@type": "Section", "@id": "urn:doc:subsection-amendment#s2", "title": "2. Contract Management"},
            {"@type": "Clause", "@id": "urn:doc:subsection-amendment#c8", "content": ["All contracts shall be managed by a designated Contract Owner."]},
            {"@type": "Clause", "@id": "urn:doc:subsection-amendment#c9", "content": ["Contract renewals require approval from the Finance Director."]}
          ]
        }
      }
      """
    And I update the signed PDF with the amended payload through /update
    Then the amended PDF is longer than the original
    And the amended PDF preserves the original bytes as a prefix
    And a fresh compile of the amended payload has different page content from the compiled PDF

    # Re-render from the amended document — must match the amended payload, not v1.
    When I extract and recompile the embedded JSON-LD from the "amended" PDF
    Then the recompiled PDF page content matches a fresh compile of the amended payload

    # CPO signs again and compliance verifies the full lifecycle.
    When I apply a PAdES signature to the amended PDF at field "sig-cpo"
    Then the re-signed PDF contains a valid PAdES signature
    When I extract and recompile the embedded JSON-LD from the "re-signed" PDF
    Then the recompiled PDF page content matches a fresh compile of the amended payload
    When I verify the "re-signed" PDF through /verify
    And I extract and recompile the embedded JSON-LD from the "verified" PDF
    Then the recompiled PDF page content matches a fresh compile of the amended payload

    And all saved PDF artifacts are validated by c2patool and dockerized veraPDF CLI
