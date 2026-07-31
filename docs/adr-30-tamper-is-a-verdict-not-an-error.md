# ADR-30: A tampered artifact is a verification verdict, not an internal error

Status: accepted
Date: 2026-07-29

## Context

Encrypting IPFS artifacts at rest (ADR-28) changed what happens when a stored
artifact does not match what the instance wrote. Before encryption, altered
bytes were fetched and handed to the verifier, which compared the embedded
JSON-LD against a recompiled base layer and reported a hash mismatch. After
encryption, the fetch fails first: the content is sealed with the scope's CEK
and bound to the scope through GCM AAD, so altered or substituted bytes fail
authenticated decryption before any verification logic runs.

The verify endpoints treated that failure as a fetch error and returned 500.
Three BDD tamper scenarios that had passed for months began failing — not
because tampering went undetected, but because detection surfaced as a crash
instead of an answer:

```
verify contract PDF <id>: fetch contract PDF <id> from IPFS for verify:
decrypt contract <id> artifact <cid>: decrypt content: cipher: message
authentication failed
```

A second problem followed. The verify result could only distinguish failure
classes through `c2pa_manifest_found`, which is set from pdf-core's HTTP 409
"manifest present, content hash comparison failed" answer. That signal requires
reading the PDF. For bytes that cannot be decrypted there is nothing to read, so
every such failure collapsed into an undifferentiated negative — and the
scenario asserting that the system identifies the *specific* discrepancy could
no longer pass honestly.

## Decision

**An artifact that fails authenticated decryption is reported as a failed
verification, not as an error.** The CEK comes from this instance's own
repository and the AAD is derived from the scope, so an authentication failure
cannot mean a key-management fault. The only thing it can mean is that the
stored bytes are not the ones this instance sealed. That is precisely what a
caller asking "is this artifact intact?" wants to know, and the answer is "no",
not HTTP 500.

`artifactstore.Get` wraps the failure as `TamperedError`, mirroring the existing
`ShreddedError` convention ("callsites map it to a 4xx, never a 500"). The
contract and template verify paths map it to a negative verdict; signature
validation reports the *embedded signing evidence* as invalid rather than as a
fetch problem, because that evidence lives inside the very bytes that failed.

**The verify result names its failure class directly.** A new `discrepancy`
field carries one of:

| value | meaning |
|---|---|
| `content_hash_mismatch` | manifest present, content differs from the embedded JSON-LD |
| `artifact_not_authentic` | stored bytes failed authenticated decryption |
| `verification_failed` | any other check failure |
| *(empty)* | `match` is true |

Callers no longer infer the class from combinations of booleans, and
`c2pa_manifest_found` is left to mean only what it can actually attest.

## Consequences

- Tamper detection is stronger than before, not weaker. Previously the system
  reported that stored content disagreed with its own JSON-LD. It now reports
  that the stored bytes are provably not the ones written — a claim about
  authenticity rather than internal consistency, and one an attacker cannot
  forge without the CEK.
- `artifact_not_authentic` is only reachable because artifacts are encrypted.
  On an unencrypted store, substituted bytes are indistinguishable from
  legitimately stored bytes that happen to disagree with their JSON-LD.
- A caller cannot learn *how* tampered content differs. That information is
  destroyed by the AEAD check, deliberately: the plaintext is never derived from
  unauthenticated ciphertext, so there is nothing to diff. This is the right
  trade, but it means "which clause changed?" is not answerable for an artifact
  that fails authentication, only for one that decrypts and then mismatches.
- Test seams that plant bytes directly in IPFS now exercise the authenticity
  path rather than the hash-comparison path. Both remain covered: the hash
  comparison still runs for artifacts that decrypt successfully.
