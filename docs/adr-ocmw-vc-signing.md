# ADR: Why OCM-W NATS services are not used for VC signing in DCS

## Context

DCS embeds a W3C Verifiable Credential (a `ContractLifecycleCredential`) inside each C2PA manifest it generates. The VC must carry a Data Integrity LD proof (`ecdsa-rdfc-2019`) so that validators can verify the binding between the contract artefact and the lifecycle event.

When designing this, we evaluated whether to route the signing call through the Eclipse XFSC Organisational Credential Manager W-Stack (OCM-W), which DCS may co-deploy and communicate over NATS.

We researched the OCM-W NATS API via the [`nats-message-library`](https://github.com/eclipse-xfsc/nats-message-library) — the canonical message-contract library used by all OCM-W services.

## OCM-W services evaluated

| Service | NATS subject | What it actually does |
|---|---|---|
| Credential issuance service | `issuance.request` | Starts an **OID4VCI** issuance flow — returns a `CredentialOffer` (a URL/token) that a *wallet holder* redeems asynchronously to pull their credential. |
| Signer service | `signer.signToken` | Signs a **JWT** (header + payload bytes) and returns a compact JWT token. |
| Storage service | `storage.service.credential` | Stores an already-signed credential in the wallet. |
| Well-known / registration | `wellknown.issuer.registration` | Registers issuer metadata for OID4VCI discovery. |

## Why none of these fit

**Issuance (`issuance.request`)** is designed for the OID4VCI wallet-pull protocol: a human or automated agent initiates an offer, and a remote wallet later redeems it over HTTP. The flow is asynchronous, multi-step, and requires a holder DID on the other end. DCS needs synchronous in-process signing to embed the proof *before* the PDF is returned to the caller — there is no wallet, no holder, and no round-trip tolerable.

**Signer (`signer.signToken`)** produces a compact JWT, not an LD proof. A Data Integrity LD proof requires URDNA2015 canonicalization of the document and proof options, SHA-256 hashing of both, concatenation, signing via the credential key, and multibase encoding of the result. That entire construction is absent from the signer service's API; it only accepts raw header/payload bytes and returns a token.

In short, the OCM-W stack is built around the OID4VCI credential-issuance protocol for issuing credentials *to* wallet holders. DCS's requirement — synchronously sign an in-memory JSON-LD document and embed the result in a binary container — is orthogonal to that protocol.

## Decision

DCS signs VCs via the **HSM directly** (PKCS#11, `backend/internal/base/hsm`) and constructs an `ecdsa-rdfc-2019` Data Integrity proof locally (`provenance.NewHSMVCSigner`):

1. URDNA2015-canonicalize the proof options and the VC document (`piprate/json-gold`)
2. SHA-256 hash each
3. Send the concatenation to the PKCS#11-held ECDSA P-256 key for signing
4. Set `proof.proofValue` from the resulting signature

VC signing uses the same PKCS#11 key-custody layer as every other
signature kind in the system: **one key-custody mechanism, not two**
(see ADR-1).

## Consequences

- No OCM-W dependency for VC signing — VCs, C2PA claim signatures, and
  PAdES signatures all resolve to the same PKCS#11 key-custody layer
  (ADR-1).
- If DCS later needs to *issue credentials to wallet holders* via OID4VCI
  (e.g. issuing a signed contract summary to a participant's wallet), the
  OCM-W issuance service would be the right integration point for that
  separate feature.
