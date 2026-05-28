package design

import (
	. "goa.design/goa/v3/dsl"
)

var SMContractRetrieveRequest = Type("SMContractRetrieveRequest", func() {
	Description("Contract retrieve request")

	Token("token", String, "JWT token")
})

var SMContractListItem = Type("SMContractListItem", func() {
	Attribute("did", String, "DID of the contract")
	Attribute("contract_version", Int, "The version of the contract")
	Attribute("state", String, "Current state of the contract")
	Attribute("name", String, "The name of the contract")
	Attribute("description", String, "The description of the contract")
	Attribute("created_at", String, "Created at")
	Attribute("updated_at", String, "Updated at")

	Required("did", "state", "created_at", "updated_at")
})

var SMContractSigningTaskItem = Type("SMContractSigningTaskItem", func() {
	Attribute("did", String, "DID of the contract")
	Attribute("contract_version", Int, "The version of the contract")
	Attribute("state", String, "State of the review task")
	Attribute("reviewer", String, "The reviewer of the contract")
	Attribute("created_at", String, "Created at")

	Required("did", "state", "reviewer", "created_at")
})

var SMContractRetrieveResponse = Type("SMContractRetrieveResponse", func() {
	Description("Result for retrieving a contract by id")

	Attribute("contracts", ArrayOf(SMContractListItem), "A list of contracts")
	Attribute("signing_tasks", ArrayOf(SMContractSigningTaskItem), "A list of signing tasks")

	Required("contracts", "signing_tasks")
})

var SMContractRetrieveByIDRequest = Type("SMContractRetrieveByIDRequest", func() {
	Description("Contract retrieve by id request")

	Token("token", String, "JWT token")

	Attribute("did", String, "DID of the contract")

	Required("did")
})

var SMContractItem = Type("SMContractItem", func() {
	Attribute("did", String, "DID of the contract")
	Attribute("contract_version", Int, "The version of the contract")
	Attribute("state", String, "Current state of the contract")
	Attribute("name", String, "The name of the contract")
	Attribute("description", String, "The description of the contract")
	Attribute("created_at", String, "Created at")
	Attribute("updated_at", String, "Updated at")

	Required("did", "state", "created_at", "updated_at")
})

var SMContractSignatureEnvelope = Type("SMContractSignatureEnvelope", func() {
})

var SMContractRetrieveByIDResponse = Type("SMContractRetrieveByIDResponse", func() {
	Attribute("contract", SMContractItem, "The contract")
	Attribute("signature_envelope", SMContractSignatureEnvelope, "The signature_envelope of the contract")

	Required("contract", "signature_envelope")
})

var SMContractVerifyRequest = Type("SMContractVerifyRequest", func() {
	Description("Contract verify request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Required("did")
})

var SMContractVerifyResponse = Type("SMContractVerifyResponse", func() {
	Description("Result for verifying a contract")

	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("findings", ArrayOf(String), "A list of findings")

	Required("did")
})

var SMContractApplyRequest = Type("SMContractApplyRequest", func() {
	Description("Contract apply request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Attribute("updated_at", String, "The timestamp when the contract was updated")

	Required("did", "updated_at")
})

var SMContractApplyResponse = Type("SMContractApplyResponse", func() {
	Description("Result for verifying a contract")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Required("did")
})

var SMContractValidateRequest = Type("SMContractValidateRequest", func() {
	Description("Contract validate request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Required("did")
})

var SMContractValidateResponse = Type("SMContractValidateResponse", func() {
	Description("Result for verifying a contract")

	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("findings", ArrayOf(String), "A list of findings")

	Required("did")
})

var SMContractRevokeRequest = Type("SMContractRevokeRequest", func() {
	Description("Contract revoke request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Required("did")
})

var SMContractRevokeResponse = Type("SMContractRevokeResponse", func() {
	Description("Result for revoking a contract")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Required("did")
})

var SMContractAuditRequest = Type("SMContractAuditRequest", func() {
	Description("Contract audit request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Required("did")
})

var SMContractAuditResponse = Type("SMContractAuditResponse", func() {
	Description("Result for auditing a contract")

	Attribute("id", Int64, "Identifier for the outbox event")
	Attribute("component", String, "Name of the component")
	Attribute("event_type", String, "Type of the event")
	Attribute("event_data", Any, "Data of the event")
	Attribute("did", String, "Decentralized Identifier of the contract template")
	Attribute("created_at", String, "The creation date of the event")
	Attribute("res_log_pred_cid", String, "Resource audit trail predecessor on the IPFS chain")
	Attribute("global_log_pred_cid", String, "Global audit trail predecessor on the IPFS chain")

	Required("id", "component", "event_type", "event_data", "created_at")
})

var SMContractComplianceRequest = Type("SMContractComplianceRequest", func() {
	Description("Contract check compliance request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Required("did")
})

var SMContractComplianceResponse = Type("SMContractComplianceResponse", func() {
	Description("Result for contract compliance checking")

	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("findings", ArrayOf(String), "A list of findings")

	Required("did")
})

// Signature Management Service  (/signature/...)
var _ = Service("SignatureManagement", func() {
	Description("Signature Management APIs (/signature/...)")

	Method("retrieve", func() {
		Description("fetch contracts.")
		Meta("dcs:requirements", "DCS-IR-SM-01")
		Meta("dcs:ui", "Secure Contract Viewer")
		Meta("dcs:sm:components", "Signer Authorization & PoA application")

		Security(JWTAuth, func() {
			Scope("Contract Signer")
			Scope("Sys. Contract Signer")
		})

		Payload(SMContractRetrieveRequest)
		Result(SMContractRetrieveResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/signature/retrieve")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	// GET /signature/retrieve/{did}
	Method("retrieve_by_id", func() {
		Description("fetch contract & envelope by id.")
		Meta("dcs:requirements", "DCS-IR-SM-01")
		Meta("dcs:ui", "Secure Contract Viewer")
		Meta("dcs:sm:components", "Signer Authorization & PoA application")

		Security(JWTAuth, func() {
			Scope("Contract Signer")
			Scope("Sys. Contract Signer")
		})

		Payload(SMContractRetrieveByIDRequest)
		Result(SMContractRetrieveByIDResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/signature/retrieve/{did}")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("verify", func() {
		Description("check contract integrity & envelope.")
		Meta("dcs:requirements", "DCS-IR-SM-02")
		Meta("dcs:ui", "Secure Contract Viewer")
		Meta("dcs:sm:components", "Counterparty Authorization & PoA verification")

		Security(JWTAuth, func() {
			Scope("Contract Signer")
			Scope("Sys. Contract Signer")
		})

		Payload(SMContractVerifyRequest)
		Result(SMContractVerifyResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/signature/verify")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("apply", func() {
		Description("apply digital signature.")
		Meta("dcs:requirements", "DCS-IR-SM-03")
		Meta("dcs:ui", "Secure Contract Viewer")
		Meta("dcs:sm:components", "Timestamping")

		Security(JWTAuth, func() {
			Scope("Contract Signer")
			Scope("Sys. Contract Signer")
		})

		Payload(SMContractApplyRequest)
		Result(SMContractApplyResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/signature/apply")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("validate", func() {
		Description("validate applied signature. validate contract signature(s).")
		Meta("dcs:requirements", "DCS-IR-SM-04", "DCS-IR-SM-05")
		Meta("dcs:ui", "Secure Contract Viewer", "Signature Compliance Viewer")
		Meta("dcs:sm:components", "Counterparty Contract Signature Verification")

		Security(JWTAuth, func() {
			Scope("Contract Signer")
			Scope("Sys. Contract Signer")
			Scope("Contract Manager")
			Scope("Sys. Contract Manager")
		})

		Payload(SMContractValidateRequest)
		Result(SMContractValidateResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/signature/validate")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("revoke", func() {
		Description("revoke a signature.")
		Meta("dcs:requirements", "DCS-IR-SM-06")
		Meta("dcs:ui", "Signature Compliance Viewer")
		Meta("dcs:sm:components", "Timestamping")

		Security(JWTAuth, func() {
			Scope("Contract Manager")
			Scope("Sys. Contract Manager")
		})

		Payload(SMContractRevokeRequest)
		Result(SMContractRevokeResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/signature/revoke")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("audit", func() {
		Description("retrieve compliance/audit logs.")
		Meta("dcs:requirements", "DCS-IR-SM-08")
		Meta("dcs:ui", "Signature Compliance Viewer")
		Meta("dcs:sm:components", "")

		Security(JWTAuth, func() {
			Scope("Auditor")
			Scope("Compliance Officer")
			Scope("System Administrator")
		})

		Payload(SMContractAuditRequest)
		Result(ArrayOfRequired(SMContractAuditResponse))

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/signature/audit")
			Param("did")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("compliance", func() {
		Description("run compliance check.")
		Meta("dcs:requirements", "DCS-IR-SM-07")
		Meta("dcs:ui", "Signature Compliance Viewer")
		Meta("dcs:sm:components", "")

		Security(JWTAuth, func() {
			Scope("Contract Manager")
			Scope("Sys. Contract Manager")
		})

		Payload(SMContractComplianceRequest)
		Result(SMContractComplianceResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/signature/compliance")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})
})
