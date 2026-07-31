package design

import (
	. "goa.design/goa/v3/dsl"
)

var _ = Service("DIDService", func() {
	Description("Returns the DID document for contracts, templates, and the service itself, plus the federation agreement credential and rules document (ADR-19)")

	Method("GetServiceDID", func() {
		Description("Returns the service's own DID document (domain-level, no path)")

		Result(Any)
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/.well-known/did.json")
			GET("/api/.well-known/did.json")

			Response(StatusOK, func() {
				ContentType("application/did+json")
			})
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("GetAgreementCredential", func() {
		Description("Returns this instance's self-signed federation agreement credential (ADR-19): a W3C Verifiable Credential naming the embedded federation rules by policy ID and hash")

		Result(Any)
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/.well-known/dcs-agreement-credential.json")
			GET("/api/.well-known/dcs-agreement-credential.json")

			Response(StatusOK, func() {
				ContentType("application/json")
			})
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("GetFederationRules", func() {
		Description("Serves the federation rules document embedded in this instance's binary (ADR-19); its content hash is the value the agreement credential's termsOfUse.hash names")

		Result(Bytes)
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/.well-known/dcs-federation-rules.md")
			GET("/api/.well-known/dcs-federation-rules.md")

			Response(StatusOK, func() {
				ContentType("text/markdown")
			})
			Response("internal_error", StatusInternalServerError)
		})
	})
})
