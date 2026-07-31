package design

import (
	. "goa.design/goa/v3/dsl"
)

var ArchiveRetrieveRequest = Type("ArchiveRetrieveRequest", func() {
	Description("Archive retrieve request")

	Token("token", String, "JWT token")
})
var ArchiveRetrieveResponse = Type("ArchiveRetrieveResponse", func() {
	Description("Result for retrieving the archive")

	Attribute("contracts", ArrayOf(ContractItem), "A list of contracts")
})

var ArchiveSearchRequest = Type("ArchiveSearchRequest", func() {
	Description("Archive search request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("contract_version", Int, "The version number of the contract")
	Attribute("state", String, "The state of the contract")
	Attribute("name", String, "The name of the contract")
	Attribute("description", String, "A description for that contract")
	Attribute("contract_data", String, "Search value for full text search in contract data")
	Attribute("tag", String, "Return only archive entries carrying this annotation tag (DCS-FR-CSA-11)")
	Attribute("party", String, "Return only contracts where this DID is a contract party — creator or counterparty (DCS-FR-CSA-10, DCS-FR-CSA-13)")
	Attribute("valid_from", String, "Return only contracts whose validity period starts at or after this RFC3339 timestamp (DCS-FR-CSA-10, DCS-FR-CSA-13)")
	Attribute("valid_until", String, "Return only contracts whose validity period ends at or before this RFC3339 timestamp (DCS-FR-CSA-10, DCS-FR-CSA-13)")
})

var ArchiveRecentAction = Type("ArchiveRecentAction", func() {
	Description("One recent archive-affecting audit event")

	Attribute("actor", String, "Who performed the operation")
	Attribute("occurred_at", String, "When the operation happened (RFC3339)")
	Attribute("event_type", String, "The archive operation (store/retrieve/search/delete/annotate)")
	Attribute("did", String, "The archived contract the operation concerned")

	Required("actor", "occurred_at", "event_type", "did")
})

var ArchiveExpiringContract = Type("ArchiveExpiringContract", func() {
	Description("An archived contract whose expiration date is approaching")

	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("name", String, "The contract's name")
	Attribute("exp_date", String, "The contract's expiration timestamp (RFC3339)")

	Required("did", "exp_date")
})

var ArchiveStatisticsResponse = Type("ArchiveStatisticsResponse", func() {
	Description("Archive dashboard overview (DCS-FR-CSA-21): archived-contract statistics, recent actions, storage volume, expiring contracts, and compliance status")

	Attribute("archived_total", Int, "Number of stored archive entries, all contract versions")
	Attribute("archived_contracts", Int, "Number of distinct archived contracts")
	Attribute("deleted_total", Int, "Number of soft-deleted archive entries")
	Attribute("storage_bytes", Int64, "Storage volume of archived snapshots, evidence, and receipts, in bytes")
	Attribute("compliant_total", Int, "Archive entries carrying complete evidence (content hash, TSA receipt, signature metadata)")
	Attribute("flagged_total", Int, "Archive entries with incomplete evidence — flagged per DCS-FR-CSA-19")
	Attribute("recent_actions", ArrayOfRequired(ArchiveRecentAction), "Most recent archive-affecting audit events, newest first")
	Attribute("expiring_contracts", ArrayOfRequired(ArchiveExpiringContract), "Archived contracts expiring within the next 30 days, soonest first")

	Required("archived_total", "archived_contracts", "deleted_total", "storage_bytes",
		"compliant_total", "flagged_total", "recent_actions", "expiring_contracts")
})

var ArchiveAnnotationResponse = Type("ArchiveAnnotationResponse", func() {
	Description("The archive entry annotation after an annotate call (DCS-FR-CSA-11)")

	Attribute("did", String, "Decentralized Identifier of the annotated contract")
	Attribute("summary", String, "The stored summary (caller-provided, or system-generated from the contract metadata when none was supplied)")
	Attribute("tags", ArrayOf(String), "The stored tag set")

	Required("did", "summary")
})

var ArchiveErasurePeerStatus = Type("ArchiveErasurePeerStatus", func() {
	Description("The erasure-handshake state of one counterparty instance: pending while the peer erase request is still queued for retry, confirmed once the peer acknowledged shredding its wrapped CEKs")

	Attribute("peer_did", String, "Decentralized Identifier of the counterparty instance")
	Attribute("status", String, "Handshake state: pending or confirmed", func() {
		Enum("pending", "confirmed")
	})
	Attribute("requested_at", String, "When the peer erase was triggered (RFC3339)")
	Attribute("confirmed_at", String, "When the peer confirmed shredding its CEKs (RFC3339); absent while pending")
	Attribute("retry_count", Int, "Number of failed delivery attempts so far")
	Attribute("last_tried_at", String, "When delivery was last attempted (RFC3339); absent before the first retry")

	Required("peer_did", "status", "requested_at", "retry_count")
})

var ArchiveErasureStatusResponse = Type("ArchiveErasureStatusResponse", func() {
	Description("The erasure state of a contract's content-encryption keys (DCS-NFR-COMP-03, DCS-NFR-SEC-13): live or shredded locally, plus the per-peer erase-handshake state on federated contracts")

	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("local_status", String, "Local CEK state: live (content decryptable) or shredded (keys destroyed, content erased)", func() {
		Enum("live", "shredded")
	})
	Attribute("shredded_at", String, "When the local CEK records were destroyed (RFC3339); absent while live")
	Attribute("shredded_by", String, "Who destroyed the local CEK records — a local participant or the requesting peer DID; absent while live")
	Attribute("shred_reason", String, "The recorded destruction reason; absent while live")
	Attribute("peers", ArrayOfRequired(ArchiveErasurePeerStatus), "Erase-handshake state per counterparty instance; empty for purely local contracts")

	Required("did", "local_status", "peers")
})

// Contract Storage & Archive Service  (/archive/...)
var _ = Service("ContractStorageArchive", func() {
	Description("Contract Storage & Archive APIs (/archive/...)")

	Method("retrieve", func() {
		Description("retrieve archived items.")
		Meta("dcs:requirements", "DCS-IR-CSA-01", "DCS-IR-CSA-05")

		Security(JWTAuth, func() {
			Scope("Archive Manager")
			Scope("Contract Observer")
		})

		Payload(ArchiveRetrieveRequest)
		Result(ArchiveRetrieveResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/archive/retrieve")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("search", func() {
		Description("search archived records. search records by criteria.")
		Meta("dcs:requirements", "DCS-IR-CSA-01", "DCS-IR-CSA-05", "DCS-FR-CSA-10", "DCS-FR-CSA-13")
		Security(JWTAuth, func() {
			Scope("Archive Manager")
			Scope("Contract Observer")
		})
		Payload(ArchiveSearchRequest)
		Result(ArrayOfRequired(ContractItem))

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/archive/search")
			Param("did")
			Param("contract_version")
			Param("state")
			Param("name")
			Param("description")
			Param("contract_data")
			Param("tag")
			Param("party")
			Param("valid_from")
			Param("valid_until")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("store", func() {
		Description("store new contract or evidence.")
		Meta("dcs:requirements", "DCS-IR-CSA-02", "DCS-IR-CSA-06")
		Security(JWTAuth, func() {
			Scope("Archive Manager")
		})
		Payload(func() {
			Token("token", String, "JWT token")
		})
		HTTP(func() {
			POST("/archive/store")
			Response(StatusOK)
		})
		Result(String)
	})

	Method("delete", func() {
		Description("Permanently delete an archived contract entry (DCS-FR-CSA-17). This is a soft delete: the archive entry is marked deleted_at/deleted_by/deletion_reason rather than physically removed, so evidence remains discoverable for compliance/dispute resolution, and requires a justification that is logged with the deletion's audit event.")
		Meta("dcs:requirements", "DCS-IR-CSA-03", "DCS-IR-CSA-06", "DCS-FR-CSA-17")
		Security(JWTAuth, func() {
			Scope("Archive Manager")
		})
		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("did", String, "Decentralized Identifier of the archived contract to delete")
			Attribute("justification", String, "Justification for the deletion (DCS-FR-CSA-17); logged with the deletion audit event")
			Required("did", "justification")
		})

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			DELETE("/archive/delete")
			Param("did")
			Param("justification")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
		Result(Int)
	})

	Method("annotate", func() {
		Description("Annotate an archived contract with a summary and tags (DCS-FR-CSA-11). The summary may be supplied by the caller or, when omitted, is generated from the archived contract's metadata; tags replace the entry's tag set when provided. Only the annotation is mutable — the archive entry's snapshot and evidence stay immutable.")
		Meta("dcs:requirements", "DCS-FR-CSA-11")
		Security(JWTAuth, func() {
			Scope("Archive Manager")
		})
		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("did", String, "Decentralized Identifier of the archived contract to annotate")
			Attribute("summary", String, "Manual summary; when omitted (and none is stored yet) a summary is generated from the contract metadata")
			Attribute("tags", ArrayOf(String), "Tags for thematic categorization and discovery; replaces the entry's tag set")
			Required("did")
		})
		Result(ArchiveAnnotationResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/archive/annotate")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("erasure_status", func() {
		Description("Return the erasure state of a contract's content-encryption keys (DCS-NFR-COMP-03, DCS-NFR-SEC-13): whether the local wrapped CEKs are live or shredded (with timestamp, actor, and reason), and — for federated contracts — whether each counterparty instance has confirmed shredding its own CEKs or the erase request is still pending retry.")
		Meta("dcs:requirements", "DCS-NFR-COMP-03", "DCS-NFR-SEC-13", "DCS-IR-CSA-03")
		Security(JWTAuth, func() {
			Scope("Archive Manager")
			Scope("Auditor")
		})
		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("did", String, "Decentralized Identifier of the contract")
			Required("did")
		})
		Result(ArchiveErasureStatusResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/archive/erasure-status")
			Param("did")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("statistics", func() {
		Description("Archive dashboard overview (DCS-FR-CSA-21): archived-contract statistics, recent actions, storage volume, expiring contracts, and compliance status. Drill-down happens per contract via the retrieve/audit methods.")
		Meta("dcs:requirements", "DCS-FR-CSA-21")
		Security(JWTAuth, func() {
			Scope("Auditor")
			Scope("Archive Manager")
		})
		Payload(func() {
			Token("token", String, "JWT token")
		})

		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/archive/statistics")
			Response(StatusOK)
			Response("internal_error", StatusInternalServerError)
		})
		Result(ArchiveStatisticsResponse)
	})

	Method("audit", func() {
		Description("Retrieve the archive audit log: actor, timestamp, operation, and contract DID for every recorded archive-affecting event (store/retrieve/search/delete) — DCS-IR-CSA-04, UC-07-03.")
		Meta("dcs:requirements", "DCS-IR-CSA-04")
		Security(JWTAuth, func() {
			Scope("Auditor")
			Scope("Archive Manager")
		})
		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("did", String, "Optional archived contract DID filter")
			Attribute("justification", String, "Required audit justification", func() { MinLength(1) })
			Required("justification")
		})

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/archive/audit")
			Param("did")
			Param("justification")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
		Result(ArrayOfRequired(PACAuditResponse))
	})

})
