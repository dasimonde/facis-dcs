# ADR-26: Provenance is re-anchored after signing, and the signature keeps priority

Status: Accepted (2026-07-27).

## Context

A C2PA manifest's `c2pa.hash.data` assertion binds the whole file. Anything
appended after the manifest is written changes the bytes it hashed, so the
binding no longer matches. **The manifest must therefore be last.**

A PAdES signature covers the document up to its own `/ByteRange`. It commits to
what the signer saw. **The signature must therefore be last.**

Both cannot be last, and a contract's signing flow needs both. The current
order is deliberate — `stampLifecycleForSigning` writes the lifecycle manifest
*before* PAdES signing, "so the signature commits to the PDF's final
lifecycle-bearing content, and the signed artefact never needs a
post-signature revision".

The observable consequence, on artifacts produced by the two-instance vertical:

| Stage | C2PA verdict |
|---|---|
| offer, counter, counter, settle (both instances) | **Valid** |
| signed, double-signed (both instances) | `assertion.dataHash.mismatch` |

The signature layer appends roughly 99 KB and three PDF revisions after the
last manifest, so that manifest's hard binding cannot match the file it now
lives in.

## Decision

**The signature is applied over the provenance, and the provenance is then
re-anchored over the signature.** Both hold.

The order stays as it is — the lifecycle manifest is written first, so the
signature commits to the provenance it covers. Afterwards a **provenance-only**
C2PA update manifest is appended, whose hard binding covers the signed bytes.
An incremental update leaves the signature's `/ByteRange` untouched, so the
signature remains cryptographically valid.

The accepted cost is that PDF readers report the document as **modified since
signed**. That is a true statement about what happened: provenance was recorded
after signing. A signature that still verifies with that note is preferable to
one whose provenance silently fails a whole-file hash, and it is the trade this
project accepts deliberately rather than by omission.

**Signature validity keeps priority in any future conflict.** Nothing may be
appended that would break the signature's byte range, and no provenance
requirement outranks a signature verifying in external tools.

**C2PA provenance must hold through offer and negotiation too**, where no PAdES
layer exists. Those artifacts validate, and a regression there is a defect.

**A `dataHash` mismatch is a real failure everywhere except on the narrow window
between signing and re-anchoring.** It must not be treated as routine: once the
re-anchor runs, a signed contract validates like any other.

## What DSS reports, measured

Validated through the project's DSS instance, before and after re-anchoring the
same signed contract:

| | before | after |
|---|---|---|
| Indication | INDETERMINATE / NO_CERTIFICATE_CHAIN_FOUND | unchanged |
| Signature scope | `Full PDF` | `Partial PDF` |
| AdES warning | — | "The document contains undefined object modifications after the signature revision!" |

The only *error* is the dev certificate's missing trust anchor, which is the
project's known bar and is unaffected. With a trusted certificate the signature
still passes; what changes is that DSS sees a revision it cannot account for.

That warning is exactly what genuine post-signature tampering produces, so it
must not simply be tolerated. Reporting a re-anchored contract as good requires
proving the post-signature delta **is** the re-anchor: the amendment
verification path re-derives an appended revision from the document's own
bytes, so "good" means the only thing appended after signing is provenance this
instance produced, verified byte for byte — not that the warning was ignored.

## Consequences

A signed contract carries one more manifest than an unsigned one: the
lifecycle manifest the signature commits to, and the re-anchor that commits to
the signature. The chain reads in that order and says exactly what happened.

PDF readers annotate the document as modified after signing. Anyone presenting
a signed contract should expect that note and be able to say what produced it.

The signature's byte range is load-bearing for everything appended afterwards.
A future change that rewrites rather than appends would break it silently in
external tools while every check here still passed, so appends stay appends.

## Alternatives considered

**Leave the binding stopping at the signature.** Documented as expected and
interpreted condition-aware, so a signed contract would never claim a green
C2PA verdict. Rejected once the "modified since signed" note was judged
acceptable: it gives up a real property — whole-file provenance on the artifact
that matters most — to avoid a reader annotation that accurately describes what
occurred.

**Exclude the signature region from the hard binding.** C2PA data-hash
assertions carry exclusion ranges, and the compiler already emits them for the
manifest stream. Rejected as not currently reachable: the range is fixed when
the manifest is written, and signing happens externally (wallet plus DSS)
afterwards, so its size and offset are unknown at that point. It becomes
possible only if the signature occupies a pre-allocated, fixed-size region
filled in place rather than appended — a different signing pipeline, and the
avenue to revisit if both properties are ever required at once.

**Drop PAdES and rely on C2PA alone.** Rejected: C2PA is not an electronic
signature under eIDAS, and external tools cannot check it.
