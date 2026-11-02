package design

import (
	. "goa.design/goa/v3/dsl"

	"digital-contracting-service/internal/templaterepository/datatype/partytype"
)

var ContractTemplateCreateRequest = Type("ContractTemplateCreateRequest", func() {
	Description("Contract template create request")

	Token("token", String, "JWT token")

	Attribute("template_type", String, "The type of the template")
	Attribute("party_type", String, "Party classification", func() { Enum(partytype.SingleParty.String(), partytype.MultiParty.String()) })

	Attribute("name", String, "The name of the contract template")
	Attribute("description", String, "A description for that template")
	Attribute("template_data", Any, "The template data of the contract template")
	Attribute("jurisdiction", String, "Jurisdiction the template is drafted for (DCS-FR-TR-10)")
	Attribute("industry", String, "Industry the template is drafted for (DCS-FR-TR-10)")
	Attribute("template_level", String, "Hierarchy level of the template (DCS-FR-TR-02)", func() { Enum("FRAME_AGREEMENT", "AGREEMENT", "SUB_AGREEMENT", "ADDENDUM") })
	Attribute("required_templates", ArrayOf(String), "DIDs of the templates a parent contract must be made from (DCS-FR-TR-23)")

	Required("template_type", "party_type")
})

var ContractTemplateCreateResponse = Type("ContractTemplateCreateResponse", func() {
	Description("Result for creating a contract template")

	Attribute("did", String, "Decentralized Identifier of the contract template")

	Required("did")
})

var ContractTemplateCopyRequest = Type("ContractTemplateCopyRequest", func() {
	Description("Contract template create request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract template that should be copied")

	Required("did")
})

var ContractTemplateCopyResponse = Type("ContractTemplateCopyResponse", func() {
	Description("Result for creating a contract template")

	Attribute("did", String, "Decentralized Identifier of the contract template copy")

	Required("did")
})

var ContractTemplateSubmitRequest = Type("ContractTemplateSubmitRequest", func() {
	Description("Contract template submit request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract template")

	Attribute("updated_at", String, "The timestamp when the contract template was updated")

	Attribute("forward_to", String, "Action flag: approval | draft")
	Attribute("comments", ArrayOf(String), "Review comments; required when the approver returns a reviewed template to review (DCS-IR-TR-05)")

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

	Attribute("template_type", String, "The type of the template")
	Attribute("party_type", String, "Party classification", func() { Enum(partytype.SingleParty.String(), partytype.MultiParty.String()) })
	Attribute("name", String, "The name of the contract template")
	Attribute("description", String, "A description for that template")
	Attribute("template_data", Any, "The template data of the contract template")
	Attribute("jurisdiction", String, "Jurisdiction the template is drafted for (DCS-FR-TR-10)")
	Attribute("industry", String, "Industry the template is drafted for (DCS-FR-TR-10)")
	Attribute("template_level", String, "Hierarchy level of the template (DCS-FR-TR-02)", func() { Enum("FRAME_AGREEMENT", "AGREEMENT", "SUB_AGREEMENT", "ADDENDUM") })
	Attribute("required_templates", ArrayOf(String), "DIDs of the templates a parent contract must be made from (DCS-FR-TR-23)")

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

	Attribute("updated_at", String, "The timestamp when the contract template was updated")

	Attribute("template_type", String, "The type of the template")
	Attribute("party_type", String, "Party classification", func() { Enum(partytype.SingleParty.String(), partytype.MultiParty.String()) })
	Attribute("name", String, "The name of the contract template")
	Attribute("description", String, "A description for that template")
	Attribute("template_data", Any, "The template data of the contract template")
	Attribute("jurisdiction", String, "Jurisdiction the template is drafted for (DCS-FR-TR-10)")
	Attribute("industry", String, "Industry the template is drafted for (DCS-FR-TR-10)")
	Attribute("template_level", String, "Hierarchy level of the template (DCS-FR-TR-02)", func() { Enum("FRAME_AGREEMENT", "AGREEMENT", "SUB_AGREEMENT", "ADDENDUM") })
	Attribute("required_templates", ArrayOf(String), "DIDs of the templates a parent contract must be made from (DCS-FR-TR-23)")

	Required("did", "updated_at")
})

var ContractTemplateUpdateManageResponse = Type("ContractTemplateUpdateManageResponse", func() {
	Description("Result for updating a contract template")

	Attribute("did", String, "Decentralized Identifier of the contract template")

	Required("did")
})

var ContractTemplateSearchRequest = Type("ContractTemplateSearchRequest", func() {
	Description("Contract template search request")

	Token("token", String, "JWT token")

	Attribute("offset", Int, "Start index of results")
	Attribute("limit", Int, "Page size of results")

	Attribute("did", String, "Decentralized Identifier of the contract template")
	Attribute("version", Int, "The version of the contract template")
	Attribute("template_type", String, "The type of the template")
	Attribute("party_type", String, "Party classification", func() { Enum(partytype.SingleParty.String(), partytype.MultiParty.String()) })
	Attribute("state", String, "The state of the contract template")
	Attribute("name", String, "The name of the contract template")
	Attribute("description", String, "A description for that template")
	Attribute("template_data", String, "Search value for full text search in template data")
	Attribute("jurisdiction", String, "Jurisdiction the template is drafted for (DCS-FR-TR-10)")
	Attribute("industry", String, "Industry the template is drafted for (DCS-FR-TR-10)")
	Attribute("compliance_status", String, "Regulatory compliance status from the template policy audit at approval (DCS-FR-TR-10)", func() { Enum("UNASSESSED", "COMPLIANT", "NON_COMPLIANT") })
	Attribute("created_by", String, "Part of the identifier of who created the template (DCS-IR-TR-08)")
	Attribute("created_from", String, "Only templates created on or after this day (DCS-IR-TR-08)", func() { Format(FormatDate) })
	Attribute("created_to", String, "Only templates created on or before this day (DCS-IR-TR-08)", func() { Format(FormatDate) })
})

var ContractTemplateSearchResponse = Type("ContractTemplateSearchResponse", func() {
	Description("Result for searching a contract templates")

	Attribute("did", String, "Decentralized Identifier of the contract template")

	Attribute("version", Int, "The version number of the contract template")
	Attribute("state", String, "The state of the contract template")
	Attribute("template_type", String, "The type of the template")
	Attribute("party_type", String, "Party classification", func() { Enum(partytype.SingleParty.String(), partytype.MultiParty.String()) })
	Attribute("name", String, "The name of the contract template")
	Attribute("description", String, "A description for that template")
	Attribute("jurisdiction", String, "Jurisdiction the template is drafted for (DCS-FR-TR-10)")
	Attribute("industry", String, "Industry the template is drafted for (DCS-FR-TR-10)")
	Attribute("template_level", String, "Hierarchy level of the template (DCS-FR-TR-02)", func() { Enum("FRAME_AGREEMENT", "AGREEMENT", "SUB_AGREEMENT", "ADDENDUM") })
	Attribute("required_templates", ArrayOf(String), "DIDs of the templates a parent contract must be made from (DCS-FR-TR-23)")
	Attribute("compliance_status", String, "Regulatory compliance status from the template policy audit at approval (DCS-FR-TR-10)", func() { Enum("UNASSESSED", "COMPLIANT", "NON_COMPLIANT") })

	Attribute("created_by", String, "Identifier of who created the contract template (DCS-IR-TR-08)")
	Attribute("created_at", String, "The timestamp when the contract template was created")

	Attribute("updated_at", String, "The timestamp when the contract template was updated")

	Required("did", "state", "template_type", "created_by", "created_at", "updated_at", "version", "party_type")
})

var ContractTemplateRetrieveRequest = Type("ContractTemplateRetrieveRequest", func() {
	Description("Contract template retrieve request")

	Token("token", String, "JWT token")

	Attribute("offset", Int, "Start index of results")
	Attribute("limit", Int, "Page size of results")
})

var ContractTemplateItem = Type("ContractTemplateItem", func() {
	Attribute("did", String, "DID of the contract template")
	Attribute("version", Int, "Version")
	Attribute("state", String, "State")
	Attribute("template_type", String, "The type of the template")
	Attribute("party_type", String, "Party classification", func() { Enum(partytype.SingleParty.String(), partytype.MultiParty.String()) })
	Attribute("name", String, "Name")
	Attribute("description", String, "Description")
	Attribute("created_by", String, "Created by")
	Attribute("created_at", String, "Created at")
	Attribute("updated_at", String, "Updated at")
	Attribute("latest_did", String, "The DID of the newest contract template")
	Attribute("jurisdiction", String, "Jurisdiction the template is drafted for (DCS-FR-TR-10)")
	Attribute("industry", String, "Industry the template is drafted for (DCS-FR-TR-10)")
	Attribute("template_level", String, "Hierarchy level of the template (DCS-FR-TR-02)", func() { Enum("FRAME_AGREEMENT", "AGREEMENT", "SUB_AGREEMENT", "ADDENDUM") })
	Attribute("required_templates", ArrayOf(String), "DIDs of the templates a parent contract must be made from (DCS-FR-TR-23)")
	Attribute("compliance_status", String, "Regulatory compliance status from the template policy audit at approval (DCS-FR-TR-10)", func() { Enum("UNASSESSED", "COMPLIANT", "NON_COMPLIANT") })

	Required("did", "state", "template_type", "created_by", "created_at", "updated_at", "version", "party_type")
})

var ReviewTaskItem = Type("ReviewTaskItem", func() {
	Attribute("did", String, "DID of the contract template")
	Attribute("version", Int, "Version")
	Attribute("state", String, "State of the review task")
	Attribute("reviewer", String, "The reviewer of the contract template")
	Attribute("created_at", String, "Created at")

	Required("did", "state", "reviewer", "created_at", "version")
})

var ApprovalTaskItem = Type("ApprovalTaskItem", func() {
	Attribute("did", String, "DID of the contract template")
	Attribute("version", Int, "Version")
	Attribute("state", String, "State of the approval task")
	Attribute("approver", String, "The approver for the contract template")
	Attribute("created_at", String, "Created at")

	Required("did", "state", "approver", "created_at", "version")
})

var ContractTemplateRetrieveResponse = Type("ContractTemplateRetrieveResponse", func() {
	Description("Result for retrieving a contract template by id")

	Attribute("contract_templates", ArrayOf(ContractTemplateItem), "A list of contract templates")

	Attribute("review_tasks", ArrayOf(ReviewTaskItem), "A list of review tasks")

	Attribute("approval_tasks", ArrayOf(ApprovalTaskItem), "A list of approval tasks")

	Required("contract_templates", "review_tasks", "approval_tasks")
})

var TemplateProvenanceRetrieveRequest = Type("TemplateProvenanceRetrieveRequest", func() {
	Description("Template provenance credentials retrieve request")

	Token("token", String, "JWT token")

	Attribute("did", String, "DID of the contract template")

	Required("did")
})

var TemplateProvenanceCredentialResponse = Type("TemplateProvenanceCredentialResponse", func() {
	Description("One registered template version's signed W3C provenance credential (DCS-FR-TR-09), linked to its predecessor")

	Attribute("version", Int, "Template version the credential seals")
	Attribute("vc_id", String, "Credential ID (urn:dcs:vc:template-provenance:<hash>)")
	Attribute("previous_vc_id", String, "Credential ID of the previous version's credential; absent for the first version")
	Attribute("credential", Any, "The signed W3C Verifiable Credential (JSON-LD, ecdsa-rdfc-2019 Data Integrity proof)")

	Required("version", "vc_id", "credential")
})

var ContractTemplateHistoryRetrieveByIDRequest = Type("ContractTemplateHistoryRetrieveByIDRequest", func() {
	Description("Contract template retrieve by id request")

	Token("token", String, "JWT token")

	Attribute("did", String, "DID of the contract template")

	Required("did")
})

var ContractTemplateHistoryRetrieveByIDResponse = Type("ContractTemplateHistoryRetrieveByIDResponse", func() {
	Description("Result for retrieving a contract template by id")

	Attribute("did", String, "Decentralized Identifier of the contract template")

	Attribute("version", Int, "The version number of the contract template")

	Attribute("state", String, "The state of the contract template")
	Attribute("template_type", String, "The type of the template")
	Attribute("party_type", String, "Party classification", func() { Enum(partytype.SingleParty.String(), partytype.MultiParty.String()) })

	Attribute("name", String, "The name of the contract template")
	Attribute("description", String, "A description for that template")

	Attribute("created_by", String, "Identifier of who created the contract template")
	Attribute("created_at", String, "The timestamp when the contract template was created")

	Attribute("updated_at", String, "The timestamp when the contract template was updated")

	Attribute("template_data", Any, "The template data of the contract template")
	Attribute("jurisdiction", String, "Jurisdiction of the revision (DCS-FR-TR-10)")
	Attribute("industry", String, "Industry of the revision (DCS-FR-TR-10)")
	Attribute("compliance_status", String, "Compliance status of the revision (DCS-FR-TR-10)")
	Attribute("template_level", String, "Hierarchy level of the revision (DCS-FR-TR-02)")
	Attribute("required_templates", ArrayOf(String), "Templates the revision required (DCS-FR-TR-23)")

	Required("did", "state", "template_type", "created_by", "created_at", "updated_at", "template_data", "version", "party_type")
})

var ContractTemplateRetrieveByIDRequest = Type("ContractTemplateRetrieveByIDRequest", func() {
	Description("Contract template retrieve by id request")

	Token("token", String, "JWT token")

	Attribute("did", String, "DID of the contract template")

	Required("did")
})

var TemplateReviewNote = Type("TemplateReviewNote", func() {
	Description("A comment of a review or approval decision, or a rejection reason (DCS-IR-TR-04, DCS-IR-TR-05)")

	Attribute("kind", String, "The decision", func() {
		Enum("RETURNED_TO_DRAFT", "FORWARDED_TO_APPROVAL", "RETURNED_TO_REVIEW", "REJECTED", "APPROVED")
	})
	Attribute("text", ArrayOf(String), "The comments, the rejection reason or the decision notes")
	Attribute("by", String, "Holder DID of the person who decided")
	Attribute("created_at", String, "When the decision was made")

	Required("kind", "text", "created_at")
})

var ContractTemplateRetrieveByIDResponse = Type("ContractTemplateRetrieveByIDResponse", func() {
	Description("Result for retrieving a contract template by id")

	Attribute("did", String, "Decentralized Identifier of the contract template")

	Attribute("version", Int, "The version number of the contract template")

	Attribute("state", String, "The state of the contract template")
	Attribute("template_type", String, "The type of the template")
	Attribute("party_type", String, "Party classification", func() { Enum(partytype.SingleParty.String(), partytype.MultiParty.String()) })

	Attribute("name", String, "The name of the contract template")
	Attribute("description", String, "A description for that template")

	Attribute("created_by", String, "Identifier of who created the contract template")
	Attribute("created_at", String, "The timestamp when the contract template was created")

	Attribute("updated_at", String, "The timestamp when the contract template was updated")

	Attribute("template_data", Any, "The template data of the contract template")
	Attribute("jurisdiction", String, "Jurisdiction the template is drafted for (DCS-FR-TR-10)")
	Attribute("industry", String, "Industry the template is drafted for (DCS-FR-TR-10)")
	Attribute("template_level", String, "Hierarchy level of the template (DCS-FR-TR-02)", func() { Enum("FRAME_AGREEMENT", "AGREEMENT", "SUB_AGREEMENT", "ADDENDUM") })
	Attribute("required_templates", ArrayOf(String), "DIDs of the templates a parent contract must be made from (DCS-FR-TR-23)")
	Attribute("compliance_status", String, "Regulatory compliance status from the template policy audit at approval (DCS-FR-TR-10)", func() { Enum("UNASSESSED", "COMPLIANT", "NON_COMPLIANT") })
	Attribute("review_notes", ArrayOf(TemplateReviewNote), "Review comments, rejection reasons and decision notes of this version, oldest first; returned to the template roles only")

	Required("did", "state", "template_type", "created_by", "created_at", "updated_at", "template_data", "version", "party_type")
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

	Attribute("reason", String, "Reason for rejecting the contract template (DCS-IR-TR-05)", func() { Pattern(`\S`) })

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
	Attribute("findings", ArrayOf(String), "Findings that block the review: envelope errors, error findings of the template policy audit and of the Semantic Hub shapes, provenance and catalogue findings")
	Attribute("warnings", ArrayOf(String), "Warning and information findings of the template policy audit and the Semantic Hub shapes; they do not block")

	Required("did", "findings")
})

var ContractTemplateArchiveRequest = Type("ContractTemplateArchiveRequest", func() {
	Description("Contract template archive request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract template")

	Attribute("updated_at", String, "The timestamp when the contract template was updated")
	Attribute("justification", String, "Why the template is archived; stored in the audit event (DCS-IR-TR-07)", func() { Pattern(`\S`) })

	Required("did", "updated_at", "justification")
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
	Attribute("version", Int, "The version of the contract template")

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
	Attribute("justification", String, "Required audit justification", func() { Pattern(`\S`) })

	Required("did", "justification")
})

var ContractTemplateAuditResponse = Type("ContractTemplateAuditResponse", func() {
	Description("Result for auditing a contract template")

	Attribute("id", Int64, "Identifier for the outbox event")
	Attribute("component", String, "Name of the component")
	Attribute("event_type", String, "Type of the event")
	Attribute("event_data", Any, "Data of the event")
	Attribute("did", String, "Decentralized Identifier of the contract template")
	Attribute("created_at", String, "The creation date of the event")
	Attribute("res_log_pred_cid", String, "Resource audit trail predecessor on the IPFS chain")

	Required("id", "component", "event_type", "event_data", "created_at")
})

var ContractTemplatePublishRequest = Type("ContractTemplatePublishRequest", func() {
	Description("Contract template publish request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract template")

	Attribute("updated_at", String, "The timestamp when the contract template was updated")

	Required("did", "updated_at")
})

var ContractTemplatePublishResponse = Type("ContractTemplatePublishResponse", func() {
	Description("Result for publish a approved contract template")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract template")

	Required("did")
})

// Template Repository Service  (/template/...)
// templateVerifyScopes are the roles that verify a template on either verify
// route. The Template Approver reads the validation results before the
// decision (DCS-IR-TR-05); only a reviewer's verification completes a task.
var templateVerifyScopes = func() {
	Scope("Template Reviewer")
	Scope("Template Approver")
	Scope("Template Manager")
	Scope("Contract Creator")
}

var _ = Service("TemplateRepository", func() {
	Description("Template Repository APIs (/template/...)")

	// POST /template/create
	Method("create", func() {
		Description("Create a new template.")
		Meta("dcs:requirements", "DCS-IR-TR-01", "DCS-FR-TR-02", "DCS-FR-TR-23")

		Security(JWTAuth, func() {
			Scope("Template Creator")
			Scope("Template Manager")
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

	// POST /template/copy
	Method("copy", func() {
		Description("Copy a template. Behavior depends on the source template's state: if it is not yet REGISTERED/PUBLISHED, the copy starts a new, independent version lineage (version=1, base_template=own new DID); if the source is already REGISTERED/PUBLISHED, the copy becomes the next version of the same lineage (version+1, inherits the source's base_template). A DB guard rejects the copy if a REGISTERED/PUBLISHED template with that version already exists in the lineage.")

		Security(JWTAuth, func() {
			Scope("Template Creator")
			Scope("Template Manager")
		})

		Payload(ContractTemplateCopyRequest)
		Result(ContractTemplateCopyResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/template/copy")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	// POST /template/submit
	Method("submit", func() {
		Description(`with action flag { forwardTo: "approval" | "draft" } and optional reviewComments. allow resubmission path with approver comments.`)
		Meta("dcs:requirements", "DCS-IR-TR-03", "DCS-IR-TR-04", "DCS-IR-TR-05", "DCS-FR-TR-26")

		Security(JWTAuth, func() {
			Scope("Template Creator")
			Scope("Template Reviewer")
			Scope("Template Approver")
			Scope("Template Manager")
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

		Security(JWTAuth, func() {
			Scope("Template Creator")
			Scope("Template Reviewer")
			Scope("Template Manager")
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

	// POST /template/update_manage
	Method("update_manage", func() {
		Description("update template data.")
		Meta("dcs:requirements", "DCS-IR-TR-07")

		Security(JWTAuth, func() {
			Scope("Template Manager")
		})

		Payload(ContractTemplateUpdateManageRequest)
		Result(ContractTemplateUpdateManageResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/template/update_manage")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	// GET /template/search
	Method("search", func() {
		Description("perform filtered searches.")
		Meta("dcs:requirements", "DCS-IR-TR-02", "DCS-IR-TR-07", "DCS-FR-TR-10")

		Security(JWTAuth, func() {
			Scope("Template Creator")
			Scope("Template Reviewer")
			Scope("Template Approver")
			Scope("Template Manager")
		})

		Payload(ContractTemplateSearchRequest)
		Result(ArrayOfRequired(ContractTemplateSearchResponse))

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/template/search")
			Param("offset")
			Param("limit")

			Param("did")
			Param("version")
			Param("template_type")
			Param("party_type")
			Param("state")
			Param("name")
			Param("description")
			Param("template_data")
			Param("jurisdiction")
			Param("industry")
			Param("compliance_status")
			Param("created_by")
			Param("created_from")
			Param("created_to")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	// GET /template/retrieve
	Method("retrieve", func() {
		Description("load submitted template and history/provenance summary. fetch reviewed template with metadata, review history, and validation results. fetch all template entries for dashboard view.")
		Meta("dcs:requirements", "DCS-IR-TR-02", "DCS-IR-TR-03", "DCS-IR-TR-05", "DCS-IR-TR-08")

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
			Param("offset")
			Param("limit")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	// GET /template/provenance/{did}
	Method("provenance", func() {
		Description("DCS-FR-TR-09 Template Provenance and Versioning: the per-version signed W3C provenance credentials (creator/reviewer/approver/registrar claims, content hash, previous-credential linkage) a template user verifies a template's provenance with.")
		Meta("dcs:requirements", "DCS-FR-TR-09")

		Security(JWTAuth, func() {
			Scope("Template Creator")
			Scope("Template Reviewer")
			Scope("Template Approver")
			Scope("Template Manager")
			Scope("Contract Creator")
		})

		Payload(TemplateProvenanceRetrieveRequest)
		Result(ArrayOfRequired(TemplateProvenanceCredentialResponse))

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/template/provenance/{did}")
			Param("did")

			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	// GET /template/history/{did}
	Method("retrieve_history_by_id", func() {
		Description("fetch history of a contract template")

		Security(JWTAuth, func() {
			Scope("Template Creator")
			Scope("Template Reviewer")
			Scope("Template Approver")
			Scope("Template Manager")
		})

		Payload(ContractTemplateHistoryRetrieveByIDRequest)
		Result(ArrayOfRequired(ContractTemplateHistoryRetrieveByIDResponse))

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/template/history/{did}")
			Param("did")

			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	// GET /template/{did} — the template's resource IRI
	Method("resolve", func() {
		Description("Dereference a template's resource IRI ({DCS_PUBLIC_URL}/template/{did}): serves the canonical JSON-LD template document under the same authorization as retrieve_by_id. This route is what makes a template's @id follow-your-nose resolvable.")
		Meta("dcs:requirements", "DCS-FR-TR-01")

		Security(JWTAuth, func() {
			Scope("Template Creator")
			Scope("Template Reviewer")
			Scope("Template Approver")
			Scope("Template Manager")
		})

		Payload(ContractTemplateRetrieveByIDRequest)
		Result(Any)

		Error("bad_request", ErrorResult, "Bad request")
		Error("not_found", ErrorResult, "No template with this DID that the caller may read")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/template/{did}")
			Param("did")

			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("not_found", StatusNotFound)
			Response("internal_error", StatusInternalServerError)
		})
	})

	// GET /template/retrieve/{did}
	Method("retrieve_by_id", func() {
		Description("Retrieve a template by template id.")
		Meta("dcs:requirements", "DCS-IR-TR-02", "DCS-IR-TR-03", "DCS-FR-TR-19")

		Security(JWTAuth, func() {
			Scope("Template Creator")
			Scope("Template Reviewer")
			Scope("Template Approver")
			Scope("Template Manager")
			// Deriving a contract binds the originator to one of the two roles
			// the template's machine-readable rules declare, so the Contract
			// Creator has to read the registered template they instantiate —
			// the role choice cannot be made from a list summary.
			Scope("Contract Creator")
		})

		Payload(ContractTemplateRetrieveByIDRequest)
		Result(ContractTemplateRetrieveByIDResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("not_found", ErrorResult, "No template with this DID that the caller may read; a caller without a template role reads registered, published and deprecated templates only")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/template/retrieve/{did}")
			Param("did")

			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("not_found", StatusNotFound)
			Response("internal_error", StatusInternalServerError)
		})
	})

	// POST /template/verify
	Method("verify", func() {
		Description("check the canonical envelope, run the template policy audit of the approval and the Semantic Hub SHACL shapes the template declares, verify the provenance credentials and compare the content hash with the registered one; return findings and warnings. Every verification is logged with its result.")
		Meta("dcs:requirements", "DCS-IR-TR-03", "DCS-FR-TR-09", "DCS-FR-TR-20")

		Security(JWTAuth, templateVerifyScopes)

		Payload(ContractTemplateVerifyRequest)
		Result(ContractTemplateVerifyResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/template/verify")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	// GET /template/verify/{did}
	Method("verify_by_id", func() {
		Description("verify the template the path names, with the same checks, scopes and audit as POST /template/verify; return findings.")
		Meta("dcs:requirements", "DCS-FR-TR-20")

		Security(JWTAuth, templateVerifyScopes)

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
		Meta("dcs:requirements", "DCS-IR-TR-05", "DCS-IR-TR-06", "DCS-FR-TR-26")

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
		Description("Register a template with the Federated Catalogue integration. Two distinct modes depending on whether the given DID already exists locally: if it does, only its state is flipped to REGISTERED; if it does not, the template (identified by did + version) is pulled from the Federated Catalogue and imported as a brand-new local DID in state DRAFT.")
		Meta("dcs:requirements", "DCS-IR-TR-07")

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
		Description("archive a template with a justification: a REGISTERED or PUBLISHED template becomes DEPRECATED (event ARCHIVE_CONTRACT_TEMPLATE); a DRAFT, REJECTED or DEPRECATED template becomes DELETED (event DELETE_CONTRACT_TEMPLATE), unless contracts that are not terminated were made from it. A template in review or approved is refused.")
		Meta("dcs:requirements", "DCS-IR-TR-07", "DCS-FR-TR-17", "DCS-FR-TR-18")

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

		Security(JWTAuth, func() {
			Scope("Auditor")
			Scope("Template Manager")
			Scope("Compliance Officer")
		})

		Payload(ContractTemplateAuditRequest)
		Result(ArrayOfRequired(ContractTemplateAuditResponse))

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/template/audit")
			Param("did")
			Param("justification")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	// POST /template/publish
	Method("publish", func() {
		Description("publish a local approved template to the XFSC Catalogue.")

		Security(JWTAuth, func() {
			Scope("Template Manager")
		})

		Payload(ContractTemplatePublishRequest)
		Result(ContractTemplatePublishResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/template/publish")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})
})
