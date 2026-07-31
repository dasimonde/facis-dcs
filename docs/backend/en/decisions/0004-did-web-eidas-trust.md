# ADR-0004: did:web + eIDAS Certificates as Peer Trust Anchor

**Status:** Accepted (derived from existing code)
**Affects:** `backend/internal/base/identity/`, `backend/internal/dcstodcs/`

## Context

DCS instances run by different, mutually independent operators must synchronize shared contracts (see [ADR-0005](0005-single-writer-peer-sync.md)). This requires each instance to unambiguously establish the identity of a requesting peer — a central auth server (like Hydra for end users) doesn't apply here, since there is no shared authority between independent organizations. At the same time, the Gaia-X/eIDAS context carries a regulatory requirement: peer identity should be provable not just cryptographically, but also with a **legal anchor**.

## Decision

Every DCS instance owns its own `did:web` document (RSA key pair), retrievable via the standard `/.well-known/did.json` path. Trust between two peers is established through three independent layers:

1. **eIDAS certificate chain:** the x5c chain in the DID document is validated against an EU trust pool (member-state LOTL/TSL), including hostname match and QcCompliance statement.
2. **Per-request challenge-response signature:** instead of a token, the requesting peer signs a random value (`rand.Text()`) with its private key; the receiver resolves the public key via `did:web` and verifies the signature (proof of possession, no replayable token in circulation).
3. **Local trusted-peer allowlist:** in addition to cryptographic validity, the peer must be explicitly listed in a local `trusted_peers` table — a valid eIDAS chain alone is not sufficient.

## Alternatives Considered

- **Centralized federated auth server (e.g. a shared Hydra/OIDC instance across organizational boundaries):** rejected, since there is no neutral central authority accepted by all operators, and it would introduce a single point of failure/trust.
- **mTLS between instances:** rejected in favor of `did:web`, because DID documents can additionally carry business metadata (verification methods, key rotation) and fit seamlessly into the already-present SSI/Gaia-X stack.
- **Plain API keys between known peers:** rejected as insufficiently rotatable and lacking eIDAS binding for a regulator-grade identity proof.

## Consequences

- **Positive:** Peer identity is anchored both cryptographically and regulatorily (eIDAS) — relevant for legal validity in the Gaia-X context.
- **Positive:** No central trust service between organizations required; each instance decides its own trusted-peer list.
- **Negative:** Runtime dependency on the reachability of the counterpart's `/.well-known/did.json` for every sync operation (no caching of the peer DID document observed) — an outage of a peer's hosting prevents signature verification for its incoming requests.
- **Negative:** The EU trust pool must be kept current (a refresh mechanism exists); stale trust lists could either wrongly reject valid peers or, worse, recognize invalid certificates too late.
