package design

import (
	. "goa.design/goa/v3/dsl"
)

var SemanticSchemaItem = Type("SemanticSchemaItem", func() {
	Description("One stored, versioned Semantic Hub schema entry (DCS-FR-TR-03)")

	Attribute("name", String, "Schema name")
	Attribute("version", Int, "Schema version (monotonic per name+kind)")
	Attribute("kind", String, "Schema kind: context (JSON-LD), shapes (SHACL), or profile (validation profile)")
	Attribute("media_type", String, "Media type of the content (e.g. application/ld+json, text/turtle)")
	Attribute("content", String, "The schema document, verbatim")
	Attribute("active", Boolean, "Whether this is the active version documents are produced against")
	Attribute("created_by", String, "Who registered this version")
	Attribute("created_at", String, "When this version was registered")

	Required("name", "version", "kind", "media_type", "content", "active", "created_by", "created_at")
})

var ClauseCatalogType = Type("ClauseCatalogType", func() {
	Description("One typed clause (a SHACL NodeShape targeting a class) — the palette listing; the form itself is generated client-side from the response's raw shapes Turtle")

	Attribute("type", String, "The clause's target class, compacted against the active hub context's prefixes (full IRI when outside every declared namespace)")
	Attribute("label", String, "Human-readable label (rdfs:label on the shape, falls back to the type's local name)")
	Attribute("shape", String, "The NodeShape's IRI within the shapes graph")

	Required("type", "label", "shape")
})

var ClauseCatalogResponse = Type("ClauseCatalogResponse", func() {
	Description("The active clause catalog: a pre-digested JSON form-schema plus the raw SHACL shapes it was derived from")

	Attribute("version", Int, "The clause-catalog hub version this catalog was read from")
	Attribute("clauses", ArrayOf(ClauseCatalogType), "Pre-digested clause type form-schemas")
	Attribute("shapes", String, "The raw SHACL Turtle the catalog was derived from")

	Required("version", "clauses", "shapes")
})

var SemanticSchemaListEntry = Type("SemanticSchemaListEntry", func() {
	Description("One (name, kind) Semantic Hub entry with its version summary")

	Attribute("name", String, "Schema name")
	Attribute("kind", String, "Schema kind: context, shapes, or profile")
	Attribute("media_type", String, "Media type of the active version's content")
	Attribute("active_version", Int, "The currently active version")
	Attribute("latest_version", Int, "The highest registered version")
	Attribute("updated_at", String, "When the latest version was registered")

	Required("name", "kind", "media_type", "active_version", "latest_version", "updated_at")
})

var SemanticSchemaRegisterResponse = Type("SemanticSchemaRegisterResponse", func() {
	Description("Result of registering a Semantic Hub schema version")

	Attribute("name", String, "Schema name")
	Attribute("version", Int, "The assigned version")
	Attribute("kind", String, "Schema kind")
	Attribute("active", Boolean, "Whether the new version was activated")

	Required("name", "version", "kind", "active")
})

// Semantic Hub Service (/semantic/...) — DCS-FR-TR-03, UC-02-08: versioned
// storage for the JSON-LD contexts, SHACL shapes, and validation profiles
// every DCS document is produced against. Reads are public (like
// /.well-known/did.json): produced artifacts carry hub-served anchors
// that external verifiers must be able to resolve without a DCS login.
var _ = Service("SemanticHub", func() {
	Description("Semantic Hub APIs (/semantic/...): versioned JSON-LD context, SHACL shape, and validation-profile storage (DCS-FR-TR-03, UC-02-08)")

	Method("register", func() {
		Description("Register a new version of a schema (context/shapes/profile). Versions are immutable and monotonic per name+kind; when activate is set the new version becomes the one all newly produced documents anchor to.")
		Meta("dcs:requirements", "DCS-FR-TR-03")

		Security(JWTAuth, func() {
			Scope("Template Manager")
		})

		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("name", String, "Schema name")
			Attribute("kind", String, "Schema kind", func() {
				Enum("context", "shapes", "profile", "ontology")
			})
			Attribute("media_type", String, "Media type of the content")
			Attribute("content", String, "The schema document, verbatim (omit when source_url is given)")
			Attribute("source_url", String, "Fetch the schema from this URL (http/https, follows redirects) instead of inline content; the fetched bytes are snapshotted as the new immutable version. A DCS hub anchor (/semantic/shapes/{name}?version=N) is unwrapped to the schema document it serves, so a library another instance published can be installed from the anchor a template declares")
			Attribute("version", Int, "Register at exactly this version instead of the next one — how a shape library another instance published is installed here under the number that instance assigned it, so a document pinning ?version=N resolves the same graph on both hubs. Refused when this hub already holds that version")
			Attribute("activate", Boolean, "Make the new version active immediately")
			Required("name", "kind", "media_type")
		})
		Result(SemanticSchemaRegisterResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/semantic/schema/register")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("rollback", func() {
		Description("Make a previously registered version the active one again (UC-02-08: schema versioning and rollback).")
		Meta("dcs:requirements", "DCS-FR-TR-03")

		Security(JWTAuth, func() {
			Scope("Template Manager")
		})

		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("name", String, "Schema name")
			Attribute("kind", String, "Schema kind", func() {
				Enum("context", "shapes", "profile", "ontology")
			})
			Attribute("version", Int, "The version to activate")
			Required("name", "kind", "version")
		})
		Result(SemanticSchemaRegisterResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("not_found", ErrorResult, "No such schema version")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/semantic/schema/rollback")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("not_found", StatusNotFound)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("retrieve", func() {
		Description("Retrieve a schema version (the active one when version is omitted). Public, like /.well-known/did.json — produced artifacts carry hub anchors external verifiers resolve without a DCS login.")
		Meta("dcs:requirements", "DCS-FR-TR-03")
		NoSecurity()

		Payload(func() {
			Attribute("name", String, "Schema name")
			Attribute("kind", String, "Schema kind", func() {
				Enum("context", "shapes", "profile", "ontology")
			})
			Attribute("version", Int, "Specific version; active version when omitted")
			Required("name", "kind")
		})
		Result(SemanticSchemaItem)

		Error("not_found", ErrorResult, "No such schema")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/semantic/schema/retrieve")
			Param("name")
			Param("kind")
			Param("version")
			Response(StatusOK)
			Response("not_found", StatusNotFound)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("versions", func() {
		Description("List every stored version of a schema, oldest first (UC-02-08).")
		Meta("dcs:requirements", "DCS-FR-TR-03")
		NoSecurity()

		Payload(func() {
			Attribute("name", String, "Schema name")
			Attribute("kind", String, "Schema kind", func() {
				Enum("context", "shapes", "profile", "ontology")
			})
			Required("name", "kind")
		})
		Result(ArrayOf(SemanticSchemaItem))

		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/semantic/schema/versions")
			Param("name")
			Param("kind")
			Response(StatusOK)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("resolve_context", func() {
		Description("Serve a registered JSON-LD context document itself (the resolvable anchor produced documents' @context points at). Returns the parsed JSON-LD document as the response body.")
		Meta("dcs:requirements", "DCS-FR-TR-03")
		NoSecurity()

		Payload(func() {
			Attribute("name", String, "Context schema name")
			Attribute("version", Int, "Specific version; active version when omitted")
			Required("name")
		})
		Result(Any)

		Error("not_found", ErrorResult, "No such context")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/semantic/context/{name}")
			Param("version")
			Response(StatusOK)
			Response("not_found", StatusNotFound)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("resolve_shapes", func() {
		Description("Serve a registered SHACL shapes version at the anchor URL produced documents carry (sh:shapesGraph, ADR-8) — semantichub.AnchorURL emits /semantic/shapes/{name}?version=N, so this path must dereference for external verifiers the same way /semantic/context/{name} does for the JSON-LD context.")
		Meta("dcs:requirements", "DCS-FR-TR-03")
		NoSecurity()

		Payload(func() {
			Attribute("name", String, "Shapes schema name")
			Attribute("version", Int, "Specific version; active version when omitted")
			Required("name")
		})
		Result(SemanticSchemaItem)

		Error("not_found", ErrorResult, "No such shapes schema")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/semantic/shapes/{name}")
			Param("version")
			Response(StatusOK)
			Response("not_found", StatusNotFound)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("resolve_ontology", func() {
		Description("Serve a registered ontology version — the dereference target of the dcst: taxonomy IRIs (via the w3id.org redirect) and of /semantic/ontology/{name} directly. Ontology entries are parsed configuration (the domain-field index, the ODRL operator vocabulary), not axioms: what constrains a dcs: envelope term is the shapes graph, served by resolve_shapes.")
		Meta("dcs:requirements", "DCS-FR-TR-03")
		NoSecurity()

		Payload(func() {
			Attribute("name", String, "Ontology schema name")
			Attribute("version", Int, "Specific version; active version when omitted")
			Required("name")
		})
		Result(SemanticSchemaItem)

		Error("not_found", ErrorResult, "No such ontology schema")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/semantic/ontology/{name}")
			Param("version")
			Response(StatusOK)
			Response("not_found", StatusNotFound)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("resolve_profile", func() {
		Description("Serve a registered validation-profile version at /semantic/profile/{name}?version=N. Documents carry no dcterms:conformsTo anchor — one envelope profile said nothing about the vocabulary a contract's own data uses — but profiles stay readable and their rules still run at validation time.")
		Meta("dcs:requirements", "DCS-FR-TR-03")
		NoSecurity()

		Payload(func() {
			Attribute("name", String, "Profile schema name")
			Attribute("version", Int, "Specific version; active version when omitted")
			Required("name")
		})
		Result(SemanticSchemaItem)

		Error("not_found", ErrorResult, "No such profile schema")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/semantic/profile/{name}")
			Param("version")
			Response(StatusOK)
			Response("not_found", StatusNotFound)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("list", func() {
		Description("List every (name, kind) entry the hub holds, with active/latest version summary. Public like retrieve: the hub's inventory is not more sensitive than its content.")
		Meta("dcs:requirements", "DCS-FR-TR-03")
		NoSecurity()

		Result(ArrayOf(SemanticSchemaListEntry))

		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/semantic/schema/list")
			Response(StatusOK)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("clauses", func() {
		Description("List the active clause catalog (DCS-FR-TR-03/TR-04, Phase 3, ADR-10): typed clause NodeShapes the template builder's palette generates a form from, pre-digested into a JSON form-schema server-side (each clause type's target class, label, and properties with their datatype/sh:in/min-max constraints) plus the raw SHACL Turtle for a future shacl-form-style client. Public, like resolve_context: a produced contract's typed clauses are validated against these same shapes (validateAgainstHubShapes), and an external verifier resolving sh:shapesGraph needs to read them too.")
		Meta("dcs:requirements", "DCS-FR-TR-03", "DCS-FR-TR-04")
		NoSecurity()

		Result(ClauseCatalogResponse)

		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/semantic/clauses")
			Response(StatusOK)
			Response("internal_error", StatusInternalServerError)
		})
	})
})
