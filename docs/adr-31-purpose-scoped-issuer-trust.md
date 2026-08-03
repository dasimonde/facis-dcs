# ADR-31: Issuer trust is scoped by purpose, bound to an organization, and resolved by a configurable mechanism

Status: accepted
Date: 2026-07-29

## Context

Trust configuration answered one question — "is this issuer's signature
acceptable?" — and every caller reused that single answer. `trust.json` listed
issuers with their JWKS, `IssuerTrusted(iss)` returned a boolean, and login,
counterparty PoA verification and PID verification all consulted it.

That conflates authenticity with authorization, and on a two-instance
federation it fails concretely: a PoA minted by instance A's issuer produced a
valid session on instance B. Worse, the
credential's `organization` claim becomes the session's participant identity
and the attribution recorded in the audit trail — so the holder acted **as
A's party inside B**, and the two parties the federation exists to distinguish
collapsed into one.

ADR-29 made each demo issuer stamp its own host as the organization, and a
first attempt at a fix required a login credential's organization to equal the
instance's own participant DID. That was the wrong invariant: it encodes "one
instance, one organization", which the BDD suite disproves immediately — its
identities belong to Acme Corp, TechVendor Inc and a per-scenario organization,
all on the same deployment. It also left the underlying weakness untouched:
nothing bound an *issuer* to the organizations it may speak for, so any trusted
issuer could assert any party and the check depended on every issuer being
well-behaved.

Two further requirements land on the same structure:

- **QES needs a PID issuer**, and a PID is meaningfully a *third party's*
  attestation of a natural person — not something the relying party issues to
  itself. There is no PID issuer today.
- **The production issuance and trust model is not yet known.** Deployments may
  resolve issuer keys through x5c chains, bare JWKS, `did:jwk`, `did:web`,
  `did:ebsi`, or something not yet on the list. Hard-coding today's two
  mechanisms would force a code change for each new one.

## Decision

**Trust is declared per purpose, per issuer, and bound to organizations.**

```json
{
  "vcts": ["urn:dcs:poa:v1", "urn:eudi:pid:de:1"],
  "issuers": {
    "https://dcs-ionos.facis.cloud/issuer": {
      "purposes": ["login", "peer"],
      "organizations": ["did:web:dcs-ionos.facis.cloud"],
      "mechanism": "jwks",
      "jwks": { "keys": [ ... ] }
    },
    "https://dcs-osc.facis.cloud/issuer": {
      "purposes": ["peer"],
      "organizations": ["did:web:dcs-osc.facis.cloud"],
      "mechanism": "jwks",
      "jwks": { "keys": [ ... ] }
    }
  }
}
```

### Purposes

| purpose | meaning |
|---|---|
| `login` | may grant a session on **this** instance |
| `peer` | its credentials may be verified in a signing ceremony, and its attestation of a counterparty's authority to sign is believed |
| `pid` | may attest the identity of a natural person (signing ceremonies, QES) |

Purposes are configuration, not policy baked into the code. **An operator
decides which issuers grant access to their deployment** — in production that is
plausibly several: a corporate IdP-backed issuer, a federation issuer, a
customer's own. The verifier enforces only that an issuer stays within the
purposes it was granted.

The demonstration configures the narrow case, because it is the one that makes
the two-party story legible:

```
on instance A:   issuer A → login, peer      PID issuer → pid
on instance B:   issuer B → login, peer      PID issuer → pid
```

A credential from issuer B is refused at A's login because A's operator granted
it nothing — not because the code says so. A different deployment granting
`login` to five issuers is equally valid and needs no code change.

### The mutual binding

A signature is applied on one instance and read on the other. Until now only its
*claim* crossed: sealing an agreement stamps `dcs:hasPowerOfAttorney` onto the
signing party's node, and the receiving instance could read that IRI but had
nothing to check it against — a peer asserting its own authority.

**The credential now travels with the signature.** The ceremony retains the
Power of Attorney the signatory presented, next to the PID presentation it
already stored, and the synchronizer ships it with the contract once that
contract carries signatures (`signatory_poas` on the PDF ship). The receiver
verifies it before it persists anything of the ship:

- the issuer is trusted for `peer` and entitled to attest that organization,
- the credential's `vct`, signature and validity window hold,
- its **status list** says it is not revoked,
- the credential authorizes exactly the party that signed, and is held by
  exactly the signatory that party's node records.

The one part that cannot travel is the key-binding JWT. It proves a holder
answered a *specific* request, with a nonce and an audience issued by the
verifier that asked; the receiving instance never asked, so it re-derives
nothing from that segment. The holder binding it does check is the credential's
own (`sub` against `cnf.jwk`) plus the requirement that this holder is the
recorded signatory.

**A presentation is therefore a bearer credential here**, and two rules keep it
from being one that can be pointed anywhere:

- A peer may only ship evidence for **its own** party. Without that, a
  credential a holder presented in some unrelated exchange verifies on its own
  merits and vouches for a party the shipper has nothing to do with. Only a
  `did:web` organization can be held against the peer's identity; an authored
  contract may name its parties anything, and there the issuer's entitlement to
  attest that organization is the only bound.
A peer is deliberately **not** required to ship evidence for every party the
contract records as authorized. That rule was tried and removed: a contract
signed on both instances records two such parties, each authorized by a
different peer, and neither peer holds the other's presentation — the receive
path verifies inbound evidence without retaining it. Requiring all of it made
the return leg of every two-instance signing unshippable, while a peer that
wants nothing checked still just sends an empty list. It could only ever refuse
an honest peer.

What still cannot be established is freshness or contract-binding: the same
presentation replayed on another contract between the same two parties verifies
identically, until it expires or is revoked.

**What the payload is and is not.** The party, its signatory and its claimed
authorization are read from the contract the same ship carried rather than from
fields beside the credential. The content gate establishes that the PDF's visible
text is the deterministic re-render of its own embedded payload —
self-consistency, not authenticity — so the payload remains the peer's
assertion, and an attacker's residual task is to hold a credential this instance
trusts for that organization and write the contract around it.

**That is accepted.** A contract does not arrive as a fait accompli: negotiation
puts the artifact in front of the counterparty again, and a party that never
agreed to the terms a peer asserts sees them before it signs. The peer's
assertion is a proposal the other side reads, not a record it is bound by. This
verification exists to stop an authority being *invented*, not to remove the
counterparty's own reading of what it is signing.

**A peer may only vouch for itself.** Evidence naming a `did:web` organization
other than the shipping peer is refused: B ships the credential behind B's
signature, and a credential for A reaching us from B is one obtained in some
other exchange. The presentation carries no audience or nonce we could check, so
without that rule it would verify on its own merits and vouch for a party the
shipper has nothing to do with.

**What that establishes, precisely: an attestation of authority.** An issuer
this instance trusts, entitled to speak for that organization, says the holder
may act for it, and has not revoked it. It does **not** establish who the human
is — no identity proofing crosses the boundary, and the PID that identified the
signatory was verified on the instance where the ceremony ran, against that
instance's trust anchors. A counterparty's DCS is still trusted to have run its
own ceremony honestly; what it can no longer do is assert an authority nobody
issued.

**Failure is asymmetric, deliberately.** Evidence that is present and does not
verify refuses the exchange and raises a trust-gate denial incident, like any
other denial (ADR-19). Evidence that is **absent** does not: a peer that retains
none must still federate, and a party that signed without a Power of Attorney is
what the Signature Compliance Viewer has always reported from the contract
itself. Turning absence into a hard failure would convert a finding into an
outage.

Note what that asymmetry costs: omitting the field is a free opt-out, available
to exactly the adversary the gate exists for. The gate therefore raises the
floor — a peer that ships evidence cannot ship *false* evidence — rather than
compelling anyone to prove authority.

**The signatory is recorded on the party that signed.** It used to be stamped on
the node the open party placeholder resolved to, which is the accepting
counterparty. Those coincide only when the counterparty signs first; when the
originator does — which every two-instance flow drives — the originator's
signatory and Power of Attorney were recorded against the *other* party. Nothing
read those fields closely enough to notice until a peer began verifying them.

**Retention — a known unsolved problem.** The presentation is stored on the
signing ceremony (`poa_vp_token`), alongside the PID presentation already held
there, and is **not** covered by contract erasure: `AuditedShredder` destroys
wrapped CEKs, and nothing clears the ceremony's token columns. That matches how
`vp_token` and `pid_claims` are already treated, so it is not new — but this
commit widens what survives an erasure to a holder-bound credential carrying a
disclosed `organization`, `roles`, and a `sub` naming the signatory's key.

This is recorded as **unsolved**, not as accepted. Erasure that leaves personal
data behind is a defect under DCS-NFR-COMP-03 / DCS-NFR-SEC-13 whichever table
it sits in, and the fix is a general one — ceremony evidence has to be erasable
the way contract artifacts are — rather than something to bolt onto this
column.

**What `peer` gates, and why it is now overloaded.** The purpose is used in two
places: a signing ceremony verifying the Power of Attorney presented at it, and
the receiving instance verifying a counterparty's. Those are different
questions — "whose attestation may authorize a signature **here**" and "whose
attestation of a **peer's** authority do we believe" — and federation now
requires the second: instance A must grant B's issuer `peer` for B's signatures
to be accepted at all.

Granting it also lets a holder of a B-issued credential satisfy a signing
ceremony on A, which is the hazard the earlier revision of this ADR warned
about. One purpose cannot honestly serve both, and the split is an **open
point**: `peer` should mean the counterparty attestation only, with the local
ceremony moved to a purpose of its own. Until that lands, an operator granting
`peer` to a counterparty's issuer is accepting that second meaning too.

**Peers are not enumerated, and `peer_dynamic` ships off.** `peer_dynamic` lets
a did:web-resolvable issuer be verified without an entry, with its key taken from
its own DID document, bounded by the identifier: an issuer at `did:web:X:issuer`
may attest `did:web:X` and nothing else. It was added because listing every
counterparty issuer is an allowlist wearing federation's clothes — whether this
instance deals with a peer at all is already decided fail-closed by the ADR-19
trust gate (the peer's self-signed agreement credential must verify against its
own `did.json` and carry this instance's federation rules hash) and by the local
policy endpoint (`DCS_TRUST_PDP_URL`).

That membership argument now genuinely applies to one half of `peer` — the
counterparty attestation the path above verifies. It does not apply to the
other half, and while a single purpose carries both, enabling the flag would
let a party authorize a signature **on this instance** off a document it
publishes about itself, which is self-attestation with extra steps. So the
default stays `false`, and what unblocks it is the purpose split, not this
verification path on its own.

The flag also turns a credential's `iss` into a server-side fetch. Resolution
therefore refuses redirects and will not dial loopback, link-local or multicast
addresses — the cloud metadata endpoint is not a DID registry —, and
`OID4VP_RESOLVER_ALLOWED_HOSTS` narrows it to a named set where a deployment
wants that. Private ranges stay reachable because an in-cluster peer or ORCE
resolver genuinely lives there.

`login` is likewise deliberately **not** dynamic. Who may obtain a session on
this deployment is local policy, and an operator states it explicitly.

### Organization binding

An issuer may only attest organizations listed in its own entry. A credential
whose `organization` is absent from that list is refused regardless of purpose,
so a trusted issuer cannot speak for a party nobody granted it.

**An instance hosts many organizations.** The organization claim is a party
identifier, not the deployment's identity — a single instance legitimately
serves Acme and TechVendor at once. So the rule is about which issuer may name
which party; it is *not* "the organization must be this instance". That
formulation only looks right when a deployment happens to host one party, and it
breaks every multi-tenant one.

Where the issuer *is* the tenant registry for its deployment, enumerating its
organizations in trust configuration would mean editing that file on every
onboarding. Such an issuer declares the explicit wildcard `"*"`. It must be
written out: treating an absent list as "any" is how an issuer silently gains
the right to speak for a party nobody granted it.

`pid` issuers are exempt from the requirement entirely: a PID attests a natural
person, not an organization.

### Revocation

**Every accepted credential is checked against its status list, on every path
and for every purpose.** Login, counterparty PoA, PID: no exception, and no
mechanism opts out — an x5c chain that validates says the issuer signed it, not
that the issuer still stands behind it.

A credential whose status list cannot be reached is refused, not admitted with a
warning. An unreachable revocation list is an unknown revocation state, and a
verifier that treats unknown as valid has no revocation.

### Mechanism

Each issuer declares how its verification key is resolved:

| mechanism | resolution |
|---|---|
| `jwks` | keys bundled in the entry, matched by `kid` (or single-key) |
| `x5c` | certificate chain in the credential header, verified to configured roots |
| `did:jwk` | key decoded from the issuer identifier itself |
| `did:web` | key fetched from the issuer's DID document over HTTPS |
| `orce` | delegated to a configured ORCE flow endpoint |

A mechanism that is declared but not compiled in is **refused at load**, not at
first use: a deployment learns its trust configuration is unsupported when it
starts, not when a wallet arrives. `did:ebsi` and any future scheme reach the
verifier through `orce` without a code change — the flow returns the key, and
ORCE is where a deployment's registry-specific resolution belongs.

### PID issuance

A PID issuer is a **third party** to the relying party. The DCS must not issue
the identity credential it later accepts as proof of who signed; that is the
relying party attesting to itself, and no signature over it means anything
about the signatory.

For the demo this is approximated honestly rather than faked: a PID issuer that
is a separate release with its **own** key and its **own** DID
(`did:web:<host>:pid-issuer`), serving both instances the way a national or QTSP
issuer would, and trusted by each for `pid` and nothing else. It issues
`urn:dcs:pid:demo:v1` describing a person — given name, family name, date of birth
— and carries neither roles nor an organization, because authority to act for a
party is what a PoA is for and an identity document must not grant permissions.

It presents its **certificate chain** (`x5c`), issued by the CA its own PKI flow
provisions, exactly as a real EUDI PID does — and each DCS trusts it with
`mechanism: x5c`, verifying that chain against anchors configured at
`OID4VP_X5C_TRUST_ANCHORS_PATH`. The dev CA is therefore load-bearing rather
than decorative: it had been provisioned and published all along while nothing
verified against it, because the credential asserted identity through `did:web`
instead.

A valid chain to a configured anchor is necessary but not sufficient. The leaf
must also identify the issuer it speaks for — by a SAN URI equal to the issuer
identifier, a SAN DNS name equal to its authority, or an exact CN — and must not
be a certificate issued for another purpose: a leaf asserting `serverAuth` or
`clientAuth` is refused, because an ordinary TLS certificate for the issuer's own
host would otherwise satisfy the DNS branch and sign credentials under the same
anchor.

That makes this a stepping stone rather than a mock to be thrown away: a genuine
national or QTSP issuer differs by an anchor and a trust entry, not by code, and
the chain-validation path a real PID depends on is exercised by the suite.

It is *not* yet wired in the two demo deployments: they neither mount the root CA
at `OID4VP_X5C_TRUST_ANCHORS_PATH` nor set `OID4VP_PID_DCQL_QUERY`, so the PID
path there is configured but unexercised until both land.

What it is not: a real identity proofing process. Nobody checks that the person
is who the form says. The demo shows the *shape* — a third party attests, the
relying party verifies — and the substance arrives with a real issuer.

## Consequences

- A compromised counterparty issuer can no longer mint a session on this
  instance, nor speak for an organization it was not entitled to.
- The trust file gains structure, and every existing file must be migrated:
  a bare `{jwks}` entry no longer loads. This is deliberate — silently
  defaulting an unscoped entry to "all purposes" would reintroduce exactly the
  conflation this ADR removes.
- Adding a resolution mechanism is a configuration change when it can be
  expressed as an ORCE flow, and a small resolver otherwise. Neither requires
  touching the verification path.
- The demo can state a defensible trust story: each instance grants access only
  on its own issuer's word, and treats identity as something a third party
  attests.
- Mutual Power-of-Attorney binding across instances is implemented, and it
  changes what an operator must configure: a counterparty's signatures are only
  accepted where that counterparty's issuer is trusted for `peer` and entitled
  to its organization. A federation member added without that entry has its
  signed ships refused, with an incident naming why.
- `peer` carries two meanings until it is split, and granting it to a
  counterparty's issuer grants both. That is the one thing this ADR still asks
  an operator to accept knowingly.
- Retained evidence is a holder-bound token at rest. It lives on the signing
  ceremony, next to the PID presentation already stored there, so it inherits
  that record's lifetime — including the fact that a contract erasure shreds
  content-encryption keys and does not reach ceremony rows.
- A credential is verified as received, not as of the moment it was used. A
  Power of Attorney that expires or is revoked after the signature makes later
  ships of that same contract fail closed. That is the correct reading of
  revocation, and it means a PoA's lifetime has to outlive the exchange.
- QES remains blocked on identity proofing. There is now a third-party PID
  issuer and the chain-validation path it exercises is the real one, but nobody
  verifies that the person is who the form says. The architecture no longer
  hides that.
