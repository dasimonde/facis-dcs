# EUCS Annex A control mapping

Maps the security objectives of the EUCS (EU cybersecurity certification scheme for cloud services) to the controls of the DCS and to the responsibilities of the operator (DCS-NFR-COMP-02).

**Source.** ENISA, "EUCS – Cloud Services Scheme", candidate scheme, draft of December 2020, Annex A "Security objectives and requirements". EUCS is not adopted; no certificates are issued under it. Objective IDs and titles below are those of the draft. The draft labels the third DEV objective "DEV-02" a second time; this mapping calls it DEV-03.

**Scope.** The certified subject of EUCS is a cloud service and its provider (CSP). The DCS is software that an operator deploys on its own Kubernetes cluster; the operator is the CSP. The operator selects the assurance level (basic, substantial, high) and is the party that is certified.

**Owner** values:

| Owner | Meaning |
|---|---|
| DCS | The DCS implements the control. Evidence names the traceability row (`docs/TRACEABILITY_SRS_BDD.md`), test or file |
| Shared | The DCS provides the technical control; the operator configures, runs or documents it |
| Operator | Organisational or infrastructure control of the CSP. The DCS has no part in it |

## A.1 Organisation of information security (OIS), A.2 Information security policies (ISP), A.3 Risk management (RM), A.4 Human resources (HR)

| Objective | Owner | DCS contribution |
|---|---|---|
| OIS-01, OIS-03, OIS-04 | Operator | – |
| OIS-02 Segregation of duties | Shared | Role separation in the product: template creator, reviewer, approver and registrar are different roles, and the Template Manager does not review or approve (DCS-FR-UC-01-3, `TestTemplateManagerDoesNotReviewOrApprovePostgres`). The operator separates the duties of its staff |
| ISP-01 to ISP-03 | Operator | – |
| RM-01 to RM-03 | Operator | Input: the recorded deviations ([ADR-38](adr-38-recorded-deviations-of-the-pilot.md)) and the deviation register of the traceability matrix |
| HR-01 to HR-06 | Operator | – |

## A.5 Asset management (AM), A.6 Physical security (PS)

| Objective | Owner | DCS contribution |
|---|---|---|
| AM-01 Asset inventory | Shared | Every workload, Secret and volume is declared in the Helm chart (`deployment/helm`); the SBOM workflow (`.github/workflows/sbom.yml`) lists the software components |
| AM-02 to AM-05 | Operator | – |
| PS-01 to PS-05 | Operator | – |

## A.7 Operational security (OPS)

| Objective | Owner | DCS contribution |
|---|---|---|
| OPS-01 Capacity management – planning | Shared | k6 load test with one and two replicas and a capacity table (DCS-NFR-PER-02, `tests/load/run_load.py`) |
| OPS-02 Capacity management – monitoring | Shared | Prometheus metrics and alert rules (`deployment/helm/templates/prometheusrule.yaml`); HPA (`deployment/helm/templates/hpa.yaml`) |
| OPS-03 Capacity management – controlling of resources | Shared | Resource requests and limits per workload in the chart values; per-caller rate limit (`DCS_API_RATE_LIMIT_PER_MINUTE`) |
| OPS-04, OPS-05 Protection against malware | Shared | Images run as non-root with a read-only root file system (`deployment/helm/values.yaml` `securityContext`); optional image digests pin the image content. Malware scanning of nodes and registry is the operator's |
| OPS-06 Data backup and recovery – policies | Shared | Postgres backup CronJob, also of the Hydra database (`deployment/helm/templates/backup-cronjob.yaml`; DCS-NFR-SF-03); [backup integration guide](backup-integration-guide.md) |
| OPS-07 Data backup and recovery – monitoring | DCS | Alerts `DCSBackupFailed`, `DCSBackupStale`, `DCSBackupNeverSucceeded` (`prometheusrule.yaml`) |
| OPS-08, OPS-09 Data backup – regular testing, storage | Operator | Restore procedure in the [backup integration guide](backup-integration-guide.md); the operator tests it and selects the remote location |
| OPS-10 Logging and monitoring – policies | Shared | Audit trail and system logging (DCS-NFR-SEC-09, DCS-FR-UC-09-2); optional log store with retention |
| OPS-11 Derived data management | Shared | Audit reads need a recorded purpose (DCS-FR-PACM-04); erasure also clears plaintext copies of contract content (DCS-NFR-SEC-13) |
| OPS-12 Identification of events | DCS | Alerts `DCSAuthFailureSpike`, `DCSServerErrors`, `DCSBackendDown`, `DCSBackendRestarting`, `DCSSlowResponses` (DCS-NFR-SEC-11) |
| OPS-13 Access, storage and deletion of logs | DCS | Append-only audit tables, hash chains and ORCE anchoring with RFC 3161 time (DCS-FR-PACM-01, DCS-NFR-BR-05, DCS-OR-C2PA-009); role-based audit access (DCS-FR-PACM-04) |
| OPS-14 Attribution | DCS | Every audit entry records the actor, organization and role (DCS-FR-PACM-01, DCS-FR-UC-09-2) |
| OPS-15 Configuration of logging | Shared | Audit configuration is part of the chart; access to it follows Kubernetes RBAC |
| OPS-16 Availability of logging | Shared | External checkpoint sink and workflow gate on a broken chain ([operations guide](audit-checkpoint-workflow-gate-operations.md)) |
| OPS-17 Managing vulnerabilities – policies | Shared | gosec in golangci-lint and govulncheck for backend and pdf-core in CI (`.github/workflows/lint-and-format.yml`; DCS-NFR-SEC-15) |
| OPS-18 Online registers | Operator | – |
| OPS-19 Vulnerability identification | Shared | govulncheck and gosec on every push (`lint-and-format.yml`); [security review](sonar-security-review-2026-10-30.md) |
| OPS-20 Measurements, analyses and assessments | Operator | – |
| OPS-21 System hardening | Shared | Restricted pod security contexts, NetworkPolicies, TLS 1.3 in the cluster (DCS-NFR-SEC-01, DCS-NFR-SEC-12, DCS-NFR-SEC-14) |
| OPS-22 Separation of datasets | DCS | Contract read policy per party organization ([ADR-31](adr-31-purpose-scoped-issuer-trust.md)); per-contract encryption keys of the IPFS artifacts ([ADR-28](adr-28-ipfs-encryption-at-rest-key-shredding.md)) |

## A.8 Identity, authentication and access control management (IAM)

| Objective | Owner | DCS contribution |
|---|---|---|
| IAM-01 Policies for access control | Shared | Roles and scopes per endpoint in the goa design (`backend/design`); roles from verifiable credentials (DCS-NFR-SEC-03) |
| IAM-02 Management of user accounts | Shared | Users hold role credentials; the DCS keeps no passwords. System users are managed by the administrator with audited changes (DCS-FR-UC-09-1) |
| IAM-03 Locking, unlocking and revocation | DCS | Lockout after repeated invalid credentials and admin unlock (DCS-FR-UC-01-4, `TestUnlockLiftsAnActiveLockout`); revoked credentials are refused at login and refresh (DCS-IR-CI-09, DCS-NFR-BR-06) |
| IAM-04 Management of access rights | DCS | Rights come from the role credential, checked on each request (DCS-NFR-SEC-03) |
| IAM-05 Regular review of access rights | Operator | The issuer of the role credentials reviews them |
| IAM-06 Privileged access rights | Shared | Administrative roles are separate scopes; remote administration only through the authenticated API (DCS-NFR-SF-02) |
| IAM-07 Authentication mechanisms | DCS | OpenID4VP wallet login with holder binding (DCS-IR-SI-08, DCS-NFR-BR-01) |
| IAM-08 Protection and strength of credentials | DCS | Credentials stay in the user's wallet; keys of the instance in the PKCS#11 token (DCS-IR-HI-01, DCS-NFR-SEC-06) |
| IAM-09 General access restrictions | DCS | NetworkPolicies admit only the ingress, the release and named peers (DCS-NFR-SEC-14); federation trust gate for peers ([ADR-19](adr-19-federation-agreement-credential.md)) |

## A.9 Cryptography and key management (CKM)

| Objective | Owner | DCS contribution |
|---|---|---|
| CKM-01 Policies for encryption and key management | DCS | [Key management concept](key-management-concept.md) |
| CKM-02 Encryption of data in transit | DCS | TLS 1.3 at the ingress, in the cluster and for outbound clients (DCS-IR-CI-01, DCS-NFR-SEC-01) |
| CKM-03 Encryption of data at rest | Shared | Per-contract encryption of the IPFS artifacts (DCS-NFR-SEC-14); PostgreSQL on an encrypted StorageClass, an operator prerequisite (DCS-NFR-SEC-08) |
| CKM-04 Secure key management | DCS | Non-extractable keys in the PKCS#11 token, key shredding on erasure (DCS-IR-HI-01, DCS-NFR-SEC-13) |

## A.10 Communication security (CS)

| Objective | Owner | DCS contribution |
|---|---|---|
| CS-01 Technical safeguards | DCS | TLS 1.3, NetworkPolicies, rate limit (DCS-NFR-SEC-01, DCS-NFR-SEC-14) |
| CS-02 Security requirements to connect within the CSP | DCS | Internal TLS 1.3 verified against the internal CA (`global.internalTLS`) |
| CS-03 Monitoring of connections | Shared | Request logging and alerts (DCS-NFR-SEC-09, DCS-NFR-SEC-11) |
| CS-04 Cross-network access | DCS | Peer requests signed per request and accepted once (DCS-NFR-SQ-06; `TestAPeerChallengeIsAcceptedOnce`) |
| CS-05 Networks for administration | Shared | Hydra admin API and IPFS RPC API admit only the backend (`deployment/helm/templates/networkpolicy.yaml`) |
| CS-06 Traffic segregation in shared networks | Shared | NetworkPolicies per component (`networkpolicy.yaml`); the operator's CNI enforces them |
| CS-07 Network topology documentation | Shared | Chart and [deployment README](../deployment/README.md) describe components and routes |
| CS-08 Software defined networking | Operator | – |
| CS-09 Data transmission policies | DCS | Peer exchange carries encrypted artifacts and JAdES provenance (DCS-NFR-SEC-14, DCS-FR-SM-02) |

## A.11 Portability and interoperability (PI)

| Objective | Owner | DCS contribution |
|---|---|---|
| PI-01 Documentation and security of interfaces | DCS | OpenAPI from the goa design (DCS-IR-CI-02); every API authenticated |
| PI-02 Contractual agreements for the provision of data | Shared | Export of contracts, evidence and archive queries (DCS-IR-CI-02); the agreement is the operator's |
| PI-03 Secure deletion of data | DCS | Key shredding on every instance that holds a copy, plaintext copies cleared (DCS-NFR-SEC-13, `features/26_encryption_at_rest/key_shredding.feature`) |

## A.12 Change and configuration management (CCM), A.13 Development of information systems (DEV)

| Objective | Owner | DCS contribution |
|---|---|---|
| CCM-01 Policies for changes | Shared | Changes through pull requests with CI gates |
| CCM-02 Risk assessment of changes | Operator | – |
| CCM-03 Testing changes | DCS | CI: Go tests with PostgreSQL, vitest, BDD on kind with two instances, Playwright e2e, k6 load test, Helm chart tests (DCS-OE-04) |
| CCM-04 Approvals for provision in production | Operator | – |
| CCM-05 Performing and logging changes | Shared | Versioned SQL migrations (`backend/migrations/sql`); Helm releases |
| CCM-06 Version control | DCS | Git; versioned chart and images |
| DEV-01 Policies for development | Shared | Lint and format gates (DCS-NFR-SQ-01) |
| DEV-02 Development supply chain security | Shared | Go module verification and SBOM (`lint-and-format.yml`, `sbom.yml`); optional image digests |
| DEV-03 Secure development environment | Operator | – |
| DEV-04 Separation of environments | Shared | Separate value profiles for dev, BDD, acceptance and prod (`deployment/helm/values.*.yml`) |
| DEV-05 Development of security features | DCS | Security requirements of the SRS with tests (DCS-NFR-SEC-01 to DCS-NFR-SEC-18) |
| DEV-06 Identification of vulnerabilities | DCS | gosec, govulncheck (DCS-NFR-SEC-15) |
| DEV-07 Outsourcing of development | Operator | – |

## A.14 Procurement management (PM), A.15 Incident management (IM), A.16 Business continuity (BC)

| Objective | Owner | DCS contribution |
|---|---|---|
| PM-01 to PM-05 | Operator | – |
| IM-01 to IM-06 | Operator | Input: alerts (DCS-NFR-SEC-11) and audit trail (DCS-FR-PACM-01) |
| IM-07 Incident evidence preservation | DCS | Tamper-evident audit trail anchored in ORCE with RFC 3161 time (DCS-FR-PACM-01, DCS-OR-C2PA-009) |
| BC-01, BC-02 | Operator | – |
| BC-03 Business continuity planning | Shared | Replicas, PodDisruptionBudget, backup (DCS-NFR-PER-03, DCS-NFR-SF-03) |
| BC-04 Tests and exercises | Shared | DCS restart scenario in BDD (DCS-NFR-SF-01); restore tests are the operator's |

## A.17 Compliance (CO), A.18 User documentation (DOC), A.19 Investigation requests (INQ)

| Objective | Owner | DCS contribution |
|---|---|---|
| CO-01 to CO-04 | Operator | Input: traceability matrix and regulatory rule pack ([ADR-37](adr-37-regulatory-rule-pack.md)) |
| DOC-01 Guidelines for cloud customers | Shared | [Deployment README](../deployment/README.md), operations guides in `docs/` |
| DOC-02 Online register of known vulnerabilities | Operator | – |
| DOC-03 Locations of data processing and storage | Operator | The operator selects the cluster and storage location |
| DOC-04 Justification of the assurance level | Operator | – |
| DOC-05, DOC-06 Composition | Shared | Recorded deviations ([ADR-38](adr-38-recorded-deviations-of-the-pilot.md)) name the operator prerequisites |
| INQ-01 to INQ-03 | Operator | – |

## A.20 Product safety and security (PSS)

| Objective | Owner | DCS contribution |
|---|---|---|
| PSS-01 Error handling and logging | DCS | Typed API errors without internal detail; refused attempts logged with identity, time and resource (DCS-FR-UC-01-4) |
| PSS-02 Session management | DCS | Hydra access tokens of 5 minutes in prod and acceptance; credential status re-checked at refresh (DCS-NFR-BR-06) |
| PSS-03 Software defined networking | Operator | – |
| PSS-04 Images for containers | DCS | Multi-stage images, non-root, read-only root file system, optional digests (DCS-NFR-SQ-03, DCS-OE-02) |
| PSS-05 Locations of data processing and storage | Operator | – |

## Result

The DCS implements or provides the technical controls of IAM, CKM, CS, PI, PSS and the logging, backup and vulnerability parts of OPS, CCM and DEV. The organisational categories (OIS, ISP, RM, HR, AM, PS, PM, most of IM and BC, CO, INQ) are controls of the operator as CSP. Certification at an assurance level applies to the operator's service and is not a property of the software ([ADR-38](adr-38-recorded-deviations-of-the-pilot.md) records the operator prerequisites).
