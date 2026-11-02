# DCS documentation

## Manuals

| Document | Audience |
|---|---|
| [User manual](manuals/user-manual.pdf) | Users of the web client |
| [Technical manual](manuals/technical-manual.pdf) | Architects and developers |
| [Deployment guide](manuals/deployment-guide.pdf) | Operators |

## Operation

| Document | Content |
|---|---|
| [Helm chart and local development](../deployment/README.md) | Values, secrets, profiles, local stack |
| [Backup and restore](backup-integration-guide.md) | What to back up, restore order, erasure in backups |
| [Key management](key-management-concept.md) | Every key of an instance: custody, provisioning, rotation, erasure |
| [Audit checkpoint and workflow gate](audit-checkpoint-workflow-gate-operations.md) | ORCE endpoints, credentials and timeouts |
| [Federated Catalogue](fc-integration/fc-integration-guide.md) | Deployment and operation of the FC integration |

## Architecture

| Document | Content |
|---|---|
| [Architecture decisions](architecture-decisions.md) | Decisions implemented in the code, by system area |
| [Backend architecture](backend/README.md) | Backend structure, CQRS, audit trail, federation, and ADRs 0001 to 0007 |
| [Semantic architecture](semantic-ontology/README.md) | Ontology, LinkML schema, SHACL shapes, ODRL profile |
| [Vocabulary namespace](w3id/facis/README.md) | `w3id.org/facis` identity and resolution |
| [Audit policies](policies/) | Default template and contract-content audit rules (read by the backend) |
| [pdf-core](../pdf-core/README.md), [backend](../backend/README.md), [BDD suite](../tests/bdd/README.md), [test wallet](../testWallet/README.md) | Component READMEs |

## Requirements and compliance

| Document | Content |
|---|---|
| [SRS](SRS_FACIS_DCS.pdf) | Software requirements specification ([text form](SRS_FACIS_DCS.txt), read by the traceability check) |
| [Traceability matrix](TRACEABILITY_SRS_BDD.md) | Each SRS requirement mapped to its evidence |
| [Recorded deviations](adr-38-recorded-deviations-of-the-pilot.md) | Scope decisions of the pilot with their reasons |
| [EUCS control mapping](eucs-control-mapping.md) | EUCS Annex A objectives mapped to DCS controls and the operator |
| [Security review](sonar-security-review-2026-10-30.md) | SonarQube security findings and their handling |

## Decision records

| ADR | Decision |
|---|---|
| [1](adr-1-key-custody.md) | PKCS#11/HSM as the single key-custody mechanism |
| [2](adr-2-contract-state-machine.md) | Contract-formation state machine reconciles SRS §1.2 and §3.1.1 |
| [3](adr-3-signing-semantics.md) | Signing semantics: org-key AES with PID identity binding under the signature |
| [4](adr-4-c2pa-embedding.md) | C2PA embedding transport and the remote-manifest fallback |
| [5](adr-5-xfsc-component-posture.md) | XFSC component posture: what is retained, what is substituted |
| [6](adr-6-odrl-profile-enforcement.md) | Real ODRL under a DCS profile, enforced server-side |
| [7](adr-7-hierarchy-direction.md) | Contract hierarchy links point child → parent only, never the reverse |
| [8](adr-8-semantic-hub-version-pinning.md) | Semantic Hub version pinning for enforcement |
| [9](adr-9-shacl-engine-gordflib.md) | goRDFlib as the SHACL engine, replacing the hand-rolled subset matcher |
| [10](adr-10-clause-catalog-transport.md) | Clause catalog transport: pre-digested JSON, raw Turtle alongside |
| [11](adr-11-opa-odrl-enforcement.md) | OPA/Rego as the ODRL evaluation engine, replacing the hand-rolled operator switch |
| [12](adr-12-wallet-driven-signing.md) | Wallet-driven remote AES: the DCS is the relying party, not the signatory |
| [13](adr-13-pdf-exchange-federation.md) | DCS-to-DCS federation is a PDF (+ JAdES) exchange, not a state broadcast |
| [14](adr-14-jsonld-single-expanded-form.md) | One JSON-LD form internally: expanded, expand-at-edges |
| [15](adr-15-placeholder-typed-node.md) | One typed placeholder node: SHACL-typed, @id-linked, self-contained |
| [16](adr-16-audit-checkpoints-external-anchoring.md) | Audit-trail tamper evidence is Merkle checkpoints, anchored externally |
| [17](adr-17-machine-signing-is-not-an-aes-signature.md) | A System User cannot produce an AES signature |
| [18](adr-18-fc-catalogue-single-service-account.md) | FC's Participant is this DCS deployment, not the individual publisher |
| [19](adr-19-federation-agreement-credential.md) | Federation trust is layered: identity (eIDAS/did:web), rule acceptance (agreement credential), and a local policy endpoint |
| [20](adr-20-signing-acceptance-hardening.md) | Signing acceptance-path hardening: nonce binding, byte pinning, level-aware gates, and dropping EUDIPLO |
| [21](adr-21-system-contract-sealer.md) | System Contract Sealer: the seal ADR-17 deliberately deferred |
| [22](adr-22-contract-field-model.md) | Contract fields: the typed node is a field, not a placeholder |
| [23](adr-23-generic-contract-data-graph.md) | The contract-data graph is generic: any SHACL vocabulary, field leaves, in-document resolution |
| [24](adr-24-v1-signature-scope.md) | v1 signature scope: wallet-based AES, level determined by the provider |
| [25](adr-25-contract-target-registry.md) | Contract target systems are a configured registry, and each contract designates its own |
| [26](adr-26-provenance-reanchored-after-signing.md) | Provenance is re-anchored after signing, and the signature keeps priority |
| [27](adr-27-machine-credentials-issued-not-configured.md) | Machine credentials are issued, not configured |
| [28](adr-28-ipfs-encryption-at-rest-key-shredding.md) | IPFS artifacts are encrypted with random per-contract keys, and erasure is the logged destruction of their wrapped copies |
| [29](adr-29-poa-issuer-authority-and-integration-manager-scope.md) | PoA organizational authority is issuer-bound, and Integration Manager scopes integrations, not access |
| [30](adr-30-tamper-is-a-verdict-not-an-error.md) | A tampered artifact is a verification verdict, not an internal error |
| [31](adr-31-purpose-scoped-issuer-trust.md) | Issuer trust is scoped by purpose, bound to an organization, and resolved by a configurable mechanism |
| [32](adr-32-scheduled-compliance-monitoring.md) | Continuous compliance monitoring runs as a scheduled sweep, not an in-process loop |
| [33](adr-33-the-target-system-observes-the-dcs-records.md) | The target system reports verdicts, the DCS records them |
| [34](adr-34-status-lists-are-signed-by-the-issuer-that-issued-the-credential.md) | A status list is signed by the issuer whose credential it governs |
| [35](adr-35-peer-issuers-are-anchored-not-enumerated.md) | Peer issuers are anchored, not enumerated; login issuers are enumerated |
| [36](adr-36-artifacts-are-stored-in-kubo-directly.md) | Artifacts are stored in Kubo directly |
| [37](adr-37-regulatory-rule-pack.md) | Regulatory rules are gate findings, not a legal assessment |
| [38](adr-38-recorded-deviations-of-the-pilot.md) | Recorded deviations of the pilot |
| [39](adr-39-durable-event-delivery.md) | Events are delivered through JetStream durable consumers |
| [40](adr-40-qes-archives-are-pades-b-lta.md) | Archives of QES contracts are PAdES-B-LTA |
| [OCM-W](adr-ocmw-vc-signing.md) | Why OCM-W NATS services are not used for VC signing in DCS |
