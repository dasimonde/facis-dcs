package design

import (
	. "goa.design/goa/v3/dsl"
)

var ContractTemplateCreateRequest = Type("ContractTemplateCreateRequest", func() {
	Description("Contract template create request")

	Token("token", String, "JWT token")

	Attribute("template_type", String, "The type of the template")

	Attribute("name", String, "The name of the contract template")
	Attribute("description", String, "A description for that template")
	Attribute("template_data", Any, "The template data of the contract template")

	Required("template_type")
})

var ContractTemplateCreateResponse = Type("ContractTemplateCreateResponse", func() {
	Description("Result for creating a contract template")

	Attribute("did", String, "Decentralized Identifier of the contract template")

	Required("did")
})

var ContractTemplateSubmitRequest = Type("ContractTemplateSubmitRequest", func() {
	Description("Contract template submit request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract template")

	Attribute("updated_at", String, "The timestamp when the contract template was updated")

	Attribute("reviewers", ArrayOf(String), "A list of reviewers for that contract template")
	Attribute("approver", String, "The approver for that contract template")

	Attribute("forward_to", String, "Action flag: approval | draft")
	Attribute("comments", ArrayOf(String), "Optional comments")

	Required("did", "updated_at")
})

var ContractTemplateSubmitResponse = Type("ContractTemplateSubmitResponse", func() {
	Description("Result for submitting a contract template")

	Attribute("did", String, "Decentralized Identifier of the contract template")

	Required("did")
})

var ContractTemplateUpdateRequest = Type("ContractTemplateUpdateRequest", func() {
	Description("Contract template update request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract template")
	Attribute("updated_at", String, "The timestamp when the contract template was updated")

	Attribute("document_number", String, "The number of the contract template")
	Attribute("version", Int, "The version of the contract template")
	Attribute("template_type", String, "The type of the template")
	Attribute("name", String, "The name of the contract template")
	Attribute("description", String, "A description for that template")
	Attribute("template_data", Any, "The template data of the contract template")

	Required("did", "updated_at")
})

var ContractTemplateUpdateResponse = Type("ContractTemplateUpdateResponse", func() {
	Description("Result for updating a contract template")

	Attribute("did", String, "Decentralized Identifier of the contract template")

	Required("did")
})

var ContractTemplateUpdateManageRequest = Type("ContractTemplateUpdateManageRequest", func() {
	Description("Contract template update manage request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract template")

	Attribute("state", String, "The state of the contract template")
	Attribute("updated_at", String, "The timestamp when the contract template was updated")

	Attribute("document_number", String, "The number of the contract template")
	Attribute("version", Int, "The version of the contract template")
	Attribute("template_type", String, "The type of the template")
	Attribute("name", String, "The name of the contract template")
	Attribute("description", String, "A description for that template")
	Attribute("template_data", Any, "The template data of the contract template")

	Required("did", "updated_at")
})

var ContractTemplateUpdateManageResponse = Type("ContractTemplateUpdateManageResponse", func() {
	Description("Result for updating a contract template")

	Attribute("did", String, "Decentralized Identifier of the contract template")
	Attribute("document_number", String, "The number of the contract template")
	Attribute("version", Int, "The version of the contract template")

	Required("did")
})

var ContractTemplateSearchRequest = Type("ContractTemplateSearchRequest", func() {
	Description("Contract template search request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract template")
	Attribute("document_number", String, "The number of the contract template")
	Attribute("version", Int, "The version of the contract template")
	Attribute("template_type", String, "The type of the template")
	Attribute("state", String, "The state of the contract template")
	Attribute("name", String, "The name of the contract template")
	Attribute("description", String, "A description for that template")
	Attribute("filter", String, "Search value for full text search in template data")
})

var ContractTemplateSearchResponse = Type("ContractTemplateSearchResponse", func() {
	Description("Result for searching a contract templates by filter")

	Attribute("did", String, "Decentralized Identifier of the contract template")

	Attribute("document_number", String, "The document number of the contract template")
	Attribute("version", Int, "The version number of the contract template")
	Attribute("state", String, "The state of the contract template")
	Attribute("template_type", String, "The type of the template")
	Attribute("name", String, "The name of the contract template")
	Attribute("description", String, "A description for that template")

	Attribute("created_at", String, "The timestamp when the contract template was created")

	Attribute("updated_at", String, "The timestamp when the contract template was updated")

	Required("did", "state", "template_type", "created_at", "updated_at")
})

var ContractTemplateRetrieveRequest = Type("ContractTemplateRetrieveRequest", func() {
	Description("Contract template retrieve request")

	Token("token", String, "JWT token")
})

var ContractTemplateItem = Type("ContractTemplateItem", func() {
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

	Required("did", "state", "template_type", "created_by", "created_at", "updated_at")
})

var ReviewTaskItem = Type("ReviewTaskItem", func() {
	Attribute("did", String, "DID of the contract template")
	Attribute("document_number", String, "Document number")
	Attribute("version", Int, "Version")
	Attribute("state", String, "State of the review task")
	Attribute("reviewer", String, "The reviewer of the contract template")
	Attribute("created_at", String, "Created at")

	Required("did", "state", "reviewer", "created_at")
})

var ApprovalTaskItem = Type("ApprovalTaskItem", func() {
	Attribute("did", String, "DID of the contract template")
	Attribute("document_number", String, "Document number")
	Attribute("version", Int, "Version")
	Attribute("state", String, "State of the approval task")
	Attribute("approver", String, "The approver for the contract template")
	Attribute("created_at", String, "Created at")

	Required("did", "state", "approver", "created_at")
})

var ContractTemplateRetrieveResponse = Type("ContractTemplateRetrieveResponse", func() {
	Description("Result for retrieving a contract template by id")

	Attribute("contract_templates", ArrayOf(ContractTemplateItem), "A list of contract templates")

	Attribute("review_tasks", ArrayOf(ReviewTaskItem), "A list of review tasks")

	Attribute("approval_tasks", ArrayOf(ApprovalTaskItem), "A list of approval tasks")

	Required("contract_templates", "review_tasks", "approval_tasks")
})

var ContractTemplateRetrieveByIDRequest = Type("ContractTemplateRetrieveByIDRequest", func() {
	Description("Contract template retrieve by id request")

	Token("token", String, "JWT token")

	Attribute("did", String, "DID of the contract template")

	Required("did")
})

var ContractTemplateRetrieveByIDResponse = Type("ContractTemplateRetrieveByIDResponse", func() {
	Description("Result for retrieving a contract template by id")

	Attribute("did", String, "Decentralized Identifier of the contract template")

	Attribute("document_number", String, "The document number of the contract template")
	Attribute("version", Int, "The version number of the contract template")

	Attribute("state", String, "The state of the contract template")
	Attribute("template_type", String, "The type of the template")

	Attribute("name", String, "The name of the contract template")
	Attribute("description", String, "A description for that template")

	Attribute("created_by", String, "Identifier of who created the contract template")
	Attribute("created_at", String, "The timestamp when the contract template was created")

	Attribute("updated_at", String, "The timestamp when the contract template was updated")

	Attribute("template_data", Any, "The template data of the contract template")

	Required("did", "state", "template_type", "created_by", "created_at", "updated_at", "template_data")
})

var ContractTemplateApproveRequest = Type("ContractTemplateApproveRequest", func() {
	Description("Contract template approve request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract template")

	Attribute("updated_at", String, "The timestamp when the contract template was updated")

	Attribute("decision_notes", ArrayOf(String), "A list of decision notes")

	Required("did", "updated_at")
})

var ContractTemplateApproveResponse = Type("ContractTemplateApproveResponse", func() {
	Description("Result for approving a contract template")

	Attribute("did", String, "Decentralized Identifier of the contract template")

	Required("did")
})

var ContractTemplateRejectRequest = Type("ContractTemplateRejectRequest", func() {
	Description("Contract template retrieve by id request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract template")

	Attribute("updated_at", String, "The timestamp when the contract template was updated")

	Attribute("reason", String, "Reason for rejecting the contract template")

	Required("did", "updated_at", "reason")
})

var ContractTemplateRejectResponse = Type("ContractTemplateRejectResponse", func() {
	Description("Result for rejecting a contract template")

	Attribute("did", String, "Decentralized Identifier of the contract template")

	Required("did")
})

var ContractTemplateVerifyRequest = Type("ContractTemplateVerifyRequest", func() {
	Description("Contract template verify request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract template")

	Required("did")
})

var ContractTemplateVerifyResponse = Type("ContractTemplateVerifyResponse", func() {
	Description("Result for verifying a contract template")

	Attribute("did", String, "Decentralized Identifier of the contract template")
	Attribute("findings", ArrayOf(String), "A list of findings")

	Required("did", "findings")
})

var ContractTemplateArchiveRequest = Type("ContractTemplateArchiveRequest", func() {
	Description("Contract template archive request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract template")

	Attribute("updated_at", String, "The timestamp when the contract template was updated")

	Required("did", "updated_at")
})

var ContractTemplateArchiveResponse = Type("ContractTemplateArchiveResponse", func() {
	Description("Result for archiving a contract template")

	Attribute("did", String, "Decentralized Identifier of the contract template")

	Required("did")
})

var ContractTemplateRegisterRequest = Type("ContractTemplateRegisterRequest", func() {
	Description("Contract template register request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract template")

	Required("did")
})

var ContractTemplateRegisterResponse = Type("ContractTemplateRegisterResponse", func() {
	Description("Result for register a contract template")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract template")

	Required("did")
})

var ContractTemplateAuditRequest = Type("ContractTemplateAuditRequest", func() {
	Description("Contract template audit request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract template")

	Required("did")
})

var ContractTemplateAuditResponse = Type("ContractTemplateAuditResponse", func() {
	Description("Result for auditing a contract template")

	Attribute("did", String, "Decentralized Identifier of the contract template")

	Required("did")
})

// Template Repository Service  (/template/...)
var _ = Service("TemplateRepository", func() {
	Description("Template Repository APIs (/template/...)")

	// POST /template/create
	Method("create", func() {
		Description("Create a new template.")
		Meta("dcs:requirements", "DCS-IR-TR-01")
		Meta("dcs:tr:components", "Single- or multi-tiered template generation")
		Meta("dcs:ui", "Template Builder")

		Security(JWTAuth, func() {
			Scope("Template Creator")
		})

		Payload(ContractTemplateCreateRequest)
		Result(ContractTemplateCreateResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/template/create")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	// POST /template/submit
	Method("submit", func() {
		Description(`with action flag { forwardTo: "approval" | "draft" } and optional reviewComments. allow resubmission path with approver comments.`)
		Meta("dcs:requirements", "DCS-IR-TR-03", "DCS-IR-TR-04", "DCS-IR-TR-05")
		Meta("dcs:tr:components", "Single- or multi-tiered template generation")
		Meta("dcs:ui", "Template Builder, Template Review, Template Approver")

		Security(JWTAuth, func() {
			Scope("Template Creator")
			Scope("Template Reviewer")
			Scope("Template Approver")
		})

		Payload(ContractTemplateSubmitRequest)
		Result(ContractTemplateSubmitResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/template/submit")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	// PUT /template/update
	Method("update", func() {
		Description("persist reviewer edits (template data/clauses/semantics).")
		Meta("dcs:requirements", "DCS-IR-TR-03")
		Meta("dcs:tr:components", "Template Versioning")
		Meta("dcs:ui", "Template Builder, Template Review")

		Security(JWTAuth, func() {
			Scope("Template Creator")
			Scope("Template Reviewer")
		})

		Payload(ContractTemplateUpdateRequest)
		Result(ContractTemplateUpdateResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			PUT("/template/update")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	// POST /template/update
	Method("update_manage", func() {
		Description("update template data or status.")
		Meta("dcs:requirements", "DCS-IR-TR-07")
		Meta("dcs:roles", "Template Manager")
		Meta("dcs:tr:components", "Template Versioning")
		Meta("dcs:ui", "Template Management Dashboard")

		Security(JWTAuth, func() {
			Scope("Template Manager")
		})

		Payload(ContractTemplateUpdateManageRequest)
		Result(ContractTemplateUpdateManageResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/template/update")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	// GET /template/search
	Method("search", func() {
		Description("perform filtered searches.")
		Meta("dcs:requirements", "DCS-IR-TR-02", "DCS-IR-TR-07")
		Meta("dcs:tr:components", "Search Capabilities")
		Meta("dcs:ui", "Template Builder, Template Management Dashboard")

		Security(JWTAuth, func() {
			Scope("Template Creator")
			Scope("Template Manager")
		})

		Payload(ContractTemplateSearchRequest)
		Result(ArrayOfRequired(ContractTemplateSearchResponse))

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/template/search")
			Param("did")
			Param("document_number")
			Param("version")
			Param("template_type")
			Param("state")
			Param("name")
			Param("description")
			Param("filter")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	// GET /template/retrieve
	Method("retrieve", func() {
		Description("load submitted template and history/provenance summary. fetch reviewed template with metadata, review history, and validation results. fetch all template entries for dashboard view.")
		Meta("dcs:requirements", "DCS-IR-TR-02", "DCS-IR-TR-03", "DCS-IR-TR-05", "DCS-IR-TR-08")
		Meta("dcs:tr:components", "Template Versioning")
		Meta("dcs:ui", "Template Builder, Template Approver, Template Management Dashboard")

		Security(JWTAuth, func() {
			Scope("Template Creator")
			Scope("Template Reviewer")
			Scope("Template Approver")
			Scope("Template Manager")
		})

		Payload(ContractTemplateRetrieveRequest)
		Result(ContractTemplateRetrieveResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/template/retrieve")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	// GET /template/retrieve/{did}
	Method("retrieve_by_id", func() {
		Description("Retrieve a template by template id.")
		Meta("dcs:requirements", "DCS-IR-TR-02", "DCS-IR-TR-03", "DCS-FR-TR-19")
		Meta("dcs:tr:components", "Template Versioning")
		Meta("dcs:ui", "Template Builder, Template Approver, Template Management Dashboard")

		Security(JWTAuth, func() {
			Scope("Template Creator")
			Scope("Template Reviewer")
			Scope("Template Approver")
			Scope("Template Manager")
		})

		Payload(ContractTemplateRetrieveByIDRequest)
		Result(ContractTemplateRetrieveByIDResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/template/retrieve/{did}")
			Param("did")

			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	// GET /template/verify/{did}
	Method("verify", func() {
		Description("run policy, schema, and semantic validations; return findings.")
		Meta("dcs:requirements", "DCS-IR-TR-03")
		Meta("dcs:tr:components", "Semantic Hub")
		Meta("dcs:ui", "Template Review")

		Security(JWTAuth, func() {
			Scope("Template Reviewer")
		})

		Payload(ContractTemplateVerifyRequest)
		Result(ContractTemplateVerifyResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/template/verify/{did}")
			Param("did")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	// POST /template/approve
	Method("approve", func() {
		Description("mark template as approved, with optional decision notes.")
		Meta("dcs:requirements", "DCS-IR-TR-05", "DCS-IR-TR-06")
		Meta("dcs:tr:components", "Template Versioning")
		Meta("dcs:ui", "Template Approver")

		Security(JWTAuth, func() {
			Scope("Template Approver")
		})

		Payload(ContractTemplateApproveRequest)
		Result(ContractTemplateApproveResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/template/approve")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	// POST /template/reject
	Method("reject", func() {
		Description("mark template as rejected, requiring reason field.")
		Meta("dcs:requirements", "DCS-IR-TR-05")
		Meta("dcs:tr:components", "")
		Meta("dcs:ui", "Template Approver")

		Security(JWTAuth, func() {
			Scope("Template Approver")
		})

		Payload(ContractTemplateRejectRequest)
		Result(ContractTemplateRejectResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/template/reject")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	// POST /template/register
	Method("register", func() {
		Description("register new template into the repository and the XFSC Catalogue.")
		Meta("dcs:requirements", "DCS-IR-TR-07")
		Meta("dcs:tr:components", "Contract Templates Storage & Provenance")
		Meta("dcs:ui", "Template Management Dashboard")

		Security(JWTAuth, func() {
			Scope("Template Manager")
		})

		Payload(ContractTemplateRegisterRequest)
		Result(ContractTemplateRegisterResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/template/register")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	// POST /template/archive
	Method("archive", func() {
		Description("archive obsolete template.")
		Meta("dcs:requirements", "DCS-IR-TR-07")
		Meta("dcs:tr:components", "Contract Templates Storage & Provenance")
		Meta("dcs:ui", "Template Management Dashboard")

		Security(JWTAuth, func() {
			Scope("Template Manager")
		})

		Payload(ContractTemplateArchiveRequest)
		Result(ContractTemplateArchiveResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/template/archive")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	// GET /template/audit
	Method("audit", func() {
		Description("retrieve audit history of template actions.")
		Meta("dcs:requirements", "DCS-IR-TR-07", "DCS-IR-TR-08")
		Meta("dcs:tr:components", "")
		Meta("dcs:ui", "Template Management Dashboard")

		Security(JWTAuth, func() {
			Scope("Template Manager")
		})

		Payload(ContractTemplateAuditRequest)
		Result(ContractTemplateAuditResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/template/audit")
			Param("did")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})
})
