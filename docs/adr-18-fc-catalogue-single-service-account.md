# ADR-18: FC's Participant is this DCS deployment, not the individual publisher

## Status

Accepted (2026-07-24). Revised same day after a quota-capped testbed
deployment showed the co-deployed assumption below does not hold everywhere.
Revised 2026-07-27 to make that trust boundary an enforced deployment
decision.

## Context

DCS publishes contract templates as assets to the Federated Catalogue (FC)
via `backend/internal/templaterepository/command/publish.go`, authenticating
with a Keycloak `client_credentials` service account (`federated-catalogue`,
see `deployment/helm/templates/fc-realm-provision-job.yaml`).

fc-service-server's own authorization model (`SessionUtils.checkParticipantAccess`,
called from `AssetService` on every asset create/update/delete) requires the
caller's JWT `participant_id` claim to match the asset's `issuer` field,
unless the caller holds `ADMIN_ALL` or the catalogue-admin role (`Ro-MU-CA`).

The asset's `issuer` was originally `HolderDID` — the **per-user** DID from
the request session (`middleware.GetHolderDID(ctx)`, the individual employee
who clicked publish). That's the wrong identity concept for FC: Gaia-X's
Participant is an organization/deployment, not an individual end user — the
same mismatch as trying to log an individual employee into a B2B catalogue
account. Fixed by publishing under `signing.issuerDID` instead (already
wired as `s.IssuerDID`, the DCS instance's own did:web, used elsewhere for
C2PA/provenance authority) — see `PublishCmd.InstanceDID`. This value is
**constant per deployment**, not per request, which is what makes the
options below realistic instead of requiring per-user infrastructure.

Two more precise alternatives were considered for how the *service account's*
JWT gets a matching `participant_id` and are not yet built:

- **Per-deployment Keycloak client + protocol mapper (the actual next step).**
  Register this instance's did:web as an FC Participant once (needs
  `Ro-MU-CA` to bootstrap, one-time), give this deployment's own Keycloak
  client a protocol mapper hardcoding `participant_id` to that same did:web,
  and drop the blanket role grant below in favor of just `ASSET_*`/`SCHEMA_*`.
  Small and mechanical *given the issuer is now fixed* — this was not
  feasible when the issuer varied per user.
- **OAuth2 token exchange (RFC 8693) / on-behalf-of, or a per-user "Connect
  Catalogue" consent flow.** Both were considered and rejected as solving a
  problem that no longer exists now that the issuer is per-deployment, not
  per-user — they'd add participant-per-human infrastructure for an identity
  concept (individual employee) FC was never asking for in the first place.

## Decision

For now, grant DCS's own FC service account every functional
`federated-catalogue` client role (all of them except `uma_protection`,
which is Keycloak-internal), including `ADMIN_ALL` — see
`fc-realm-provision-job.yaml`'s role-mapping step. This remains a stand-in
for the per-deployment client above, not the end state.

The Helm chart now enforces the distinction between an owned catalogue and a
remote one. When FC integration is enabled without co-deploying `fcservice`,
rendering fails unless the operator explicitly sets
`federatedCatalogue.remote.acknowledgeAdminAllTrustBoundary=true`
(`deployment/helm/templates/deployment.yaml:1-3`,
`deployment/helm/values.yaml:346-365`). This acknowledgement is not tenant
isolation and does not reduce the account's authority; it records that the
remote catalogue is inside one mutually trusted administrative boundary.

## Consequences

- Every contract template DCS publishes to its own FC instance is
  effectively published with catalogue-admin rights. Since the issuer is now
  this deployment's own identity (not the individual publisher's), the
  narrower per-deployment-client fix above is a well-scoped, low-risk
  follow-up whenever someone picks it up — it no longer requires the
  larger per-user designs this ADR previously proposed.
- **A quota-capped namespace is the concrete reason this cannot stay
  "co-deployed always."** Where a `ResourceQuota` caps Services (10 is a real
  example) a second DCS deployment realistically has to point at a *first*
  deployment's already-running FC instead of co-deploying its own full
  Keycloak+Fuseki+Postgres+FC stack. That's cross-tenant: today, any DCS
  instance sharing that FC would hold `ADMIN_ALL` and could edit every other
  tenant's catalogue entries. The per-deployment-client fix above is the
  prerequisite before FC is shared across DCS deployments that don't fully
  trust each other — do not point a second DCS at a shared, non-co-deployed
  FC while this service account still holds `ADMIN_ALL`.
- Misconfigured remote use now fails during Helm rendering instead of silently
  deploying cross-tenant administrator credentials. The lifecycle contract
  verifies both rejection without acknowledgement and acceptance with it
  (`deployment/helm/tests/federated_catalogue_lifecycle_test.sh:309-318`).
