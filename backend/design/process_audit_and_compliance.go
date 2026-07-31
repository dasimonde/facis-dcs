package design

import (
	. "goa.design/goa/v3/dsl"
)

var PACAuditRequest = Type("PACAuditRequest", func() {
	Description("Process audit request")

	Token("token", String, "JWT token")

	Attribute("scope", String, "Scope that should be audited")
	Attribute("did", String, "Optional resource DID filter")
	Attribute("resource_id", String, "Optional resource DID filter alias used by audit-executor clients")
	Attribute("justification", String, "Required audit justification", func() { MinLength(1) })

	Required("scope", "justification")
})

var PACResourceAuditTrailEntry = Type("PACResourceAuditTrailEntry", func() {
	Description("Resource audit trails entry")

	Attribute("id", Int64, "Identifier for the outbox event")
	Attribute("component", String, "Name of the component")
	Attribute("event_type", String, "Type of the event")
	Attribute("event_data", Any, "Data of the event")
	Attribute("did", String, "Decentralized Identifier of the resource")
	Attribute("created_at", String, "The creation date of the event")
	Attribute("res_log_pred_cid", String, "Resource audit trail predecessor on the IPFS chain")
	Attribute("kind", String, "Entry kind: TIMELINE or CHECK")
	Attribute("result", String, "Check result: PASSED or FAILED")
	Attribute("rule_id", String, "Stable integrity rule identifier")
	Attribute("reason", String, "Human-readable check reason")

	Required("id", "component", "event_type", "event_data", "created_at")
})

// PACAuditResponse remains the shared timeline response used by the archive
// component API. POST /pac/audit itself returns PACExternalAuditResponse.
var PACAuditResponse = Type("PACAuditResponse", func() {
	Description("Resource audit trail")
	Attribute("did", String, "Decentralized Identifier of the resource")
	Attribute("component", String, "Name of the component")
	Attribute("created_at", String, "Creation date of the audit response")
	Attribute("audit_trail", ArrayOfRequired(PACResourceAuditTrailEntry), "Resource audit trails entries")
	Required("did", "component", "created_at", "audit_trail")
})

var PACAuditExecutor = Type("PACAuditExecutor", func() {
	Description("Identity of the external audit executor")
	Attribute("id", String, "Stable executor identifier")
	Attribute("version", String, "Executor implementation version")
	Required("id", "version")
})

var PACAuditFinding = Type("PACAuditFinding", func() {
	Description("A finding produced by the configured external audit executor")
	Attribute("rule_id", String, "Stable rule identifier")
	Attribute("result", String, "PASSED, FAILED, or REVIEW")
	Attribute("reason", String, "Human-readable finding reason")
	Attribute("severity", String, "Finding severity")
	Attribute("evidence_refs", ArrayOf(String), "References into the submitted evidence")
	Required("rule_id", "result", "reason")
})

var PACExternalAuditResponse = Type("PACExternalAuditResponse", func() {
	Description("Validated result returned by the configured external audit executor")
	Attribute("contract_version", String, "Versioned audit-executor HTTP contract")
	Attribute("audit_id", String, "Audit run UUID")
	Attribute("correlation_id", String, "Request correlation UUID")
	Attribute("scope", String, "Audited scope")
	Attribute("resource", Any, "Optional audited resource identity")
	Attribute("executor", PACAuditExecutor, "Executor identity and version")
	Attribute("executed_at", String, "Execution timestamp (RFC3339)")
	Attribute("findings", ArrayOfRequired(PACAuditFinding), "Executor-produced findings")
	Attribute("receipt", Any, "Optional executor receipt")
	Attribute("timeline", ArrayOfRequired(PACResourceAuditTrailEntry), "Optional DCS-procured timeline evidence retained for compatibility")
	Required("contract_version", "audit_id", "correlation_id", "scope", "executor", "executed_at", "findings")
})

var PACComplianceRisk = Type("PACComplianceRisk", func() {
	Description("A single compliance risk detected by continuous monitoring")

	Attribute("did", String, "Decentralized Identifier of the affected contract")
	Attribute("risk_type", String, "Machine-readable risk class (e.g. MISSING_APPROVAL)")
	Attribute("detail", String, "Human-readable description of the detected risk")
	Attribute("detected_at", String, "When the risk was detected (RFC3339)")

	Required("did", "risk_type", "detail", "detected_at")
})

var PACIncidentFinding = Type("PACIncidentFinding", func() {
	Description("A single non-compliance finding submitted as part of an incident report (DCS-IR-PACM-04)")

	Attribute("risk_type", String, "Machine-readable risk class (e.g. MISSING_APPROVAL)")
	Attribute("detail", String, "Human-readable description of the finding")

	Required("risk_type", "detail")
})

var PACMonitorResponse = Type("PACMonitorResponse", func() {
	Description("Continuous-monitoring snapshot of policy adherence (DCS-IR-PACM-03)")

	Attribute("checked_at", String, "When the monitoring sweep ran (RFC3339)")
	Attribute("risks", ArrayOfRequired(PACComplianceRisk), "Detected compliance risks; empty when all monitored workflows adhere")

	Required("checked_at", "risks")
})

// PACCheckpointHead is the publishable head of an audit-trail checkpoint
// (ADR-16): hashes, counts and a trusted timestamp only — nothing derived from
// the entries it commits to, so it is safe to hand to an external notary such
// as an ORCE flow.
var PACCheckpointHead = Type("PACCheckpointHead", func() {
	Description("Publishable head of an audit-trail Merkle checkpoint")

	Attribute("seq", Int64, "Checkpoint sequence number")
	Attribute("root", String, "Merkle root over the batch this checkpoint commits to")
	Attribute("prev_root", String, "Root of the preceding checkpoint; chaining makes one published root commit to the whole prefix")
	Attribute("leaf_count", Int, "Number of audit entries committed to")
	Attribute("created_at", String, "When the checkpoint was anchored (RFC3339)")
	Attribute("tsa_timestamp", String, "RFC 3161 timestamp token over the root; absent while the TSA has not answered yet")
	Attribute("timestamped_at", String, "When the timestamp was obtained (RFC3339)")

	Required("seq", "root", "leaf_count", "created_at")
})

// PACCheckpointProof is evidence that one audit entry is committed to by a
// timestamped root.
var PACCheckpointProof = Type("PACCheckpointProof", func() {
	Description("Merkle inclusion proof tying one audit entry to a checkpoint")

	Attribute("entry_cid", String, "IPFS CID of the audit entry the proof is for")
	Attribute("leaf_hash", String, "Blinded leaf hash of that entry")
	Attribute("leaf_index", Int, "Position of the leaf in the checkpoint")
	Attribute("siblings", ArrayOf(String), "Sibling hashes from the leaf up to the root")
	Attribute("head", PACCheckpointHead, "Head of the checkpoint the entry belongs to")

	Required("entry_cid", "leaf_hash", "leaf_index", "siblings", "head")
})

var PACWorkflowGateReview = Type("PACWorkflowGateReview", func() {
	Attribute("status", String, "PENDING or DECIDED")
	Attribute("decision", String, "approve or reject")
	Attribute("justification", String, "Compliance Officer justification")
	Attribute("decided_by", String, "Participant DID of the Compliance Officer")
	Attribute("decided_at", String, "Decision timestamp (RFC3339)")
	Required("status")
})

var PACWorkflowGateRun = Type("PACWorkflowGateRun", func() {
	Description("Immutable execution evidence for one contract snapshot and workflow gate")
	Attribute("run_id", String)
	Attribute("correlation_id", String)
	Attribute("snapshot_id", String)
	Attribute("contract_did", String)
	Attribute("contract_version", Int)
	Attribute("gate", String)
	Attribute("status", String, "DISPATCHING, SUCCESS, REVIEW, or BLOCKED")
	Attribute("content_hash", String)
	Attribute("effective_shapes", ArrayOf(String))
	Attribute("profile_version", Int)
	Attribute("review", PACWorkflowGateReview)
	Required("run_id", "correlation_id", "snapshot_id", "contract_did", "contract_version", "gate", "status", "content_hash", "effective_shapes", "profile_version")
})

var PACWorkflowGateReviewDecision = Type("PACWorkflowGateReviewDecision", func() {
	Attribute("run_id", String)
	Attribute("decision", String)
	Attribute("justification", String)
	Attribute("decided_by", String)
	Attribute("decided_at", String)
	Required("run_id", "decision", "justification", "decided_by", "decided_at")
})

// Process Audit & Compliance Management Service  (/pac/...)
var _ = Service("ProcessAuditAndCompliance", func() {
	Description("Process Audit & Compliance Management APIs (/pac/...)")

	Method("audit", func() {
		Description("trigger an audit on selected scope.")
		Meta("dcs:requirements", "DCS-IR-PACM-01")

		Security(JWTAuth, func() {
			Scope("Auditor")
			Scope("Archive Manager")
		})

		Payload(PACAuditRequest)
		Result(PACExternalAuditResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("forbidden", ErrorResult, "Forbidden")
		Error("internal_error", ErrorResult, "Internal server error")
		Error("executor_error", ErrorResult, "External audit executor infrastructure error")

		HTTP(func() {
			POST("/pac/audit")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("forbidden", StatusForbidden)
			Response("internal_error", StatusInternalServerError)
			Response("executor_error", StatusBadGateway)
		})
	})

	Method("audit_report", func() {
		Description("generate and retrieve audit reports.")
		Meta("dcs:requirements", "DCS-IR-PACM-02")
		Security(JWTAuth, func() {
			Scope("Auditor")
			Scope("Archive Manager")
		})
		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("scope", String, "Scope that should be reported")
			Attribute("format", String, "Report format: json, csv, or pdf")
			Attribute("did", String, "Optional resource DID filter")
			Attribute("justification", String, "Required report justification", func() { MinLength(1) })
			Required("justification")
		})
		Error("forbidden", ErrorResult, "Forbidden")
		Error("bad_request", ErrorResult, "Bad request")
		Error("not_found", ErrorResult, "No matching persisted audit run")
		Error("internal_error", ErrorResult, "Internal server error")
		Result(Bytes)
		HTTP(func() {
			GET("/pac/report")
			Param("scope")
			Param("format")
			Param("did")
			Param("justification")
			Response(StatusOK)
			Response("forbidden", StatusForbidden)
			Response("bad_request", StatusBadRequest)
			Response("not_found", StatusNotFound)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("monitor", func() {
		Description("Continuous compliance monitoring sweep: flags contracts pending approval that still have OPEN approval tasks (a missing required approval) and records the sweep in the audit trail. Technical/contractual conformance only — autonomous legal-conformity assessment (DCS-FR-PACM-03) is descoped per the client decision of 2026-07-26 (ADR-24).")
		Meta("dcs:requirements", "DCS-IR-PACM-03")
		Security(JWTAuth, func() {
			Scope("Compliance Officer")
		})
		Payload(func() {
			Token("token", String, "JWT token")
		})
		Result(PACMonitorResponse)

		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/pac/monitor")
			Response(StatusOK)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("incident_report", func() {
		Description("submit non-compliance findings as case records.")
		Meta("dcs:requirements", "DCS-IR-PACM-04")
		Security(JWTAuth, func() {
			Scope("Compliance Officer")
		})
		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("contract_did", String, "Contract DID the findings are linked to")
			Attribute("template_did", String, "Template DID the findings are linked to")
			Attribute("findings", ArrayOf(PACIncidentFinding), "Non-compliance findings raised by this report")
		})

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/pac/report")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
		Result(Any)
	})

	Method("checkpoint_head", func() {
		Description("retrieve the newest audit-trail checkpoint head (ADR-16). Contains only hashes, counts and a trusted timestamp, so the response may be published onward — an external notary that stores one head pins the whole log before it, because every root chains to its predecessor.")
		Meta("dcs:requirements", "DCS-IR-PACM-01")

		Security(JWTAuth, func() {
			Scope("Auditor")
			Scope("Archive Manager")
			// The System User class an external notary authenticates as: it may
			// read this head and nothing else (ADR-16).
			Scope("Sys. Auditor")
		})

		Payload(func() {
			Token("token", String, "JWT token")
		})
		Result(PACCheckpointHead)

		Error("not_found", ErrorResult, "Nothing anchored yet")
		Error("forbidden", ErrorResult, "Forbidden")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/pac/audit/checkpoint/head")
			Response(StatusOK)
			Response("not_found", StatusNotFound)
			Response("forbidden", StatusForbidden)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("checkpoint_by_sequence", func() {
		Description("retrieve one public audit-trail checkpoint projection by sequence for strictly ordered external publication")
		Meta("dcs:requirements", "DCS-FR-PACM-02")
		Security(JWTAuth, func() {
			Scope("Auditor")
			Scope("Archive Manager")
			Scope("Sys. Auditor")
		})
		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("seq", Int64, "Checkpoint sequence number", func() { Minimum(1) })
			Required("seq")
		})
		Result(PACCheckpointHead)
		Error("not_found", ErrorResult)
		Error("forbidden", ErrorResult)
		Error("internal_error", ErrorResult)
		HTTP(func() {
			GET("/pac/audit/checkpoint/{seq}")
			Response(StatusOK)
			Response("not_found", StatusNotFound)
			Response("forbidden", StatusForbidden)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("checkpoint_proof", func() {
		Description("retrieve the inclusion proof tying one anchored audit entry to a timestamped checkpoint root (ADR-16). The entry bytes themselves are NOT part of this response; a verifier hashes the entry it already holds, nonce included, walks the siblings and compares against a root obtained from the external anchor.")
		Meta("dcs:requirements", "DCS-IR-PACM-01")

		Security(JWTAuth, func() {
			Scope("Auditor")
		})

		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("entry_cid", String, "IPFS CID of the audit entry")
			Required("entry_cid")
		})
		Result(PACCheckpointProof)

		Error("not_found", ErrorResult, "No checkpoint commits to that entry")
		Error("forbidden", ErrorResult, "Forbidden")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/pac/audit/checkpoint/proof/{entry_cid}")
			Response(StatusOK)
			Response("not_found", StatusNotFound)
			Response("forbidden", StatusForbidden)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("workflow_gate_run", func() {
		Description("read a persisted workflow-gate run and its manual-review state")
		Meta("dcs:requirements", "DCS-FR-PACM-03")
		Security(JWTAuth, func() { Scope("Compliance Officer") })
		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("run_id", String)
			Required("run_id")
		})
		Result(PACWorkflowGateRun)
		Error("not_found", ErrorResult)
		Error("internal_error", ErrorResult)
		HTTP(func() {
			GET("/pac/workflow-gates/{run_id}")
			Response(StatusOK)
			Response("not_found", StatusNotFound)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("workflow_gate_review", func() {
		Description("record a Compliance Officer decision for a REVIEW workflow-gate run")
		Meta("dcs:requirements", "DCS-FR-PACM-03")
		Security(JWTAuth, func() { Scope("Compliance Officer") })
		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("run_id", String)
			Attribute("decision", String, func() { Enum("approve", "reject") })
			Attribute("justification", String, func() { MinLength(1) })
			Required("run_id", "decision", "justification")
		})
		Result(PACWorkflowGateReviewDecision)
		Error("bad_request", ErrorResult)
		Error("not_found", ErrorResult)
		Error("internal_error", ErrorResult)
		HTTP(func() {
			POST("/pac/workflow-gates/review")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("not_found", StatusNotFound)
			Response("internal_error", StatusInternalServerError)
		})
	})
})
