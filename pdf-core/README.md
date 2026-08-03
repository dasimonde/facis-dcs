# dcs-pdf-core

Deterministic semantic PDF/A-3a compiler and ledger engine.

Same JSON-LD payload always produces identical human-readable page content. The compiled PDF embeds the canonical JSON-LD as an attachment and carries a C2PA provenance chain that covers every byte of visible page content.

---

## Overview

`dcs-pdf-core` takes structured legal/contractual content expressed as JSON-LD, compiles it into a PDF/A-3a document, and attaches cryptographic provenance (C2PA) to the result. The key guarantee is **re-rendering**: extracting the embedded JSON-LD from any version of the PDF (original, amended, signed) and recompiling it produces byte-for-byte identical page content streams.

The system is designed for deterministic document management where:
- The same contract text must always render identically regardless of when or where it is compiled
- All human-visible content must be traceable to a specific payload via C2PA
- Amendments are tracked as incremental PDF updates, preserving the full history
- External signatures are supported via AcroForm signature fields

---

## Architecture

```
JSON-LD payload
       │
       ▼
CanonicalizePayload
  URDNA2015 N-Quads → SHA-256 → PayloadHash
  JSON-LD compaction with stable context
       │
       ▼
CompilePDF
  Extract document model (IRI-based, language-independent)
  Layout: title, sections, subsections, glossary, signature fields
  Render: PDF/A-3a content streams with tagged structure tree
  Embed: canonical JSON-LD as /EmbeddedFile attachment
  Attach: C2PA manifest (covers all page bytes; signed if keys present)
       │
       ▼
PDF/A-3a output
```

Amendments append to the PDF via incremental update (preserving original bytes). C2PA manifests stack; the latest manifest covers the current page content.

---

## Payload Format

Payloads are JSON-LD documents. The `@context` must map terms to the `dcs-pdf-core` namespace.

### Minimal example

```json
{
  "@context": {
    "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
    "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
  },
  "@id": "urn:doc:my-agreement",
  "@type": "dcs-pdf-core:Document",
  "title": "Service Agreement",
  "sections": [
    {
      "@type": "dcs-pdf-core:Section",
      "heading": "1. Definitions",
      "clauses": ["\"Service\" means the platform described herein."]
    }
  ]
}
```

### Sections and clauses

Sections have a `heading` and a list of `clauses`. Each clause is either a plain string or a mixed-content array:

```json
"clauses": [
  "Plain text clause.",
  {"content": [
    "Subject to the applicable ",
    {"@id": "odrl:Policy"},
    "."
  ]}
]
```

Clause segment types:
- `prose` — plain text
- `ontology-link` — term from the ontology glossary (renders as hyperlink to glossary)
- `external-link` — external URI
- `typed-value` — value with optional unit (`{"@value": "30", "unit": "days"}`)

### Subsections

Sections nest to arbitrary depth via the `subsections` property. Each subsection is itself a `Section` with `heading`, `clauses`, and optional `subsections`:

```json
"sections": [
  {
    "@type": "dcs-pdf-core:Section",
    "heading": "1. Main Section",
    "clauses": ["Main clause."],
    "subsections": [
      {
        "@type": "dcs-pdf-core:Section",
        "heading": "1.1 First Subsection",
        "clauses": ["Sub clause."],
        "subsections": [
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "1.1.1 Deep Subsection",
            "clauses": ["Deep clause."]
          }
        ]
      }
    ]
  }
]
```

Subsections are indented (18pt per depth level), use smaller heading font sizes (H2/H3/H4 tags), and appear in the PDF outline hierarchy.

### Signature fields

```json
"signatureFields": [
  {
    "@type": "dcs-pdf-core:SignatureField",
    "name": "sig-party-a",
    "label": "Party A Signature"
  }
]
```

Signature fields render on a dedicated page at the end of the document.

### Non-rendered semantic extensions

Any additional JSON-LD properties not defined in the `dcs-pdf-core` ontology are silently carried through to the embedded JSON-LD attachment. They are not rendered in the PDF. This enables attaching machine-readable semantic data (ODRL policies, provenance graphs, etc.) alongside the human-readable content:

```json
{
  "@context": {
    "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
    "odrl": "http://www.w3.org/ns/odrl/2/"
  },
  "title": "Licensed Agreement",
  "sections": [...],
  "odrl:hasPolicy": {
    "@type": "odrl:Policy",
    "odrl:permission": [...]
  }
}
```

The ODRL data is preserved in the embedded attachment and verifiable via the C2PA manifest; it does not appear in the page content.

---

## API Endpoints

All endpoints are served on `DCS_PDF_CORE_ADDR` (default `0.0.0.0:8080`).

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/download` | Compile JSON-LD payload → PDF/A-3a |
| `POST` | `/verify` | Verify PDF integrity; append C2PA witness |
| `POST` | `/update` | Amend PDF with new payload (multipart: `pdf` + `payload`) |
| `POST` | `/claim` | Bind an external payload to a PDF lacking embedded metadata |
| `GET` | `/ontology/dcs-pdf-core` | JSON-LD context document |
| `GET` | `/ontology/dcs-pdf-core.owl` | OWL ontology as JSON-LD |

### POST /download

```bash
curl -X POST http://localhost:8080/download \
  -H "Content-Type: application/ld+json" \
  --data-binary @contract.jsonld \
  -o document.pdf
```

### POST /verify

Verifies that the PDF was compiled from its embedded payload (re-renders and compares BT/ET content blocks), then appends a C2PA verification witness.

```bash
curl -X POST http://localhost:8080/verify \
  -H "Content-Type: application/pdf" \
  --data-binary @document.pdf \
  -o verified.pdf
```

### POST /update

Amends an existing PDF with a new payload. Returns 409 Conflict if the payload produces identical content.

```bash
curl -X POST http://localhost:8080/update \
  -F "pdf=@document.pdf" \
  -F "payload=@amended.jsonld" \
  -o amended.pdf
```

### POST /claim

Verifies that an external payload produces the same page content as the submitted PDF. Returns the canonical PDF (with embedded payload and C2PA witness) as evidence.

```bash
curl -X POST http://localhost:8080/claim \
  -F "pdf=@external.pdf" \
  -F "payload=@contract.jsonld" \
  -o claimed.pdf
```

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DCS_PDF_CORE_ADDR` | `0.0.0.0:8080` | Listen address |
| `DCS_PDF_CORE_ONTOLOGY_BASE_URL` | `http://127.0.0.1:8080` | Base URL for ontology IRIs in payloads and served context |

C2PA manifest signing is a two-step prepare/embed flow, and pdf-core holds no
signing material at all — no key and no certificate: the render endpoints return
the compiled PDF with zeroed 64-byte COSE signature slots plus the COSE
Sig_structures to be signed. The DCS backend signs those Sig_structures
in-process with its PKCS#11 dcs-c2pa key (ES256, COSE alg -7) and posts the
64-byte r||s signatures back to `POST /c2pa/embed`, which only fills the slots.

The certificate chain that goes into the COSE protected header is supplied by
the caller on every request in the `X-DCS-C2PA-X5Chain` header, base64-encoded
PEM. It is not configured on pdf-core.

### Ontology IRI configuration

When deployed behind a public URL, set `DCS_PDF_CORE_ONTOLOGY_BASE_URL` to the public base. The served JSON-LD context and OWL documents will use that base URL, and payloads submitted with that base URL will be correctly parsed:

```bash
DCS_PDF_CORE_ONTOLOGY_BASE_URL=https://docs.example.com ./dcspdfcore
```

Payloads must then use the matching `@vocab`:

```json
{
  "@context": {
    "@vocab": "https://docs.example.com/ontology/dcs-pdf-core#"
  }
}
```

---

## Deployment

### Docker

```bash
docker build -t dcs-pdf-core .

docker run -p 8080:8080 \
  -e DCS_PDF_CORE_ONTOLOGY_BASE_URL=https://docs.example.com \
  dcs-pdf-core
```

### Build from source

```bash
make build
./dcspdfcore
```

Requires Go 1.25+.

---

## Development

```bash
make gen        # Regenerate Goa transport (needed after design/design.go changes)
make build      # Build binary
make test       # Unit and integration tests
make bdd        # BDD scenarios (requires running server)
make test-all   # Everything
```

BDD scenarios require a running server instance and the `godog` CLI. Start the server before running `make bdd`.

---

## Limitations

- Single font (Helvetica built-in); no custom fonts or images
- English-only rendering; no RTL or non-Latin script support
- Page size: US Letter (612×792 pt)
- No interactive form filling; signature fields are AcroForm placeholders only
- C2PA manifest covers page content; embedded attachment is not independently C2PA-signed

### No parallel / order-independent multi-signing

Multiple PAdES signatures on a single PDF/A-3 document are necessarily **sequential**.
There is no way to let several parties sign the *same* compiled document independently
and offline, then merge ("consolidate") their signatures into one file while keeping it
PDF/A-3 conformant. This is a fundamental constraint of the standards, not an
implementation gap.

The reason is ISO 19005-3 (PDF/A-3) clause 6.4.3, which requires that every signature's
`/ByteRange` cover the **entire file except that signature's own `/Contents`**. veraPDF
enforces this exactly (`doesByteRangeCoverEntireDocument`): for each signature it
recomputes the expected `[0, contentsStart, contentsEnd, toEOF]` range from the raw bytes
and demands a byte-for-byte match. The consequences:

- Each signature must cover every other signature that appears before it in the file. So
  signatures are inherently nested/ordered — signer *N* attests to the document *as
  already signed by* signers *1…N-1*.
- "Mutual exclusion" (no signature covering another's bytes) is the direct opposite of
  what the clause demands, and would require an impossible cycle (A signs B while B signs
  A). It cannot be PDF/A-3 conformant.
- Reserving large placeholder slots is also blocked independently: clause 6.1.13 forbids
  any string longer than 32767 bytes, and clause 6.4.3 t2/t3 require each `/Contents` to
  be a valid single-signer PKCS#7 — so empty/oversized reserved slots fail on their own.

Practically: sign in a fixed order using `/update` (each amendment + signature is a new
incremental revision over the prior signed bytes). If genuinely order-independent,
parallel attestations are ever required, they must be carried as detached CMS in PDF/A-3
associated files or as C2PA assertions — **not** as AcroForm signature fields (which
means no native viewer "signature panel").
