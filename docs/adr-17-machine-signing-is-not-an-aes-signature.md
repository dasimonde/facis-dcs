# ADR-17: A System User cannot produce an AES signature

## Status

Accepted (2026-07-22). Constrains the SRS System User class **System Contract
Signer** (SRS §2.4 Table 5). Builds on
[ADR-12](adr-12-wallet-driven-signing.md) (signing is wallet-driven remote AES
under the signatory's sole control) and
[ADR-16](adr-16-audit-checkpoints-external-anchoring.md) (machine callers are
authenticated as System Users by client credentials).

## Context

SRS §2.4 Table 5 defines **System Contract Signer** as *"Performs API-based
digital signing for autonomous or IoT systems"*, and the API granted that class
the same signing scopes as a human Contract Signer: `startCeremony`,
`prepareSignature`, `submitSignature`, `publishSignatureRequest`.

eIDAS does not permit this reading.

- Art. 3(9) defines a **signatory** as *a natural person* who creates an
  electronic signature.
- Art. 3(11) defines an electronic signature as data the **signatory** uses to
  sign; an *advanced* one additionally requires (Art. 26) that it is uniquely
  linked to and capable of identifying the signatory, and created using data
  the signatory can use **under their sole control**.
- A legal person's analogue is an electronic **seal** (Art. 3(25)–(27)) — a
  distinct instrument with distinct legal effect (Art. 35: a seal enjoys a
  presumption of integrity and origin, not of the signatory's will).

An unattended client authenticating with an OAuth2 client secret has no natural
person behind it and no sole control by one. Whatever it produced would be, at
best, a seal — and calling it an AES signature would misstate the legal effect
of every contract signed that way. The SRS's own target is AES (QES descoped),
so this is not a gap we can close by lowering the assurance level.

## Decision

**The System Contract Signer class holds no signing scope.** It is removed from
every endpoint that creates or carries a signature:

| Endpoint | Machine class |
|---|---|
| `startCeremony`, `prepareSignature`, `submitSignature`, `publishSignatureRequest` | **removed** |
| `retrieve`, `retrieve_by_id`, `verify`, `provenance`, `ceremonyStatus`, `view` | retained |

An integrated system may therefore watch, verify and audit a signing process it
cannot perform. That split is the point: automation keeps its read paths, and
the act that carries legal weight stays with a natural person and their wallet.

**The class is kept, not deleted.** It is in the SRS, and deleting it would
turn a documented, tested refusal into a silent absence. A deployment can
configure a machine client with `Sys. Contract Signer`
(`systemClients` in the Helm values) and observe it being refused at every
signing endpoint — which is what the BDD coverage asserts.

**Electronic seals are not implemented.** If sealing by a legal person is ever
wanted — an organization attesting a document rather than a person signing it —
it is a separate instrument needing its own key custody, its own certificate
profile (seal certificates under Annex III), its own PAdES profile and its own
SRS requirement. It must not be reached by relaxing this decision.

## Consequences

- SRS §2.4 Table 5's System Contract Signer is **not implementable as
  specified**. This ADR is the deviation record; the SRS text should be revised
  to describe an integrated system that *initiates or observes* signing rather
  than one that performs it.
- Autonomous/IoT signing use cases are not served. The honest alternative for
  them is a natural person's wallet performing the signature — remotely, under
  their sole control — with the machine orchestrating everything around it.
- The refusal is behaviour, not documentation: BDD asserts that a client
  holding only `Sys. Contract Signer` is refused at the signing endpoints and
  still permitted on the read endpoints, so a future scope change cannot
  quietly re-open machine signing.
