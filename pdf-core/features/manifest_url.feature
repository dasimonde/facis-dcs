Feature: Remote manifest URL embedding (DCS-OR-C2PA-008)
  # Note: c2pa-rs 0.85.1 (c2patool 0.26.61) rejects remote_manifests in V2 claims
  # ("unknown V2 claim field: remote_manifests"). When the /update endpoint is
  # given a manifest_url in the multipart body, the URL is embedded via a
  # normal "dcs.remote_manifests" C2PA assertion (mirroring dcs.lifecycle) and
  # an XMP dcterms:provenance link — never as a field on the claim itself.

  Scenario: Update without manifest_url produces no remote_manifests entry
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:manifest-url-absent",
        "@type": "ContractTemplate",
        "documentTitle": "No Remote Manifest",
        "metadata": {
          "@type": "TemplateMetadata",
          "title": "No Remote Manifest"
        },
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {
              "@type": "LayoutNode",
              "isRoot": true,
              "children": ["urn:doc:manifest-url-absent#s1"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:manifest-url-absent#s1",
              "children": ["urn:doc:manifest-url-absent#c1"]
            }
          ],
          "blocks": [
            {
              "@type": "Section",
              "@id": "urn:doc:manifest-url-absent#s1",
              "title": "1. Provenance"
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:manifest-url-absent#c1",
              "content": ["Base document."]
            }
          ]
        }
      }
      """
    And I compile the payload through /download
    When I amend the PDF with a new payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:manifest-url-absent",
        "@type": "ContractTemplate",
        "documentTitle": "No Remote Manifest",
        "metadata": {
          "@type": "TemplateMetadata",
          "title": "No Remote Manifest"
        },
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {
              "@type": "LayoutNode",
              "isRoot": true,
              "children": ["urn:doc:manifest-url-absent#s1"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:manifest-url-absent#s1",
              "children": ["urn:doc:manifest-url-absent#c1", "urn:doc:manifest-url-absent#c2"]
            }
          ],
          "blocks": [
            {
              "@type": "Section",
              "@id": "urn:doc:manifest-url-absent#s1",
              "title": "1. Provenance"
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:manifest-url-absent#c1",
              "content": ["Base document."]
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:manifest-url-absent#c2",
              "content": ["Amendment one."]
            }
          ]
        }
      }
      """
    Then the response status is 200
    And the updated PDF C2PA manifest contains no remote manifest URL

  Scenario: Update with manifest_url embeds a dcs.remote_manifests assertion, not a claim field
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:manifest-url-present",
        "@type": "ContractTemplate",
        "documentTitle": "Remote Manifest Present",
        "metadata": {
          "@type": "TemplateMetadata",
          "title": "Remote Manifest Present"
        },
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {
              "@type": "LayoutNode",
              "isRoot": true,
              "children": ["urn:doc:manifest-url-present#s1"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:manifest-url-present#s1",
              "children": ["urn:doc:manifest-url-present#c1"]
            }
          ],
          "blocks": [
            {
              "@type": "Section",
              "@id": "urn:doc:manifest-url-present#s1",
              "title": "1. Provenance"
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:manifest-url-present#c1",
              "content": ["Base document."]
            }
          ]
        }
      }
      """
    And I compile the payload through /download
    When I amend the PDF with a new payload and manifest_url "http://127.0.0.1:8080/api/c2pa/manifest/urn:doc:manifest-url-present":
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:manifest-url-present",
        "@type": "ContractTemplate",
        "documentTitle": "Remote Manifest Present",
        "metadata": {
          "@type": "TemplateMetadata",
          "title": "Remote Manifest Present"
        },
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {
              "@type": "LayoutNode",
              "isRoot": true,
              "children": ["urn:doc:manifest-url-present#s1"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:manifest-url-present#s1",
              "children": ["urn:doc:manifest-url-present#c1", "urn:doc:manifest-url-present#c2"]
            }
          ],
          "blocks": [
            {
              "@type": "Section",
              "@id": "urn:doc:manifest-url-present#s1",
              "title": "1. Provenance"
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:manifest-url-present#c1",
              "content": ["Base document."]
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:manifest-url-present#c2",
              "content": ["Amendment one."]
            }
          ]
        }
      }
      """
    Then the response status is 200
    And the updated PDF C2PA claim has no remote_manifests field
    And the updated PDF C2PA manifest embeds a dcs.remote_manifests assertion for "http://127.0.0.1:8080/api/c2pa/manifest/urn:doc:manifest-url-present"

  Scenario: POST /manifest/extract returns the embedded JUMBF manifest store
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:manifest-extract",
        "@type": "ContractTemplate",
        "documentTitle": "Manifest Extract Test",
        "metadata": {
          "@type": "TemplateMetadata",
          "title": "Manifest Extract Test"
        },
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {
              "@type": "LayoutNode",
              "isRoot": true,
              "children": ["urn:doc:manifest-extract#s1"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:manifest-extract#s1",
              "children": ["urn:doc:manifest-extract#c1"]
            }
          ],
          "blocks": [
            {
              "@type": "Section",
              "@id": "urn:doc:manifest-extract#s1",
              "title": "1. Content"
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:manifest-extract#c1",
              "content": ["Extract the manifest."]
            }
          ]
        }
      }
      """
    And I compile the payload through /download
    When I extract the C2PA manifest store from the compiled PDF
    Then the response status is 200
    And the response content type is "application/octet-stream"
    And the manifest store response contains the JUMBF marker

  Scenario: POST /manifest/extract rejects non-PDF content type
    Given the compiler service is running
    When I POST plain text to "/manifest/extract"
    Then the response status is 415
