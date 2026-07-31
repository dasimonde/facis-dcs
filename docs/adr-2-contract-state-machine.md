# ADR-2: Contract-formation state machine reconciles SRS §1.2 and §3.1.1

> **Superseded in part by [ADR-13](adr-13-pdf-exchange-federation.md)
> (2026-07-19).** The origin-as-single-writer / full-state-broadcast /
> peer-task-sync model this ADR's DCS-to-DCS synchronizer originally
> implied is **withdrawn**: federation exchanges a signed PDF (+ JAdES), not
> a broadcast of this state machine's internal state. The state machine
> itself — the transition table below, reconciling SRS §1.2 and §3.1.1 — is
> unchanged and still binding for each instance's own local contract state.

## Context

The SRS describes the contract lifecycle in two places that do not agree.
§1.2's narrative lifecycle reads *offered → accepted → executed → active →
terminated → archived*. §3.1.1's interface requirements (IR-CWE-05 through
IR-CWE-10) name a different state vocabulary built around
`SUBMITTED`/`REVIEWED`/`APPROVED`. Neither list is a strict superset of the
other, and building literally either one in isolation would leave the other
unimplementable.

## Decision

DCS implements one state machine with one transition table
(`backend/internal/contractworkflowengine/datatype/contractstate`) that is
the reconciliation of both lists, not a choice between them:

```
DRAFT → OFFERED → NEGOTIATION → SUBMITTED → REVIEWED → APPROVED → SIGNED → ACTIVE → TERMINATED / EXPIRED
                                                                          ↘ REVOKED
   ↘ REJECTED          ↘ WITHDRAWN
```

- `SUBMITTED`, `REVIEWED`, `APPROVED` are kept **verbatim** because
  IR-CWE-05..10 and SRS §6 Table 6 require them by name.
- `OFFERED`, `WITHDRAWN`, `ACTIVE`, `REVOKED` are added around them to make
  §1.2's offer/accept/withdraw narrative a first-class part of the state
  machine rather than a derived view bolted on afterward.
- `APPROVED` is declared **equivalent to** §1.2's "accepted" — the same
  state satisfies both readings; there is no separate "accepted" state.

Every transition is validated by a single transition table
(`ValidateTransition`), reached identically whether the transition is
triggered by the local UI/API path or a remote peer's `POST
/peer/contracts/action` (DCS-NFR-BR-08) — one enforcement point, not two.

## Consequences

- Every BDD scenario and every C2PA lifecycle-assertion mapping is written
  against this one vocabulary; nothing downstream needs its own
  state-translation layer.
- The reconciliation is recorded here explicitly (rather than silently
  picking one SRS reading) so it is a documented interpretation, not an
  inconsistency discovered during acceptance.
