package design

import (
	. "goa.design/goa/v3/dsl"
)

var ContractCreateRequest = Type("ContractCreateRequest", func() {
	Description("Contract create request")

	Token("token", String, "JWT token")

	Attribute("template_did", String, "The did of the contract template, that is to use to create a new contract")

	Attribute("counterparty", String, "The single peer DCS (a did:web) this contract is offered to and negotiated with (ADR-13). Together with the origin it forms the two parties: the PDF ship target and the signature-field slots. Reviewer/approver/negotiator are internal RBAC roles, isolated per instance — never peer DIDs.")
	Attribute("parties", ArrayOf(String), "Organizations authorized to read this contract (legal names, matched against the OID4VP organization claim; stored as dcs:parties). Read authorization only — the contract's ODRL rule parties are bound from workflow evidence: the originator at creation via originator_role, the counterparty when signing completes.")
	Attribute("originator_role", String, "The contractual role the creating organization declares for itself (e.g. provider, customer); binds the origin DID to that role's party node in the contract's ODRL rules. The counterpart role stays open until the counterparty accepts by signing.")

	Required("template_did")
})

var ContractCreateResponse = Type("ContractCreateResponse", func() {
	Description("Result for creating a contract")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Required("did")
})

var ContractUpdateRequest = Type("ContractUpdateRequest", func() {
	Description("Contract update request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Attribute("updated_at", String, "The timestamp when the contract was updated")

	Attribute("start_date", String, "The timestamp when the contract starts")
	Attribute("exp_date", String, "The timestamp when the contract expired")
	Attribute("exp_policy", String, "The policy what should happen if the contract is expired")
	Attribute("exp_notice_period", Int, "The notice period before contract expiration (in days)")

	Attribute("name", String, "The name of the contract")
	Attribute("description", String, "A description for that")
	Attribute("contract_data", Any, "The data of the contract")

	Required("did", "updated_at")
})

var ContractUpdateResponse = Type("ContractUpdateResponse", func() {
	Description("Result for updating a contract")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Required("did")
})

var ContractSubmitRequest = Type("ContractSubmitRequest", func() {
	Description("Contract submit request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Attribute("updated_at", String, "The timestamp when the contract was updated")

	Attribute("forward_to", String, "Action flag: approval | reject")
	Attribute("comments", ArrayOf(String), "Optional comments")
	Attribute("contract_data", Any, "Optional updated contract data to persist atomically before submit validation")

	Required("did", "updated_at")
})

var ContractSubmitResponse = Type("ContractSubmitResponse", func() {
	Description("Result for submitting a contract")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Attribute("current_state", String, "The current state of the contract")

	Required("did", "current_state")
})

var ContractOfferRequest = Type("ContractOfferRequest", func() {
	Description("Contract offer request: first transmission of a draft contract to the counterparty (DRAFT -> OFFERED)")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Attribute("updated_at", String, "The timestamp when the contract was updated")

	Required("did", "updated_at")
})

var ContractOfferResponse = Type("ContractOfferResponse", func() {
	Description("Result for offering a contract")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Required("did")
})

var ContractWithdrawRequest = Type("ContractWithdrawRequest", func() {
	Description("Contract withdraw request: initiator retracts the contract before it has been approved")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Attribute("updated_at", String, "The timestamp when the contract was updated")

	Required("did", "updated_at")
})

var ContractWithdrawResponse = Type("ContractWithdrawResponse", func() {
	Description("Result for withdrawing a contract")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Required("did")
})

var ContractHistoryRetrieveByIDRequest = Type("ContractHistoryRetrieveByIDRequest", func() {
	Description("Contract history retrieve request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Required("did")
})

var ContractHistoryRetrieveByIDResponse = Type("ContractHistoryRetrieveByIDResponse", func() {
	Attribute("did", String, "DID of the contract")
	Attribute("contract_version", Int, "The version of the contract")
	Attribute("state", String, "Current state of the contract")
	Attribute("name", String, "The name of the contract")
	Attribute("description", String, "The description of the contract")
	Attribute("created_by", String, "Identifier of who created the contract negotiation")
	Attribute("created_at", String, "Created at")
	Attribute("updated_at", String, "Updated at")
	Attribute("template_did", Any, "The DID of the used template")
	Attribute("template_version", Any, "The version of the used template")
	Attribute("start_date", String, "The timestamp when the contract starts")
	Attribute("exp_date", String, "The timestamp when the contract expired")
	Attribute("exp_policy", String, "The policy what should happen if the contract is expired")
	Attribute("exp_notice_period", Int, "The notice period before contract expiration (in days)")
	Attribute("responsible", Any, "Responsible for this contract, including the creator, approvers, reviewers, and negotiators")
	Attribute("contract_data", Any, "The data of that contract")

	Required("did", "state", "created_by", "created_at", "updated_at", "contract_version", "template_did", "template_version")
})

var ContractRetrieveRequest = Type("ContractRetrieveRequest", func() {
	Description("Contract retrieve request")

	Token("token", String, "JWT token")

	Attribute("offset", Int, "Start index of results")
	Attribute("limit", Int, "Page size of results")

	Attribute("parent_did", String, "Full-scope hierarchy filter: return only contracts whose dcs:parentContract references this DID (DCS-FR-CWE-29)")
})

var ContractItem = Type("ContractItem", func() {
	Attribute("did", String, "DID of the contract")
	Attribute("contract_version", Int, "The version of the contract")
	Attribute("state", String, "Current state of the contract")
	Attribute("name", String, "The name of the contract")
	Attribute("description", String, "The description of the contract")
	Attribute("created_by", String, "Identifier of who created the contract negotiation")
	Attribute("created_at", String, "Created at")
	Attribute("updated_at", String, "Updated at")
	Attribute("template_did", Any, "The DID of the used template")
	Attribute("template_version", Any, "The version of the used template")
	Attribute("start_date", String, "The timestamp when the contract starts")
	Attribute("exp_date", String, "The timestamp when the contract expired")
	Attribute("exp_policy", String, "The policy what should happen if the contract is expired")
	Attribute("exp_notice_period", Int, "The notice period before contract expiration (in days)")
	Attribute("responsible", Any, "Responsible for this contract, including the creator, approvers, reviewers, and negotiators")
	Attribute("latest_template_did", String, "The DID of the latest template for this contract")
	Attribute("template_is_deprecated", Boolean, "Whether the template is deprecated")
	Attribute("parent_contract_did", String, "The DID of the parent contract, if this is a sub-contract")
	Attribute("evidence", Any, "Archive evidence blob (only populated for archived contracts), including a deployment sub-object with correlation_id/payload_hash/receipt_hash/tsa_token/activated_at (DCS-FR-SM-10, DCS-FR-SM-12)")
	Attribute("archive_summary", String, "Archive annotation summary (only populated for archived contracts; DCS-FR-CSA-11)")
	Attribute("archive_tags", ArrayOf(String), "Archive annotation tags (only populated for archived contracts; DCS-FR-CSA-11)")

	Required("did", "state", "created_by", "created_at", "updated_at", "contract_version", "template_did", "template_version")
})

var ContractReviewTaskItem = Type("ContractReviewTaskItem", func() {
	Attribute("did", String, "DID of the contract")
	Attribute("contract_version", Int, "The version of the contract")
	Attribute("state", String, "State of the review task")
	Attribute("reviewer", String, "The reviewer of the contract")
	Attribute("created_at", String, "Created at")

	Required("did", "state", "reviewer", "created_at", "contract_version")
})

var ContractApprovalTaskItem = Type("ContractApprovalTaskItem", func() {
	Attribute("did", String, "DID of the contract ")
	Attribute("contract_version", Int, "The version of the contract")
	Attribute("state", String, "State of the approval task")
	Attribute("approver", String, "The approver for the contract")
	Attribute("created_at", String, "Created at")

	Required("did", "state", "approver", "created_at", "contract_version")
})

var ContractNegotiationTaskItem = Type("ContractNegotiationTaskItem", func() {
	Attribute("did", String, "DID of the contract ")
	Attribute("contract_version", Int, "The version of the contract")
	Attribute("state", String, "State of the approval task")
	Attribute("negotiator", String, "The negotiator for the contract")
	Attribute("created_at", String, "Created at")

	Required("did", "state", "negotiator", "created_at", "contract_version")
})

var ContractRetrieveResponse = Type("ContractRetrieveResponse", func() {
	Description("Result for retrieving a contract by id")

	Attribute("contracts", ArrayOf(ContractItem), "A list of contracts")
	Attribute("review_tasks", ArrayOf(ContractReviewTaskItem), "A list of review tasks")
	Attribute("approval_tasks", ArrayOf(ContractApprovalTaskItem), "A list of approval tasks")
	Attribute("negotiation_tasks", ArrayOf(ContractNegotiationTaskItem), "A list of negotiation tasks")

	Required("contracts", "review_tasks", "approval_tasks", "negotiation_tasks")
})

var ContractRetrieveByIDRequest = Type("ContractRetrieveByIDRequest", func() {
	Description("Contract retrieve by id request")

	Token("token", String, "JWT token")

	Attribute("did", String, "DID of the contract")

	Required("did")
})

var ContractNegotiationDecisionItem = Type("ContractNegotiationDecisionItem", func() {

	Attribute("negotiator", String, "Negotiator who has to decide this negotiation decision")
	Attribute("decision", String, "Decision that was taken")
	Attribute("rejection_reason", String, "Reason why it was rejected")

	Required("negotiator")
})

var ContractNegotiationItem = Type("ContractNegotiationItem", func() {
	Attribute("id", String, "id of the negotiation")
	Attribute("change_request", Any, "Change request")
	Attribute("created_by", String, "Identifier of who created the contract negotiation")
	Attribute("created_at", String, "Created at")
	Attribute("contract_version", Int, "Version of the contract for that the negotiation is")

	Attribute("negotiation_decisions", ArrayOf(ContractNegotiationDecisionItem), "List with decisions for that negotiation")

	Required("id", "change_request", "created_by", "created_at", "negotiation_decisions", "contract_version")
})

var ContractRetrieveByIDResponse = Type("ContractRetrieveByIDResponse", func() {
	Attribute("did", String, "DID of the contract")
	Attribute("contract_version", Int, "The version of the contract")
	Attribute("state", String, "Current state of the contract")
	Attribute("extrinsic_lifecycle", String, "Peer-facing negotiation lifecycle inferred from the intrinsic state (proposed/agreed/executed), ADR-13")
	Attribute("name", String, "The name of the contract")
	Attribute("description", String, "The description of the contract")

	Attribute("created_by", String, "Identifier of who created the contract")
	Attribute("created_at", String, "Created at")
	Attribute("updated_at", String, "Updated at")

	Attribute("template_did", Any, "The DID of the used template")
	Attribute("template_version", Any, "The version of the used template")

	Attribute("start_date", String, "The timestamp when the contract starts")
	Attribute("exp_date", String, "The timestamp when the contract expired")
	Attribute("exp_policy", String, "The policy what should happen if the contract is expired")
	Attribute("exp_notice_period", Int, "The notice period before contract expiration (in days)")

	Attribute("responsible", Any, "Responsible for this contract, including the creator, approvers, reviewers, and negotiators")

	Attribute("contract_data", Any, "The data of that contract")

	Attribute("negotiations", ArrayOf(ContractNegotiationItem), "List with negotiations for that contract")

	Attribute("kpis", ArrayOf(ContractDeploymentKPIItem), "KPI values reported via deployment callback for this contract (DCS-FR-CWE-31, DCS-FR-CWE-09)")
	Attribute("kpi_violations", ArrayOf(String), "Metric names whose latest reported value violates its contractual SLA threshold (DCS-FR-CWE-09)")

	Required("did", "state", "created_by", "created_at", "updated_at", "contract_data", "negotiations", "contract_version", "template_did", "template_version")
})

var ContractDeploymentKPIItem = Type("ContractDeploymentKPIItem", func() {
	Description("A single KPI value reported via the deployment callback (DCS-FR-CWE-09, DCS-FR-CWE-31)")

	Attribute("metric", String, "KPI metric name")
	Attribute("value", String, "Reported KPI value")
	Attribute("observed_at", String, "When the KPI was reported")
	Attribute("violation", Boolean, "Whether this KPI value violates its contractual SLA threshold")

	Required("metric", "value", "observed_at")
})

var ContractReviewRequest = Type("ContractReviewRequest", func() {
	Description("Contract review request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Required("did")
})

var ContractReviewResponse = Type("ContractReviewResponse", func() {
	Description("Result for reviewing contract")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Required("did")
})

var ContractSearchRequest = Type("ContractSearchRequest", func() {
	Description("Contract search request")

	Token("token", String, "JWT token")

	Attribute("offset", Int, "Start index of results")
	Attribute("limit", Int, "Page size of results")

	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("contract_version", Int, "The version number of the contract")
	Attribute("state", String, "The state of the contract")
	Attribute("name", String, "The name of the contract")
	Attribute("description", String, "A description for that contract")
	Attribute("contract_data", String, "Search value for full text search in contract data")

	Attribute("parent_did", String, "Full-scope hierarchy filter: return only contracts whose dcs:parentContract references this DID (DCS-FR-CWE-29)")
})

var ContractSearchResponse = Type("ContractSearchResponse", func() {
	Description("Result for searching a contract by filter")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Attribute("contract_version", Int, "The version number of the contract")
	Attribute("state", String, "The state of the contract")
	Attribute("name", String, "The name of the contract")
	Attribute("description", String, "A description for that contract")

	Attribute("start_date", String, "The timestamp when the contract starts")
	Attribute("exp_date", String, "The timestamp when the contract expired")
	Attribute("exp_policy", String, "The policy what should happen if the contract is expired")
	Attribute("exp_notice_period", Int, "The notice period before contract expiration (in days)")

	Attribute("responsible", Any, "Responsible for this contract, including the creator, approver, reviewers, and negotiators")

	Attribute("created_at", String, "The timestamp when the contract template was created")

	Attribute("updated_at", String, "The timestamp when the contract template was updated")

	Required("did", "state", "created_at", "updated_at", "contract_version")
})

var ContractNegotiationRequest = Type("ContractNegotiationRequest", func() {
	Description("Contract negotiation request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Attribute("updated_at", String, "The timestamp when the contract was updated")

	Attribute("negotiated_by", String, "The name of the negotiator")
	Attribute("change_request", Any, "The change request for the negotiation")

	Required("did", "negotiated_by", "change_request", "updated_at")
})

var ContractNegotiationResponse = Type("ContractNegotiationResponse", func() {
	Description("Result for creating a contract negotiation")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Required("did")
})

var ContractNegotiationRespondRequest = Type("ContractNegotiationRespondRequest", func() {
	Description("Contract negotiation decision request")

	Token("token", String, "JWT token")

	Attribute("id", String, "ID of the negotiation")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Attribute("action_flag", String, "Decision for that negotiation (ACCEPTING | REJECTING)")
	Attribute("rejection_reason", String, "The reason for that rejection")

	Required("id", "did", "action_flag")
})

var ContractNegotiationRespondResponse = Type("ContractNegotiationRespondResponse", func() {
	Description("Result for creating a contract negotiation decision")

	Attribute("id", String, "ID of the negotiation")

	Required("id")
})

var ContractApproveRequest = Type("ContractApproveRequest", func() {
	Description("Contract approve request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Attribute("updated_at", String, "The timestamp when the contract was updated")

	Required("did", "updated_at")
})

var ContractApproveResponse = Type("ContractApproveResponse", func() {
	Description("Result for approving a contract")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Required("did")
})

var ContractRejectRequest = Type("ContractRejectRequest", func() {
	Description("Contract reject request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Attribute("updated_at", String, "The timestamp when the contract was updated")

	Attribute("reason", String, "Reason for rejecting the contract")

	Required("did", "updated_at", "reason")
})

var ContractRejectResponse = Type("ContractRejectResponse", func() {
	Description("Result for rejecting a contract")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Required("did")
})

var ContractStoreRequest = Type("ContractStoreRequest", func() {
	Description("Contract store evidence request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("updated_at", String, "Updated at")

	Required("did", "updated_at")
})

var ContractStoreResponse = Type("ContractStoreResponse", func() {
	Description("Result for store evidence")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Required("did")
})

var ContractTerminateRequest = Type("ContractTerminateRequest", func() {
	Description("Contract terminate request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("reason", String, "Reason for terminating contract")
	Attribute("updated_at", String, "Updated at")

	Required("did", "reason", "updated_at")
})

var ContractTerminateResponse = Type("ContractTerminateResponse", func() {
	Description("Result for terminating a contract")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Required("did")
})

var ContractRenewRequest = Type("ContractRenewRequest", func() {
	Description("Contract renew request: create a new linked contract instance from an existing one (DCS-FR-CWE-11/22, DCS-FR-CSA-15). The original contract is not mutated; the new instance starts in DRAFT carrying the original's template reference, metadata, and responsible parties, plus a dcs:renewsContract reference back to the original's DID and version.")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract to renew")
	Attribute("updated_at", String, "The caller's view of the original contract's last update timestamp (optimistic concurrency guard)")

	Attribute("new_start_date", String, "Optional start date for the new renewal term; defaults to the original's start date if omitted")
	Attribute("new_exp_date", String, "Optional expiry date for the new renewal term; defaults to the original's expiry date if omitted")
	Attribute("new_exp_policy", String, "Optional expiry policy for the new renewal term; defaults to the original's expiry policy if omitted")
	Attribute("new_exp_notice_period", Int, "Optional notice period (in days) for the new renewal term; defaults to the original's notice period if omitted")

	Required("did", "updated_at")
})

var ContractRenewResponse = Type("ContractRenewResponse", func() {
	Description("Result for renewing a contract")

	Attribute("did", String, "Decentralized Identifier of the newly created renewal contract")
	Attribute("renews_did", String, "Decentralized Identifier of the original contract this renewal references")
	Attribute("renews_contract_version", Int, "Contract version of the original contract at the time of renewal")

	Required("did", "renews_did", "renews_contract_version")
})

var ContractAuditRequest = Type("ContractAuditRequest", func() {
	Description("Contract audit request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Required("did")
})

var ContractAuditResponse = Type("ContractAuditResponse", func() {
	Description("Result for auditing a contract")

	Attribute("id", Int64, "Identifier for the outbox event")
	Attribute("component", String, "Name of the component")
	Attribute("event_type", String, "Type of the event")
	Attribute("event_data", Any, "Data of the event")
	Attribute("did", String, "Decentralized Identifier of the contract template")
	Attribute("created_at", String, "The creation date of the event")
	Attribute("res_log_pred_cid", String, "Resource audit trail predecessor on the IPFS chain")

	Required("id", "component", "event_type", "event_data", "created_at")
})

var ApprovedContractTemplateRetrieveRequest = Type("ApprovedContractTemplateRetrieveRequest", func() {
	Description("Approved contract template retrieve request")

	Token("token", String, "JWT token")
})

var ApprovedContractTemplateRetrieveResponse = Type("ApprovedContractTemplateRetrieveResponse", func() {
	Attribute("did", String, "DID of the contract template")
	Attribute("document_number", String, "Document number")
	Attribute("version", Int, "Version")
	Attribute("state", String, "State")
	Attribute("template_type", String, "The type of the template")
	Attribute("name", String, "Name")
	Attribute("description", String, "Description")
	Attribute("created_by", String, "Created by")
	Attribute("created_at", String, "Created at")
	Attribute("updated_at", String, "Updated at")
	Attribute("responsible", Any, "Responsible for this contract template, including the creator, approver and reviewers")

	Required("did", "state", "template_type", "created_by", "created_at", "updated_at", "version")
})

var ContractDeployRequest = Type("ContractDeployRequest", func() {
	Description("Contract deploy request: submit a SIGNED contract to the configured Contract Target System (UC-05-01)")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("updated_at", String, "The timestamp when the contract was updated")

	Required("did", "updated_at")
})

var ContractDeployResponse = Type("ContractDeployResponse", func() {
	Description("Result of deploying a contract to the Contract Target System")

	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("contract_version", Int, "The version of the deployed contract")
	Attribute("content_hash", String, "SHA-256 content hash of the deployment payload")
	Attribute("timestamp", String, "When the deployment was dispatched")
	Attribute("correlation_id", String, "Correlation ID for matching the target's later ack/status/KPI callbacks")
	Attribute("payload", Any, "The machine-readable JSON-LD payload sent to the contract target, including the odrl:Set policy")

	Required("did", "contract_version", "content_hash", "timestamp", "correlation_id", "payload")
})

var ContractDeploymentReceiptPayload = Type("ContractDeploymentReceiptPayload", func() {
	Description("Execution-evidence receipt carried in a deployment acknowledgement callback")

	Attribute("correlation_id", String, "Correlation ID of the deployment being acknowledged")
	Attribute("payload_hash", String, "Content hash of the payload the target received and verified")
	Attribute("activated_at", String, "When the target activated the deployed contract")
})

var ContractDeploymentKPIReport = Type("ContractDeploymentKPIReport", func() {
	Description("A single KPI value report carried in a deployment callback")

	Attribute("metric", String, "KPI metric name")
	Attribute("value", String, "Reported KPI value")
})

var ContractDeploymentCallbackRequest = Type("ContractDeploymentCallbackRequest", func() {
	Description("Contract Target System -> DCS deployment callback: ack/status update and/or KPI report, authenticated via a shared-secret header (not a JWT)")

	Attribute("callback_secret", String, "Shared secret proving the caller is the configured contract target")
	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("correlation_id", String, "Correlation ID from the original deployment")
	Attribute("status", String, "Deployment status (e.g. ACKNOWLEDGED)")
	Attribute("receipt", ContractDeploymentReceiptPayload, "Execution-evidence receipt for a deployment acknowledgement")
	Attribute("kpi", ContractDeploymentKPIReport, "A single KPI value report")

	Required("did", "correlation_id")
})

var ContractDeploymentCallbackResponse = Type("ContractDeploymentCallbackResponse", func() {
	Description("Result of accepting a deployment callback")

	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("status", String, "Resulting deployment/contract status")

	Required("did")
})

// Contract Workflow Engine Service  (/contract/...)
var _ = Service("ContractWorkflowEngine", func() {
	Description("Contract Workflow Engine APIs (/contract/...)")

	Method("create", func() {
		Description("initiate a new contract draft from an approved template.")
		Meta("dcs:requirements", "DCS-IR-CWE-01", "DCS-IR-CWE-02")
		Meta("dcs:cwe:components", "Contract Assembling")
		Meta("dcs:ui", "Contract Creation")

		Security(JWTAuth, func() {
			Scope("Contract Creator")
			Scope("Sys. Contract Creator")
		})

		Payload(ContractCreateRequest)
		Result(ContractCreateResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/contract/create")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("update", func() {
		Description("update contract draft before submitting.")
		Meta("dcs:cwe:components", "Contract Assembling")
		Meta("dcs:ui", "Contract Creation")

		Security(JWTAuth, func() {
			Scope("Contract Creator")
			Scope("Sys. Contract Creator")
		})

		Payload(ContractUpdateRequest)
		Result(ContractUpdateResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			PUT("/contract/update")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("submit", func() {
		Description("Overloaded state transition whose effect depends on the contract's current state: DRAFT/REJECTED -> NEGOTIATION; NEGOTIATION -> SUBMITTED once all negotiators have accepted (or stays in NEGOTIATION with contract_version+1 if accepted change requests still need merging); SUBMITTED -> REVIEWED or back to NEGOTIATION depending on action_flag; REVIEWED -> SUBMITTED (re-review). Requires updated_at for optimistic concurrency and is forwarded to the contract's origin peer if the local node is not the origin.")
		Description(`With action flag { forwardTo: "approval" | "reject" } and optional comments. Allows a resubmission path with reviewer/approver comments.`)
		Meta("dcs:requirements", "DCS-IR-CWE-01", "DCS-IR-CWE-03", "DCS-IR-CWE-06", "DCS-IR-CWE-09")
		Meta("dcs:cwe:components", "")
		Meta("dcs:downstream:sm:component", "Signer Authorization & PoA application")
		Meta("dcs:ui", "Contract Creation", "Contract Review", "Contract Approval")

		Security(JWTAuth, func() {
			Scope("Contract Creator")
			Scope("Sys. Contract Creator")
			Scope("Contract Negotiator")
			Scope("Contract Reviewer")
			Scope("Sys. Contract Reviewer")
			// The counterparty drives its inbound contract as Contract Manager
			// (SRS §4); per-contract authorization is the negotiator/party check in
			// the command handler, not local RBAC.
			Scope("Contract Manager")
			Scope("Contract Approver")
			Scope("Sys. Contract Approver")
		})

		Payload(ContractSubmitRequest)
		Result(ContractSubmitResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/contract/submit")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("offer", func() {
		Description("Transmit a draft contract to the counterparty for the first time (DRAFT -> OFFERED, SRS 2.2.6). Triggers the DCS-to-DCS PostSync broadcast. Requires updated_at for optimistic concurrency and is forwarded to the contract's origin peer if the local node is not the origin.")
		Meta("dcs:requirements", "DCS-IR-CWE-01")
		Meta("dcs:cwe:components", "")
		Meta("dcs:ui", "Contract Creation")

		Security(JWTAuth, func() {
			Scope("Contract Creator")
			Scope("Sys. Contract Creator")
		})

		Payload(ContractOfferRequest)
		Result(ContractOfferResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/contract/offer")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("withdraw", func() {
		Description("Initiator retracts a contract before it has been approved (allowed from OFFERED, NEGOTIATION, SUBMITTED, REVIEWED — never once APPROVED). Requires updated_at for optimistic concurrency and is forwarded to the contract's origin peer if the local node is not the origin.")
		Meta("dcs:requirements", "DCS-IR-CWE-01")
		Meta("dcs:cwe:components", "")
		Meta("dcs:ui", "Contract Creation")

		Security(JWTAuth, func() {
			Scope("Contract Creator")
			Scope("Sys. Contract Creator")
		})

		Payload(ContractWithdrawRequest)
		Result(ContractWithdrawResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/contract/withdraw")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("negotiate", func() {
		Description("propose changes.")
		Meta("dcs:requirements", "DCS-IR-CWE-03")
		Meta("dcs:cwe:components", "Contract Assembling", "Contract Versioning")
		Meta("dcs:ui", "Contract Negotiation")

		Security(JWTAuth, func() {
			Scope("Contract Creator")
			Scope("Sys. Contract Creator")
			Scope("Contract Negotiator")
			Scope("Contract Reviewer")
			Scope("Sys. Contract Reviewer")
			// The Responder negotiates an inbound offer through the role that
			// manages its received contracts (SRS §4 Contract Negotiation &
			// Review); per-contract authorization for an inbound offer is the
			// counterparty gate in command/negotiate.go, not local RBAC.
			Scope("Contract Manager")
		})

		Payload(ContractNegotiationRequest)
		Result(ContractNegotiationResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/contract/negotiate")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("respond", func() {
		Description("Accept or reject a specific negotiation change request (action_flag: ACCEPTING | REJECTING). Forwarded to the contract's origin peer if the local node is not the origin. Unlike most other state-mutating contract endpoints, this one does not require updated_at and is therefore not covered by the optimistic-concurrency timestamp check.")
		Meta("dcs:requirements", "DCS-IR-CWE-03", "DCS-IR-CWE-05", "DCS-IR-CWE-06")
		Meta("dcs:cwe:components", "Contract Versioning")
		Meta("dcs:ui", "Contract Creator", "Contract Review")

		Security(JWTAuth, func() {
			Scope("Contract Creator")
			Scope("Sys. Contract Creator")
			Scope("Contract Negotiator")
			Scope("Contract Reviewer")
			Scope("Sys. Contract Reviewer")
			// The counterparty drives its inbound contract as Contract Manager
			// (SRS §4); per-contract authorization is the negotiator/party check in
			// the command handler, not local RBAC.
			Scope("Contract Manager")
		})

		Payload(ContractNegotiationRespondRequest)
		Result(ContractNegotiationRespondResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/contract/respond")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("review", func() {
		Description("Record that the contract's latest draft was opened for review. This does not return contract data (use retrieve_by_id for that) — it only logs a review-tracking event into the audit trail, as a write side effect behind a GET request.")
		Meta("dcs:requirements", "DCS-IR-CWE-04")
		Meta("dcs:cwe:components", "Contract Versioning")
		Meta("dcs:ui", "Contract Negotiation", "Contract Review")

		Security(JWTAuth, func() {
			Scope("Contract Creator")
			Scope("Sys. Contract Creator")
			Scope("Contract Negotiator")
			Scope("Contract Reviewer")
			Scope("Sys. Contract Reviewer")
		})

		Payload(ContractReviewRequest)
		Result(ContractReviewResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/contract/review")
			Param("did")

			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	// GET /contract/retrieve
	Method("retrieve", func() {
		Description("fetch contracts and their review, approval, and negotiation tasks")
		Meta("dcs:cwe:components", "")
		Meta("dcs:ui", "Contract Negotiation", "Contract Review", "Contract Approval", "Contract Management Dashboard")

		Security(JWTAuth, func() {
			Scope("Contract Creator")
			Scope("Sys. Contract Creator")
			Scope("Contract Negotiator")
			Scope("Contract Reviewer")
			Scope("Sys. Contract Reviewer")
			Scope("Contract Approver")
			Scope("Sys. Contract Approver")
			Scope("Contract Manager")
			Scope("Sys. Contract Manager")
			Scope("Contract Observer")
		})

		Payload(ContractRetrieveRequest)
		Result(ContractRetrieveResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/contract/retrieve")
			Param("offset")
			Param("limit")
			Param("parent_did")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	// GET /contract/kpis/{did}
	Method("kpi_observations", func() {
		Description("The KPI values reported for a deployed contract as a JSON-LD observation set: dcs:KPIObservation nodes anchored to the Semantic Hub context, machine-readable alongside the human-facing kpis field of retrieve (DCS-FR-CWE-09/-31).")
		Meta("dcs:requirements", "DCS-FR-CWE-09", "DCS-FR-CWE-31")
		Meta("dcs:ui", "Contract Management Dashboard")

		Security(JWTAuth, func() {
			Scope("Contract Creator")
			Scope("Sys. Contract Creator")
			Scope("Contract Negotiator")
			Scope("Contract Reviewer")
			Scope("Sys. Contract Reviewer")
			Scope("Contract Approver")
			Scope("Sys. Contract Approver")
			Scope("Contract Manager")
			Scope("Sys. Contract Manager")
			Scope("Contract Observer")
		})

		Payload(ContractRetrieveByIDRequest)
		Result(Any)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/contract/kpis/{did}")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	// GET /contract/{did} — the contract's resource IRI
	Method("resolve", func() {
		Description("Dereference a contract's resource IRI ({DCS_PUBLIC_URL}/contract/{did}): serves the canonical JSON-LD contract document under the same party read authorization as retrieve_by_id. This route is what makes a contract's @id follow-your-nose resolvable.")
		Meta("dcs:requirements", "DCS-FR-CWE-02")

		Security(JWTAuth, func() {
			Scope("Contract Creator")
			Scope("Sys. Contract Creator")
			Scope("Contract Negotiator")
			Scope("Contract Reviewer")
			Scope("Sys. Contract Reviewer")
			Scope("Contract Approver")
			Scope("Sys. Contract Approver")
			Scope("Contract Manager")
			Scope("Sys. Contract Manager")
			Scope("Contract Observer")
		})

		Payload(ContractRetrieveByIDRequest)
		Result(Any)

		Error("bad_request", ErrorResult, "Bad request")
		Error("forbidden", ErrorResult, "Caller is not an authorized party of this contract")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/contract/{did}")
			Param("did")

			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("forbidden", StatusForbidden)
			Response("internal_error", StatusInternalServerError)
		})
	})

	// GET /contract/retrieve/{did}
	Method("retrieve_by_id", func() {
		Description("fetch submitted contract. fetch reviewed contract. fetch contract(s).")
		Meta("dcs:requirements", "DCS-IR-CWE-05", "DCS-IR-CWE-08", "DCS-IR-CWE-11", "DCS-IR-CWE-13")
		Meta("dcs:cwe:components", "")
		Meta("dcs:downstream:sm:component", "Signer Authorization & PoA application")
		Meta("dcs:ui", "Contract Negotiation", "Contract Review", "Contract Approval", "Contract Management Dashboard")

		Security(JWTAuth, func() {
			Scope("Contract Creator")
			Scope("Sys. Contract Creator")
			Scope("Contract Negotiator")
			Scope("Contract Reviewer")
			Scope("Sys. Contract Reviewer")
			Scope("Contract Approver")
			Scope("Sys. Contract Approver")
			Scope("Contract Manager")
			Scope("Sys. Contract Manager")
			Scope("Contract Observer")
		})

		Payload(ContractRetrieveByIDRequest)
		Result(ContractRetrieveByIDResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("forbidden", ErrorResult, "Caller is not an authorized party of this contract")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/contract/retrieve/{did}")
			Param("did")

			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("forbidden", StatusForbidden)
			Response("internal_error", StatusInternalServerError)
		})
	})

	// GET /contract/history/{did}
	Method("retrieve_history_by_id", func() {
		Description("fetch history of a contract")
		Meta("dcs:cwe:components", "")

		Security(JWTAuth, func() {
			Scope("Contract Creator")
			Scope("Sys. Contract Creator")
			Scope("Contract Negotiator")
			Scope("Contract Reviewer")
			Scope("Sys. Contract Reviewer")
			Scope("Contract Approver")
			Scope("Sys. Contract Approver")
			Scope("Contract Manager")
			Scope("Sys. Contract Manager")
			Scope("Contract Observer")
		})

		Payload(ContractHistoryRetrieveByIDRequest)
		Result(ArrayOfRequired(ContractHistoryRetrieveByIDResponse))

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/contract/history/{did}")
			Param("did")

			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("search", func() {
		Description("locate contracts by data or state. filter/search across lifecycle states.")
		Meta("dcs:requirements", "DCS-IR-CWE-07", "DCS-IR-CWE-11")
		Meta("dcs:cwe:components", "")
		Meta("dcs:ui", "Contract Review", "Contract Management Dashboard")

		Security(JWTAuth, func() {
			Scope("Contract Creator")
			Scope("Sys. Contract Creator")
			Scope("Contract Negotiator")
			Scope("Contract Reviewer")
			Scope("Sys. Contract Reviewer")
			Scope("Contract Approver")
			Scope("Sys. Contract Approver")
			Scope("Contract Manager")
			Scope("Sys. Contract Manager")
		})

		Payload(ContractSearchRequest)
		Result(ArrayOfRequired(ContractSearchResponse))

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/contract/search")
			Param("offset")
			Param("limit")

			Param("did")
			Param("contract_version")
			Param("state")
			Param("name")
			Param("description")
			Param("contract_data")
			Param("parent_did")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("approve", func() {
		Description("approve and forward contract.")
		Meta("dcs:requirements", "DCS-IR-CWE-09", "DCS-IR-CWE-10")
		Meta("dcs:cwe:components", "Contract Deployment for Service Provisioning")
		Meta("dcs:downstream:sm:component", "Signer Authorization & PoA application")
		Meta("dcs:ui", "Contract Approval")

		Security(JWTAuth, func() {
			Scope("Contract Approver")
			Scope("Sys. Contract Approver")
		})

		Payload(ContractApproveRequest)
		Result(ContractApproveResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/contract/approve")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("reject", func() {
		Description("reject with explanation.")
		Meta("dcs:requirements", "DCS-IR-CWE-09")
		Meta("dcs:cwe:components", "")
		Meta("dcs:downstream:sm:component", "Signer Authorization & PoA application")
		Meta("dcs:ui", "Contract Approval")

		Security(JWTAuth, func() {
			Scope("Contract Approver")
			Scope("Sys. Contract Approver")
		})

		Payload(ContractRejectRequest)
		Result(ContractRejectResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/contract/reject")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("store", func() {
		Description("store evidence.")
		Meta("dcs:requirements", "DCS-IR-CWE-12")
		Meta("dcs:cwe:components", "Contract Performance Tracking")
		Meta("dcs:ui", "Contract Management Dashboard")

		Security(JWTAuth, func() {
			Scope("Contract Manager")
			Scope("Sys. Contract Manager")
		})

		Payload(ContractStoreRequest)
		Result(ContractStoreResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/contract/store")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("terminate", func() {
		Description("terminate a contract.")
		Meta("dcs:requirements", "DCS-IR-CWE-12")
		Meta("dcs:cwe:components", "")
		Meta("dcs:ui", "Contract Management Dashboard")

		Security(JWTAuth, func() {
			Scope("Contract Manager")
			Scope("Sys. Contract Manager")
		})

		Payload(ContractTerminateRequest)
		Result(ContractTerminateResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/contract/terminate")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("renew", func() {
		Description("renew a contract: create a new linked contract instance from an existing one, retaining references to the original's DID, version, and signatures (DCS-FR-CWE-11/22, DCS-FR-CSA-15).")
		Meta("dcs:requirements", "DCS-FR-CWE-11", "DCS-FR-CWE-22", "DCS-FR-CSA-15")
		Meta("dcs:cwe:components", "")
		Meta("dcs:ui", "Contract Management Dashboard")

		Security(JWTAuth, func() {
			Scope("Contract Manager")
			Scope("Sys. Contract Manager")
		})

		Payload(ContractRenewRequest)
		Result(ContractRenewResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/contract/renew")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("audit", func() {
		Description("retrieve the audit trail (event log and policy trail) for a contract.")
		Meta("dcs:requirements", "DCS-IR-CWE-12", "DCS-IR-CWE-13")
		Meta("dcs:cwe:components", "")
		Meta("dcs:ui", "Contract Management Dashboard")

		Security(JWTAuth, func() {
			Scope("Auditor")
			Scope("Compliance Officer")
		})

		Payload(ContractAuditRequest)
		Result(ArrayOfRequired(ContractAuditResponse))

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/contract/audit")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	// GET /contract/templates
	Method("retrieve_templates", func() {
		Description("fetch approved templates")

		Security(JWTAuth, func() {
			Scope("Contract Creator")
		})

		Payload(ApprovedContractTemplateRetrieveRequest)
		Result(ArrayOf(ApprovedContractTemplateRetrieveResponse))

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/contract/templates")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("deploy", func() {
		Description("Deploy a SIGNED contract to the configured Contract Target System (UC-05-01).")
		Meta("dcs:requirements", "DCS-FR-SM-12")
		Meta("dcs:cwe:components", "Contract Deployment for Service Provisioning")
		Meta("dcs:ui", "Contract Management Dashboard")

		Security(JWTAuth, func() {
			Scope("Contract Manager")
			Scope("Sys. Contract Manager")
		})

		Payload(ContractDeployRequest)
		Result(ContractDeployResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/contract/deploy")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("deploymentCallback", func() {
		Description("Accept a deployment ack/status update and/or KPI report from the configured Contract Target System, authenticated by a shared-secret header (DCS-IR-SI-05).")
		Meta("dcs:requirements", "DCS-IR-SI-05")

		NoSecurity()

		Payload(ContractDeploymentCallbackRequest)
		Result(ContractDeploymentCallbackResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("unauthorized", ErrorResult, "Missing or invalid shared secret")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/contract/deployment/callback")
			Header("callback_secret:X-Deployment-Callback-Secret")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("unauthorized", StatusUnauthorized)
			Response("internal_error", StatusInternalServerError)
		})
	})
})
