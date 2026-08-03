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
	Attribute("target_id", String, "Registered target system this contract deploys to (ADR-25); absent when none is designated")
	Attribute("target_name", String, "Name of that target system, so the destination is readable without a second lookup")
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

var ContractNegotiationSupersessionItem = Type("ContractNegotiationSupersessionItem", func() {
	Attribute("superseded_by", String, "id of the later accepted change request whose values the merge kept instead")
	Attribute("fields", ArrayOf(String), "Change request fields whose values did not reach the merged contract version")

	Required("superseded_by", "fields")
})

var ContractNegotiationItem = Type("ContractNegotiationItem", func() {
	Attribute("id", String, "id of the negotiation")
	Attribute("change_request", Any, "Change request")
	Attribute("created_by", String, "Identifier of who created the contract negotiation")
	Attribute("created_at", String, "Created at")
	Attribute("contract_version", Int, "Version of the contract for that the negotiation is")

	Attribute("negotiation_decisions", ArrayOf(ContractNegotiationDecisionItem), "List with decisions for that negotiation")
	Attribute("superseded", ArrayOf(ContractNegotiationSupersessionItem), "Set when this change request was accepted but a later accepted request overwrote it, so the merged contract version does not carry its content (last-accepted-wins). Absent when nothing of it was discarded.")

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

	Attribute("kpis", ArrayOf(ContractDeploymentKPIItem), "KPI values reported via deployment callback for this contract, each carrying the verdict the target system reached (DCS-FR-CWE-31, DCS-FR-CWE-09, ADR-33)")

	Attribute("target_id", String, "Registered target system this contract deploys to (ADR-25); absent when none is designated yet")
	Attribute("target_name", String, "Name of that target system, so the destination is readable without a second lookup")

	Required("did", "state", "created_by", "created_at", "updated_at", "contract_data", "negotiations", "contract_version", "template_did", "template_version")
})

var ContractDeploymentKPIItem = Type("ContractDeploymentKPIItem", func() {
	Description("A single KPI observation reported via the deployment callback, with the verdict the target system reached on it (DCS-FR-CWE-09, DCS-FR-CWE-31, ADR-33)")

	Attribute("metric", String, "KPI metric name")
	Attribute("value", String, "Reported KPI value")
	Attribute("observed_at", String, "When the KPI was reported")
	Attribute("verdict", String, "What the target system concluded: satisfied, violated, or not_evaluated. A report that carried no verdict is recorded as not_evaluated, never as satisfied", func() {
		Enum("satisfied", "violated", "not_evaluated")
	})
	Attribute("rule", String, "@id of the ODRL rule the verdict concerns, as it travels in the deployment envelope's odrl:policy; absent when the target named none")

	Required("metric", "value", "observed_at", "verdict")
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

var ContractOfferAcceptRequest = Type("ContractOfferAcceptRequest", func() {
	Description("Accept an inbound offer as-is, with no change request (OFFERED -> NEGOTIATION). Distinct from ContractNegotiationRespondRequest, which decides one already-proposed change request.")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Attribute("updated_at", String, "The timestamp when the contract was updated")

	Attribute("accepted_by", String, "The name of the accepting negotiator")

	Required("did", "updated_at", "accepted_by")
})

var ContractOfferAcceptResponse = Type("ContractOfferAcceptResponse", func() {
	Description("Result for accepting an offer")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Required("did")
})

var ContractNegotiationDraftSaveRequest = Type("ContractNegotiationDraftSaveRequest", func() {
	Description("Save (upsert) the calling party's negotiation draft for a contract")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("change_request", Any, "The staged change request, same shape as the negotiate payload")

	Required("did", "change_request")
})

var ContractNegotiationDraftRetrieveRequest = Type("ContractNegotiationDraftRetrieveRequest", func() {
	Description("Retrieve the calling party's negotiation draft for a contract")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Required("did")
})

var ContractNegotiationDraftResponse = Type("ContractNegotiationDraftResponse", func() {
	Description("The calling party's negotiation draft; change_request is absent when no draft is stored")

	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("change_request", Any, "The staged change request, if a draft exists")
	Attribute("updated_at", String, "When the draft was last saved")

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
	Attribute("target_id", String, "Optional: dispatch to this registered target instead of the one the contract designates (ADR-25). A re-dispatch may be directed elsewhere; the contract's own designation is unchanged.")

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
	Attribute("target_id", String, "Registered target system the contract was dispatched to (ADR-25)")
	Attribute("target_name", String, "Name of that target system, as configured")

	Required("did", "contract_version", "content_hash", "timestamp", "correlation_id", "payload")
})

// ---- Contract target registry (ADR-25, UC-09-01 system configuration) -------

var ContractTarget = Type("ContractTarget", func() {
	Description("A configured Contract Target System deployments may be dispatched to (DCS-IR-SI-05)")

	Attribute("id", String, "Identifier of the registered target")
	Attribute("name", String, "Operator-facing name, unique within this deployment")
	Attribute("url", String, "Endpoint the deployment payload is POSTed to")
	Attribute("description", String, "What this system is, for whoever picks it")
	Attribute("enabled", Boolean, "Disabled targets stay referenceable so a contract naming one keeps a readable destination, but dispatch to them is refused")
	Attribute("created_at", String, "When the target was registered")
	Attribute("updated_at", String, "When the target was last changed")
	Attribute("oauth_client_id", String, "The OAuth2 client this target authenticates its callbacks as. Absent until a credential is issued (ADR-27)")
	Attribute("secret_issued_at", String, "When the current secret was issued. The secret itself is not stored and cannot be read back")

	Required("id", "name", "url", "enabled")
})

// ---- Machine credentials (ADR-27) -------------------------------------------

var MachineCredential = Type("MachineCredential", func() {
	Description("A freshly issued machine credential. The secret is returned by this response and by no other: Hydra keeps only a hash of it, so it cannot be read back and must be rotated if lost")

	Attribute("client_id", String, "OAuth2 client id to authenticate as")
	Attribute("client_secret", String, "The secret, shown this once")
	Attribute("token_url", String, "Token endpoint to present it to, with grant_type=client_credentials")
	Attribute("issued_at", String, "When it was issued")

	Required("client_id", "client_secret")
})

var MachineIdentity = Type("MachineIdentity", func() {
	Description("A registered non-human caller: an SRS Table 5 System User reaching DCS over its API (ADR-27)")

	Attribute("id", String, "Identifier of the registered identity")
	Attribute("name", String, "Operator-facing name, unique within this deployment")
	Attribute("oauth_client_id", String, "The OAuth2 client it authenticates as")
	Attribute("participant_did", String, "Identity its actions are attributed to in the audit trail")
	Attribute("roles", ArrayOf(String), "What it may do. Read from here by client_id and never from the token, so a caller cannot widen its own reach")
	Attribute("description", String, "What this integration is, for whoever inherits it")
	Attribute("enabled", Boolean, "A disabled identity is refused at once, without waiting for its secret to expire")
	Attribute("secret_issued_at", String, "When the current secret was issued")
	Attribute("created_at", String, "When it was registered")
	Attribute("updated_at", String, "When it was last changed")

	Required("id", "name", "oauth_client_id", "participant_did", "roles", "enabled")
})

var MachineIdentityListRequest = Type("MachineIdentityListRequest", func() {
	Description("List the registered machine identities")
	Token("token", String, "JWT token")
})

var MachineIdentityListResponse = Type("MachineIdentityListResponse", func() {
	Description("The registered machine identities")
	Attribute("identities", ArrayOf(MachineIdentity), "Registered identities")
	Required("identities")
})

var MachineIdentityCreateRequest = Type("MachineIdentityCreateRequest", func() {
	Description("Register a machine identity and issue its first credential")
	Token("token", String, "JWT token")

	Attribute("name", String, "Operator-facing name, unique within this deployment")
	Attribute("participant_did", String, "Identity its actions are attributed to")
	Attribute("roles", ArrayOf(String), "What it may do")
	Attribute("description", String, "What this integration is")

	Required("name", "participant_did", "roles")
})

var MachineIdentityCreateResponse = Type("MachineIdentityCreateResponse", func() {
	Description("The registered identity together with its one-time credential")

	Attribute("identity", MachineIdentity, "The registered identity")
	Attribute("credential", MachineCredential, "Its secret, shown this once")

	Required("identity", "credential")
})

var MachineIdentityUpdateRequest = Type("MachineIdentityUpdateRequest", func() {
	Description("Change a registered machine identity. The credential is untouched: rotate it separately")
	Token("token", String, "JWT token")

	Attribute("id", String, "Identifier of the registered identity")
	Attribute("name", String, "Operator-facing name")
	Attribute("participant_did", String, "Identity its actions are attributed to")
	Attribute("roles", ArrayOf(String), "What it may do")
	Attribute("description", String, "What this integration is")
	Attribute("enabled", Boolean, "Whether it may call at all")

	Required("id", "name", "participant_did", "roles", "enabled")
})

var MachineIdentityDeleteRequest = Type("MachineIdentityDeleteRequest", func() {
	Description("Remove a machine identity and the OAuth2 client behind it, so the credential cannot outlive the entry")
	Token("token", String, "JWT token")

	Attribute("id", String, "Identifier of the registered identity")
	Required("id")
})

var MachineIdentityRotateRequest = Type("MachineIdentityRotateRequest", func() {
	Description("Issue a new secret for a registered identity. The previous one stops working immediately")
	Token("token", String, "JWT token")

	Attribute("id", String, "Identifier of the registered identity")
	Required("id")
})

var ContractTargetRotateRequest = Type("ContractTargetRotateRequest", func() {
	Description("Issue a new callback credential for a registered target. The previous one stops working immediately")
	Token("token", String, "JWT token")

	Attribute("id", String, "Identifier of the registered target")
	Required("id")
})

var ContractTargetListRequest = Type("ContractTargetListRequest", func() {
	Description("List the registered Contract Target Systems")
	Token("token", String, "JWT token")
})

var ContractTargetCreateRequest = Type("ContractTargetCreateRequest", func() {
	Description("Register a Contract Target System")

	Token("token", String, "JWT token")

	Attribute("name", String, "Operator-facing name, unique within this deployment")
	Attribute("url", String, "Endpoint the deployment payload is POSTed to")
	Attribute("description", String, "What this system is, for whoever picks it")
	Attribute("enabled", Boolean, "Whether deployments may be dispatched to it")

	Required("name", "url")
})

var ContractTargetUpdateRequest = Type("ContractTargetUpdateRequest", func() {
	Description("Change a registered Contract Target System")

	Token("token", String, "JWT token")

	Attribute("id", String, "Identifier of the registered target")
	Attribute("name", String, "Operator-facing name, unique within this deployment")
	Attribute("url", String, "Endpoint the deployment payload is POSTed to")
	Attribute("description", String, "What this system is, for whoever picks it")
	Attribute("enabled", Boolean, "Whether deployments may be dispatched to it")

	Required("id", "name", "url")
})

var ContractTargetDeleteRequest = Type("ContractTargetDeleteRequest", func() {
	Description("Remove a registered Contract Target System")

	Token("token", String, "JWT token")

	Attribute("id", String, "Identifier of the registered target")

	Required("id")
})

var ContractTargetDesignateRequest = Type("ContractTargetDesignateRequest", func() {
	Description("Designate the target system a contract deploys to (ADR-25)")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("updated_at", String, "The timestamp when the contract was updated")
	Attribute("target_id", String, "Registered target to designate, or empty to clear the designation")

	Required("did", "updated_at")
})

var ContractDeploymentReceiptPayload = Type("ContractDeploymentReceiptPayload", func() {
	Description("Execution-evidence receipt carried in a deployment acknowledgement callback")

	Attribute("correlation_id", String, "Correlation ID of the deployment being acknowledged")
	Attribute("payload_hash", String, "Content hash of the payload the target received and verified")
	Attribute("activated_at", String, "When the target activated the deployed contract")
})

var ContractDeploymentKPIReport = Type("ContractDeploymentKPIReport", func() {
	Description("A single KPI report carried in a deployment callback: what the target system observed, and what it concluded from it (ADR-33)")

	Attribute("metric", String, "KPI metric name")
	Attribute("value", String, "Reported KPI value")
	Attribute("verdict", String, "What the target system concluded about the rule it names: satisfied, violated, or not_evaluated. Absent is recorded as not_evaluated", func() {
		Enum("satisfied", "violated", "not_evaluated")
	})
	Attribute("rule", String, "@id of the ODRL rule this verdict concerns, verbatim from the odrl:policy the deployment carried")
})

var ContractDeploymentCallbackRequest = Type("ContractDeploymentCallbackRequest", func() {
	Description("Contract Target System -> DCS deployment callback: ack/status update and/or KPI report, authenticated as the target's own OAuth2 client (ADR-27)")

	Token("token", String, "JWT token issued to the target system")
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

	Method("accept_offer", func() {
		Description("Accept an inbound offer as-is, with no change request: mints this instance's negotiation task for the offer's current round and moves the contract OFFERED -> NEGOTIATION. Only the counterparty may call it — the origin is refused. Not to be confused with respond (action_flag ACCEPTING), which decides one already-proposed change request.")
		Meta("dcs:requirements", "DCS-IR-CWE-03", "DCS-FR-CWE-18")

		Security(JWTAuth, func() {
			Scope("Contract Creator")
			Scope("Sys. Contract Creator")
			Scope("Contract Negotiator")
			// The Responder drives its inbound contracts as Contract Manager
			// (SRS §4); per-contract authorization is the counterparty gate in
			// command/acceptoffer.go, not local RBAC.
			Scope("Contract Manager")
		})

		Payload(ContractOfferAcceptRequest)
		Result(ContractOfferAcceptResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/contract/accept-offer")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	// SRS §3.1.1 Contract Negotiation UI lists "Save draft" among its controls,
	// distinct from "Propose change": a negotiator stages modifications and
	// proposes them later. Drafts are party-scoped — stored per (contract,
	// party), shared by the party's authorized negotiators, never replicated
	// to the peer, never part of the negotiation audit trail; proposing
	// (POST /contract/negotiate) or discarding deletes them.
	Method("save_negotiation_draft", func() {
		Description("save (upsert) the calling party's staged change request for a contract in negotiation; it is shared among this party's authorized negotiators and not transmitted to the counterparty until proposed.")
		Meta("dcs:requirements", "DCS-IR-CWE-03")

		Security(JWTAuth, func() {
			Scope("Contract Creator")
			Scope("Sys. Contract Creator")
			Scope("Contract Negotiator")
			Scope("Contract Reviewer")
			Scope("Sys. Contract Reviewer")
			Scope("Contract Manager")
		})

		Payload(ContractNegotiationDraftSaveRequest)
		Result(ContractNegotiationDraftResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			PUT("/contract/negotiation_draft")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("retrieve_negotiation_draft", func() {
		Description("retrieve the calling party's staged change request for a contract; change_request is absent when no draft is stored.")
		Meta("dcs:requirements", "DCS-IR-CWE-03")

		Security(JWTAuth, func() {
			Scope("Contract Creator")
			Scope("Sys. Contract Creator")
			Scope("Contract Negotiator")
			Scope("Contract Reviewer")
			Scope("Sys. Contract Reviewer")
			Scope("Contract Manager")
		})

		Payload(ContractNegotiationDraftRetrieveRequest)
		Result(ContractNegotiationDraftResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/contract/negotiation_draft/{did}")
			Param("did")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("delete_negotiation_draft", func() {
		Description("discard the calling party's staged change request for a contract.")
		Meta("dcs:requirements", "DCS-IR-CWE-03")

		Security(JWTAuth, func() {
			Scope("Contract Creator")
			Scope("Sys. Contract Creator")
			Scope("Contract Negotiator")
			Scope("Contract Reviewer")
			Scope("Sys. Contract Reviewer")
			Scope("Contract Manager")
		})

		Payload(ContractNegotiationDraftRetrieveRequest)
		Result(ContractNegotiationDraftResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			DELETE("/contract/negotiation_draft/{did}")
			Param("did")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("respond", func() {
		Description("Accept or reject a specific negotiation change request (action_flag: ACCEPTING | REJECTING). Forwarded to the contract's origin peer if the local node is not the origin. Unlike most other state-mutating contract endpoints, this one does not require updated_at and is therefore not covered by the optimistic-concurrency timestamp check.")
		Meta("dcs:requirements", "DCS-IR-CWE-03", "DCS-IR-CWE-05", "DCS-IR-CWE-06")

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
		Description("The KPI reports of a deployed contract as a JSON-LD observation set: dcs:KPIObservation nodes anchored to the Semantic Hub context, each carrying the target system's verdict and the ODRL rule it names, machine-readable alongside the human-facing kpis field of retrieve (DCS-FR-CWE-09/-31, ADR-33).")
		Meta("dcs:requirements", "DCS-FR-CWE-09", "DCS-FR-CWE-31")

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
			// The contract list renders its search box for every role that can
			// open it, and search returns a strict subset of retrieve's fields.
			Scope("Contract Observer")
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

	// ---- Contract target registry (ADR-25) ---------------------------------

	Method("listContractTargets", func() {
		Description("List the registered Contract Target Systems a contract may be deployed to (ADR-25).")
		Meta("dcs:requirements", "DCS-IR-SI-05")

		// Readable by whoever has to pick one, writable only by the roles that
		// configure integrations.
		Security(JWTAuth, func() {
			Scope("Sys. Administrator")
			Scope("Integration Manager")
			Scope("Contract Manager")
			Scope("Sys. Contract Manager")
		})

		Payload(ContractTargetListRequest)
		Result(ArrayOf(ContractTarget))

		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/contract/targets")
			Response(StatusOK)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("createContractTarget", func() {
		Description("Register a Contract Target System (UC-09-01 system configuration).")
		Meta("dcs:requirements", "DCS-IR-SI-05")

		Security(JWTAuth, func() {
			Scope("Sys. Administrator")
			Scope("Integration Manager")
		})

		Payload(ContractTargetCreateRequest)
		Result(ContractTarget)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/contract/targets")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("updateContractTarget", func() {
		Description("Change a registered Contract Target System (UC-09-01 system configuration).")
		Meta("dcs:requirements", "DCS-IR-SI-05")

		Security(JWTAuth, func() {
			Scope("Sys. Administrator")
			Scope("Integration Manager")
		})

		Payload(ContractTargetUpdateRequest)
		Result(ContractTarget)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			PUT("/contract/targets")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("deleteContractTarget", func() {
		Description("Remove a registered Contract Target System. Refused while a contract still designates it, so no contract is left undeployable with no record of where it was meant to go.")
		Meta("dcs:requirements", "DCS-IR-SI-05")

		Security(JWTAuth, func() {
			Scope("Sys. Administrator")
			Scope("Integration Manager")
		})

		Payload(ContractTargetDeleteRequest)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			DELETE("/contract/targets")
			Response(StatusNoContent)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("designateContractTarget", func() {
		Description("Designate the target system a contract deploys to (ADR-25). The automatic trigger on signing completion has no human present to choose one, so the destination belongs to the contract.")
		Meta("dcs:requirements", "DCS-FR-SM-12", "DCS-IR-SI-05")

		Security(JWTAuth, func() {
			Scope("Contract Manager")
			Scope("Sys. Contract Manager")
		})

		Payload(ContractTargetDesignateRequest)
		Result(ContractRetrieveByIDResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/contract/target/designate")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("rotateContractTargetSecret", func() {
		Description("Issue a new callback credential for a registered Contract Target System. The secret is returned once and the previous one stops working immediately (ADR-27).")
		Meta("dcs:requirements", "DCS-IR-SI-05")

		Security(JWTAuth, func() {
			Scope("Sys. Administrator")
			Scope("Integration Manager")
		})

		Payload(ContractTargetRotateRequest)
		Result(MachineCredential)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/contract/targets/{id}/credential")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("listMachineIdentities", func() {
		Description("List the registered machine identities: the SRS Table 5 System Users that reach DCS over its API (ADR-27).")

		Security(JWTAuth, func() {
			Scope("Sys. Administrator")
		})

		Payload(MachineIdentityListRequest)
		Result(MachineIdentityListResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/machine-identities")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("createMachineIdentity", func() {
		Description("Register a machine identity and issue its first credential. The secret is returned once and cannot be read back (ADR-27).")

		Security(JWTAuth, func() {
			Scope("Sys. Administrator")
		})

		Payload(MachineIdentityCreateRequest)
		Result(MachineIdentityCreateResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/machine-identities")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("updateMachineIdentity", func() {
		Description("Change a registered machine identity. Disabling one refuses its calls at once, without waiting for the secret to expire (ADR-27).")

		Security(JWTAuth, func() {
			Scope("Sys. Administrator")
		})

		Payload(MachineIdentityUpdateRequest)
		Result(MachineIdentity)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			PUT("/machine-identities/{id}")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("deleteMachineIdentity", func() {
		Description("Remove a machine identity and the OAuth2 client behind it, so no credential outlives the entry that justified it (ADR-27).")

		Security(JWTAuth, func() {
			Scope("Sys. Administrator")
		})

		Payload(MachineIdentityDeleteRequest)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			DELETE("/machine-identities/{id}")
			Response(StatusNoContent)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("rotateMachineIdentitySecret", func() {
		Description("Issue a new secret for a registered machine identity. It is returned once and the previous one stops working immediately (ADR-27).")

		Security(JWTAuth, func() {
			Scope("Sys. Administrator")
		})

		Payload(MachineIdentityRotateRequest)
		Result(MachineCredential)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/machine-identities/{id}/credential")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("deploymentCallback", func() {
		Description("Accept a deployment ack/status update and/or KPI report from the Contract Target System the deployment was dispatched to, authenticated as that target's own OAuth2 client (DCS-IR-SI-05, ADR-27).")
		Meta("dcs:requirements", "DCS-IR-SI-05")

		Security(JWTAuth, func() {
			Scope("Contract Target System")
		})

		Payload(ContractDeploymentCallbackRequest)
		Result(ContractDeploymentCallbackResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("unauthorized", ErrorResult, "Not the target system this deployment was dispatched to")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/contract/deployment/callback")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("unauthorized", StatusUnauthorized)
			Response("internal_error", StatusInternalServerError)
		})
	})
})
