# ADR-20: Signing acceptance-path hardening — nonce binding, byte pinning, level-aware gates, and dropping EUDIPLO

## Status

Accepted; revised 2026-07-29 for the completed one-hop PoA trust and
cross-instance revalidation baseline. Originally accepted 2026-07-25.
**Supersedes acceptance-path clauses of
[ADR-12](adr-12-wallet-driven-signing.md)**: the "possible refinement" framing
around to-be-signed byte pinning is retracted — it is now a hard requirement,
implemented — and ADR-12's QES descope (citing SRS §199) is retracted. Builds
on [ADR-12](adr-12-wallet-driven-signing.md) (the DCS is the OID4VP relying
party and validator, holds no contract-signing key) and
[ADR-17](adr-17-machine-signing-is-not-an-aes-signature.md) (no System User
may produce an AES/QES signature — unaffected here; this ADR is entirely about
the acceptance path for a signature a *natural person's* wallet produced).

## Context

ADR-12 built the wallet-driven acceptance path — prepare, publish, the
Document-Retrieval ceremony, submit — but a security review of that path
found it incomplete in ways that mattered:

1. **The ceremony callback was an unauthenticated bearer-token hole.** The
   request-object, document, and callback endpoints are `NoSecurity()` by
   design (the caller is the signatory's wallet, not a logged-in user) — but
   the signed-document branch checked only `state == ceremonyID`, already
   public in the URL, and never the `RequestNonce` generated at publish and
   embedded in the Document-Retrieval JAR. Anyone holding the ceremony URL
   within its 15-minute TTL could fetch the to-be-signed document, sign it
   with any key, POST it back, and reach SIGNED attributed to the real
   person's verified PID.
2. **Byte pinning didn't cover the document, only an attachment.**
   `SubmitSignature` re-ran `prepare()` — which is not byte-deterministic
   (evidence embedding, VC issuance, timestamps) and re-executes side effects
   (agreement sealing, summary-VC issuance) in a committing transaction — and
   compared only the embedded JSON-LD attachment against a freshly re-derived
   copy. A submitted PDF with tampered visible pages but an intact attachment
   was accepted, and `finalize` recorded a `contentHash` for content the
   stored artefact didn't display.
3. **Consumption wasn't atomic with finalize.** `ConsumedAt` was checked at
   the top of the callback, but `MarkCeremonyConsumed` ran in a separate,
   later transaction. Two concurrent callbacks could both pass the early
   check and both finalize.
4. **The sole-control gate didn't check who signed.** `AssertValidAES`
   accepted any signature that wasn't TOTAL-FAILED and carried *some*
   certificate — a shared org key or a self-signed throwaway passed. ADR-12
   claimed the gate ensured the cert "identifies the ceremony's signatory";
   it didn't.
5. **One gate for every level.** `submitSignature` applied the same
   permissive AES gate regardless of the ceremony's declared level, so a
   contract requiring QES would accept a self-signed AES.
6. **The JAdES was recorded unvalidated.** The callback took
   `signatureObject[0]` and handed it to `finalize` without validating it —
   an arbitrary string could be archived as the signature over the
   machine-readable JSON-LD.
7. Separately: **EUDIPLO**, the remote reference PID-presentation service the
   ceremony's PID-verification webhook was built to receive callbacks from,
   **is broken** and is being dropped as a dependency entirely — its removal
   is folded into this ADR because it touches the same ceremony-verification
   code path items 1–6 harden.

QES is also brought into scope. SRS FR-SM-01 requires SES/AES/QES to be
selectable **per contract**; ADR-12's descope citing "SRS §199" was a
narrative-section overread, not a formal requirement — it is retracted here.

## Decision

### 1. Nonce binding

The signed-document response's authenticity is bound to the ceremony's fresh
`RequestNonce`, carried as a custom `nonce` claim inside the **JAdES**
signature's protected header — content the JAdES's own ES256 signature
already covers, so it cannot be stripped or substituted without invalidating
that signature. The callback extracts the claim only **after** DSS has
validated the JAdES, then compares it to the ceremony's pinned
`RequestNonce`. A wallet-ceremony callback (one with a published `RequestNonce`)
now **requires** a JAdES for exactly this reason — nonce binding rides it,
and the JAdES is validated (item 6) regardless. The test wallet
(`testWallet/dcs_wallet/jades_signer.py`) mirrors this: it signs the
ceremony's payload with the SAME certificate as the PAdES, with `nonce` in the
header.

To give the wallet something to fetch and sign as the JAdES, the
Document-Retrieval request object now offers **two** documents
(`document_digests`/`document_locations`, ADR-12's own description of the
intended shape): the PDF and, at a new `GET
/signature/request/{ceremony_id}/payload` endpoint, the pinned canonical
JSON-LD payload.

The authenticated `/signature/submit` path (JWT, `Contract Signer` scope, no
ceremony ever published) has no request nonce to bind — a JAdES there, if
present, is still validated but not nonce-checked; the JWT is the caller's
authentication.

### 2. TBS byte pinning is a hard requirement, not a refinement

ADR-12 left "persisting the exact to-be-signed bytes" as "a possible
refinement, not required for the acceptance path to be sound." That
assessment is retracted: it *was* required, and the gap it left (item 2
above) was real. The fix:

- **Every** `Prepare()` call — the wallet-ceremony path and the desktop
  `/signature/prepare` path alike — pins the exact to-be-signed PDF **and**
  the canonical JAdES payload on the ceremony row, plus the finalize metadata
  computed alongside them (`contentHash`, `rendererVersion`, `signedCount`,
  `contractVersion`, the contract's required credential level).
- `SubmitSignature` is now a **pure validate-and-record step**: it never
  calls `prepare()` again — no re-sealing the agreement, no re-issuing the
  summary VC, no re-stamping the C2PA lifecycle, no re-deriving any hash.
  Everything it needs was computed exactly once, at prepare, and pinned.
- The submitted PDF is checked with `bytes.HasPrefix(signedPDF, pinnedPDF)`.
  PAdES signing is itself an incremental update — appending bytes after the
  original file's `%%EOF` while leaving every earlier byte untouched — so "the
  same document, plus our own signature" is exactly a byte-prefix
  relationship. Tampering anywhere in the visible content fails this
  regardless of whether the signature itself validates, closing exactly the
  hole in item 2. The old attachment-only comparison
  (`assertSubmittedPayloadIsOurs`, `PDFCore.ExtractPayload` round-trip) is
  deleted — the prefix check subsumes it and is cheaper.
- The JAdES payload is checked the same way: the JWS payload segment,
  decoded, must byte-equal the pinned payload.

### 3. Atomic consumption

`MarkCeremonyConsumed`'s `UPDATE ... WHERE consumed_at IS NULL` guard now
runs **inside the same transaction** as `finalize`'s writes, in
`SubmitSignature`, and commits or rolls back with them. Two concurrent
submits for one ceremony can no longer both finalize: Postgres serializes the
guarded UPDATE under READ COMMITTED, so the second transaction's guard
observes the first's committed `consumed_at` and affects zero rows, failing
the whole transaction. The service layer's former separate post-finalize
`MarkCeremonyConsumed` call is removed (it would now always fail — consumed
by the time it ran).

### 4. Cert-subject to PID name matching (sole control)

The signing certificate's subject must name the ceremony's verified PID:
`GIVEN_NAME`/`SURNAME` compared, case/whitespace normalized, against the
PID's `given_name`/`family_name`. **Mandatory for QES** (eIDAS Annex I
requires a qualified certificate to carry the signatory's verified name) and
**policy-configurable for AES** (`DCS_AES_CERT_NAME_MATCH_REQUIRED`, default
**enabled** — the whole point of this gate is closing the shared/self-signed-
key hole in item 4, so leaving it off by default would reopen exactly that).
A repeat signatory on the same contract must also use a **consistent**
certificate across their own signatures — a mid-contract certificate swap for
one signer is the signal a compromised or shared key would produce.

The certificate is read directly from the **submitted PDF's own CMS
SignerInfo** (`signerCertificateFromIncrementalUpdate`,
`github.com/digitorus/pkcs7`), not from DSS's validation report. That started
as the design (prefer DSS's structured `GivenName`/`Surname`, fall back to
parsing the `SignedBy` DN string) but DSS's `simpleReport` never carries
structured name attributes at all — only a CommonName-derived `SignedBy` — and
its `diagnosticData` per-certificate entries came back empty in CI for a
non-qualified dev-CA certificate (DSS appears to populate those name fields
only for certificates it recognizes as qualified). Byte pinning (item 2)
already proves the submitted PDF is exactly `ceremony.PreparedPDF` plus one
incremental update, so the bytes after that prefix can only be the signature
just submitted — reading the certificate from there has no dependency on what
DSS chooses to report and cannot be ambiguous about which signature it names.
`dss.Report.SubjectGivenName`/`SubjectSurname`/`ParseSubjectAttributes` remain
in `dss/client.go` (DSS's `Indication`/`SubIndication`/`Qualification`/
`SignedBy` are still sourced from its report and still matter — see item 5)
but are no longer what the sole-control gate itself calls. The validated
certificate's subject and serial are recorded on the ceremony
(`signer_cert_subject`, `signer_cert_serial`) for the Signature Compliance
Viewer (SM-26) and the cross-signature consistency check above.

### 5. Level-aware acceptance

- **DSS report attribution is scoped to the submission's own signature, not
  the document's first one.** A multi-signer contract's second-and-later
  signatories always submit a document that already carries an earlier
  signatory's signature — PAdES incremental updates only append — so DSS's
  `simpleReport.signatureOrTimestampOrEvidenceRecord` carries one entry per
  signature, oldest first. Walking the whole JSON response for the first
  `Indication`/`SubIndication`/`SignedBy`/qualification match (the original
  implementation) silently attributed every submission to the *oldest*
  signature on the document, not the one just submitted.
  `dss.latestSignatureEntry` scopes extraction to the last Signature-typed
  entry in that list before either the fast-path fields or the item 4
  certificate resolution run, with a whole-document-walk fallback for DSS
  response shapes that don't nest that way.
- `dss.Report.AssertValidAES()` is unchanged: cryptographically sound,
  carries a certificate, tolerates `INDETERMINATE`/`NO_CERTIFICATE_CHAIN_FOUND`
  (a non-qualified CA is a trust-list gap, not a crypto failure — AES needs
  only integrity and unique linkage to the signatory, Art. 26 a/b/d, never a
  qualified trust chain).
- `dss.Report.AssertValidQES()` requires everything `AssertValidAES` does,
  **plus** `Indication == TOTAL-PASSED` (a trust-chain gap that AES tolerates
  disqualifies a QES claim outright — trust-list membership *is* the QES
  property AES doesn't need) **and** DSS's `SignatureQualification ==
  "QESIG"` (qualified certificate + QSCD, ETSI TS 119 172-4).
- The contract's own declared requirement per field
  (`dcs:requiredCredentialType` on the `dcs:SignatureField` node, default
  `AES`) is pinned at prepare and is what submit gates on — **not** the
  caller-supplied `credential_type` request parameter, which is only a hint
  to the JAR's `signatureQualifier`. A ceremony requesting AES for a
  QES-required field fails fast at prepare; a signature that achieves less
  than the pinned requirement fails at submit regardless of what was
  requested. The **achieved** level (QES if `AssertValidQES` passes,
  otherwise AES), not the requested one, is what gets recorded.
- **QES scope**: SM-01 is a MUST for SES/AES/QES, selectable per contract.
  ADR-12's descope citing "SRS §199" is retracted — that citation was a
  narrative-section overread of the SRS, not a formal requirement; the
  per-contract level enforcement above is the SM-01 mechanism.
- **Remaining implementation gap, explicitly**: the QES gate itself
  (`AssertValidQES`) is implemented and enforced unconditionally — there is
  no code path that weakens it. What is **not yet done** is provisioning a
  mock EU trusted list into the CI DSS instance so a QES BDD scenario has a
  realistic happy path (the dev CA cannot honestly be "qualified" against the
  real EU LOTL DSS validates against by default). Until it lands, QES BDD
  coverage is correctness-of-rejection only (AES-vs-QES-required is
  covered); the QES-succeeds happy path is the explicit remaining gap.

  Researched but not yet implemented: the DSS demo webapp
  (`conectx/dss-demo`, wrapping `esig/dss-demonstrations`'
  `dss-demo-webapp`) supports an *additional*, LOTL-independent trust
  anchor via `trusted.source.keystore.{type,filename,password}`
  (`dss.properties`) — `DSSBeanConfig.trustedCertificateSource()` loads a
  `KeyStoreCertificateSource` from that keystore and adds it to the
  certificate verifier **alongside** the LOTL source, which is enough to
  turn `NO_CERTIFICATE_CHAIN_FOUND` into `TOTAL-PASSED` for the dev CA. It is
  **not** enough on its own for `AssertValidQES`: DSS's `QESIG`
  qualification determination is derived from **trusted-list SERVICE
  metadata** (ETSI TS 119 172-4 service-type identifiers and qualifiers,
  e.g. QC + QSCD), which a bare trusted keystore does not carry — only a
  proper trusted-list entry does. So the real remaining work is standing up
  a minimal, correctly-structured mock trusted list (not just a trusted
  keystore) naming the dev CA as a QC/QSCD-qualified TSP, mounted into the
  DSS container and pointed to via `current.lotl.url` (or the parallel
  `tl.loader.ades.*` AdES-LOTL properties, which look like the
  purpose-built override point rather than replacing the primary LOTL). This
  is a CI-provisioning work item against `deployment/helm/charts/dss`; it
  needs a live DSS instance to iterate against, which a local dev
  environment doesn't reliably provide either.

### 6. JAdES validation

The submitted JAdES (mandatory on a published ceremony, item 1) is validated
via DSS (the same `SignatureValidator.ValidatePDF` — DSS's
`validateSignature` endpoint format-detects; a JAdES is just bytes with a
different structure) and must pass `AssertValidAES()` — cryptographic
soundness and a real certificate — regardless of the contract's required
level (the JAdES's role is nonce binding and payload integrity, not the
primary level assertion; the PAdES carries that). An invalid JAdES is
rejected, never silently recorded.

### 7. Removal of the transitional DCS-as-signatory path (completed)

`POST /signature/apply`, the pdf-core `/sign`/`/internal/pades/sign`
contract-signing endpoint, and the `PDFCoreSigner` were already gone from the
codebase by the time this ADR landed — a prior, undocumented migration step.
This ADR completes what remained: the `dcs-contract-pades` HSM key and its
provisioning (`scripts/hsm-provision.sh`, the Helm HSM-provisioning job, the
`pades-x5chain-pem` secret field), and the `KeyVersion` metadata `finalize`
recorded against it (`ContractSignature.KeyVersion`, `ActiveKeyVersion`) —
meaningless and misleading once the DCS no longer produces the signature
itself.

### 8. PID provisioning: EUDIPLO is gone

The remote EUDIPLO PID service is broken and is removed as a dependency,
including the `ceremonyWebhook` design method, its shared-secret
authentication (`NFR-SEC-18`), and every EUDIPLO reference in code, BDD
steps, and non-historical docs.

- Ceremony PID(+PoA) verification is **wallet-presented OID4VP only**: the
  wallet direct_posts a `vp_token` — keyed by the PID and PoA DCQL credential
  query ids — to the ceremony's own callback, cryptographically verified
  (`oid4vp.Verifier.VerifyPID`/`Verify`) against the ceremony's own nonce and
  the configured trust anchors before anything is persisted.
  `VerifyPID`'s status-list check, previously skipped because EUDIPLO PIDs
  carried no status claim, is **re-enabled** — a self-issued dev PID now
  carries a real one, so SM-18 status checks run for real, not vacuously.
- In dev/test, the PID/PoA issuer is **self-issuance** tooling
  (`testWallet/scripts/issue_pid_credentials.py`, mirroring the local signing
  primitives already used for the Power of Attorney credential in
  `dcs_wallet/issuer.py`). Its x5c chain terminates at the project Dev Root.
  **This is a dev/demo substitution and must never become an implicit
  production trust anchor.** The production chart has OID4VP trust disabled,
  an empty trust-data path and no x5c anchor; only the BDD/BDD2 overlays opt in
  to the development trust files (`deployment/helm/values.yaml:62-85`,
  `deployment/helm/values.bdd.yml:73-80`,
  `deployment/helm/values.bdd2.yml:59-64`). A production deployment supplies
  its issuer registry and trust anchors as operator configuration; this ADR
  does not define that profile.
- Self-issued PIDs' `given_name`/`family_name` are threaded into the test
  wallet's signing certificates as `GIVEN_NAME`/`SURNAME` (item 4's
  groundwork — see `dcs_wallet/signer.py`'s `ensure_signing_material`), so
  the cert↔PID name-match gate has real, aligned data on the happy path, and
  a deliberate mismatch (explicit override) is the negative test.

### 9. One-hop PoA evidence and peer revalidation

The v1 PoA profile is `urn:dcs:poa:v1` with the delegation depth fixed at one
by ADR-24. An x5c-bearing PoA therefore contains exactly an issuer leaf and a
self-signed CA root, and that chain must terminate at a separately configured
trust anchor. Empty trust configuration and untrusted roots fail closed
(`backend/internal/auth/oid4vp/sdjwt/keys.go:101-167`,
`backend/internal/auth/oid4vp/sdjwt/payload.go:138-170`). Holder binding,
the original nonce/audience and live credential status are checked before the
ceremony is accepted (`backend/internal/auth/oid4vp/verify.go:231-274`,
`backend/internal/auth/oid4vp/statuslist_verify.go:46-88`).

The signing summary binds the already verified PoA presentation together with
its original nonce and audience, without embedding PID disclosures
(`backend/internal/pdfgeneration/provenance/signing_summary.go:12-32`,
`backend/internal/pdfgeneration/provenance/signing_summary.go:80-96`,
`backend/internal/signingmanagement/command/apply.go:738-817`). A receiving DCS
does not trust that assertion merely because the peer transported it. Before
adopting the contract content-encryption key or persisting synchronization
provenance, it revalidates the presentation signature, status, holder,
nonce/audience, issuer-authoritative organization and signing party. Rejection
creates a traceable PAC trust-gate finding
(`backend/internal/service/dcs_to_dcs.go:160-184`,
`backend/internal/service/dcs_to_dcs.go:192-240`,
`backend/internal/service/dcs_to_dcs.go:245-289`).

### Long-term validation for archived QES signatures (decided, not deferred)

A QES-level signature validated and accepted today stops being independently
verifiable once its signing certificate expires or its issuing CA's
revocation data ages out — PAdES-B-B/B-T carry no long-term proof. **Decision
(recorded here, not left as an implicit gap):** a contract whose required
level is QES gets its archived PDF **augmented to PAdES-B-LT/B-LTA** —
qualified timestamp plus embedded revocation data (CRL/OCSP) — **at
archival**, i.e. at the same point `archiveSignedContract` already runs
(first-signature SIGNED transition, `DCS-FR-CWE-20`). AES-level contracts are
unaffected; PAdES-B-B/B-T remains sufficient for AES's assurance target.

This is **not implemented in this change** — it is a decided position with an
owner-visible acceptance criterion, tracked as its own follow-on work item:
*"Archived QES contracts verify successfully against DSS with no external
timestamp/revocation lookup after their signing certificate's validity
period has passed."* That criterion, not a code comment, is what closes this
item. It belongs in a dedicated ADR when implemented (the augmentation touches
the archive pipeline, not the acceptance path this ADR covers), referencing
this decision as its origin.

## Consequences

- The acceptance path now rejects: a signature bound to the wrong (or no)
  request nonce; a submitted document whose visible content diverges from
  what was prepared, even with an intact attachment; a second concurrent
  submit racing a first; a cryptographically valid AES on a QES-required
  field; a certificate that doesn't name the ceremony's verified PID (subject
  to the AES policy toggle); a certificate inconsistent with the same
  signatory's prior signatures on the contract; an invalid or absent (on a
  published ceremony) JAdES.
- `SubmitSignature` has no side effects and re-derives nothing — every prior
  side effect (agreement sealing, summary VC issuance, lifecycle stamping)
  happens exactly once, at prepare.
- The `dcs-contract-pades` HSM key, its provisioning, and the `KeyVersion`
  signature metadata are gone — nothing in the accepted-signature path
  attributes a signature to a DCS-held key anymore, because nothing does.
- EUDIPLO is not a dependency of this DCS anywhere — build, deploy, docs, or
  test. The self-issuance dev-trust substitution is explicitly and
  permanently marked as dev-only.
- Peer delivery of a signed acceptance now carries the original PoA
  presentation context. Missing, altered, stale-status or wrong-party evidence
  is rejected before peer key material or signature provenance is stored, and
  the rejection remains auditable.
- QES contracts are not yet independently verifiable after certificate
  expiry — tracked, not silent (see above).
- The QES-succeeds BDD happy path needs a mock EU trusted list provisioned
  into CI's DSS instance — tracked, not silent (see Decision §5).

## SRS coverage

| SRS | Covered by |
| --- | --- |
| SM-01 (SES/AES/QES, selectable per contract; QES no longer descoped) | `dcs:requiredCredentialType` pinned at prepare, enforced at submit via `AssertMeetsLevel` |
| SM-02, SM-11 (PAdES + JAdES, same content hash) | Document-Retrieval offers both; JAdES payload pinned and byte-checked against the same content the PAdES covers |
| SM-16 (secure key usage; integrity validation upon signing) | Byte pinning + DSS validation + sole-control gate, all before finalize |
| SM-18 (status/integrity/timestamp validation) | `checkStatusList` re-enabled for PID; DSS validation for the signature itself |
| SM-26 (Signature Compliance Viewer: signer identity, cert, level, qualification) | `signer_cert_subject`/`signer_cert_serial` recorded on the ceremony; achieved level recorded on the signature |
| DCS-FR-SM-03/-04 (PoA presented, status-checked and checked again by a receiving peer) | One-hop `urn:dcs:poa:v1` is holder-, nonce-, audience-, issuer-organization- and party-bound; receiver rejection precedes CEK/provenance persistence |
| DCS-FR-SM-05, DCS-IR-CI-09 (VC/status integration and refresh) | Signed W3C and retained XFSC status formats are normalized; missing, invalid, unknown or unavailable status evidence fails closed |

Not yet covered (see Consequences): archived QES long-term validation
(B-LT/B-LTA); the QES-succeeds CI happy path pending DSS mock-trust-list
provisioning.
