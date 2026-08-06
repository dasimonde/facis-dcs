# ADR-35: Peer issuers are anchored, not enumerated; login issuers are enumerated

Status: accepted
Date: 2026-08-05

Supersedes the `peer_dynamic` mechanism and the single flat anchor bundle of
ADR-31, and closes the open point that ADR recorded: that `peer` carried two
meanings and could not safely be made dynamic.

## Context

ADR-31 scoped issuer trust by purpose and made every issuer an entry in
`trust.json`. It also recorded, as an open point, that enumerating counterparty
issuers is "an allowlist wearing federation's clothes" — and that the escape it
shipped, `peer_dynamic`, had to default to `false`. The reason was structural:
`peer` gates both the counterparty attestation we receive and the Power of
Attorney presented at a signing ceremony **here**, so enabling dynamic trust
would let a party authorize its own signature off a document it publishes about
itself.

Two things have changed since.

ADR-34 moved every issuer this project deploys onto `x5c`: credentials and the
status lists they name are signed by the same leaf, chained to a root, and the
anchors an instance mounts are assembled from what each issuer actually serves
and compared by fingerprint. A certificate chain is therefore already the way
an issuer's key reaches this verifier.

And the trust document's issuer keys now match what credentials actually carry.
The demo issuers previously put a `did:web` identifier in `iss` while the trust
file was keyed the same way; they now put the issuer's HTTPS base URL there, as
ADR-31's own configuration example always showed. That removed the last reason
the https form was described in `build-trust.py` as naming "no credential's
`iss`".

So the material for anchoring exists, and enumeration is the part that does not
scale: a federation whose membership is a file edited on every instance whenever
a member joins is not a federation.

## Decision

**A peer is trusted because its issuer's certificate chain reaches an anchor we
hold, not because it appears in a list. A login issuer is trusted only because
it appears in a list.**

### The three purposes, and where each one terminates

| purpose | admitted by | verified against |
|---|---|---|
| `login` | an explicit `trust.json` entry, always | the **leaf**: the key that entry pins in `x5c_leaf_keys` |
| `peer` | an entry **or** a chain to the PoA CA list | the **root**: the PoA CA list |
| `pid` | an entry **or** a chain to the PID CA list | the **root**: the PID CA list |

**`login` terminates at the leaf.** An operator names their own organization's
issuers and pins each one's key as base64 DER `SubjectPublicKeyInfo` in
`x5c_leaf_keys`; the chain above it is not walked and no CA is consulted.

The mechanism stays `x5c` and the pin is a separate field, deliberately. An
entry bundling a `jwks` beside an `x5c` mechanism would state two resolutions
for one issuer, and only one of them can be what the operator meant — that
combination stays refused at load. `x5c_leaf_keys` does not offer an
alternative to the chain; it constrains which leaf under that chain is
this deployment's own issuer.

A key rather than a whole certificate, because the issuer re-mints its leaf when
its public URL changes and keeps the key — pinning the certificate would refuse
a legitimate reissue. DER rather than a JWK because it then covers every key
type a real issuer might hold, and the comparison is byte equality over the
leaf's own encoded key rather than a field-by-field one that a key type this
code cannot take apart would silently pass. This is stronger than enumeration alone. If a login issuer were
verified to the PoA CA list, then any CA in that list could introduce a new
login issuer for this deployment simply by issuing a certificate — and the
operator would never have named it. A session here is exactly the decision that
must not be delegated to a third party.

**`peer` and `pid` terminate at a root**, because neither can be enumerated in
advance. For peers, a federation whose membership is a file edited on every
instance whenever a member joins is not a federation. For PID, nobody can
anticipate who issues one — a holder may arrive with a PID from any national or
qualified issuer, and in production this deployment may issue them itself.

### Two CA trust lists, not one per purpose

The single `OID4VP_X5C_TRUST_ANCHORS_PATH` bundle is replaced by two:
`OID4VP_X5C_TRUST_ANCHORS_POA_PATH` and `OID4VP_X5C_TRUST_ANCHORS_PID_PATH`.

There is no third list for login, because login consults no CA at all. And the
PoA list is not split between "my issuers" and "their issuers": a login issuer
and a peer issuer are **the same certificate seen from two sides**. The Power of
Attorney a holder obtains at login here travels inside the signed PDF to the
counterparty, who verifies it as peer evidence against their PoA list — which
must therefore contain the CA that issued our login issuer. Publishing that root
into two lists would mean keeping the copies in step for no gain.

One flat pool meant a certificate under any configured anchor could sign for any
`x5c` issuer, held apart only by the leaf naming the issuer and by the entry's
purposes. With peers and PID issuers no longer enumerated, that second check
disappears for both, and a flat pool would let a CA that attests persons speak
for a party.

### What an unlisted peer or PID issuer must satisfy

Order matters, because trust is now partly a cryptographic fact:

1. The credential carries an `x5c` chain. Without one there is nothing to anchor
   and it is refused — an unlisted issuer has no other way in.
2. The chain verifies to the CA list for its purpose — PoA or PID.
3. The leaf identifies the issuer the credential names, by the rules ADR-31
   already set: a SAN URI equal to `iss`, a SAN DNS name equal to its authority,
   or an exact CN — and not a TLS certificate bound only by DNS name.
4. Only then is the authorization question asked, with the fact that the chain
   anchored passed to the policy as input. `login` is never admitted this way,
   whatever it chains to.

Steps 1–3 stay in Go, where the crypto lives. Step 4 is `policy/trust.rego`.

### What an unlisted peer may attest

Its own authority and no other. The bound comes from the identifier rather than
from configuration, which is the same rule `peer_dynamic` used, generalized from
`did:web` to the https form the issuers now carry: an issuer at
`https://example.com/issuer` — or `did:web:example.com:issuer` — may attest
`did:web:example.com`.

An entry, when one exists, still bounds the issuer explicitly and the wildcard
`"*"` still has to be written out.

### ORCE carries the additional policy

Anchoring says a chain reaches a CA we hold. It does not say we want to deal
with this counterparty today. That decision already exists and does not move:
the ADR-19 trust gate requires the peer's self-signed agreement credential to
verify against its own `did.json` and carry this instance's federation rules
hash, and the local policy endpoint (`DCS_TRUST_PDP_URL`) must approve the
interaction. A deployment that wants more — a revocation feed, a jurisdiction
rule, an allowlist after all — expresses it there, in the low-code flow, without
a code change.

That is the division: the anchor is the membership statement, ORCE is the policy
on top of it.

### `peer_dynamic` is removed

Not deprecated. It resolved an unlisted peer's key from a document that peer
publishes about itself, which is the self-attestation this ADR replaces with a
chain to a CA. Leaving both would leave the weaker one reachable by
configuration.

## Consequences

- **The anchor is the boundary for `peer` and `pid`.** Anyone holding a
  certificate under the PoA list, with a leaf naming their own issuer
  identifier, is a peer; the same holds for PID under the PID list.
  That is the intended model — a CA's issuance *is* the membership decision —
  and it means the peer anchor set must contain only CAs whose issuance is
  meaningful. Putting a public web PKI root in it would make every domain holder
  a federation member.

- **For the demo, the PoA list is still assembled from the members.**
  `build-x5c-anchors.py` collects each issuer's runtime-minted root by
  fingerprint, so membership remains an edit — moved from the issuer level to
  the CA level, not removed. The demo therefore exercises the verification path
  this ADR is about without yet demonstrating the property.

  What closes it is one CA that issues to members instead of each issuer minting
  its own root: the mechanism already exists (`orce.pkiRootCA.devFixture` hands
  an issuer a root rather than letting it generate one), so it is a matter of
  provisioning one federation root and handing it to each member's issuer, after
  which the PoA list holds a single anchor and onboarding touches nobody else's
  configuration. Until then this is a limitation of the stand-in issuers
  (ADR-34's scope note), not of the verification path.

- **A status list is now bound to the credential it governs.** ADR-34 recorded
  as an outstanding defect that no handler did this: a signature said only that
  SOME trusted issuer published the list, so any of them could publish one at
  another issuer's URI and un-revoke credentials they had no authority over. All
  three handlers now require the list's issuer to be the credential's issuer.

  That is also what lets the status-list verifier hold the union of both CA
  lists. It is one object for the process, reached without a purpose in hand, so
  it cannot be given a scoped set — and with the binding in place it does not
  need one: a list anchored for one purpose cannot speak for a credential
  admitted under another.

- **The trust decision is no longer a pure function of configuration.** For an
  unlisted peer it depends on the credential in hand, so the same issuer is
  trusted with a chain and refused without one. Denial reasons must say which of
  the two happened, or an operator cannot tell a missing anchor from a missing
  entry.

- **`peer` is no longer overloaded, and that is what makes anchoring it safe.**
  ADR-31 recorded that `peer` gated two different questions and that the second
  one blocked dynamic trust. They are now separated:

  - `peer` means **another DCS instance**. Its issuer's credentials reach us one
    way: the Power of Attorney behind a signature applied on that instance,
    embedded **inside the contract PDF** — with the signing summary that attests
    the signature it stands behind — and verified on receipt
    (`VerifyCounterpartyPoA`). It authorizes nothing locally.

    The PDF is the wire format (ADR-13), so the evidence rides in it and nowhere
    else. Every signing party embeds its OWN evidence as a PDF/A-3 associated
    file **before** applying its own signature, so that party's PAdES signature
    covers its own authorization: one attachment per signing event, carrying
    `{"summary": <ContractSigningSummaryCredential>, "poa_presentation": <the
    Power of Attorney as presented at the ceremony>}`. The embed is an
    append-only incremental update, which is what lets a countersignature add
    its own to a document the originator already signed without invalidating
    that signature (DCS-OR-C2PA-002). A countersigned contract therefore carries
    both parties' evidence, and the receiver verifies everything from the PDF.

    **Every attachment is verified, not the newest one per field.** An embed
    appends rather than replacing, so a field can legitimately appear more than
    once; taking only the last would let an appended attachment for an
    already-verified field ride along unexamined while the contract still
    carries it. Each summary is checked against the assertion key of the
    instance that OWNS the field it names — the shipper's for its own fields,
    ours for ours, and the resolved `did:web` document for anyone else — which
    is what stops a peer from embedding evidence for a party it has nothing to
    do with. Evidence for a field this instance owns is authenticated the same
    way but its Power of Attorney is not re-judged as peer evidence: that
    ceremony ran here, where the credential is a `login` question.

    Worth stating because the two are easy to conflate: the **contract lifecycle
    credential** is embedded in every PDF this deployment generates, and it is
    embedded once by the renderer. The Power of Attorney and the signing summary
    are embedded per signature, by the party making it.
  - This instance's own signing ceremony verifies the signatory's PoA as
    **`login`** — the wallet is here, and the credential came from this
    deployment's own issuer, which is enumerated and leaf-pinned.

  The ceremony previously ran under `peer`, which is what made the purpose
  ambiguous. Under this ADR that would have been a real widening: a chain to the
  PoA CA list would authorize a signature on this instance. It does not, because
  authority to act HERE is the enumerated, pinned question and nothing anchored
  reaches it.

- **Every trust file must be migrated.** A flat `x5c` anchor path is no longer
  read, and a login issuer that resolves by certificate chain must now pin its
  key — the configuration is refused at load otherwise, naming the reason.

- **A login issuer's key has to be knowable when the trust file is written.**
  That is a real operational constraint the leaf rule introduces, and it is met
  two ways. The demo instances **seed** it: `scripts/orce_trust_seed.py` reads
  the leaf the issuer publishes at `/pki/issuer.pem` and writes the key into the
  trust document, which `build-trust.py` now does for each instance's own
  issuer. The dev and BDD stacks instead mount a **fixture** issuer key
  (`deployment/helm/charts/orce/pki-dev/issuer.key`), the way they already mount
  a fixture root, so a committed `trust.dev.json` can name it and the unit tests
  stay hermetic.

  Seeding is not resolving. The operator reads the key once, at deploy time, and
  it becomes configuration the startup attestation hashes and pins. Reading it
  during verification would be the issuer telling us which key to believe it by
  — self-attestation, and for `login` that is the one thing the check exists to
  prevent.
