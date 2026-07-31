Feature: Large document page wrapping
  Scenario: A document with many sections, ontology terms, and signatures wraps across multiple pages
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:large-document",
        "@type": "ContractTemplate",
        "documentTitle": "Master Services Agreement",
        "metadata": {
          "@type": "TemplateMetadata",
          "title": "Master Services Agreement"
        },
        "signatureFields": [
          {"@type": "SignatureField", "@id": "urn:doc:large-document#ClientSignature", "signatoryName": "ClientSignature"},
          {"@type": "SignatureField", "@id": "urn:doc:large-document#ProviderSignature", "signatoryName": "ProviderSignature"},
          {"@type": "SignatureField", "@id": "urn:doc:large-document#WitnessSignature", "signatoryName": "WitnessSignature"}
        ],
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {
              "@type": "LayoutNode",
              "isRoot": true,
              "children": [
                "urn:doc:large-document#s1",
                "urn:doc:large-document#s2",
                "urn:doc:large-document#s3",
                "urn:doc:large-document#s4",
                "urn:doc:large-document#s5",
                "urn:doc:large-document#s6",
                "urn:doc:large-document#s7",
                "urn:doc:large-document#s8"
              ]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:large-document#s1",
              "children": [
                "urn:doc:large-document#c1",
                "urn:doc:large-document#c2",
                "urn:doc:large-document#c3"
              ]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:large-document#s2",
              "children": [
                "urn:doc:large-document#c4",
                "urn:doc:large-document#c5",
                "urn:doc:large-document#c6"
              ]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:large-document#s3",
              "children": [
                "urn:doc:large-document#c7",
                "urn:doc:large-document#c8",
                "urn:doc:large-document#c9"
              ]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:large-document#s4",
              "children": [
                "urn:doc:large-document#c10",
                "urn:doc:large-document#c11",
                "urn:doc:large-document#c12"
              ]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:large-document#s5",
              "children": [
                "urn:doc:large-document#c13",
                "urn:doc:large-document#c14",
                "urn:doc:large-document#c15"
              ]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:large-document#s6",
              "children": [
                "urn:doc:large-document#c16",
                "urn:doc:large-document#c17",
                "urn:doc:large-document#c18"
              ]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:large-document#s7",
              "children": [
                "urn:doc:large-document#c19",
                "urn:doc:large-document#c20",
                "urn:doc:large-document#c21"
              ]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:large-document#s8",
              "children": [
                "urn:doc:large-document#c22",
                "urn:doc:large-document#c23",
                "urn:doc:large-document#c24"
              ]
            }
          ],
          "blocks": [
            {"@type": "Section", "@id": "urn:doc:large-document#s1", "title": "1. Scope and Purpose"},
            {
              "@type": "Clause",
              "@id": "urn:doc:large-document#c1",
              "content": [
                "This agreement governs all services delivered as a ",
                "prov:Activity",
                " by the provider, constituting a formal ",
                "odrl:Policy",
                " binding on all parties."
              ]
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:large-document#c2",
              "content": ["The scope of this document extends to all subsidiaries, affiliates, and third-party contractors operating under the authority of the principal party as defined in section two."]
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:large-document#c3",
              "content": ["Nothing in this section shall be construed to limit the rights granted under applicable law or any superseding regulatory framework in the relevant jurisdiction."]
            },

            {"@type": "Section", "@id": "urn:doc:large-document#s2", "title": "2. Definitions"},
            {
              "@type": "Clause",
              "@id": "urn:doc:large-document#c4",
              "content": [
                "Service means the deterministic document compilation platform. Each compiled document is a ",
                "prov:Entity",
                " whose provenance is fully captured in the embedded JSON-LD graph."
              ]
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:large-document#c5",
              "content": [
                "Payload means any structured JSON-LD graph submitted to the compilation endpoint. The canonical form of the payload is the authoritative source for the ",
                "prov:wasDerivedFrom",
                " relationship recorded in the manifest."
              ]
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:large-document#c6",
              "content": [
                "Artifact means any PDF produced by the service. The artifact is a ",
                "prov:Entity",
                " generated by a ",
                "prov:Activity",
                " associated with a ",
                "prov:SoftwareAgent",
                " acting as the compiler runtime."
              ]
            },

            {"@type": "Section", "@id": "urn:doc:large-document#s3", "title": "3. Obligations of the Provider"},
            {
              "@type": "Clause",
              "@id": "urn:doc:large-document#c7",
              "content": ["The provider shall maintain the compilation service in an operational state for no less than ninety-nine point nine percent of each calendar month, measured across all documented API endpoints."]
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:large-document#c8",
              "content": [
                "The provider shall ensure that each compiled artifact faithfully represents the semantic content of the original ",
                "prov:Entity",
                " graph, with all ",
                "odrl:Permission",
                " grants recorded accurately."
              ]
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:large-document#c9",
              "content": ["The provider shall not retain submitted payload data beyond the duration required to produce the compiled artifact, except where retention is required by applicable law."]
            },

            {"@type": "Section", "@id": "urn:doc:large-document#s4", "title": "4. Obligations of the Client"},
            {
              "@type": "Clause",
              "@id": "urn:doc:large-document#c10",
              "content": ["The client shall ensure that all submitted payloads comply with the JSON-LD 1.1 specification and the ontology constraints published at the service ontology endpoint."]
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:large-document#c11",
              "content": [
                "The client acknowledges that each submission constitutes a ",
                "prov:Activity",
                " and accepts responsibility for the accuracy of all asserted ",
                "prov:wasDerivedFrom",
                " relationships."
              ]
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:large-document#c12",
              "content": ["The client shall retain copies of all compiled artifacts and bears sole responsibility for their subsequent storage, distribution, and legal validity."]
            },

            {"@type": "Section", "@id": "urn:doc:large-document#s5", "title": "5. Intellectual Property"},
            {
              "@type": "Clause",
              "@id": "urn:doc:large-document#c13",
              "content": [
                "All intellectual property rights in the compilation service remain vested in the provider. Full specification is available at ",
                "https://schema.org/softwareVersion",
                "."
              ]
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:large-document#c14",
              "content": ["The client retains all intellectual property rights in the payload content submitted to the service and in the compiled artifacts produced therefrom."]
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:large-document#c15",
              "content": ["No licence is granted to either party to use the other party's trademarks, trade names, or registered marks without prior written consent."]
            },

            {"@type": "Section", "@id": "urn:doc:large-document#s6", "title": "6. Limitation of Liability"},
            {
              "@type": "Clause",
              "@id": "urn:doc:large-document#c16",
              "content": [
                "In no event shall the provider be liable for any indirect or consequential damages. The permitted remedies are constrained by the ",
                "odrl:Constraint",
                " declared in the applicable ",
                "odrl:Policy",
                "."
              ]
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:large-document#c17",
              "content": [
                "The provider's total cumulative liability shall not exceed the total fees paid by the client in the twelve months preceding the claim, expressed as the ",
                "100",
                " GBP cap unless otherwise agreed in writing."
              ]
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:large-document#c18",
              "content": ["Nothing in this section shall exclude liability for death or personal injury caused by negligence, fraud, or any other liability that cannot be excluded by law."]
            },

            {"@type": "Section", "@id": "urn:doc:large-document#s7", "title": "7. Governing Law"},
            {
              "@type": "Clause",
              "@id": "urn:doc:large-document#c19",
              "content": ["This agreement shall be governed by and construed in accordance with the laws of the applicable jurisdiction without regard to its conflict of law provisions."]
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:large-document#c20",
              "content": ["Any dispute arising under or in connection with this agreement shall be subject to the exclusive jurisdiction of the courts of the designated forum."]
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:large-document#c21",
              "content": ["The parties agree to attempt in good faith to resolve any dispute through mediation before initiating formal legal proceedings."]
            },

            {"@type": "Section", "@id": "urn:doc:large-document#s8", "title": "8. Amendments and Severability"},
            {
              "@type": "Clause",
              "@id": "urn:doc:large-document#c22",
              "content": [
                "Any amendment must be made in writing. Incremental updates applied through the ",
                "prov:Activity",
                " mechanism of the compilation service constitute valid written amendments, each producing a new ",
                "prov:Entity",
                " in the provenance chain."
              ]
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:large-document#c23",
              "content": ["If any provision of this agreement is held to be invalid, illegal, or unenforceable, the remaining provisions shall continue in full force and effect."]
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:large-document#c24",
              "content": ["The invalidity of a provision in one jurisdiction shall not affect the validity of that provision in any other jurisdiction."]
            }
          ]
        }
      }
      """
    When I compile the payload through /download
    Then the response content type is "application/pdf"
    And the compiled PDF spans at least 2 pages
    And the PDF contains these markers:
      | marker              |
      | /AcroForm           |
      | /FT /Sig            |
      | /T (ClientSignature)    |
      | /T (ProviderSignature)  |
      | /T (WitnessSignature)   |
      | /Outlines           |
      | /XYZ 54.00          |
