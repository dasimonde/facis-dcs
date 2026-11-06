# Sonar security review, 30.10.2026

Scope: a new SonarQube analysis of the consolidated delivery (SonarQube Community 26.10, all languages, test code included). The list covers every security finding of this analysis, not only the 10 hotspots of the earlier report.

| Measure | Value |
|---|---|
| Lines analyzed | 134,706 (Go, TypeScript, JavaScript, Python, YAML, Dockerfile, CSS) |
| Security findings before the fixes below | 75 |
| Security findings after the fixes | 74, each reviewed below |
| Findings in Go code | 0 |

## Fixed

| Rule | Location | Fix |
|---|---|---|
| S2068 hard-coded password | `deployment/helm/charts/orce/settings.js` | The ORCE settings no longer contain the fixed editor password of the ORCE image. The editor and the admin API run only when `ORCE_ADMIN_PASSWORD_HASH` holds a bcrypt hash; otherwise they are off. The same change removes `httpStatic: '/data'`, which served every file in the Node-RED data directory without authentication. |
| File permission in test code | `backend/internal/contractworkflowengine/query/contracttemplate/realistic_contract_field_artifact_pipeline_test.go`, `pdf-core/signing_harness_test.go` | Test files are written with mode 0600. |
| Sort bugs | `NegotiationList.vue`, `ontology-domain-fields.ts`, `shape-library.ts`, `PermissionList.vue`, `ComplianceViewerView.vue` | Sorts work on copies and use explicit comparators. The new analysis reports no sort finding. |

## Safe

| Rule | Count | Location | Reason |
|---|---|---|---|
| S5332 clear-text HTTP | 27 | BDD steps, test wallet, ORCE flow tests, `backend/tsa-relay.py`, `pdf-core/features` | Test code and a local development stub. The URLs point to services inside the test cluster or to `127.0.0.1`. Production traffic uses TLS at the ingress. |
| S5659 JWT without signature check | 5 | BDD steps, test wallet | The tests read the claims of tokens that the product issued or checked. The product code has no finding of this rule. |
| S5443 publicly writable directory | 3 | `backend/tsa-relay.py`, BDD steps | Local development stub and test code. No production component uses these paths. |
| S6418 token in code | 2 | BDD steps, test wallet | Fixture strings (`not-a-valid-rfc3161-token`, a dummy SD-JWT). No secret. |
| S2612 file permission | 1 | `pdf-core/features/steps/dcs_pdf_core_steps.py` | Mode 0755 on a test tool that the test downloads and runs. Only the owner can write it. |
| S6504 copied file writable by the runtime user | 3 | `deployment/docker/Dockerfile` | The runtime user owns `/app` by design. A change to the scripts needs code execution as that user, which already has the same rights. Each pod start uses the unchanged image. |
| S6865 service account token mounted | 13 | Helm deployments | These pods use the namespace default service account. No Role or ClusterRole binding of the charts names it, so the token grants only the Kubernetes default discovery access. Components that need the Kubernetes API use dedicated service accounts with their own roles. |
| S6864, S6870 no memory or storage limit | 20 | Helm deployments | The templates take `resources` from the values of each environment, because the cluster sizes differ. The operator sets them per environment, or the namespace LimitRange applies. |
