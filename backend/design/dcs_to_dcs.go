package design

import (
	. "goa.design/goa/v3/dsl"
)

var DCSToDCSEphemeralPublicKey = Type("DCSToDCSEphemeralPublicKey", func() {
	Description("The sender's ephemeral P-256 public key of an ECDH-ES+A256KW CEK wrap, as a JWK")

	Attribute("kty", String, "JWK key type (EC)")
	Attribute("crv", String, "JWK curve (P-256)")
	Attribute("x", String, "Base64url-encoded X coordinate")
	Attribute("y", String, "Base64url-encoded Y coordinate")

	Required("kty", "crv", "x", "y")
})

var DCSToDCSWrappedCEK = Type("DCSToDCSWrappedCEK", func() {
	Description("The contract's content-encryption key wrapped to the receiving peer's keyAgreement public key via ECDH-ES+A256KW (DCS-NFR-SEC-14) — the same shape a content_encryption_keys.wrapped_cek record has. The receiver unwraps it with its HSM and re-wraps it to its own keyAgreement key, so both instances hold the same contract CEK, each unwrappable only by its own HSM.")

	Attribute("alg", String, "CEK wrap algorithm (ECDH-ES+A256KW)")
	Attribute("kid", String, "The receiver's verification method the CEK was wrapped to, as the receiver's own did.json publishes it under keyAgreement (RFC 7516 §4.1.6). The sender names the key it resolved instead of the receiver having to publish exactly one: a receiver that rotates keys resolves this id in its own keyAgreement relationship and knows which key to unwrap with")
	Attribute("epk", DCSToDCSEphemeralPublicKey, "The sender's ephemeral public key")
	Attribute("wrapped", Bytes, "The wrapped CEK (RFC 3394 key wrap output)")

	Required("alg", "kid", "epk", "wrapped")
})

var DCSToDCSSignatoryPoA = Type("DCSToDCSSignatoryPoA", func() {
	Description("The Power of Attorney a signatory presented at the ceremony on the shipping instance, carried so the receiver can verify a counterparty's authority to sign instead of taking the contract's dcs:hasPowerOfAttorney claim on trust (ADR-31, UC-14). The receiver re-derives the party and its signatory from the shipped contract itself, so these fields are checked against it rather than believed.")

	Attribute("party", String, "The party (organization) the credential authorizes, matching a dcs:parties node of the shipped contract")
	Attribute("presentation", String, "The dc+sd-jwt Power of Attorney exactly as the signatory's wallet delivered it at the ceremony")
	Attribute("summary", String, "The ContractSigningSummaryCredential the shipping instance issued for that signature (DCS-FR-SM-08), so the receiver can verify which party signed and by whom without taking the shipper's word for it. Carried here rather than read from the PDF: only the first signer's evidence survives embedding, and adding a second attachment would mutate an already-signed document")

	Required("party", "presentation")
})

var DCSToDCSContractPdfRequest = Type("DCSToDCSContractPdfRequest", func() {
	Description("A contract PDF shipped to the counterparty (ADR-13). The PDF is the wire format: it carries the machine-readable JSON-LD, the C2PA provenance chain, and any signatures. A bare PDF is a proposal (offer or negotiation counter); a PDF accompanied by a JAdES is a signature (acceptance).")

	Attribute("secret_value", String, "Secret value")
	Attribute("secret_hash", Bytes, "Secret hash")

	Attribute("from_peer_did", String, "The did of the peer shipping the PDF")
	Attribute("contract_iri", String, "IRI of the contract the PDF represents")
	Attribute("pdf", Bytes, "The contract PDF")
	Attribute("jades_signature", String, "The sender's JAdES over the contract, present only when this ship is a signature (acceptance); empty for a proposal")
	Attribute("contract_state", String, "The sender's contract state at ship time. Informational, except REVOKED: a revocation ship from the authenticated counterparty — the party revoking its own signature — is adopted by the receiver (DCS-NFR-BR-06)")
	Attribute("wrapped_cek", DCSToDCSWrappedCEK, "The contract's CEK wrapped to the receiver's keyAgreement key (DCS-NFR-SEC-14). Sent with every ship; the receiver adopts it only when it holds no live CEK for the contract yet, so repeats are idempotent")
	Attribute("signatory_poas", ArrayOf(DCSToDCSSignatoryPoA), "The Power of Attorney behind each signature the shipping instance applied, sent once the contract carries signatures. Present-but-unverifiable evidence is refused (ADR-31); absent evidence is accepted and left to the compliance viewer, so a peer that retains none can still federate")

	Required("from_peer_did", "contract_iri", "pdf", "secret_value", "secret_hash")
})

var DCSToDCSContractEraseRequest = Type("DCSToDCSContractEraseRequest", func() {
	Description("A counterparty's request to shred this instance's wrapped CEKs for a contract (DCS-NFR-COMP-03, DCS-NFR-SEC-13): erasure of a federated contract completes only when both instances have destroyed their key records. Authenticated with the same body-level did:web challenge-response the PDF ship uses.")

	Attribute("secret_value", String, "Secret value")
	Attribute("secret_hash", Bytes, "Secret hash")

	Attribute("from_peer_did", String, "The did of the peer requesting the erasure")
	Attribute("contract_iri", String, "IRI of the contract whose wrapped CEKs are to be shredded")

	Required("from_peer_did", "contract_iri", "secret_value", "secret_hash")
})

var DCSToDCSContractEraseResponse = Type("DCSToDCSContractEraseResponse", func() {
	Description("Result for a contract erasure request")

	Attribute("from_peer_did", String, "Decentralized Identifier of the confirming peer")

	Required("from_peer_did")
})

var DCSToDCSContractPdfResponse = Type("DCSToDCSContractPdfResponse", func() {
	Description("Result for receiving a contract PDF")

	Attribute("from_peer_did", String, "Decentralized Identifier of the receiving peer")

	Required("from_peer_did")
})

var DCSToDCSSyncProvenanceResponse = Type("DCSToDCSSyncProvenanceResponse", func() {
	Description("The stored JAdES provenance artifact for a contract received from a peer (DCS-FR-SM-02)")

	Attribute("did", String, "IRI of the received contract")
	Attribute("contract_version", Int, "Contract version the signature covers")
	Attribute("from_peer_did", String, "The peer that signed the shipped contract")
	Attribute("jades_signature", String, "The verified JAdES baseline-B compact JWS as received")
	Attribute("received_at", String, "When the signed ship was accepted")

	Required("did", "contract_version", "from_peer_did", "jades_signature", "received_at")
})

var _ = Service("DcsToDcs", func() {
	Description("DCS-to-DCS federation: two DCS instances exchange the contract PDF per lifecycle step and the JAdES after signing (ADR-13). Each instance runs its own workflow/RBAC; no contract state or task ledger crosses the boundary.")

	Method("post_pdf", func() {
		Description("Receive a contract PDF shipped by the counterparty (ADR-13). The receiver verifies the sender via a did:web challenge-response signature (secret_value signed with the sender's private key, verified against its did:web document and eIDAS certificate chain) — not JWT, since there is no shared end-user identity across DCS instances run by different operators — then asks pdf-core to extract the embedded JSON-LD and upserts its own local copy of the contract. A bare PDF is a proposal (the local copy moves to negotiation); a PDF with a JAdES is the counterparty's signature.")

		Payload(DCSToDCSContractPdfRequest)
		Result(DCSToDCSContractPdfResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/peer/contracts/pdf")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("erase", func() {
		Description("Shred this instance's wrapped CEKs for a contract on request of the authenticated counterparty (DCS-NFR-COMP-03, DCS-NFR-SEC-13). The requester is verified via the same did:web challenge-response signature as post_pdf and must be a party of the contract. All CEK records of the contract scope are marked destroyed (never hard-deleted — the shredded row is the destruction record) and a KEY_SHREDDED audit event is written. Idempotent: repeating the request against an already-shredded contract confirms again.")

		Payload(DCSToDCSContractEraseRequest)
		Result(DCSToDCSContractEraseResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/peer/contracts/erase")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("get_provenance", func() {
		Description("Return the stored JAdES provenance artifact for a contract this instance received from a peer (DCS-FR-SM-02): the sender's baseline-B compact JWS over the contract, verified at receipt and persisted for independent re-verification. JWT-secured — read by local users inspecting a received contract's cross-instance provenance.")

		Security(JWTAuth, func() {
			Scope("Contract Creator")
			Scope("Sys. Contract Creator")
			Scope("Contract Reviewer")
			Scope("Sys. Contract Reviewer")
			Scope("Contract Approver")
			Scope("Sys. Contract Approver")
			Scope("Contract Manager")
			Scope("Sys. Contract Manager")
			Scope("Contract Observer")
			Scope("Auditor")
			Scope("Compliance Officer")
		})

		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("did", String, "IRI of the received contract")
			Required("did")
		})
		Result(DCSToDCSSyncProvenanceResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("not_found", ErrorResult, "No sync provenance stored for this contract")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/peer/contracts/provenance")
			Param("did")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("not_found", StatusNotFound)
			Response("internal_error", StatusInternalServerError)
		})
	})
})
