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

var SMSignatureFieldStatus = Type("SMSignatureFieldStatus", func() {
	Description("Signing status of one declared signature field (DCS-FR-SM-07, DCS-FR-SM-24)")
	Attribute("field_name", String, "Declared signature field (dcs:signatoryName)")
	Attribute("status", String, "SIGNED or UNSIGNED")
	Attribute("remote", Boolean, "True when the field belongs to a counterparty that signs on its own instance")
	Attribute("signer_did", String, "DID of the signer, when signed on this instance")
	Attribute("signed_at", String, "ISO-8601 timestamp of the signature, when known")

	Required("field_name", "status", "remote")
})

var SMContractRetrieveByIDResponse = Type("SMContractRetrieveByIDResponse", func() {
	Attribute("contract", SMContractItem, "The contract")
	Attribute("signature_envelope", SMContractSignatureEnvelope, "The most recent signature record of any status; absent for an APPROVED-unsigned contract that has no signature yet")
	Attribute("signature_fields", ArrayOf(SMSignatureFieldStatus), "Signing status of every signature field the contract declares")

	Required("contract")
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

var SMProvenanceRequest = Type("SMProvenanceRequest", func() {
	Description("Request for a contract's C2PA provenance chain")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Required("did")
})

var SMProvenanceEntry = Type("SMProvenanceEntry", func() {
	Description("One C2PA manifest in the JUMBF store, with its dcs.lifecycle assertion")

	Attribute("label", String, "The manifest's JUMBF label (its urn:c2pa: identifier)")
	Attribute("lifecycle", MapOf(String, String), "The parsed dcs.lifecycle assertion (contract_id, status, actor, timestamp)")

	Required("label")
})

var SMProvenanceResponse = Type("SMProvenanceResponse", func() {
	Description("A contract's C2PA provenance chain, oldest manifest first")

	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("chain", ArrayOf(SMProvenanceEntry), "The manifest chain, oldest first")

	Required("did", "chain")
})

var SMContractApplyResponse = Type("SMContractApplyResponse", func() {
	Description("Result of a signature reaching SIGNED (the /signature/submit result)")

	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("signature_envelope", SMContractSignatureEnvelope, "The resulting signature envelope")

	Required("did")
})

var SMSignaturePrepareRequest = Type("SMSignaturePrepareRequest", func() {
	Description("Request to prepare the to-be-signed document for external signing")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("signer_did", String, "DID of the signer")
	Attribute("field_name", String, "For multi-signer contracts (DCS-FR-SM-07/-17): the declared signature field this signer covers.")
	Attribute("credential_type", String, "Type of credential to use (default: AES)")
	Attribute("ceremony_id", String, "The specific verified ceremony to prepare against, when the caller already has one (e.g. from startCeremony's polled result) — resolves this exact ceremony instead of \"the signer's most recent verified ceremony\", which can change if more than one ceremony is verified for the same signer/field between prepare and a later submit (ADR-20 byte pinning requires the two calls resolve the SAME ceremony).")

	Required("did", "signer_did")
})

var SMSignaturePrepareResponse = Type("SMSignaturePrepareResponse", func() {
	Description("The to-be-signed PDF, for the signatory to sign externally")

	Attribute("document", Bytes, "The unsigned PDF with the AcroForm signature field placed and the PoA/summary evidence embedded inside the byte range the signatory's signature will cover (ADR-12)")

	Required("document")
})

var SMSignatureSubmitRequest = Type("SMSignatureSubmitRequest", func() {
	Description("An externally-produced signature over the prepared document")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("signer_did", String, "DID of the signer")
	Attribute("field_name", String, "For multi-signer contracts (DCS-FR-SM-07/-17): the declared signature field this signer covers.")
	Attribute("credential_type", String, "Type of credential used (default: AES)")
	Attribute("signed_pdf", Bytes, "The signatory's PAdES-signed contract")
	Attribute("jades_signature", String, "The signatory's JAdES over the machine-readable JSON-LD (DCS-FR-SM-02/-11); empty when only the PDF was signed")
	Attribute("ceremony_id", String, "The specific ceremony this submission completes — MUST be the same ceremony_id passed to prepareSignature, so submit resolves the exact ceremony prepare pinned bytes on (ADR-20) rather than \"the signer's most recent verified ceremony\", which is ambiguous once more than one ceremony has been verified for the same signer/field.")

	Required("did", "signer_did", "signed_pdf")
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
	Attribute("dss", SMDSSReport, "EU DSS validation report for the contract's PDF signature — signer identity, signature level, timestamp, and ETSI indication (DCS-FR-SM-18/-26); absent when DSS validation is not configured or the contract carries no signed PDF")

	Required("did")
})

var SMContractRevokeRequest = Type("SMContractRevokeRequest", func() {
	Description("Contract revoke request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("signer_did", String, "DID of the signer whose signature should be revoked")
	Attribute("reason", String, "Reason for revoking the signature", func() { MinLength(1) })

	Required("did", "signer_did", "reason")
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
	Attribute("justification", String, "Required audit justification", func() { Pattern(`\S`) })

	Required("did", "justification")
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

var SMDSSReport = Type("SMDSSReport", func() {
	Description("EU DSS (ETSI EN 319 102-1) validation report for the signed contract's PDF signature — the external AdES validator's view of trust anchors, cryptographic integrity, and timestamp (DCS-FR-SM-18, DCS-IR-SM-05). Absent when DSS validation is not configured or the contract carries no signed PDF.")

	Attribute("indication", String, "ETSI EN 319 102-1 main indication: TOTAL-PASSED, INDETERMINATE, or TOTAL-FAILED")
	Attribute("sub_indication", String, "Qualifier for a non-passed indication (e.g. NO_CERTIFICATE_CHAIN_FOUND for a non-qualified CA)")
	Attribute("signed_by", String, "Subject of the signing certificate — the signer identity / credential chain the wallet used (DCS-FR-SM-26)")
	Attribute("signature_format", String, "AdES format and level DSS recognized (e.g. PAdES-BASELINE-B) — the QES/AES level evidence (DCS-FR-SM-21)")
	Attribute("signing_time", String, "Claimed/qualified signing time the signature carries (DCS-FR-SM-18 timestamp verification)")

	Required("indication")
})

var SMSignatureViewItem = Type("SMSignatureViewItem", func() {
	Description("One applied signature's metadata for the Signature Compliance Viewer (DCS-FR-SM-26): signer identity, credential class/signature level, status, timestamps, and the cryptographic integrity proof bound into the embedded ContractSigningSummaryCredential")

	Attribute("signer_did", String, "DID of the signer the signature is bound to")
	Attribute("field_name", String, "The declared signature field this signature covers (DCS-FR-SM-07/-17)")
	Attribute("credential_type", String, "Signature level / credential class (e.g. AES)")
	Attribute("status", String, "Signature status (SIGNED or REVOKED)")
	Attribute("signed_at", String, "When the signature was applied")
	Attribute("revoked_at", String, "When the signature was revoked, if it was")
	Attribute("format", String, "Signature container format")
	Attribute("jades", String, "The JAdES (ETSI TS 119 182-1) compact JWS over the machine-readable JSON-LD contract representation, the counterpart to the visible PAdES on the PDF (DCS-FR-SM-02/-11)")
	Attribute("ceremony_id", String, "The signing ceremony that produced this signature, from the embedded ContractSigningSummaryCredential")
	Attribute("content_hash", String, "SHA-256 of the JSON-LD contract source the signature covers — cryptographic integrity proof (DCS-FR-SM-26)")
	Attribute("pdf_hash", String, "SHA-256 of the base PDF bytes the signature covers — cryptographic integrity proof (DCS-FR-SM-26)")
	Attribute("kb_sd_hash", String, "KB-JWT sd_hash binding the signature to the presented credential — the credential chain link (DCS-FR-SM-26)")
	Attribute("validation_report_hash", String, "Hash of the SHACL validation report pinned at signing time (drift evidence)")
	Attribute("required_credential_type", String, "The contract's OWN declared signature-level requirement for this field (ADR-20, SM-01) — compare against credential_type (the level actually achieved) for the compliance pass/fail")
	Attribute("qualified", Boolean, "Whether DSS's qualification determination for this signature was QESIG (qualified certificate + QSCD, ADR-20) — the QES evidence distinct from credential_type alone")
	Attribute("signer_cert_subject", String, "Subject of the signing certificate that produced this signature (ADR-20 sole-control evidence, DCS-FR-SM-26)")
	Attribute("signer_cert_serial", String, "Serial number of the signing certificate that produced this signature (DCS-FR-SM-26)")
	Attribute("poa_organization", String, "Organization the signatory was authorized to act for, from the verified PoA presentation at signing (DCS-FR-SM-03/-26)")
	Attribute("poa_roles", ArrayOf(String), "Roles asserted by the verified PoA presentation at signing (DCS-FR-SM-03/-26)")
	Attribute("summary_issuer", String, "Issuer DID of the verified ContractSigningSummaryCredential that binds this signature (DCS-FR-SM-26 credential chain)")
	Attribute("credential_status", String, "Live status-list state of the Power of Attorney credential the signer presented, read at view time (DCS-FR-SM-18)", func() {
		Enum("VALID", "REVOKED", "SUSPENDED", "UNKNOWN")
	})
	Attribute("credential_status_reason", String, "Why credential_status is UNKNOWN")

	Required("signer_did", "credential_type", "status", "format", "credential_status")
})

var SMSignatureViewResponse = Type("SMSignatureViewResponse", func() {
	Description("Signature Compliance Viewer data (DCS-FR-SM-26, DCS-IR-SM-05): every applied signature's metadata plus the contract's cryptographic integrity findings and the external EU DSS validation report")

	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("contract_state", String, "Current contract lifecycle state")
	Attribute("signatures", ArrayOf(SMSignatureViewItem), "All signatures applied to the contract")
	Attribute("integrity_findings", ArrayOf(String), "Cryptographic integrity findings from the validation machinery (empty = intact)")
	Attribute("dss", SMDSSReport, "EU DSS validation report for the contract's PDF signature; absent when DSS validation is not configured or the contract carries no signed PDF")

	Required("did", "contract_state", "signatures", "integrity_findings")
})

var SMClauseReportRequest = Type("SMClauseReportRequest", func() {
	Description("Clause-segmented compliance report request (DCS-FR-PACM-07)")
	Token("token", String, "JWT token")
	Attribute("did", String, "Decentralized Identifier of the contract")
	Required("did")
})

var SMClauseReportFinding = Type("SMClauseReportFinding", func() {
	Description("One content-policy / SHACL finding with optional conservative clause attribution")
	Attribute("rule_id", String, "Stable rule identifier")
	Attribute("severity", String, "Finding severity (error/warning/info)")
	Attribute("message", String, "Human-readable finding message")
	Attribute("path", String, "SHACL result path local name when present")
	Attribute("field_iri", String, "Field IRI when present")
	Attribute("focus_node", String, "Structured SHACL focus node IRI when present")
	Attribute("source", String, "Audit source tag when present")
	Attribute("clause_ids", ArrayOf(String), "Attributed clause @ids — empty or exactly one when mapping is reliable")
	Required("severity", "message")
})

var SMClauseReportSection = Type("SMClauseReportSection", func() {
	Description("One dcs:Clause component in the clause compliance report")
	Attribute("clause_id", String, "dcs:Clause @id")
	Attribute("title", String, "Clause title when authored")
	Attribute("status", String, "not_assessed | undetermined | warnings_only | issues_identified | compliant — never compliant solely from empty findings")
	Attribute("findings", ArrayOf(SMClauseReportFinding), "Non-conformance findings attributed to this clause (excludes satisfied checks)")
	Attribute("assessment_run", String, "RFC3339 timestamp of this projection run (not proof each clause was individually assessed)")
	Attribute("applicable_checks", String, "Display label: Not established | None | comma-separated check ids")
	Attribute("applicable_checks_status", String, "not_established | none | established")
	Attribute("verdict_note", String, "Short explanation when status is undetermined or not_assessed")
	Required("clause_id", "status", "findings", "assessment_run", "applicable_checks", "applicable_checks_status")
})

var SMClauseReportCredential = Type("SMClauseReportCredential", func() {
	Description("Credential metadata of one signature over the contract; every clause is covered by the same signatures (DCS-FR-PACM-07)")
	Attribute("party", String, "Signature field (party DID) the signature covers")
	Attribute("signer_did", String, "DID of the signer")
	Attribute("signature_status", String, "SIGNED or REVOKED")
	Attribute("signed_at", String, "When the signature was applied (RFC3339)")
	Attribute("achieved_level", String, "Signature level the validated signature achieved")
	Attribute("required_level", String, "Signature level the contract requires for this party")
	Attribute("signer_cert_subject", String, "Subject of the signing certificate")
	Attribute("signer_cert_serial", String, "Serial number of the signing certificate")
	Attribute("cert_revoked_at", String, "When the CRL check found the signing certificate revoked (RFC3339)")
	Attribute("poa_organization", String, "Organization the Power of Attorney authorizes")
	Required("party", "signer_did", "signature_status", "achieved_level", "required_level")
})

var SMClauseReportResponse = Type("SMClauseReportResponse", func() {
	Description("Clause-segmented compliance projection for the Compliance Viewer By Clauses tab (DCS-FR-PACM-07)")
	Attribute("did", String, "Contract DID")
	Attribute("assessment_run", String, "RFC3339 timestamp of this projection run")
	Attribute("scope_note", String, "Human-readable scope / interpretation note")
	Attribute("clauses", ArrayOf(SMClauseReportSection), "Clause sections from the contract document")
	Attribute("unassigned_findings", ArrayOf(SMClauseReportFinding), "Findings that could not be attributed to exactly one clause")
	Attribute("credentials", ArrayOf(SMClauseReportCredential), "Credential metadata of the signatures over the contract")
	Required("did", "assessment_run", "clauses", "unassigned_findings", "credentials")
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
	Attribute("signed_at", String, "ISO-8601 timestamp when a signature submitted for this ceremony was accepted; absent until then")
	Attribute("request_expires_at", String, "ISO-8601 timestamp when the published signing request expires; absent until a request is published")
	Attribute("refusal_code", String, "Error code of the last refused wallet response for this ceremony")
	Attribute("refusal", String, "Reason of the last refused wallet response for this ceremony")

	Required("ceremony_id", "status")
})

var SMSignatureRequestPublishRequest = Type("SMSignatureRequestPublishRequest", func() {
	Description("Publish the OID4VP Document-Retrieval signing request for a verified ceremony (ADR-12): prepare the to-be-signed document and emit the signed request object as a QR/deep link.")

	Token("token", String, "JWT token")

	Attribute("ceremony_id", String, "Identifier of the verified ceremony to publish a signing request for")
	Attribute("credential_type", String, "Type of credential the signatory uses (default: AES)")

	Required("ceremony_id")
})

var SMSignatureRequestPublishResponse = Type("SMSignatureRequestPublishResponse", func() {
	Description("The published OID4VP Document-Retrieval request as QR/deep-link data (ADR-12). The wallet fetches the request object at request_uri, fetches the to-be-signed document it references, signs it with the signatory's own key, and posts the signed document back to the request object's response_uri.")

	Attribute("ceremony_id", String, "Identifier of the ceremony")
	Attribute("client_id", String, "OpenID4VP client identifier of the DCS relying party the request object is bound to, prefix included (x509_san_dns:<dns-name>)")
	Attribute("request_uri", String, "HTTPS URL the wallet fetches the signed OID4VP request object (JAR) from")
	Attribute("wallet_uri", String, "openid4vp:// deep link (request-by-reference) the signer's wallet opens")
	Attribute("nonce", String, "Fresh nonce bound into the request object")
	Attribute("expires_at", String, "ISO-8601 timestamp when the signing request expires")

	Required("ceremony_id", "client_id", "request_uri", "wallet_uri", "expires_at")
})

var SMSignatureRequestCallbackResponse = Type("SMSignatureRequestCallbackResponse", func() {
	Description("Result of accepting the wallet's signed document at the ceremony callback")

	Attribute("ceremony_id", String, "Identifier of the ceremony")
	Attribute("did", String, "DID of the contract the ceremony signed")
	Attribute("status", String, "Contract lifecycle state after finalizing the signature")

	Required("ceremony_id", "status")
})

// Signature Management Service  (/signature/...)
var _ = Service("SignatureManagement", func() {
	Description("Signature Management APIs (/signature/...)")

	Method("retrieve", func() {
		Description("fetch contracts, recording an audit-trail entry for the read.")
		Meta("dcs:requirements", "DCS-IR-SM-01")

		Security(JWTAuth, func() {
			Scope("Contract Signer")
			Scope("Sys. Contract Signer")
			Scope("Contract Observer")
			Scope("Contract Manager")
			Scope("Sys. Contract Manager")
			Scope("Auditor")
			Scope("Compliance Officer")
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

		Error("forbidden", ErrorResult)
		Error("not_found", ErrorResult)
		HTTP(func() {
			Response("not_found", StatusNotFound)
			Response("forbidden", StatusForbidden)
			GET("/signature/retrieve/{did}")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("verify", func() {
		Description("check contract integrity & envelope.")
		Meta("dcs:requirements", "DCS-IR-SM-02")

		// Read-only integrity check over the envelope retrieve_by_id already
		// hands these roles; the Secure Contract Viewer offers it as step 2 to
		// signer and manager alike.
		Security(JWTAuth, func() {
			Scope("Contract Signer")
			Scope("Sys. Contract Signer")
			Scope("Contract Manager")
			Scope("Sys. Contract Manager")
		})

		Payload(SMContractVerifyRequest)
		Result(SMContractVerifyResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		Error("forbidden", ErrorResult)
		Error("not_found", ErrorResult)
		HTTP(func() {
			Response("not_found", StatusNotFound)
			Response("forbidden", StatusForbidden)
			POST("/signature/verify")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("provenance", func() {
		Description("The C2PA provenance chain embedded in the signed/exported contract PDF (DCS-OR-C2PA-008): one entry per manifest in the JUMBF store, oldest first, with its dcs.lifecycle assertion. Powers the Secure Contract Viewer's provenance display.")
		Meta("dcs:requirements", "DCS-OR-C2PA-008")

		Security(JWTAuth, func() {
			Scope("Contract Signer")
			Scope("Sys. Contract Signer")
			Scope("Contract Manager")
			Scope("Sys. Contract Manager")
			Scope("Contract Observer")
			Scope("Auditor")
		})

		Payload(SMProvenanceRequest)
		Result(SMProvenanceResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		Error("forbidden", ErrorResult)
		Error("not_found", ErrorResult)
		HTTP(func() {
			Response("not_found", StatusNotFound)
			Response("forbidden", StatusForbidden)
			GET("/signature/provenance/{did}")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("prepareSignature", func() {
		Description("prepare the to-be-signed document for the signatory to sign externally — with their wallet/QTSP or a desktop PAdES signer. The DCS embeds the PoA/summary, places the AcroForm field, and returns the unsigned PDF; it applies no signature and holds no signing key (ADR-12, FR-SM-16).")
		Meta("dcs:requirements", "DCS-FR-SM-16")

		Security(JWTAuth, func() {
			Scope("Contract Signer")
		})

		Payload(SMSignaturePrepareRequest)
		Result(SMSignaturePrepareResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("ceremony_required", ErrorResult, "No completed PID presentation ceremony exists for this signer and contract")
		Error("counterparty_not_settled", ErrorResult, "No verified settlement from the counterparty is held for the version about to be signed — the contract is waiting for the counterparty, not for this signer")
		Error("forbidden", ErrorResult, "The caller may not read the contract or did not start the ceremony")
		Error("not_found", ErrorResult, "Contract not found")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/signature/prepare")
			Response(StatusOK)
			Response("forbidden", StatusForbidden)
			Response("not_found", StatusNotFound)
			Response("bad_request", StatusBadRequest)
			Response("ceremony_required", StatusUnprocessableEntity)
			Response("counterparty_not_settled", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("preparedDocument", func() {
		Description("serve the to-be-signed PDF that prepareSignature pinned on a ceremony as application/pdf (DCS-IR-CI-02). Only the session that started the ceremony reads it.")
		Meta("dcs:requirements", "DCS-IR-CI-02", "DCS-FR-SM-16")

		Security(JWTAuth, func() {
			Scope("Contract Signer")
		})

		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("ceremony_id", String, "The ceremony prepareSignature pinned the document on")
			Required("ceremony_id")
		})

		Error("forbidden", ErrorResult, "The caller did not start the ceremony")
		Error("not_found", ErrorResult, "No prepared document for this ceremony")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/signature/prepare/{ceremony_id}/document")
			SkipResponseBodyEncodeDecode()
			Response(StatusOK, func() {
				ContentType("application/pdf")
			})
			Response("forbidden", StatusForbidden)
			Response("not_found", StatusNotFound)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("submitSignature", func() {
		Description("accept a signature the signatory produced externally (wallet/QTSP or desktop PAdES signer) and finalize the contract once it validates and its certificate identifies the signatory (sole control, ADR-12, FR-SM-16/-18).")
		Meta("dcs:requirements", "DCS-FR-SM-16")

		Security(JWTAuth, func() {
			Scope("Contract Signer")
		})

		Payload(SMSignatureSubmitRequest)
		Result(SMContractApplyResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("ceremony_required", ErrorResult, "No completed PID presentation ceremony exists for this signer and contract")
		Error("signature_invalid", ErrorResult, "The submitted signature is not cryptographically valid")
		Error("document_mismatch", ErrorResult, "The submitted document's initial revision does not byte-match the document prepared for signing (ADR-20 TBS byte pinning)")
		Error("nonce_mismatch", ErrorResult, "The submitted signature is not bound to the ceremony's request nonce (ADR-20 nonce binding)")
		Error("level_below_required", ErrorResult, "The submitted signature's level does not meet the contract's required signature level (SM-01)")
		Error("cert_pid_mismatch", ErrorResult, "The signing certificate does not identify the ceremony's verified signatory (sole control)")
		Error("jades_invalid", ErrorResult, "The submitted JAdES signature is invalid")
		Error("counterparty_not_settled", ErrorResult, "No verified settlement from the counterparty is held for the version about to be signed — the contract is waiting for the counterparty, not for this signer")
		Error("forbidden", ErrorResult, "The caller may not read the contract or did not start the ceremony")
		Error("not_found", ErrorResult, "Contract not found")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/signature/submit")
			Response(StatusOK)
			Response("forbidden", StatusForbidden)
			Response("not_found", StatusNotFound)
			Response("bad_request", StatusBadRequest)
			Response("ceremony_required", StatusUnprocessableEntity)
			Response("counterparty_not_settled", StatusBadRequest)
			Response("signature_invalid", StatusUnprocessableEntity)
			Response("document_mismatch", StatusUnprocessableEntity)
			Response("nonce_mismatch", StatusUnprocessableEntity)
			Response("level_below_required", StatusUnprocessableEntity)
			Response("cert_pid_mismatch", StatusUnprocessableEntity)
			Response("jades_invalid", StatusUnprocessableEntity)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("startCeremony", func() {
		Description("start a signing ceremony that requests a PID presentation from the signer's wallet (FR-SM-14, UC-04-02).")
		Meta("dcs:requirements", "DCS-FR-SM-16")

		Security(JWTAuth, func() {
			Scope("Contract Signer")
		})

		Payload(SMSignatureRequestStartRequest)
		Result(SMSignatureRequestStartResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("forbidden", ErrorResult, "The caller may not read the contract or did not start the ceremony")
		Error("not_found", ErrorResult, "Contract not found")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/signature/request")
			Response(StatusOK)
			Response("forbidden", StatusForbidden)
			Response("not_found", StatusNotFound)
			Response("bad_request", StatusBadRequest)
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

		Error("forbidden", ErrorResult)
		HTTP(func() {
			Response("forbidden", StatusForbidden)
			GET("/signature/request/{ceremony_id}")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("not_found", StatusNotFound)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("publishSignatureRequest", func() {
		Description("publish the OID4VP Document-Retrieval signing request for a verified ceremony (ADR-12): run prepare to produce the to-be-signed document, store it on the ceremony, and emit the signed request object as a QR/deep link. The wallet then fetches the request object + document, signs, and posts back to the callback.")
		Meta("dcs:requirements", "DCS-FR-SM-16", "DCS-IR-SI-04")

		Security(JWTAuth, func() {
			Scope("Contract Signer")
		})

		Payload(SMSignatureRequestPublishRequest)
		Result(SMSignatureRequestPublishResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("not_found", ErrorResult, "Ceremony not found")
		Error("ceremony_required", ErrorResult, "No completed PID presentation ceremony exists for this signer and contract")
		Error("counterparty_not_settled", ErrorResult, "No verified settlement from the counterparty is held for the version about to be signed — the contract is waiting for the counterparty, not for this signer")
		Error("forbidden", ErrorResult, "The caller may not read the contract or did not start the ceremony")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/signature/request/{ceremony_id}/publish")
			Response(StatusOK)
			Response("forbidden", StatusForbidden)
			Response("bad_request", StatusBadRequest)
			Response("not_found", StatusNotFound)
			Response("ceremony_required", StatusUnprocessableEntity)
			Response("counterparty_not_settled", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("signatureRequestObject", func() {
		Description("serve the signed OID4VP Document-Retrieval request object (JAR) the wallet fetches by reference (ADR-12).")
		Meta("dcs:requirements", "DCS-FR-SM-16", "DCS-IR-SI-04")

		NoSecurity()

		Payload(func() {
			Attribute("ceremony_id", String, "Identifier of the ceremony whose signing request object is served")
			Attribute("wallet_nonce", String, "Wallet-provided nonce echoed in the authorization request object when request_uri_method=post")
			Attribute("wallet_metadata", String, "Wallet metadata JSON submitted with request_uri_method=post")
			Required("ceremony_id")
		})

		Error("bad_request", ErrorResult, "Bad request")
		Error("not_found", ErrorResult, "Ceremony not found or not published")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/signature/request/{ceremony_id}/object")
			GET("/signature/request/{ceremony_id}/object")
			SkipResponseBodyEncodeDecode()
			Response(StatusOK, func() {
				ContentType("application/oauth-authz-req+jwt")
			})
			Response("bad_request", StatusBadRequest)
			Response("not_found", StatusNotFound)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("signatureRequestDocument", func() {
		Description("serve the to-be-signed PDF the wallet fetches from the request object's document_locations (ADR-12).")
		Meta("dcs:requirements", "DCS-FR-SM-16", "DCS-IR-SI-04")

		NoSecurity()

		Payload(func() {
			Attribute("ceremony_id", String, "Identifier of the ceremony whose to-be-signed document is served")
			Required("ceremony_id")
		})

		Error("bad_request", ErrorResult, "Bad request")
		Error("not_found", ErrorResult, "Ceremony not found or not published")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/signature/request/{ceremony_id}/document")
			SkipResponseBodyEncodeDecode()
			Response(StatusOK, func() {
				ContentType("application/pdf")
			})
			Response("bad_request", StatusBadRequest)
			Response("not_found", StatusNotFound)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("signatureRequestPayload", func() {
		Description("serve the canonical machine-readable JSON-LD contract payload the wallet fetches from the request object's second document_locations entry, to produce the JAdES twin of the PAdES over the same content hash (ADR-12 SM-02/-11, ADR-20 nonce binding).")
		Meta("dcs:requirements", "DCS-FR-SM-02", "DCS-FR-SM-11", "DCS-IR-SI-04")

		NoSecurity()

		Payload(func() {
			Attribute("ceremony_id", String, "Identifier of the ceremony whose to-be-signed payload is served")
			Required("ceremony_id")
		})

		Error("bad_request", ErrorResult, "Bad request")
		Error("not_found", ErrorResult, "Ceremony not found, not published, or has no pinned payload")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/signature/request/{ceremony_id}/payload")
			SkipResponseBodyEncodeDecode()
			Response(StatusOK, func() {
				ContentType("application/json")
			})
			Response("bad_request", StatusBadRequest)
			Response("not_found", StatusNotFound)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("signatureRequestCallback", func() {
		Description("accept the wallet's direct_post of the signed document at the request object's response_uri (ADR-12): validate it identifies the signatory (sole control) and finalize the contract, reusing the /signature/submit validate+finalize path. The wallet posts the EUDI walletdriven-signer form-urlencoded body (documentWithSignature[]/signatureObject[]/state/error), which the service parses off the raw request. Authenticated by the unguessable ceremony id, not a JWT (the caller is the signatory's wallet).")
		Meta("dcs:requirements", "DCS-FR-SM-16", "DCS-FR-SM-18", "DCS-IR-SI-04")

		NoSecurity()

		Payload(func() {
			Attribute("ceremony_id", String, "Identifier of the ceremony this signing request belongs to")
			Required("ceremony_id")
		})
		Result(SMSignatureRequestCallbackResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("not_found", ErrorResult, "Ceremony not found or not published")
		Error("signature_invalid", ErrorResult, "The submitted signature is not cryptographically valid")
		Error("document_mismatch", ErrorResult, "The submitted document's initial revision does not byte-match the document prepared for signing (ADR-20 TBS byte pinning)")
		Error("nonce_mismatch", ErrorResult, "The submitted signature is not bound to the ceremony's request nonce (ADR-20 nonce binding)")
		Error("level_below_required", ErrorResult, "The submitted signature's level does not meet the contract's required signature level (SM-01)")
		Error("cert_pid_mismatch", ErrorResult, "The signing certificate does not identify the ceremony's verified signatory (sole control)")
		Error("jades_invalid", ErrorResult, "The submitted JAdES signature is invalid")
		Error("counterparty_not_settled", ErrorResult, "No verified settlement from the counterparty is held for the version about to be signed — the contract is waiting for the counterparty, not for this signer")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/signature/request/{ceremony_id}/callback")
			SkipRequestBodyEncodeDecode()
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("not_found", StatusNotFound)
			Response("counterparty_not_settled", StatusBadRequest)
			Response("signature_invalid", StatusUnprocessableEntity)
			Response("document_mismatch", StatusUnprocessableEntity)
			Response("nonce_mismatch", StatusUnprocessableEntity)
			Response("level_below_required", StatusUnprocessableEntity)
			Response("cert_pid_mismatch", StatusUnprocessableEntity)
			Response("jades_invalid", StatusUnprocessableEntity)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("validate", func() {
		Description("validate the contract's applied signature(s) and return any compliance findings.")
		Meta("dcs:requirements", "DCS-IR-SM-04", "DCS-IR-SM-05")

		// The signer closes the Secure Contract Viewer wizard on the signature
		// they just applied; compliance users validate in the Signature
		// Compliance Viewer. Validation is a read-only check.
		Security(JWTAuth, func() {
			Scope("Contract Manager")
			Scope("Sys. Contract Manager")
			Scope("Contract Signer")
			Scope("Sys. Contract Signer")
			Scope("Auditor")
			Scope("Compliance Officer")
		})

		Payload(SMContractValidateRequest)
		Result(SMContractValidateResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		Error("forbidden", ErrorResult)
		Error("not_found", ErrorResult)
		HTTP(func() {
			Response("not_found", StatusNotFound)
			Response("forbidden", StatusForbidden)
			POST("/signature/validate")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("revoke", func() {
		Description("revoke a signature.")
		Meta("dcs:requirements", "DCS-IR-SM-06")

		Security(JWTAuth, func() {
			Scope("Contract Manager")
			Scope("Sys. Contract Manager")
			Scope("Auditor")
			Scope("Compliance Officer")
		})

		Payload(SMContractRevokeRequest)
		Result(SMContractRevokeResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		Error("forbidden", ErrorResult)
		Error("not_found", ErrorResult)
		HTTP(func() {
			Response("not_found", StatusNotFound)
			Response("forbidden", StatusForbidden)
			POST("/signature/revoke")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("audit", func() {
		Description("retrieve compliance/audit logs.")
		Meta("dcs:requirements", "DCS-IR-SM-08")

		Security(JWTAuth, func() {
			Scope("Auditor")
			Scope("Compliance Officer")
		})

		Payload(SMContractAuditRequest)
		Result(ArrayOfRequired(SMContractAuditResponse))

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		Error("forbidden", ErrorResult)
		Error("not_found", ErrorResult)
		HTTP(func() {
			Response("not_found", StatusNotFound)
			Response("forbidden", StatusForbidden)
			GET("/signature/audit")
			Param("did")
			Param("justification")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("view", func() {
		Description("Signature Compliance Viewer (DCS-FR-SM-26): per-signature signer identity, credential class/signature level, status, and timestamps, plus the contract's cryptographic integrity findings — the data behind the viewer UI.")
		Meta("dcs:requirements", "DCS-FR-SM-26", "DCS-IR-SM-05")

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

		Error("forbidden", ErrorResult)
		Error("not_found", ErrorResult)
		HTTP(func() {
			Response("not_found", StatusNotFound)
			Response("forbidden", StatusForbidden)
			GET("/signature/view")
			Param("did")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("compliance", func() {
		Description("Run the signature compliance checks for the contract (DCS-FR-SM-21: achieved against required signature level, signature status, certificate and Power of Attorney status, Power of Attorney organization and roles) and return the findings; the check — findings included — is recorded as a ComplianceValidationEvent in the audit trail.")
		Meta("dcs:requirements", "DCS-IR-SM-07")

		Security(JWTAuth, func() {
			Scope("Contract Manager")
			Scope("Sys. Contract Manager")
			Scope("Auditor")
			Scope("Compliance Officer")
		})

		Payload(SMContractComplianceRequest)
		Result(SMContractComplianceResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		Error("forbidden", ErrorResult)
		Error("not_found", ErrorResult)
		HTTP(func() {
			Response("not_found", StatusNotFound)
			Response("forbidden", StatusForbidden)
			POST("/signature/compliance")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("clause_report", func() {
		Description("Compliance report by clauses.")
		Meta("dcs:requirements", "DCS-FR-PACM-07")

		Security(JWTAuth, func() {
			Scope("Contract Manager")
			Scope("Sys. Contract Manager")
			Scope("Contract Signer")
			Scope("Sys. Contract Signer")
			Scope("Contract Observer")
			Scope("Auditor")
			Scope("Compliance Officer")
		})

		Payload(SMClauseReportRequest)
		Result(SMClauseReportResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		Error("forbidden", ErrorResult)
		Error("not_found", ErrorResult)
		HTTP(func() {
			Response("not_found", StatusNotFound)
			Response("forbidden", StatusForbidden)
			GET("/signature/clause-report")
			Param("did")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})
})
