# ADR-27 — Machine credentials are issued, not configured

Status: accepted, 2026-07-28.

Supersedes the callback authentication described in
[ADR-25](adr-25-contract-target-registry.md). Related:
[ADR-17](adr-17-machine-signing-is-not-an-aes-signature.md) for why no machine
credential can sign, [ADR-16](adr-16-audit-checkpoints-external-anchoring.md)
for the notary that consumes one.

## Context

Two kinds of non-human caller reach DCS, and each authenticated differently.

**System Users** (SRS §2.4 Table 5) authenticated as OAuth2 clients registered
in Hydra, with the client id and secret written into the deployment's Helm
values, and their roles listed alongside under `systemClients`. Consequences:

- the secret sat in plaintext in a values file, so it lived wherever that file
  was copied, and was as reviewable as any other line of configuration
- adding an integrator, changing what it may do, or rotating its credential
  meant editing YAML and redeploying
- there was no record of when a credential was issued

**Contract Target Systems** authenticated their deployment callback with a
single shared secret, `DEPLOYMENT_CALLBACK_SECRET`, held in the same values
file. A callback therefore proved only that *some* target was calling. With
several targets registered — which is the entire point of ADR-25 — one
compromised target could acknowledge, and report KPIs against, deployments
dispatched to any other. Rotating it broke every target at once.

Both problems are the same problem: a credential that is *configured* rather
than *issued* cannot be scoped to one holder, cannot be rotated independently,
and cannot be revoked without a deployment.

## Decision

**Every machine caller gets its own OAuth2 client, provisioned in Hydra through
its admin API, and its secret is displayed exactly once.**

- A `machine_identities` registry row records the OAuth2 client, the participant
  DID its actions are attributed to, its roles, and whether it is enabled.
- Creating an identity provisions the client and returns the generated secret in
  that one response. **DCS never stores it.** Hydra holds a hash and has no API
  to read it back, so "shown once" is a property of the system rather than a
  convention the UI observes.
- Rotation issues a new secret and invalidates the previous one immediately. A
  lost credential is rotated, never recovered.
- Deleting an identity deletes the client, so no credential outlives the entry
  that justified it.
- A contract target's callback credential works the same way and is bound to its
  registry row. The callback is accepted only when the caller's client is the one
  recorded on the target the deployment was dispatched to.
- Roles are read from the registry by `client_id` and never from the token. A
  client-credentials token carries no `ext` claims; anything it did carry must
  not widen what the caller may do.
- Disabling an identity refuses it at the next request, without waiting for a
  secret to expire.

`DCS_SYSTEM_CLIENTS` remains, reduced to a **seed**: its entries are reconciled
into the registry at startup, because a deployment needs callers that exist
before a human can log in to create them — the BDD harness authenticates as one
to reach the API at all. Runtime resolution reads the registry only, so there is
one path, not two.

## Consequences

A target can acknowledge its own deployments and no others, which is the
property the shared secret could never have. Credentials become operational
rather than architectural: issuing, rotating and revoking one is an admin action
with an audit trail, not a redeploy.

This is a **breaking change to DCS-IR-SI-05**. A target system now presents a
bearer token obtained with `grant_type=client_credentials` instead of the
`X-Deployment-Callback-Secret` header, and a deployment dispatched to a target
that has no credential issued cannot be acknowledged at all. There is no
fallback: keeping the shared secret alive as a second accepted path would
preserve exactly the weakness this removes.

The blast radius of Hydra widens — it is now on the path for issuing and
rotating machine credentials, not only for login. It was already a hard
dependency: the backend refuses to start when OIDC discovery fails.

`Contract Target System` becomes a real role. It authorises the deployment
callback and nothing else, and it is carried by an issued credential rather than
configured by hand. It remains impossible to configure a machine caller that can
sign: `Sys. Contract Signer` holds no signing scope (ADR-17).

## Alternatives considered

**Per-target shared secrets, hashed at rest.** Fixes the blast radius and allows
one-time display, but keeps a second bespoke authentication scheme beside the
OAuth2 one, with its own hashing, comparison and rotation code to get right.
Hydra already does this correctly, and every other caller already speaks OAuth2.

**mTLS for target callbacks.** Stronger, and worth revisiting where a target can
manage certificates. It needs certificate lifecycle management DCS does not have
and would not share with the System User path, so it would leave the two kinds of
caller on different mechanisms again.

**Leaving System Users in values and fixing only the targets.** Smaller change,
but it keeps plaintext secrets in configuration for the callers that hold the
broadest roles — creating contracts, managing them — which is the wrong half to
leave alone.
