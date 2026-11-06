# ADR-40: Archives of QES contracts are PAdES-B-LTA

Status: Accepted (2026-11-02).

## Context

The wallet signs through the DSS `signDocument` service at PAdES-B-T
(`testWallet/dcs_wallet/remote_signer.py`): a CMS signature with a signature
timestamp. That level carries no revocation data and no proof of existence
that outlives the signing certificate. When the certificate expires, DSS
reports `INDETERMINATE` (for example `OUT_OF_BOUNDS_NO_POE`) and
`dss.Report.AssertValidQES` fails. The production and acceptance profiles
keep archived contracts for 10 years (`archive.retentionYears`), longer than
a qualified certificate is valid.

[ADR-20](adr-20-signing-acceptance-hardening.md) decided that the archive of
a QES contract is augmented to PAdES-B-LT/B-LTA. This ADR records how.

## Decision

1. **When.** `submitSignature` (`backend/internal/signingmanagement/command/apply.go`)
   extends the PDF when the submitted signature completes the local
   signatures (`signingCompletes`) and the contract carries a QES: the
   submitted signature validated as QES, or a recorded `SIGNED` signature has
   credential type QES. This is the submission that archives the contract
   (DCS-FR-CSA-08, DCS-FR-CWE-20). An AES contract is not extended.
2. **How.** `archivePDF` sends the submitted PDF to DSS
   `POST /services/rest/signature/one-document/extendDocument` with
   `signatureLevel` `PAdES_BASELINE_LTA` (`dss.Client.ExtendPDF`). DSS appends
   the validation data (`/DSS`) and a document timestamp as an incremental
   update.
3. **Checks.** The extended PDF must start with the submitted bytes and be
   longer. DSS then validates it again (`ValidatePDFSignatures`): every
   signature must report a `SignatureFormat` that ends in `-LTA`, and at
   least as many signatures as the contract records as QES must pass
   `AssertValidQES` (`dss.AssertQualifiedSignatures`).
4. **Order.** The extension runs after the validation of the submitted
   signature and its JAdES, and before the per-contract regeneration lock,
   for the reason ADR-20 gives for the validation round trips. The signer
   certificate and the sole-control checks read the submitted bytes. The
   extended bytes go to `finalize`, which appends the C2PA re-anchor
   ([ADR-26](adr-26-provenance-reanchored-after-signing.md)) after the
   document timestamp, then stores, hashes and archives them.
5. **Failure.** Any failure (DSS or its TSA does not answer, the result does
   not append, a signature is not LTA, the QES count does not validate) is
   `ErrLongTermValidationUnavailable`, answered as `service_unavailable`
   (503, temporary). The transaction rolls back before the ceremony is
   consumed, so nothing is written and the wallet can send the same
   signature again.
6. **Validation and verification.** The compliance viewer
   (`POST /signature/validate`) and the PDF verifier count the QES signatures
   the contract records and require that many signatures to pass
   `AssertValidQES`. A QES that lost its long-term validation shows as a
   finding and as an invalid PDF signature.
7. **Timestamp authority.** `dss.tsp.url` sets the TSA of the co-deployed DSS
   (an `OnlineTSPSource`, `deployment/helm/charts/dss`). A production-like
   release with the co-deployed DSS does not render without it. Empty, the
   DSS image uses its own in-process TSA, which no trusted list names.
   ORCE stands in for the qualified trust service provider in a test
   deployment: its local TSA answers RFC 3161 over `POST /tsa`, and
   `dss.tsp.url` can point there. Those timestamps are not qualified; a
   production release names the TSA of its QTSP instead.
8. **Later lifecycle updates.** pdf-core reads the trailer of the section the
   last `startxref` names (a classic trailer or a cross-reference stream) and
   matches the catalog with any whitespace, so a lifecycle update after the
   DSS revision keeps `/DSS`, numbers its objects past the DSS objects and
   lists its credential in `/EmbeddedFiles`.

ADR-20 named "a contract whose required level is QES". The trigger is the
achieved level: a QES offered for a field that requires AES is also
extended.

## Consequences

- An archived QES contract validates as QES after its signing certificate
  expires, as long as the document timestamp is valid.
- Limit: the document timestamp is not renewed. Before the certificate of
  the TSA unit expires, or before its algorithms become weak, an operator
  must append a new document timestamp (DSS `extendDocument` at
  `PAdES_BASELINE_LTA` again). Nothing schedules this.
- Limit: DSS extends every signature in the document. A contract with AES
  and QES signatures needs revocation data (CRL or OCSP) for every signer
  certificate, the AES ones included. When DSS cannot get it, the completing
  submission is refused as retryable until the revocation source answers.
- Limit: the extension and the second validation are two more DSS round
  trips, with TSA and revocation requests, inside the transaction budget of
  the submission (`conf.TransactionTimeout`). A slow TSA or OCSP responder
  can use it up. The submission then fails and can be sent again.
- Limit: only the signature that completes the local signatures starts the
  extension. A signature that a counterparty appends later is not extended
  by this instance.
- Limit: the DSS demonstration image cannot extend a document signed by the
  development wallet: its signing certificate chains to a root that no
  trusted list names, and DSS skips revocation data for an untrusted chain.
  The tests use synthetic DSS revisions (classic cross-reference and
  cross-reference stream) and a real PDFBox PAdES-B-T revision
  (`pdf-core/compiler/testdata/dss-pades-b-t.pdf`). Real PAdES-B-LTA output
  is first checked at the first deployment with a qualified signer.
- Contracts archived before this change stay PAdES-B-T. The compliance
  viewer reports them once their signing certificate expires.
