# ADR-36: Artifacts are stored in Kubo directly

Status: Accepted (2026-08-06). Removes the eclipse-xfsc ipfs-document-manager
(and the ssi-vdr-ipfs plugin behind it) from this project's deployments. The
IPFS node itself is unchanged: the same Kubo, the same content addresses, the
same artifacts.

## Context

Every artifact this service anchors goes to IPFS: the hash-chained audit trail
writes one object per event, and every PDF, C2PA manifest and provenance record
is stored and read back by CID. That path is on the critical route of signing,
peer shipping and audit verification, so its failure mode is everyone's failure
mode.

Those writes went through the eclipse-xfsc `ipfs-document-manager`, a tenant
HTTP wrapper that delegates to the `ssi-vdr-ipfs` plugin, which in turn drives
Kubo's RPC API. This ADR records why that wrapper is removed and the RPC API is
called directly.

The evidence is a full BDD run. The suite sat at 451/17 with failures spread
across encryption-at-rest, SLA federation, external checkpoints and workflow
gates — four unrelated features whose only shared dependency is the artifact
store. The manager returned 736 5xx over the run, and its latency distribution
was bimodal with an empty middle: 100 requests under 100ms, 150 cut off at a
flat 5.00s, and nothing at all in between. A component short of capacity
spreads its latencies; one holding a lock produces exactly this. Kubo itself
logged nothing beyond `Daemon is ready` for the entire run — it was never the
thing failing.

Reading the wrapper's source explains both numbers.

**A read lists the entire pinset.** `ssi-vdr-ipfs` answers every GET by calling
`Pin().Ls(ctx)` and scanning the result for the CID before it fetches anything
(`main.go`, `Get`). A read therefore costs O(pins) — over a full run the pinset
reached 6895 — and it spends that time holding Kubo's pinner read lock, which is
the same lock a concurrent store needs to take for writing in order to pin. The
audit trail is read by walking chains of these objects, so the busiest read path
in the system is also the one that blocks its writes. That is the empty middle:
with no `Ls` in flight a store completes in milliseconds, and with one in flight
it waits seconds.

**A write shares one deadline between two operations.** `DefaultAddFile`
(`addFile.go`) creates a single `context.WithTimeout(context.Background(),
5*time.Second)` and passes it to both the add and the pin that follows. The pin
does not get five seconds; it gets whatever the add left. When the add consumes
the budget the pin fails instantly and the error returned is `failed to pin file
to ipfs node` — which names the wrong operation. That mislabelling cost this
project five rounds of tuning aimed at Kubo's pinner: node CPU reservation,
version pinning, `Datastore.NoSync`, client-side write-concurrency caps, a
hermetic swarm, and sharding the MFS tree. Every one of them measured as no
improvement, because none of them addressed a lock in the wrapper.

Both are structural, not configurational. The 5s budget is a literal in the
plugin with no environment override, and no deployment-side setting changes what
`Get` does. Neither is reachable from this repository.

## Decision

Artifacts are stored and read through the Kubo RPC API directly. The
`ipfs-document-manager` deployment, its chart, and the `IPFS_TENANT_BASE_URL`
configuration are removed; `IPFS_MFS_BASE_URL` (the Kubo RPC endpoint) becomes
the single required IPFS setting.

The client already held this path — it is what runs when no tenant URL is
configured — so the change removes a layer rather than adding one. Reads resolve
with `cat?arg=<cid>`, which touches no pin state; writes `add` and root the
result in MFS, which is what protects it from GC. Deletion continues to unpin by
CID.

This costs the tenant-scoped `DataIdentifier` index the manager maintained.
Nothing here depended on it: the store is content-addressed, every CID this
service resolves is one it recorded in its own database, and the manager's own
index was already unreliable enough that the client carried a Kubo fallback for
when it "transiently forgot" a mapping. Multi-tenancy on the artifact store is
not a property this deployment has — a DCS instance is one tenant.

## Consequences

The failure this diagnosed should disappear: reads stop taking the pinner lock,
and writes get Kubo's own timeout rather than a 5s budget already half spent.
Read latency improves on its own — an O(pins) scan is removed from the hot path.

The remedial changes made while chasing the wrapper's mislabelled error should
be revisited rather than kept. The client-side write-concurrency caps
(`maxConcurrentWrites`, `maxConcurrentBulkWrites` and `CreateFileBulk`) were
added on the theory that the node degraded under concurrent pinning; that theory
is now refuted, and they are complexity in the storage path with no measured
return. The MFS-tree sharding built on the same theory was already reverted for
the same reason.

We give up an XFSC component. ADR-5 records this project's posture on those, and
this is the second removal after ADR-34's statuslist-service: retained where the
component carries a federation contract others depend on, removed where it is an
implementation detail we can meet directly. An artifact store is the latter —
the interoperable thing is the CID, and that is unchanged. Upstream issues are
worth filing on both defects, but neither is a reason to keep the layer in the
meantime.

## References

- `eclipse-xfsc/ssi-vdr-ipfs` — `main.go` (`Get`, `Put`, `Update`, `Delete`),
  `addFile.go` (`DefaultAddFile`)
- `eclipse-xfsc/ipfs-document-manager` — `api.go`, `env.go`
- ADR-5 (XFSC component posture), ADR-34 (statuslist-service removal),
  ADR-28 (IPFS encryption at rest and key shredding)
