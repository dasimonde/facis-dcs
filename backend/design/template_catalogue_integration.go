package design

import (
	. "goa.design/goa/v3/dsl"
)

var TemplateCatalogueItem = Type("TemplateCatalogueItem", func() {
	Description("Template catalogue item returned to the client")

	Attribute("did", String, "Decentralized Identifier of the contract template")
	Attribute("version", Int, "The version of the contract template")
	Attribute("name", String, "The name of the contract template")
	Attribute("description", String, "A description for that template")
	Attribute("template_type", String, "The type of the template")
	Attribute("schema_version", Int, "Schema version of the contract template")
	Attribute("participant_id", String, "Participant id")
	Attribute("created_at", String, "The timestamp when the contract template was created")
	Attribute("updated_at", String, "The timestamp when the contract template was updated")

	Required("did")
})

var TemplateCatalogueRetrieveRequest = Type("TemplateCatalogueRetrieveRequest", func() {
	Description("Retrieve template catalogues from Federated Catalogue")

	Token("token", String, "JWT token")

	Attribute("offset", Int, "Pagination offset")
	Attribute("limit", Int, "Pagination limit")

	Required("offset", "limit")
})

var TemplateCatalogueRetrieveResponse = Type("TemplateCatalogueRetrieveResponse", func() {
	Description("Retrieve template catalogues response")

	Attribute("totalCount", Int, "Total count of matched catalogue entries")
	Attribute("items", ArrayOf(TemplateCatalogueItem), "Catalogue items")

	Required("totalCount", "items")
})

var TemplateCatalogueSearchRequest = Type("TemplateCatalogueSearchRequest", func() {
	Description("Search template catalogues in Federated Catalogue")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract template")
	Attribute("version", Int, "The version of the contract template")
	Attribute("name", String, "The name of the contract template")
	Attribute("description", String, "A description for that template")
	Attribute("offset", Int, "Pagination offset; values less than 1 start at the beginning")
	Attribute("limit", Int, "Pagination limit; values less than 1 return all matches")

	Required("offset", "limit")
})

var TemplateCatalogueRetrieveByIDRequest = Type("TemplateCatalogueRetrieveByIDRequest", func() {
	Description("Retrieve a template catalogue by did and version")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract template")
	Attribute("version", Int, "The version of the contract template")
	Required("did", "version")
})

var TemplateCatalogueRetrieveByIDResponse = Type("TemplateCatalogueRetrieveByIDResponse", func() {
	Description("Template catalogue detail response")

	Attribute("did", String, "Decentralized Identifier of the contract template")
	Attribute("version", Int, "The version of the contract template")
	Attribute("name", String, "The name of the contract template")
	Attribute("description", String, "A description for that template")
	Attribute("template_type", String, "The type of the template")
	Attribute("schema_version", Int, "Schema version of the contract template")
	Attribute("template_data", Any, "The template data of the contract template")
	// Optional participant summary
	Attribute("participant_id", String, "Issuer participant DID")
	Attribute("created_at", String, "The timestamp when the contract template was created")
	Attribute("updated_at", String, "The timestamp when the contract template was updated")

	Required("did")
})

// Template Catalogue Integration Service (TR <-> XFSC Catalogue)
var _ = Service("TemplateCatalogueIntegration", func() {
	Description("Integration APIs between the Template Repository (TR) and the XFSC Catalogue for template retrieval.")

	// GET /catalogue/template/retrieve
	Method("retrieve_template", func() {
		Description("Retrieve templates via XFSC Catalogue.")
		Meta("dcs:requirements", "DCS-IR-SI-01")

		Security(JWTAuth, func() {
			// The Template Manager owns the Template Catalogue view and the
			// register that follows it; the contract roles read the catalogue
			// to pick a template to draw a contract from.
			Scope("Template Manager")
			Scope("Contract Creator")
			Scope("Contract Reviewer")
			Scope("Contract Approver")
			Scope("Contract Manager")
			Scope("Contract Signer")
		})

		Payload(TemplateCatalogueRetrieveRequest)
		Result(TemplateCatalogueRetrieveResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/catalogue/template/retrieve")
			Param("offset")
			Param("limit")
			Response(StatusOK)
		})
	})

	// GET /catalogue/template/retrieve/{did}
	Method("retrieve_template_by_id", func() {
		Description("Retrieve template via XFSC Catalogue.")
		Meta("dcs:requirements", "DCS-IR-SI-01")

		Security(JWTAuth, func() {
			Scope("Template Manager")
			Scope("Contract Creator")
			Scope("Contract Reviewer")
			Scope("Contract Approver")
			Scope("Contract Manager")
			Scope("Contract Signer")
		})

		Payload(TemplateCatalogueRetrieveByIDRequest)
		Result(TemplateCatalogueRetrieveByIDResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/catalogue/template/retrieve/{did}")
			Param("did")
			Param("version")
			Response(StatusOK)
		})
	})

	// GET /catalogue/template/search
	Method("search_template", func() {
		Description("Search templates in XFSC Catalogue by metadata fields.")
		Meta("dcs:requirements", "DCS-IR-SI-01")

		Security(JWTAuth, func() {
			Scope("Template Manager")
			Scope("Contract Creator")
			Scope("Contract Reviewer")
			Scope("Contract Approver")
			Scope("Contract Manager")
			Scope("Contract Signer")
		})

		Payload(TemplateCatalogueSearchRequest)
		Result(TemplateCatalogueRetrieveResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/catalogue/template/search")
			Param("did")
			Param("version")
			Param("name")
			Param("description")
			Param("offset")
			Param("limit")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

})
