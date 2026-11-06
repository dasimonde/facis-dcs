# ADR-4: C2PA embedding transport and the remote-manifest fallback

## Context

DCS-OR-C2PA-008 requires provenance verification to survive tooling that
cannot parse DCS's PDF embedding. C2PA's asset-embedding profile is
mature for JPEG/PNG/MP4; a ratified, tool-interoperable embedding profile
for PDF specifically does not yet exist. DCS's PDF manifests are
JUMBF boxes attached via the PDF's own embedded-file mechanism — readable
by pdf-core's own verifier, but not by general-purpose C2PA tooling
(c2patool, third-party verifier UIs) that expects a different container.

An earlier iteration tried to route around this by adding a non-standard
`remote_manifests` field to the C2PA claim CBOR itself — c2patool rejected
the resulting claim outright ("claim could not be converted from CBOR"),
because that field is not part of the C2PA claim schema at all.

## Decision

Two provenance paths ship, not one:

1. **Embedded (DCS-native):** the JUMBF-in-PDF transport described above,
   verified by pdf-core's own verifier and the DCS backend.
2. **Remote manifest (standards-compliant fallback):** a `dcterms:provenance`
   property in the PDF's XMP metadata, pointing at an externally hosted
   copy of the same manifest. This is the C2PA-normative mechanism for
   "provenance data lives elsewhere," and any general-purpose C2PA tool
   that cannot parse the embedded transport can still resolve provenance
   via this link (`pdf-core/compiler/compiler_pdf.go`,
   `extractRemoteManifestURLFromXMP`).

The non-standard `remote_manifests` claim field was removed entirely, not
kept as a second, parallel signal.

## Consequences

- Verification degrades gracefully: DCS's own tooling gets the richer
  embedded manifest, third-party tooling gets a standards-compliant
  pointer to the same data, and neither path emits claim structures a
  standards-compliant parser would reject.
- A full C2PA-native PDF embedding profile, if/when one is ratified, is a
  drop-in replacement for the embedded transport without touching the
  remote-manifest fallback.
- A C2PA update after PAdES signing is appended as a PDF incremental update
  that does not touch any signature's `/ByteRange`: the re-anchor and the
  lifecycle manifests of states reached after signing
  ([ADR-26](adr-26-provenance-reanchored-after-signing.md), ADR-3's ordering
  rule).
- A copy whose embedded manifest was removed is verified against the remote
  manifest it references, at this instance or at a party's instance. It must
  show exactly the pages and carry the payload of the stored PDF. The page
  comparison follows every object a page reaches and tolerates object
  renumbering, key order, string and number spelling, FlateDecode
  (re)compression and a split `/Contents` array; objects held in object
  streams are not resolved, so a copy re-saved with object streams reports a
  mismatch.
