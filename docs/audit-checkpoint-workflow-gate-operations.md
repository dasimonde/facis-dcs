# External checkpoint and workflow-gate operations

This note covers the two independently configured ORCE integrations introduced
for audit checkpoint publication and consequential contract workflow gates.
They have separate endpoints, credentials and timeout budgets.

## External checkpoint sink

The sink is opt-in. When enabled, Helm requires:

- a non-empty HTTPS URL;
- a positive external request timeout and positive internal Hydra/DCS timeout;
- a Bearer token, preferably obtained from a Kubernetes Secret.

The chart validates these conditions before rendering and maps the values into
the ORCE worker
(`deployment/helm/charts/orce/templates/deployment.yaml:5-18`,
`deployment/helm/charts/orce/templates/deployment.yaml:94-116`). A minimal
values override is:

```yaml
orce:
  checkpointSink:
    enabled: true
    url: "https://notary.example/checkpoints"
    timeoutSeconds: 5
    internalTimeoutSeconds: 30
    bearerTokenSecretRef:
      name: "dcs-checkpoint-sink"
      key: "CHECKPOINT_SINK_BEARER_TOKEN"
```

Do not enable `allowInsecureTLS` outside the BDD controls. The flow ignores that
setting unless test controls are also enabled
(`deployment/helm/charts/orce/values.yaml:30-43`).

The external timeout applies only to the observable sink POST. The internal
timeout applies to obtaining the machine token and reading DCS checkpoints.
Keeping them separate prevents a deliberately short third-party service-level
budget from truncating internal control-plane work
(`deployment/helm/charts/orce/flows/audit-checkpoint-anchor-flow.json:56`).

### Publication and recovery signals

The worker publishes each sequence in order with an idempotency key derived
from sequence and root. It advances its confirmation file only after a 2xx
response. An ordinary transport or non-2xx failure therefore leaves the
sequence pending for the next scheduled run. A reported `sequence_gap` or
`previous_root_mismatch` is different: the worker persists a blocked state and
does not attempt a later checkpoint
(`deployment/helm/charts/orce/flows/audit-checkpoint-anchor-flow.json:56`).

Treat that blocked state as an integrity incident. Reconcile the last confirmed
sequence/root with the external sink before changing persistent state. There is
no automatic bypass or fallback sink.

## Workflow-gate executor

Submission, offer, approval, signature and deployment share the
`workflowGateExecutor` configuration. With the bundled ORCE chart enabled, an
empty URL selects its `/workflow-gate/run` flow; otherwise set a compatible
endpoint explicitly. The backend and ORCE use the same Bearer-token value or
Secret reference
(`deployment/helm/values.yaml:116-135`,
`deployment/helm/templates/deployment.yaml:365-383`).

```yaml
workflowGateExecutor:
  url: "https://policy.example/workflow-gate/run"
  timeout: "10s"
global:
  workflowGateTokenSecretRef:
    name: "dcs-workflow-gate"
    key: "PAC_WORKFLOW_GATE_EXECUTOR_BEARER_TOKEN"
```

The backend performs exactly one executor request for a claimed immutable
snapshot and gate. Timeout, non-2xx, malformed or mismatched responses are
persisted as blocked; they are not retried within that run
(`backend/internal/processauditandcompliance/workflowgate/workflowgate.go:98-171`,
`backend/internal/processauditandcompliance/workflowgate/workflowgate.go:265-301`).

## Manual-review continuation

A `REVIEW` result requires a Compliance Officer decision with justification.
The decision is immutable per run. Continuation attempts form a durable history;
each attempt's row is completed from `DISPATCHING` to `SUCCESS` or `FAILED`
(`backend/migrations/sql/20260729_external_checkpoint_workflow_gates.sql:28-51`).
Approval invokes the stored transition continuation without dispatching the
executor again
(`backend/internal/processauditandcompliance/workflowgate/workflowgate.go:534-634`).

Known recovery gap: if the process terminates after a continuation attempt is
persisted as `DISPATCHING` but before it records `SUCCESS` or `FAILED`, a second
attempt is refused. Recovery/lease handling for that state is a separate future
ticket; operators must treat it as requiring investigation rather than
re-submitting the review
(`backend/internal/processauditandcompliance/workflowgate/workflowgate.go:581-599`).
