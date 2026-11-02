# ADR-37: Regulatory rules are gate findings, not a legal assessment

Status: Accepted (2026-10-30). Supersedes item 6 of
[ADR-24](adr-24-v1-signature-scope.md).

## Context

`DCS-FR-PACM-03` requires automated checks against regulatory frameworks
(eIDAS, GDPR) during contract workflows. Contracts that fail them are blocked
from execution or flagged for manual review. ADR-24 descoped the requirement
as an autonomous legal-conformity assessment. Some regulatory preconditions
are facts that the DCS records, and a machine can check them.

## Decision

The deployment workflow gate evaluates a regulatory rule pack before
execution (`internal/processauditandcompliance/workflowgate/regulatory.go`).
Its findings go into the gate's local evaluation with the source
`regulatory-rule-pack`. The gate result follows from the severity: an error
blocks, a warning puts the run into Compliance Officer review
(`POST /pac/workflow-gates/review`), info and satisfied findings are evidence
only.

| Rule | Checks | Severity when it fails |
|---|---|---|
| `regulatory.eidas.signature-level` | Each signature this instance applied reached the level its signature field requires (`dcs:requiredCredentialType`) | error |
| `regulatory.eidas.qualified-timestamp` | A contract that requires QES runs on a TSA that the operator declares qualified (`DCS_TSA_QUALIFIED=true`) | error |
| `regulatory.gdpr.retention-policy` | The archived contract has a retention period; erasure is the archive deletion with key shredding (ADR-28) | info |
| `regulatory.gdpr.no-pid-in-pdf` | The contract document, which is the PDF payload and the source of its pages, contains no value of a signatory's PID claims | warning |
| `structure.hierarchy.parent-in-force` | The `dcs:parentContract` of the contract is SIGNED or ACTIVE on this instance (`DCS-FR-PACM-06`) | error; warning when the parent is not known on this instance |

Every rule writes a finding also when it is satisfied, so the run shows what
was checked.

## Consequences

- The findings are technical evidence. The DCS makes no claim of legal
  certification, and a passed rule pack is not a legal opinion.
- No QTSP is operated, so `DCS_TSA_QUALIFIED` is unset by default and a
  contract that requires QES cannot be executed.
- The retention rule is info only. The archive sets the retention period of
  an entry from `DCS_ARCHIVE_RETENTION_YEARS` (`conf.ArchiveRetentionYears`):
  `retention_until` is the contract's expiry date, or the archival date when
  the contract has no expiry, plus that number of years (at most 100). The
  default 0 sets no retention period, and deletion stays permitted.
- The PID rule matches whole string values. A PID value inside a longer text,
  and the PoA presentation embedded as signing evidence, are not checked.
- Signatures a peer applied are judged on the peer that applied them.
