# ADR-1: PKCS#11/HSM as the single key-custody mechanism

> **Superseded in part by [ADR-12](adr-12-wallet-driven-signing.md)
> (2026-07-18).** "PAdES contract signatures" is **withdrawn** from this
> ADR's list of DCS key-custody touchpoints below: the contract AES key
> lives in the signatory's wallet/QTSP, not in DCS's HSM — a DCS-held key
> cannot satisfy eIDAS sole control. The remaining four touchpoints (C2PA
> claim/lifecycle-assertion signatures, the PID-binding signing-summary VC,
> OID4VP JAR signing) are unchanged and still resolve to the one PKCS#11
> interface described below.

## Context

DCS signs several distinct kinds of artifact with **its own** identity: C2PA
claim signatures, C2PA lifecycle-assertion signatures, the PID-binding
signing-summary VC, OID4VP request objects (JAR), and the DCS-to-DCS
synchronizer's JAdES transport envelope. Each needs an asymmetric key and a
signing operation, so each could plausibly grow its own key-storage mechanism.

(The **contract AES** is deliberately absent from this list: per
[ADR-12](adr-12-wallet-driven-signing.md) it is produced by the signatory's
wallet/QTSP, not a DCS key. The DCS holds no contract-signing key.)

The SRS requires a standardized, swappable key-custody interface
(IR-HI-01) and eIDAS-aligned crypto-agility with rotation and revocation
(DCS-OR-C2PA-007). Two (or five) different custody mechanisms cannot be
rotated or revoked as one operation, and a raw PEM file checked into git is
not a credible key-custody story at all.

## Decision

All five signing touchpoints resolve to **one PKCS#11 interface**
(`backend/internal/base/hsm`), backed by SoftHSM2 in dev/CI and swappable
for a real HSM in production by configuration only (module path + token
label). Committed PEM private keys are not used anywhere in the shipped
product.

## Consequences

- One rotation/revocation drill (A5) covers every signature kind in the
  system, not five separate ones.
- Swapping SoftHSM2 for a production HSM is a `PKCS11_MODULE_PATH` /
  `PKCS11_TOKEN_LABEL` change, not a code change.
- VC signing resolves to this same mechanism; see
  [adr-ocmw-vc-signing.md](adr-ocmw-vc-signing.md) for why the OCM-W
  signer services are not used.
- Trust anchoring (which certificate chain a verifier trusts) is a
  separate, swappable concern from key *custody* (where the private key
  lives) — dev uses a custom CA with CRL, production is designed to swap to
  the EU LOTL (`eutrustpool.go`) by configuration.
