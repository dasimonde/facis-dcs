# ADR-24: v1 signature scope — wallet-based AES, level determined by the provider

Status: Accepted (2026-07-26), reflecting the client's decision memo on the
registered deviation points.

## Decision

The v1 deliverable produces **wallet-based AES signatures** (advanced
electronic signatures per eIDAS Art. 26). The signing architecture is not
AES-limited: it speaks the CSC credential-authorization protocol, validates
x5c certificate chains, and pins the EU trust list (ADR-20), so the level a
signature reaches is determined by the credential the signer's wallet
presents and the trust service behind it — not by this codebase. With v1's
test-grade providers (project-own CA, test wallet, demonstration TSA, an EU
DSS test instance as the signature-creation application) the result is AES.
Connecting a qualified trust service provider and a production wallet
upgrades the same flow toward QES without architectural change.

## Consequences and documented deviations

1. **QES is excluded from v1** (client-confirmed). Consequence, stated per
   the client's requirement: **contracts subject to a statutory
   written-form requirement (gesetzliches Schriftformerfordernis, § 126/126a
   BGB) cannot be demonstrated in v1**, because they require a qualified
   electronic signature. The limitation is one of provider integration, not
   of the signing path.
2. **Signature formats: PAdES focus** (client-accepted). JAdES ships in two
   places: the ceremony offers the JAdES payload as a second wallet-signed
   document, and the federation layer signs each cross-instance signature
   ship with an instance-key JAdES that the receiving peer verifies and
   persists as provenance (`features/17_peer_trust`, DCS-FR-SM-02). CAdES
   is not implemented — a recorded deviation.
3. **No WebAuthn step-up** (client-accepted as a documented QA exception).
   Login is exclusively wallet-based (OpenID4VP with PoA credentials), as
   the SRS user-management chapter mandates; credential status and
   revocation are checked at every login (`oid4vp.Verify`, status-list step)
   and again before signing (`VerifyPID`), evidenced by the
   revoked-status-index rejection coverage in
   `features/22_real_signing_vertical`. The login architecture keeps later
   additional mechanisms possible — role evaluation and verification are
   policy steps, not assumptions baked into the transport.
4. **PoA chain length 1** is the confirmed scope of this deliverable.
5. **The term "rQES" is retired** from all documents: the model is named
   for what v1 delivers — a wallet-based AES signature. References to the
   CSC protocol remain, since that protocol is precisely the
   qualified-provider upgrade path.
6. **DCS-FR-PACM-03 (autonomous system-side legal-conformity assessment) is
   descoped** (client decision). The technical conformance gates (SHACL hub
   conformance, policy audits), the alerting mechanism, and Compliance
   Officer / Auditor access to compliance information including check
   initiation remain in the deliverable — they assess technical and
   contractual conformance, and make no autonomous legal judgment.
