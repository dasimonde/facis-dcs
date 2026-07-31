package design

import (
	. "goa.design/goa/v3/dsl"
)

// C2PAService exposes the public C2PA manifest for signed/exported contracts
// (DCS-OR-C2PA-008). Like the DID document service
// (backend/design/did.go), these endpoints are public: they declare no
// Security scheme, so no JWT/auth is required. A C2PA manifest store is meant
// to be resolvable by any verifier from the contract's public manifest URL.
var _ = Service("C2PAService", func() {
	Description("Public C2PA manifest retrieval for signed/exported contracts (DCS-OR-C2PA-008). Unauthenticated, public sibling of GET /.well-known/did.json.")

	Error("not_found", ErrorResult, "Contract or C2PA manifest not found")
	Error("internal_error", ErrorResult, "Internal server error")

	HTTP(func() {
		Response("not_found", StatusNotFound)
		Response("internal_error", StatusInternalServerError)
	})

	// GetManifest — GET /c2pa/manifest/{contract_did}
	Method("GetManifest", func() {
		Description("Returns the raw C2PA JUMBF manifest store bytes for a signed/exported contract (Content-Type application/c2pa, HTTP 200). With ?history=true, returns a parsed JSON enumeration of the manifest chain (labels + dcs.lifecycle assertions) instead of the raw store.")
		Meta("dcs:requirements", "DCS-OR-C2PA-008")

		Payload(func() {
			Attribute("contract_did", String, "DID of the signed/exported contract")
			Attribute("history", Boolean, "When true, return a parsed JSON enumeration of the manifest chain (labels + dcs.lifecycle assertions) instead of the raw manifest store")
			Required("contract_did")
		})

		// Result carries only the response headers; the body is streamed
		// directly (raw JUMBF bytes or a JSON chain enumeration) via
		// SkipResponseBodyEncodeDecode, so the Content-Type can be chosen at
		// runtime (application/c2pa vs application/json).
		Result(func() {
			Attribute("content_type", String, "Media type of the response body (application/c2pa or application/json)")
		})

		HTTP(func() {
			GET("/c2pa/manifest/{contract_did}")
			Param("history")
			SkipResponseBodyEncodeDecode()
			Response(StatusOK, func() {
				Header("content_type:Content-Type")
			})
			Response("not_found", StatusNotFound)
			Response("internal_error", StatusInternalServerError)
		})
	})
})
