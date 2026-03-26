package design

import (
	. "goa.design/goa/v3/dsl"
)

var TemplateCatalogueSdMeta = Type("TemplateCatalogueSdMeta", func() {
	Description("Federated Catalogue meta for a self-description")

	Attribute("id", String, "Self-description id")
	Attribute("sdHash", String, "Self-description hash")
	Attribute("issuer", String, "Self-description issuer")
	Attribute("uploadDatetime", String, "Upload datetime")
	Attribute("statusDatetime", String, "Status datetime")

	Required("sdHash", "issuer", "uploadDatetime", "statusDatetime")
})

var TemplateCatalogueItem = Type("TemplateCatalogueItem", func() {
	Description("Template catalogue item returned to the client")

	Attribute("did", String, "Decentralized Identifier of the contract template")
	Attribute("document_number", String, "The number of the contract template")
	Attribute("version", Int, "The version of the contract template")
	Attribute("name", String, "The name of the contract template")
	Attribute("description", String, "A description for that template")
	Attribute("template_type", String, "The type of the template")
	Attribute("participant_id", String, "Participant id")
	Attribute("created_at", String, "The timestamp when the contract template was created")
	Attribute("updated_at", String, "The timestamp when the contract template was updated")
	Attribute("sd_meta", TemplateCatalogueSdMeta, "Federated Catalogue meta")

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

var TemplateCatalogueRetrieveByIdRequest = Type("TemplateCatalogueRetrieveByIdRequest", func() {
	Description("Retrieve a template catalogue by did")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract template")
	Required("did")
})

var TemplateCatalogueRetrieveByIdResponse = Type("TemplateCatalogueRetrieveByIdResponse", func() {
	Description("Template catalogue detail response")

	Attribute("did", String, "Decentralized Identifier of the contract template")
	Attribute("document_number", String, "The number of the contract template")
	Attribute("version", Int, "The version of the contract template")
	Attribute("name", String, "The name of the contract template")
	Attribute("description", String, "A description for that template")
	Attribute("template_type", String, "The type of the template")
	Attribute("participant_id", String, "Participant id")
	Attribute("created_at", String, "The timestamp when the contract template was created")
	Attribute("updated_at", String, "The timestamp when the contract template was updated")
	Attribute("sd_meta", TemplateCatalogueSdMeta, "Federated Catalogue meta")

	Required("did")
})

var TemplateCatalogueAddress = Type("TemplateCatalogueAddress", func() {
	Description("Address information")

	Attribute("country", String, "Country")
	Attribute("street_address", String, "Street address")
	Attribute("postal_code", String, "Postal code")
	Attribute("locality", String, "Locality")
})

var TemplateCatalogueHeadquarterAddress = Type("TemplateCatalogueHeadquarterAddress", func() {
	Description("Headquarter address information")

	Attribute("country", String, "Country")
	Attribute("street_address", String, "Headquarter street address")
	Attribute("postal_code", String, "Headquarter postal code")
	Attribute("locality", String, "Headquarter locality")
})

var TemplateCatalogueCreateParticipantRequest = Type("TemplateCatalogueCreateParticipantRequest", func() {
	Description("Create participant request")

	Token("token", String, "JWT token")

	Attribute("legal_name", String, "Legal name")
	Attribute("registration_number", String, "Registration number")
	Attribute("lei_code", String, "LEI code")
	Attribute("ethereum_address", String, "Ethereum address")
	Attribute("headquarter_address", TemplateCatalogueHeadquarterAddress, "Headquarter address")
	Attribute("legal_address", TemplateCatalogueAddress, "Legal address")
	Attribute("terms_and_conditions", String, "Terms and conditions")

	Required("legal_name", "registration_number", "lei_code", "ethereum_address", "headquarter_address", "legal_address", "terms_and_conditions")
})

var TemplateCatalogueCreateParticipantResponse = Type("TemplateCatalogueCreateParticipantResponse", func() {
	Description("Create participant response")

	Attribute("id", String, "Participant id")

	Required("id")
})

var TemplateCatalogueCreateServiceOfferingRequest = Type("TemplateCatalogueCreateServiceOfferingRequest", func() {
	Description("Create service offering request")

	Token("token", String, "JWT token")

	Attribute("end_point_url", String, "Service offering endpoint URL")

	Attribute("terms_and_conditions", String, "Terms and conditions")

	Attribute("keywords", ArrayOf(String), "Service offering keywords")

	Attribute("description", String, "Service offering description")

	Required("end_point_url", "terms_and_conditions", "keywords", "description")
})

var TemplateCatalogueCreateServiceOfferingResponse = Type("TemplateCatalogueCreateServiceOfferingResponse", func() {
	Description("Create service offering response")

	Attribute("id", String, "Service offering id")

	Required("id")
})

var TemplateCatalogueGetCurrentParticipantRequest = Type("TemplateCatalogueGetCurrentParticipantRequest", func() {
	Description("Get current participant request")

	Token("token", String, "JWT token")
})

var TemplateCatalogueGetCurrentParticipantResponse = Type("TemplateCatalogueGetCurrentParticipantResponse", func() {
	Description("Current participant response")

	Attribute("legal_name", String, "Legal name")
	Attribute("registration_number", String, "Registration number")
	Attribute("lei_code", String, "LEI code")
	Attribute("ethereum_address", String, "Ethereum address")
	Attribute("headquarter_address", TemplateCatalogueHeadquarterAddress, "Headquarter address")
	Attribute("legal_address", TemplateCatalogueAddress, "Legal address")
	Attribute("terms_and_conditions", String, "Terms and conditions")

	Required("legal_name", "registration_number", "lei_code", "ethereum_address", "headquarter_address", "legal_address", "terms_and_conditions")
})

var TemplateCatalogueGetCurrentServiceOfferingRequest = Type("TemplateCatalogueGetCurrentServiceOfferingRequest", func() {
	Description("Get current service offering request")

	Token("token", String, "JWT token")
})

var TemplateCatalogueGetCurrentServiceOfferingResponse = Type("TemplateCatalogueGetCurrentServiceOfferingResponse", func() {
	Description("Current service offering response")

	Attribute("keywords", ArrayOf(String), "Service offering keywords")
	Attribute("description", String, "Service offering description")
	Attribute("end_point_url", String, "Service offering endpoint URL")
	Attribute("terms_and_conditions", String, "Terms and conditions")

	Required("end_point_url", "terms_and_conditions", "keywords", "description")
})

var TemplateCatalogueUpdateParticipantRequest = Type("TemplateCatalogueUpdateParticipantRequest", func() {
	Description("Update current participant request")

	Token("token", String, "JWT token")

	Attribute("legal_name", String, "Legal name")
	Attribute("registration_number", String, "Registration number")
	Attribute("lei_code", String, "LEI code")
	Attribute("ethereum_address", String, "Ethereum address")
	Attribute("headquarter_address", TemplateCatalogueHeadquarterAddress, "Headquarter address")
	Attribute("legal_address", TemplateCatalogueAddress, "Legal address")
	Attribute("terms_and_conditions", String, "Terms and conditions")

	Required("legal_name", "registration_number", "lei_code", "ethereum_address", "headquarter_address", "legal_address", "terms_and_conditions")
})

var TemplateCatalogueUpdateParticipantResponse = Type("TemplateCatalogueUpdateParticipantResponse", func() {
	Description("Update participant response")

	Attribute("id", String, "Participant id")

	Required("id")
})

var TemplateCatalogueUpdateServiceOfferingRequest = Type("TemplateCatalogueUpdateServiceOfferingRequest", func() {
	Description("Update service offering request")

	Token("token", String, "JWT token")

	Attribute("end_point_url", String, "Service offering endpoint URL")

	Attribute("terms_and_conditions", String, "Terms and conditions")

	Attribute("keywords", ArrayOf(String), "Service offering keywords")

	Attribute("description", String, "Service offering description")

	Required("end_point_url", "terms_and_conditions", "keywords", "description")
})

var TemplateCatalogueUpdateServiceOfferingResponse = Type("TemplateCatalogueUpdateServiceOfferingResponse", func() {
	Description("Update service offering response")

	Attribute("id", String, "Service offering id")

	Required("id")
})

var TemplateCatalogueDeleteServiceOfferingRequest = Type("TemplateCatalogueDeleteServiceOfferingRequest", func() {
	Description("Delete current service offering request")

	Token("token", String, "JWT token")
})

var TemplateCatalogueDeleteServiceOfferingResponse = Type("TemplateCatalogueDeleteServiceOfferingResponse", func() {
	Description("Delete service offering response")

	Attribute("id", String, "Service offering id")

	Required("id")
})

var TemplateCatalogueDeleteParticipantRequest = Type("TemplateCatalogueDeleteParticipantRequest", func() {
	Description("Delete current participant request")

	Token("token", String, "JWT token")
})

var TemplateCatalogueDeleteParticipantResponse = Type("TemplateCatalogueDeleteParticipantResponse", func() {
	Description("Delete participant response")

	Attribute("id", String, "Participant id")

	Required("id")
})

// Template Catalogue Integration Service (TR <-> XFSC Catalogue)
var _ = Service("TemplateCatalogueIntegration", func() {
	Description("Integration APIs between the Template Repository (TR) and the XFSC Catalogue for template retrieval and the management of participants and service offerings.")

	// GET /catalogue/template/retrieve
	Method("retrieve", func() {
		Description("Retrieve templates via XFSC Catalogue.")
		Meta("dcs:requirements", "DCS-IR-SI-01")

		Security(JWTAuth, func() {
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
	Method("retrieve_by_id", func() {
		Description("Retrieve template via XFSC Catalogue.")
		Meta("dcs:requirements", "DCS-IR-SI-01")

		Security(JWTAuth, func() {
			Scope("Contract Creator")
			Scope("Contract Reviewer")
			Scope("Contract Approver")
			Scope("Contract Manager")
			Scope("Contract Signer")
		})

		Payload(TemplateCatalogueRetrieveByIdRequest)
		Result(TemplateCatalogueRetrieveByIdResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/catalogue/template/retrieve/{did}")
			Param("did")
			Response(StatusOK)
		})
	})

	// POST /catalogue/participant/create
	Method("create_participant", func() {
		Description("Create participant in XFSC Catalogue.")
		Meta("dcs:requirements", "DCS-IR-SI-01")

		Security(JWTAuth, func() {
			Scope("System Administrator")
		})

		Payload(TemplateCatalogueCreateParticipantRequest)
		Result(TemplateCatalogueCreateParticipantResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/catalogue/participant/create")
			Response(StatusOK)
		})
	})

	// POST /catalogue/service-offering/create
	Method("create_service_offering", func() {
		Description("Create service offering in XFSC Catalogue.")
		Meta("dcs:requirements", "DCS-IR-SI-01")

		Security(JWTAuth, func() {
			Scope("System Administrator")
		})

		Payload(TemplateCatalogueCreateServiceOfferingRequest)
		Result(TemplateCatalogueCreateServiceOfferingResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/catalogue/service-offering/create")
			Response(StatusOK)
		})
	})

	// GET /catalogue/participant/current
	Method("get_current_participant", func() {
		Description("Get current participant from XFSC Catalogue.")
		Meta("dcs:requirements", "DCS-IR-SI-01")

		Security(JWTAuth, func() {
			Scope("System Administrator")
		})

		Payload(TemplateCatalogueGetCurrentParticipantRequest)
		Result(TemplateCatalogueGetCurrentParticipantResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("not_found", ErrorResult, "Not found")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/catalogue/participant/current")
			Response(StatusOK)
			Response("not_found", StatusNotFound)
		})
	})

	// GET /catalogue/service-offering/current
	Method("get_current_service_offering", func() {
		Description("Get current service offering from XFSC Catalogue.")
		Meta("dcs:requirements", "DCS-IR-SI-01")

		Security(JWTAuth, func() {
			Scope("System Administrator")
		})

		Payload(TemplateCatalogueGetCurrentServiceOfferingRequest)
		Result(TemplateCatalogueGetCurrentServiceOfferingResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("not_found", ErrorResult, "Not found")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/catalogue/service-offering/current")
			Response(StatusOK)
			Response("not_found", StatusNotFound)
		})
	})

	// PUT /catalogue/participant/update
	Method("update_participant", func() {
		Description("Update current participant in XFSC Catalogue.")
		Meta("dcs:requirements", "DCS-IR-SI-01")

		Security(JWTAuth, func() {
			Scope("System Administrator")
		})

		Payload(TemplateCatalogueUpdateParticipantRequest)
		Result(TemplateCatalogueUpdateParticipantResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("not_found", ErrorResult, "Not found")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			PUT("/catalogue/participant/update")
			Response(StatusOK)
			Response("not_found", StatusNotFound)
		})
	})

	// PUT /catalogue/service-offering/update
	Method("update_service_offering", func() {
		Description("Update current service offering in XFSC Catalogue.")
		Meta("dcs:requirements", "DCS-IR-SI-01")

		Security(JWTAuth, func() {
			Scope("System Administrator")
		})

		Payload(TemplateCatalogueUpdateServiceOfferingRequest)
		Result(TemplateCatalogueUpdateServiceOfferingResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("not_found", ErrorResult, "Not found")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			PUT("/catalogue/service-offering/update")
			Response(StatusOK)
			Response("not_found", StatusNotFound)
		})
	})

	// DELETE /catalogue/participant/delete
	Method("delete_participant", func() {
		Description("Delete current participant in XFSC Catalogue.")
		Meta("dcs:requirements", "DCS-IR-SI-01")

		Security(JWTAuth, func() {
			Scope("System Administrator")
		})

		Payload(TemplateCatalogueDeleteParticipantRequest)
		Result(TemplateCatalogueDeleteParticipantResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			DELETE("/catalogue/participant/delete")
			Response(StatusOK)
		})
	})

	// DELETE /catalogue/service-offering/delete
	Method("delete_service_offering", func() {
		Description("Delete current service offering in XFSC Catalogue.")
		Meta("dcs:requirements", "DCS-IR-SI-01")

		Security(JWTAuth, func() {
			Scope("System Administrator")
		})

		Payload(TemplateCatalogueDeleteServiceOfferingRequest)
		Result(TemplateCatalogueDeleteServiceOfferingResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("not_found", ErrorResult, "Not found")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			DELETE("/catalogue/service-offering/delete")
			Response(StatusOK)
			Response("not_found", StatusNotFound)
		})
	})

})
