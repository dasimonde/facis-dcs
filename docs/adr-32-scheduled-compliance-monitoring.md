# ADR-32: Continuous compliance monitoring runs as a scheduled sweep, not an in-process loop

## Context

`DCS-FR-PACM-02` requires the system to *continuously* monitor contract
lifecycle activities for compliance violations, and to flag and report detected
risks. `DCS-NFR-SEC-11` adds that detection is measured — mean time to detect.

The detection rules themselves live in the Process Audit and Compliance
component and cover four violation classes: a contract awaiting a required
approval, a denied access attempt, a contract in force whose reported KPIs
breach its own ODRL obligations, and a deployment the target system never
received. They are pure derivations over persisted state, so they can be
evaluated at any moment against the database alone.

`GET /pac/monitor` exposes exactly this evaluation to a Compliance Officer. On
its own it satisfies "monitor on request", not "monitor continuously": a
violation exists only from the moment somebody happens to ask.

## Decision

**The sweep runs as a Kubernetes CronJob invoking a dedicated command
(`cmd/pacmonitor`), and alerting is deduplicated against a risk register.**

Three parts:

1. **`cmd/pacmonitor`** runs one sweep and exits. It opens the database and
   nothing else — in particular it does not run the server's startup gates (HSM
   self-test, Federated Catalogue readiness, status-list probe), so compliance
   monitoring keeps working when a neighbouring component is degraded. It does
   not join the event bus either: detections go to the transactional outbox, and
   the running server's OutboxProcessor anchors them in the audit trail and fans
   them out to webhook subscribers.

2. **The `pac-monitor` CronJob** in the chart runs it every five minutes with
   `concurrencyPolicy: Forbid`. It is enabled by default; disabling it means
   risks are only ever found by someone asking.

3. **`compliance_risk_findings`** holds one row per open violation, keyed by
   contract, risk type and a hash of the detail text. A risk alerts when it is
   first detected, and again if it recurs after being resolved — never on the
   sweeps in between.

The sweep response is unchanged: it lists every risk that currently holds,
because it answers "what is wrong right now". Deduplication governs alerting
only.

## Rationale

**Why a CronJob rather than a goroutine in the server.** Both are viable and the
repository has precedent for either. The CronJob wins on three counts that
matter for an unattended safeguard: a failed sweep is a Kubernetes object with a
status rather than a line in the server log; the interval and resource budget
are changeable declaratively without restarting the DCS; and concurrent runs are
prevented structurally rather than by leader election among replicas. It is also
directly invokable, which is what lets the acceptance suite drive a sweep
deterministically instead of waiting on a timer.

**Why the risk register is not optional.** The sweep re-derives every risk from
scratch, so without persistent state each run would re-report every open
violation. Unattended and every five minutes, that means an alert storm and an
audit trail in which one violation appears hundreds of times, burying the moment
it was actually detected. The register also carries `first_detected_at`, which
is what makes the MTTD of `DCS-NFR-SEC-11` measurable.

**Why the key includes the detail hash.** One contract can carry several risks
of the same type simultaneously — one per outstanding approver, one per denied
actor, one per breached metric. Keyed by contract and type alone, a second
denied actor would be silently swallowed as "already reported", which in a
security control is a defect rather than a simplification. The detail text is
derived only from persisted facts, never from the sweep's own clock, so the same
violation hashes identically on every run.

**Why `first_detected_at` resets on recurrence.** A violation that returns after
being resolved is a new incident, and the detection time measured against it
must describe that incident. Superseded incidents are not lost: every detection
is anchored in the tamper-evident audit trail as a `PAC_COMPLIANCE_RISK` event.

## Consequences

- The time-based rules (missing approvals, expiries) are served well by a
  five-minute cadence. The event-driven ones (unauthorized access, failed
  deployment) are reported with **up to five minutes' delay**; Kubernetes cannot
  schedule finer than one minute in any case. Where sub-minute latency is
  required, the path is an event-bus subscriber alongside the sweep, not a
  faster schedule — the underlying events already flow over NATS, and the risk
  register makes the two sources safe to combine, since whichever observes a
  violation first is the one that alerts.
- Alerts reach subscribers as the `compliance.risk_detected` webhook event.
- The sweep and the code it audits ship in the same image, so they cannot drift
  apart in a deployment.
- A detected risk is not a job failure: the command exits 0 and reports on
  stdout. A non-zero exit means the sweep could not run, which is the only
  condition worth paging an operator about.
