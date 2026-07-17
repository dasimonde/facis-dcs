package design

import (
	. "goa.design/goa/v3/dsl"
)

var SMContractRetrieveRequest = Type("SMContractRetrieveRequest", func() {
	Description("Contract retrieve request")

	Token("token", String, "JWT token")

	//	Attribute("offset", Int, "Start index of results")
	//	Attribute("limit", Int, "Page size of results")
})

var SMContractListItem = Type("SMContractListItem", func() {
	Attribute("did", String, "DID of the contract")
	Attribute("contract_version", Int, "The version of the contract")
	Attribute("state", String, "Current state of the contract")
	Attribute("name", String, "The name of the contract")
	Attribute("description", String, "The description of the contract")
	Attribute("created_by", String, "Identifier of who created the contract negotiation")
	Attribute("created_at", String, "Created at")
	Attribute("updated_at", String, "Updated at")
	Attribute("start_date", String, "The timestamp when the contract starts")
	Attribute("exp_date", String, "The timestamp when the contract expired")
	Attribute("exp_policy", String, "The policy what should happen if the contract is expired")
	Attribute("exp_notice_period", Int, "The notice period before contract expiration (in days)")
	Attribute("responsible", Any, "Responsible for this contract, including the creator, approvers, reviewers, and negotiators")

	Required("did", "state", "created_by", "created_at", "updated_at", "contract_version")
})

var SMContractSigningTaskItem = Type("SMContractSigningTaskItem", func() {
	Attribute("did", String, "DID of the contract")
	Attribute("contract_version", Int, "The version of the contract")
	Attribute("state", String, "State of the review task")
	Attribute("signer", String, "The reviewer of the contract")
	Attribute("created_at", String, "Created at")

	Required("did", "state", "signer", "created_at", "contract_version")
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
	Attribute("created_by", String, "Identifier of who created the contract negotiation")
	Attribute("created_at", String, "Created at")
	Attribute("updated_at", String, "Updated at")
	Attribute("start_date", String, "The timestamp when the contract starts")
	Attribute("exp_date", String, "The timestamp when the contract expired")
	Attribute("exp_policy", String, "The policy what should happen if the contract is expired")
	Attribute("exp_notice_period", Int, "The notice period before contract expiration (in days)")
	Attribute("responsible", Any, "Responsible for this contract, including the creator, approvers, reviewers, and negotiators")
	Attribute("contract_data", Any, "The data of that contract")

	Required("did", "state", "created_by", "created_at", "updated_at", "contract_version")
})

var SMContractSignatureEnvelope = Type("SMContractSignatureEnvelope", func() {
	Attribute("contract_did", String, "DID of the contract")
	Attribute("signer_did", String, "DID of the signer")
	Attribute("credential_type", String, "Type of credential used for signing")
	Attribute("status", String, "Signature status: PENDING, SIGNED, REVOKED")
	Attribute("signed_at", String, "ISO-8601 timestamp of signing")
	Attribute("revoked_at", String, "ISO-8601 timestamp of revocation, if applicable")
	Attribute("ipfs_cid", String, "IPFS CID of stored signature artefact, if uploaded")

	Required("contract_did", "signer_did", "credential_type", "status")
})

var SMContractRetrieveByIDResponse = Type("SMContractRetrieveByIDResponse", func() {
	Attribute("contract", SMContractItem, "The contract")
	Attribute("signature_envelope", SMContractSignatureEnvelope, "The signature_envelope of the contract")
	Attribute("key_version", Int, "HSM key version that produced the latest signature (DCS-OR-C2PA-007)")

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
	Attribute("match", Boolean, "True if re-generated PDF hash matches stored PDF hash (DCS-FR-CWE-04)")
	Attribute("jsonld_hash", String, "SHA-256 hex of the JSON-LD source")
	Attribute("base_pdf_hash", String, "SHA-256 hex of the re-generated base PDF")
	Attribute("sig_count", Int, "Number of active (non-revoked) signatures")
	Attribute("findings", ArrayOf(String), "A list of findings")

	Required("did", "match", "sig_count")
})

var SMContractApplyRequest = Type("SMContractApplyRequest", func() {
	Description("Contract apply request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("signer_did", String, "DID of the signer")
	Attribute("field_name", String, "For multi-signer contracts (DCS-FR-SM-07/-17): the declared signature field this signer covers. When omitted, the signer's most recent verified ceremony is used (single-signer flow).")
	Attribute("credential_type", String, "Type of credential to use (default: AES)")
	Attribute("updated_at", String, "The timestamp when the contract was updated")

	Required("did", "signer_did", "updated_at")
})

var SMContractApplyResponse = Type("SMContractApplyResponse", func() {
	Description("Result of applying a signature")

	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("signature_envelope", SMContractSignatureEnvelope, "The resulting signature envelope")

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
	Attribute("signer_did", String, "DID of the signer whose signature should be revoked")

	Required("did", "signer_did")
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

var SMSignatureViewRequest = Type("SMSignatureViewRequest", func() {
	Description("Signature Compliance Viewer request (DCS-FR-SM-26)")

	Token("token", String, "JWT token")
	Attribute("did", String, "Decentralized Identifier of the contract")
	Required("did")
})

var SMSignatureViewItem = Type("SMSignatureViewItem", func() {
	Description("One applied signature's metadata for the Signature Compliance Viewer (DCS-FR-SM-26): signer identity, credential class/signature level, status, and timestamps")

	Attribute("signer_did", String, "DID of the signer the signature is bound to")
	Attribute("field_name", String, "The declared signature field this signature covers (DCS-FR-SM-07/-17)")
	Attribute("credential_type", String, "Signature level / credential class (e.g. AES)")
	Attribute("status", String, "Signature status (SIGNED or REVOKED)")
	Attribute("signed_at", String, "When the signature was applied")
	Attribute("revoked_at", String, "When the signature was revoked, if it was")
	Attribute("format", String, "Signature container format")

	Required("signer_did", "credential_type", "status", "format")
})

var SMSignatureViewResponse = Type("SMSignatureViewResponse", func() {
	Description("Signature Compliance Viewer data (DCS-FR-SM-26, DCS-IR-SM-05): every applied signature's metadata plus the contract's cryptographic integrity findings")

	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("contract_state", String, "Current contract lifecycle state")
	Attribute("signatures", ArrayOf(SMSignatureViewItem), "All signatures applied to the contract")
	Attribute("integrity_findings", ArrayOf(String), "Cryptographic integrity findings from the validation machinery (empty = intact)")

	Required("did", "contract_state", "signatures", "integrity_findings")
})

var SMSignatureRequestStartRequest = Type("SMSignatureRequestStartRequest", func() {
	Description("Start a signing ceremony: request a PID presentation from the signer's wallet (FR-SM-14, UC-04-02)")

	Token("token", String, "JWT token")

	Attribute("contract_did", String, "DID of the contract to be signed")
	Attribute("field_name", String, "Name of the AcroForm signature field the ceremony binds")

	Required("contract_did", "field_name")
})

var SMSignatureRequestStartResponse = Type("SMSignatureRequestStartResponse", func() {
	Description("The started signing ceremony (FR-SM-14)")

	Attribute("ceremony_id", String, "Identifier of the started ceremony")
	Attribute("wallet_uri", String, "OID4VP request URI the signer's wallet opens to present the PID")
	Attribute("expires_at", String, "ISO-8601 timestamp when the ceremony expires")
	Attribute("status", String, "Ceremony lifecycle status")

	Required("ceremony_id", "wallet_uri", "expires_at", "status")
})

var SMSignatureRequestStatusRequest = Type("SMSignatureRequestStatusRequest", func() {
	Description("Poll a signing ceremony's lifecycle status (FR-SM-14)")

	Token("token", String, "JWT token")

	Attribute("ceremony_id", String, "Identifier of the ceremony")

	Required("ceremony_id")
})

var SMSignatureRequestStatusResponse = Type("SMSignatureRequestStatusResponse", func() {
	Description("A signing ceremony's current lifecycle status (FR-SM-14)")

	Attribute("ceremony_id", String, "Identifier of the ceremony")
	Attribute("contract_did", String, "DID of the contract the ceremony binds")
	Attribute("field_name", String, "Name of the AcroForm signature field")
	Attribute("status", String, "Ceremony lifecycle status: pending, verified, expired, failed")
	Attribute("signer_did", String, "DID of the signer resolved from the presented PID, once verified")
	Attribute("expires_at", String, "ISO-8601 timestamp when the ceremony expires")

	Required("ceremony_id", "status")
})

var SMSignatureWebhookRequest = Type("SMSignatureWebhookRequest", func() {
	Description("EUDIPLO OID4VP webhook: a completed PID presentation for a ceremony (NFR-SEC-18, FR-SM-14)")

	Attribute("webhook_secret", String, "Shared secret authenticating the webhook caller")
	Attribute("ceremony_id", String, "Identifier of the ceremony the presentation completes")
	Attribute("vp_token", String, "The SD-JWT VC + KB-JWT compact PID presentation")
	Attribute("pid_claims", Any, "The disclosed PID claims (sub, given_name, family_name)")

	Required("ceremony_id", "vp_token")
})

var SMSignatureWebhookResponse = Type("SMSignatureWebhookResponse", func() {
	Description("Result of accepting a EUDIPLO webhook presentation")

	Attribute("ceremony_id", String, "Identifier of the ceremony")
	Attribute("status", String, "Ceremony lifecycle status after processing the presentation")

	Required("ceremony_id", "status")
})

// Signature Management Service  (/signature/...)
var _ = Service("SignatureManagement", func() {
	Description("Signature Management APIs (/signature/...)")

	Method("retrieve", func() {
		Description("fetch contracts, recording an audit-trail entry for the read.")
		Meta("dcs:requirements", "DCS-IR-SM-01")
		Meta("dcs:ui", "Secure Contract Viewer")
		Meta("dcs:sm:components", "Signer Authorization & PoA application")

		Security(JWTAuth, func() {
			Scope("Contract Signer")
			Scope("Sys. Contract Signer")
			Scope("Contract Observer")
		})

		Payload(SMContractRetrieveRequest)
		Result(SMContractRetrieveResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/signature/retrieve")
			//			Param("offset")
			//			Param("limit")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	// GET /signature/retrieve/{did}
	Method("retrieve_by_id", func() {
		Description("fetch a contract and its signature envelope by DID, recording an audit-trail entry for the read.")
		Meta("dcs:requirements", "DCS-IR-SM-01")
		Meta("dcs:ui", "Secure Contract Viewer")
		Meta("dcs:sm:components", "Signer Authorization & PoA application")

		Security(JWTAuth, func() {
			Scope("Contract Signer")
			Scope("Sys. Contract Signer")
			Scope("Contract Observer")
			Scope("Contract Manager")
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
		Error("ceremony_required", ErrorResult, "No completed PID presentation ceremony exists for this signer and contract")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/signature/apply")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("ceremony_required", StatusUnprocessableEntity)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("startCeremony", func() {
		Description("start a signing ceremony that requests a PID presentation from the signer's wallet (FR-SM-14, UC-04-02).")
		Meta("dcs:requirements", "DCS-FR-SM-16")

		Security(JWTAuth, func() {
			Scope("Contract Signer")
			Scope("Sys. Contract Signer")
		})

		Payload(SMSignatureRequestStartRequest)
		Result(SMSignatureRequestStartResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/signature/request")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("presentationRequest", func() {
		Description("Returns a signed OpenID4VP authorization request JWT for a signing ceremony.")
		Meta("dcs:requirements", "DCS-FR-SM-16")
		NoSecurity()
		Payload(func() {
			Attribute("ceremony_id", String, "Signing ceremony identifier")
			Attribute("wallet_nonce", String, "Wallet-provided nonce echoed in the authorization request object when request_uri_method=post")
			Attribute("wallet_metadata", String, "Wallet metadata JSON submitted with request_uri_method=post")
			Required("ceremony_id")
		})
		Error("bad_request", ErrorResult, "Bad request")
		Error("not_found", ErrorResult, "Ceremony not found")
		Error("internal_error", ErrorResult, "Internal server error")
		HTTP(func() {
			POST("/signature/presentation/request/{ceremony_id}")
			GET("/signature/presentation/request/{ceremony_id}")
			SkipResponseBodyEncodeDecode()
			Response(StatusOK, func() {
				ContentType("application/oauth-authz-req+jwt")
			})
			Response("bad_request", StatusBadRequest)
			Response("not_found", StatusNotFound)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("ceremonyStatus", func() {
		Description("report a signing ceremony's lifecycle status (FR-SM-14).")
		Meta("dcs:requirements", "DCS-FR-SM-16")

		Security(JWTAuth, func() {
			Scope("Contract Signer")
			Scope("Sys. Contract Signer")
			Scope("Contract Manager")
			Scope("Contract Observer")
		})

		Payload(SMSignatureRequestStatusRequest)
		Result(SMSignatureRequestStatusResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("not_found", ErrorResult, "Ceremony not found")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/signature/request/{ceremony_id}")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("not_found", StatusNotFound)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("ceremonyWebhook", func() {
		Description("accept a EUDIPLO OID4VP webhook carrying a completed PID presentation for a ceremony; authenticated by a shared-secret header, not a JWT (NFR-SEC-18, FR-SM-14).")
		Meta("dcs:requirements", "DCS-FR-SM-16")

		NoSecurity()

		Payload(SMSignatureWebhookRequest)
		Result(SMSignatureWebhookResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("unauthorized", ErrorResult, "Missing or incorrect webhook shared secret")
		Error("not_found", ErrorResult, "Ceremony not found")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/signature/request/webhook")
			Header("webhook_secret:X-EUDIPLO-Webhook-Secret")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("unauthorized", StatusUnauthorized)
			Response("not_found", StatusNotFound)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("validate", func() {
		Description("validate the contract's applied signature(s) and return any compliance findings.")
		Meta("dcs:requirements", "DCS-IR-SM-04", "DCS-IR-SM-05")
		Meta("dcs:ui", "Secure Contract Viewer", "Signature Compliance Viewer")
		Meta("dcs:sm:components", "Counterparty Contract Signature Verification")

		Security(JWTAuth, func() {
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
		Meta("dcs:sm:components", "Counterparty Contract Signature Verification")

		Security(JWTAuth, func() {
			Scope("Auditor")
			Scope("Compliance Officer")
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

	Method("view", func() {
		Description("Signature Compliance Viewer (DCS-FR-SM-26): per-signature signer identity, credential class/signature level, status, and timestamps, plus the contract's cryptographic integrity findings — the data behind the viewer UI.")
		Meta("dcs:requirements", "DCS-FR-SM-26", "DCS-IR-SM-05")
		Meta("dcs:ui", "Signature Compliance Viewer")
		Meta("dcs:sm:components", "Counterparty Contract Signature Verification")

		Security(JWTAuth, func() {
			Scope("Contract Manager")
			Scope("Sys. Contract Manager")
			Scope("Contract Signer")
			Scope("Sys. Contract Signer")
			Scope("Contract Observer")
			Scope("Auditor")
			Scope("Compliance Officer")
		})

		Payload(SMSignatureViewRequest)
		Result(SMSignatureViewResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/signature/view")
			Param("did")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("compliance", func() {
		Description("Run the signature compliance checks for the contract (DCS-FR-SM-21: signature level SES/AES/QES, signature status, presence of active signed credentials) and return the findings; the check — findings included — is recorded as a ComplianceValidationEvent in the audit trail.")
		Meta("dcs:requirements", "DCS-IR-SM-07")
		Meta("dcs:ui", "Signature Compliance Viewer")
		Meta("dcs:sm:components", "Counterparty Contract Signature Verification")

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
