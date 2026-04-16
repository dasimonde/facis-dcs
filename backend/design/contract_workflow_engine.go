package design

import (
	. "goa.design/goa/v3/dsl"
)

var ContractCreateRequest = Type("ContractCreateRequest", func() {
	Description("Contract create request")

	Token("token", String, "JWT token")

	Attribute("did", String, "The did of the contract template, that is to use to create a new contract")

	Required("did")
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

	Attribute("expiration_date", String, "The timestamp when the contract expired")

	Attribute("contract_version", Int, "The version of the contract")

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

	Attribute("reviewers", ArrayOf(String), "A list of reviewers for that contract")
	Attribute("approver", String, "The approver for that contract")
	Attribute("negotiators", ArrayOf(String), "A list of negotiators for that contract")

	Required("did", "updated_at")
})

var ContractSubmitResponse = Type("ContractSubmitResponse", func() {
	Description("Result for submitting a contract")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Required("did")
})

var ContractRetrieveRequest = Type("ContractRetrieveRequest", func() {
	Description("Contract retrieve request")

	Token("token", String, "JWT token")
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

	Required("did", "state", "created_by", "created_at", "updated_at")
})

var ContractReviewTaskItem = Type("ContractReviewTaskItem", func() {
	Attribute("did", String, "DID of the contract")
	Attribute("contract_version", Int, "The version of the contract")
	Attribute("state", String, "State of the review task")
	Attribute("reviewer", String, "The reviewer of the contract")
	Attribute("created_at", String, "Created at")

	Required("did", "state", "reviewer", "created_at")
})

var ContractApprovalTaskItem = Type("ContractApprovalTaskItem", func() {
	Attribute("did", String, "DID of the contract ")
	Attribute("contract_version", Int, "The version of the contract")
	Attribute("state", String, "State of the approval task")
	Attribute("approver", String, "The approver for the contract")
	Attribute("created_at", String, "Created at")

	Required("did", "state", "approver", "created_at")
})

var ContractNegotiationTaskItem = Type("ContractNegotiationTaskItem", func() {
	Attribute("did", String, "DID of the contract ")
	Attribute("contract_version", Int, "The version of the contract")
	Attribute("state", String, "State of the approval task")
	Attribute("negotiator", String, "The negotiator for the contract")
	Attribute("created_at", String, "Created at")

	Required("did", "state", "negotiator", "created_at")
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

	Attribute("negotiation_decisions", ArrayOf(ContractNegotiationDecisionItem), "List with decisions for that negotiation")

	Required("id", "change_request", "created_by", "created_at", "negotiation_decisions")
})

var ContractRetrieveByIDResponse = Type("ContractRetrieveByIDResponse", func() {
	Attribute("did", String, "DID of the contract")
	Attribute("contract_version", Int, "The version of the contract")
	Attribute("state", String, "Current state of the contract")
	Attribute("name", String, "The name of the contract")
	Attribute("description", String, "The description of the contract")

	Attribute("created_by", String, "Identifier of who created the contract")
	Attribute("created_at", String, "Created at")
	Attribute("updated_at", String, "Updated at")

	Attribute("contract_data", Any, "The data of that contract")

	Attribute("negotiations", ArrayOf(ContractNegotiationItem), "List with negotiations for that contract")

	Required("did", "state", "created_by", "created_at", "updated_at", "contract_data", "negotiations")
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

	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("contract_version", Int, "The version number of the contract")
	Attribute("state", String, "The state of the contract")
	Attribute("name", String, "The name of the contract")
	Attribute("description", String, "A description for that contract")
	Attribute("filter", String, "Search value for full text search in contract data")
})

var ContractSearchResponse = Type("ContractSearchResponse", func() {
	Description("Result for searching a contract by filter")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Attribute("contract_version", Int, "The version number of the contract")
	Attribute("state", String, "The state of the contract")
	Attribute("name", String, "The name of the contract")
	Attribute("description", String, "A description for that contract")

	Attribute("created_at", String, "The timestamp when the contract template was created")

	Attribute("updated_at", String, "The timestamp when the contract template was updated")

	Required("did", "state", "created_at", "updated_at")
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

	Attribute("action_flag", String, "Decision for that negotiation (ACCEPTING | REJECTING)")
	Attribute("responded_by", String, "The user who responded to that negotiation")
	Attribute("RejectionReason", String, "The reason for that rejection")

	Required("id", "action_flag", "responded_by")
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

	Required("did", "reason")
})

var ContractTerminateResponse = Type("ContractTerminateResponse", func() {
	Description("Result for terminating a contract")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Required("did")
})

var ContractAuditRequest = Type("ContractAuditRequest", func() {
	Description("Contract audit request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("updated_at", String, "Updated at")

	Required("did", "updated_at")
})

var ContractAuditResponse = Type("ContractAuditResponse", func() {
	Description("Result for auditing a contract")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Required("did")
})

// Contract Workflow Engine Service  (/contract/...)
var _ = Service("ContractWorkflowEngine", func() {
	Description("Contract Workflow Engine APIs (/contract/...)")

	Method("create", func() {
		Description("initiate new contract draft from.")
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
		Description("finalize and submit contract for negotiation/review. finalize and submit negotiated version. finalize review outcome. finalize decision. finalize review outcome.")
		Description(`with action flag { forwardTo: "approval" | "rejected" } and optional reviewComments. allow resubmission path with approver comments.`)
		Meta("dcs:requirements", "DCS-IR-CWE-01", "DCS-IR-CWE-03", "DCS-IR-CWE-06", "DCS-IR-CWE-09")
		Meta("dcs:cwe:components", "")
		Meta("dcs:downstream:sm:component", "Signer Authorization & PoA application")
		Meta("dcs:ui", "Contract Creation", "Contract Negotiation", "Contract Review", "Contract Approval")

		Security(JWTAuth, func() {
			Scope("Contract Creator")
			Scope("Sys. Contract Creator")
			Scope("Contract Negotiator")
			Scope("Contract Reviewer")
			Scope("Sys. Contract Reviewer")
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

	Method("negotiate", func() {
		Description("propose changes.")
		Meta("dcs:requirements", "DCS-IR-CWE-03")
		Meta("dcs:cwe:components", "Contract Assembling", "Contract Versioning")
		Meta("dcs:ui", "Contract Negotiation")

		Security(JWTAuth, func() {
			Scope("Contract Negotiator")
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
		Description("provide feedback/findings. respond to counterpart changes.")
		Meta("dcs:requirements", "DCS-IR-CWE-03", "DCS-IR-CWE-05", "DCS-IR-CWE-06")
		Meta("dcs:cwe:components", "Contract Versioning")
		Meta("dcs:ui", "Contract Negotiation", "Contract Review")

		Security(JWTAuth, func() {
			Scope("Contract Negotiator")
			Scope("Contract Reviewer")
			Scope("Sys. Contract Reviewer")
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
		Description("retrieve latest draft for comparison.")
		Meta("dcs:requirements", "DCS-IR-CWE-04")
		Meta("dcs:cwe:components", "Contract Versioning")
		Meta("dcs:ui", "Contract Negotiation", "Contract Review")

		Security(JWTAuth, func() {
			Scope("Contract Negotiator")
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
		Description("fetch contracts and review and approval tasks")
		Meta("dcs:cwe:components", "")
		Meta("dcs:ui", "Contract Negotiation", "Contract Review", "Contract Approval", "Contract Management Dashboard")

		Security(JWTAuth, func() {
			Scope("Contract Creator")
			Scope("Contract Reviewer")
			Scope("Contract Approver")
			Scope("Contract Manager")
		})

		Payload(ContractRetrieveRequest)
		Result(ContractRetrieveResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/contract/retrieve")

			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
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
			Scope("Contract Reviewer")
			Scope("Sys. Contract Reviewer")
			Scope("Contract Approver")
			Scope("Sys. Contract Approver")
			Scope("Contract Manager")
			Scope("Sys. Contract Manager")
		})

		Payload(ContractRetrieveByIDRequest)
		Result(ContractRetrieveByIDResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/contract/retrieve/{did}")
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
			Scope("Contract Reviewer")
			Scope("Sys. Contract Reviewer")
			Scope("Contract Manager")
			Scope("Sys. Contract Manager")
		})

		Payload(ContractSearchRequest)
		Result(ArrayOfRequired(ContractSearchResponse))

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/contract/search")
			Param("did")
			Param("contract_version")
			Param("state")
			Param("name")
			Param("description")
			Param("filter")
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

	Method("audit", func() {
		Description("generate audit record.")
		Meta("dcs:requirements", "DCS-IR-CWE-12", "DCS-IR-CWE-13")
		Meta("dcs:cwe:components", "")
		Meta("dcs:ui", "Contract Management Dashboard")

		Security(JWTAuth, func() {
			Scope("Contract Manager")
			Scope("Sys. Contract Manager")
		})

		Payload(ContractAuditRequest)
		Result(ContractAuditResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/contract/audit")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})
})
