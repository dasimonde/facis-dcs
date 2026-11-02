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
	Attribute("jurisdiction", String, "Return only contracts whose jurisdiction field contains this text (DCS-FR-CSA-10)")
})

var ArchiveRecentAction = Type("ArchiveRecentAction", func() {
	Description("One recent archive-affecting audit event")

	Attribute("actor", String, "Who performed the operation; present only when the request gave a justification")
	Attribute("occurred_at", String, "When the operation happened (RFC3339)")
	Attribute("event_type", String, "The archive operation (store/retrieve/search/delete/annotate)")
	Attribute("did", String, "The archived contract the operation concerned")

	Required("occurred_at", "event_type", "did")
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
	Attribute("compliant_total", Int, "Live archive entries that pass every archive compliance rule (the verdict of GET /archive/compliance)")
	Attribute("flagged_total", Int, "Live archive entries that fail at least one archive compliance rule (DCS-FR-CSA-19)")
	Attribute("recent_actions", ArrayOfRequired(ArchiveRecentAction), "Most recent archive-affecting audit events, newest first")
	Attribute("expiring_contracts", ArrayOfRequired(ArchiveExpiringContract), "Archived contracts expiring within the next expiring_window_days, soonest first")
	Attribute("expiring_window_days", Int, "The look-ahead window of expiring_contracts in days (DCS_ARCHIVE_EXPIRING_WINDOW_DAYS)")

	Required("archived_total", "archived_contracts", "deleted_total", "storage_bytes",
		"compliant_total", "flagged_total", "recent_actions", "expiring_contracts", "expiring_window_days")
})

var ArchiveAnnotationResponse = Type("ArchiveAnnotationResponse", func() {
	Description("The archive entry annotation after an annotate call (DCS-FR-CSA-11)")

	Attribute("did", String, "Decentralized Identifier of the annotated contract")
	Attribute("summary", String, "The stored summary (caller-provided, or system-generated from the contract metadata when none was supplied)")
	Attribute("tags", ArrayOf(String), "The stored tag set")

	Required("did", "summary")
})

var ArchiveErasurePeerStatus = Type("ArchiveErasurePeerStatus", func() {
	Description("The erasure-handshake state of one counterparty instance: pending while the peer erase request is still queued for retry, confirmed once the peer acknowledged shredding its wrapped CEKs, refused when the peer holds no copy of the contract shared with this instance and the request is not sent again")

	Attribute("peer_did", String, "Decentralized Identifier of the counterparty instance")
	Attribute("status", String, "Handshake state: pending, confirmed or refused", func() {
		Enum("pending", "confirmed", "refused")
	})
	Attribute("last_error", String, "The peer's answer to the last attempt that did not confirm; absent once confirmed")
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

var ArchiveStoreResponse = Type("ArchiveStoreResponse", func() {
	Description("The stored archive record (DCS-IR-CSA-02)")

	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("kind", String, "What was stored: contract or evidence")
	Attribute("contract_version", Int, "The archived contract version (kind=contract)")
	Attribute("evidence_id", String, "Identifier of the stored evidence document (kind=evidence)")
	Attribute("section_id", String, "The archived section (kind=section)")
	Attribute("content_hash", String, "SHA-256 hash of the stored snapshot or evidence content")

	Required("did", "kind", "content_hash")
})

var ArchiveEvidenceItem = Type("ArchiveEvidenceItem", func() {
	Description("An evidence document stored for an archived contract")

	Attribute("id", String, "Evidence identifier")
	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("name", String, "Document name")
	Attribute("media_type", String, "Media type")
	Attribute("description", String, "Description")
	Attribute("content_hash", String, "SHA-256 hash of the content")
	Attribute("size_bytes", Int64, "Content size in bytes")
	Attribute("stored_by", String, "Who stored the document")
	Attribute("stored_at", String, "When the document was stored (RFC3339)")
	Attribute("content", Bytes, "The document content; absent when the contract's keys are shredded")

	Required("id", "did", "name", "media_type", "content_hash", "size_bytes", "stored_by", "stored_at")
})

var ArchiveSectionItem = Type("ArchiveSectionItem", func() {
	Description("A contract section archived for the parties assigned to it (DCS-FR-CSA-26), linked to the archive entry of its contract version")

	Attribute("id", String, "Section archive identifier")
	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("contract_version", Int, "The archived contract version the section belongs to")
	Attribute("section_id", String, "The @id of the section or clause block in the contract document")
	Attribute("parties", ArrayOf(String), "The organizations that may read the section")
	Attribute("content_hash", String, "SHA-256 hash of the archived section")
	Attribute("stored_by", String, "Who archived the section")
	Attribute("stored_at", String, "When the section was archived (RFC3339)")
	Attribute("content", Any, "The archived section block; absent when the contract's keys are shredded")

	Required("id", "did", "contract_version", "section_id", "parties", "content_hash", "stored_by", "stored_at")
})

var ArchiveComplianceCheck = Type("ArchiveComplianceCheck", func() {
	Description("The result of one archive compliance rule for one entry")

	Attribute("rule", String, "The rule", func() {
		Enum("retention", "signature_policy", "metadata_completeness", "evidence_completeness")
	})
	Attribute("passed", Boolean, "Whether the entry meets the rule")
	Attribute("reason", String, "Why the entry fails the rule; absent when it passes")

	Required("rule", "passed")
})

var ArchiveComplianceItem = Type("ArchiveComplianceItem", func() {
	Description("The compliance verdict for one live archive entry (DCS-FR-CSA-19)")

	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("contract_version", Int, "The archived contract version")
	Attribute("name", String, "The contract's name")
	Attribute("flagged", Boolean, "True when the entry fails at least one rule")
	Attribute("checks", ArrayOfRequired(ArchiveComplianceCheck), "One result per rule")

	Required("did", "contract_version", "flagged", "checks")
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
			Scope("Auditor")
			Scope("Compliance Officer")
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

	Method("detail", func() {
		Description("Read an exact immutable archive snapshot after contract party authorization")
		Security(JWTAuth, func() {
			Scope("Archive Manager")
			Scope("Contract Observer")
			Scope("Auditor")
			Scope("Compliance Officer")
		})
		Payload(func() {
			Token("token", String)
			Attribute("did", String)
			Attribute("contract_version", Int, func() { Minimum(1) })
			Required("did", "contract_version")
		})
		Result(Any)
		Error("forbidden", ErrorResult)
		Error("not_found", ErrorResult)
		Error("internal_error", ErrorResult)
		HTTP(func() {
			GET("/archive/detail")
			Param("did")
			Param("contract_version")
			Response(StatusOK)
			Response("forbidden", StatusForbidden)
			Response("not_found", StatusNotFound)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("search", func() {
		Description("search archived records. search records by criteria.")
		Meta("dcs:requirements", "DCS-IR-CSA-01", "DCS-IR-CSA-05", "DCS-FR-CSA-10", "DCS-FR-CSA-13")
		Security(JWTAuth, func() {
			Scope("Archive Manager")
			Scope("Contract Observer")
			Scope("Auditor")
			Scope("Compliance Officer")
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
			Param("jurisdiction")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("store", func() {
		Description("Store a contract, an evidence document or a contract section in the archive (DCS-IR-CSA-02, DCS-FR-CSA-25, DCS-FR-CSA-26). kind=section archives one section or clause block of the given archived version (the latest when contract_version is omitted) for the parties assigned to it; each party must be able to read the contract, and only these parties, the Auditor and the system roles read the section. kind=contract archives the current version of a SIGNED, TERMINATED or EXPIRED contract that has no archive entry for that version yet. The entry gets the archive notary receipt and the RFC 3161 TSA receipt that the signing workflow gives its archive entry. kind=evidence stores a document for an archived contract; the content is encrypted under the contract's key in IPFS and erased with it. Both are recorded in the archive audit log.")
		Meta("dcs:requirements", "DCS-IR-CSA-02", "DCS-IR-CSA-06", "DCS-FR-CSA-25", "DCS-FR-CSA-26")
		Security(JWTAuth, func() {
			Scope("Archive Manager")
		})
		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("kind", String, "What to store", func() {
				Enum("contract", "evidence", "section")
			})
			Attribute("did", String, "Decentralized Identifier of the contract to archive, or of the archived contract the evidence belongs to")
			Attribute("name", String, "Evidence document name (kind=evidence)")
			Attribute("media_type", String, "Evidence media type, for example application/pdf (kind=evidence)")
			Attribute("description", String, "Optional description of the evidence (kind=evidence)")
			Attribute("content", Bytes, "Evidence document content (kind=evidence)", func() {
				MaxLength(10 << 20)
			})
			Attribute("section_id", String, "The @id of the section or clause block to archive (kind=section)")
			Attribute("parties", ArrayOf(String), "The organizations assigned to the section, who may read it (kind=section)")
			Attribute("contract_version", Int, "The archived contract version the section belongs to; the latest archived version when omitted (kind=section)", func() { Minimum(1) })
			Required("kind", "did")
		})
		Result(ArchiveStoreResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("forbidden", ErrorResult, "The caller may not read the contract")
		Error("not_found", ErrorResult, "No contract has this DID")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/archive/store")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("forbidden", StatusForbidden)
			Response("not_found", StatusNotFound)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("evidence", func() {
		Description("List the evidence documents stored for an archived contract, with their decrypted content (DCS-IR-CSA-02). The read is recorded in the archive audit log.")
		Meta("dcs:requirements", "DCS-IR-CSA-02", "DCS-FR-CSA-24")
		Security(JWTAuth, func() {
			Scope("Archive Manager")
			Scope("Auditor")
			Scope("Compliance Officer")
		})
		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("did", String, "Decentralized Identifier of the archived contract")
			Required("did")
		})
		Result(ArrayOfRequired(ArchiveEvidenceItem))

		Error("bad_request", ErrorResult, "Bad request")
		Error("forbidden", ErrorResult, "The caller may not read the contract")
		Error("not_found", ErrorResult, "No contract has this DID")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/archive/evidence")
			Param("did")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("forbidden", StatusForbidden)
			Response("not_found", StatusNotFound)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("evidenceContent", func() {
		Description("Serve one evidence document of an archived contract as a file (DCS-IR-CI-02): a PDF as application/pdf, any other document as application/octet-stream. The read is recorded in the archive audit log.")
		Meta("dcs:requirements", "DCS-IR-CI-02", "DCS-IR-CSA-02")
		Security(JWTAuth, func() {
			Scope("Archive Manager")
			Scope("Auditor")
			Scope("Compliance Officer")
		})
		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("did", String, "Decentralized Identifier of the archived contract")
			Attribute("evidence_id", String, "Identifier of the evidence document")
			Required("did", "evidence_id")
		})
		Result(func() {
			Attribute("content_type", String, "application/pdf or application/octet-stream")
			Required("content_type")
		})

		Error("forbidden", ErrorResult, "The caller may not read the contract")
		Error("not_found", ErrorResult, "No such evidence document, or its keys are shredded")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/archive/evidence/content")
			Param("did")
			Param("evidence_id")
			SkipResponseBodyEncodeDecode()
			Response(StatusOK, func() {
				Header("content_type:Content-Type")
			})
			Response("forbidden", StatusForbidden)
			Response("not_found", StatusNotFound)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("sections", func() {
		Description("List the archived sections of a contract (DCS-FR-CSA-26) that the caller's organization is assigned to, with their content. The Auditor and the system roles read every section. A request for one section the caller is not assigned to is refused. The read is recorded in the archive audit log.")
		Meta("dcs:requirements", "DCS-FR-CSA-26", "DCS-FR-CSA-18")
		Security(JWTAuth, func() {
			Scope("Archive Manager")
			Scope("Contract Observer")
			Scope("Auditor")
			Scope("Compliance Officer")
		})
		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("did", String, "Decentralized Identifier of the archived contract")
			Attribute("section_id", String, "Only this section")
			Required("did")
		})
		Result(ArrayOfRequired(ArchiveSectionItem))

		Error("bad_request", ErrorResult, "Bad request")
		Error("forbidden", ErrorResult)
		Error("not_found", ErrorResult)
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/archive/sections")
			Param("did")
			Param("section_id")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("forbidden", StatusForbidden)
			Response("not_found", StatusNotFound)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("compliance", func() {
		Description("Verify every live archive entry against the archive compliance rules (DCS-FR-CSA-19): retention time, signature policy, metadata completeness and evidence completeness. Entries that fail a rule are flagged.")
		Meta("dcs:requirements", "DCS-FR-CSA-19")
		Security(JWTAuth, func() {
			Scope("Archive Manager")
			Scope("Auditor")
			Scope("Compliance Officer")
		})
		Payload(func() {
			Token("token", String, "JWT token")
		})
		Result(ArrayOfRequired(ArchiveComplianceItem))

		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/archive/compliance")
			Response(StatusOK)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("delete", func() {
		Description("Delete an archived contract entry and erase the contract (DCS-FR-CSA-17). Only a TERMINATED, EXPIRED, REVOKED, REJECTED or WITHDRAWN contract that the caller may read can be deleted. This is a soft delete: the archive entry is marked deleted_at/deleted_by/deletion_reason rather than physically removed, and requires a justification that is logged with the deletion's audit event. The contract's keys are then shredded here and on every counterparty instance. A repeated delete runs the erasure again.")
		Meta("dcs:requirements", "DCS-IR-CSA-03", "DCS-IR-CSA-06", "DCS-FR-CSA-17")
		Security(JWTAuth, func() {
			Scope("Archive Manager")
		})
		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("did", String, "Decentralized Identifier of the archived contract to delete")
			Attribute("justification", String, "Justification for the deletion (DCS-FR-CSA-17); logged with the deletion audit event", func() { Pattern(`\S`) })
			Required("did", "justification")
		})

		Error("bad_request", ErrorResult, "Bad request")
		Error("forbidden", ErrorResult, "The caller may not read the contract")
		Error("not_found", ErrorResult, "No contract has this DID")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			DELETE("/archive/delete")
			Param("did")
			Param("justification")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("forbidden", StatusForbidden)
			Response("not_found", StatusNotFound)
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
		Error("forbidden", ErrorResult, "The caller may not read the contract")
		Error("not_found", ErrorResult, "No contract has this DID")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/archive/annotate")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("forbidden", StatusForbidden)
			Response("not_found", StatusNotFound)
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

		Error("forbidden", ErrorResult)
		Error("not_found", ErrorResult)
		HTTP(func() {
			Response("not_found", StatusNotFound)
			Response("forbidden", StatusForbidden)
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
			Attribute("justification", String, "Why the caller reads who performed the recent actions (DCS-FR-PACM-04); without it the recent actions carry no actor", func() { Pattern(`\S`) })
		})

		Error("internal_error", ErrorResult, "Internal server error")

		Error("forbidden", ErrorResult)
		Error("not_found", ErrorResult)
		HTTP(func() {
			Response("not_found", StatusNotFound)
			Response("forbidden", StatusForbidden)
			GET("/archive/statistics")
			Param("justification")
			Response(StatusOK)
			Response("internal_error", StatusInternalServerError)
		})
		Result(ArchiveStatisticsResponse)
	})

	Method("audit", func() {
		Description("Retrieve the archive audit log per readable archived contract: every recorded archive operation (store, retrieve, search, annotate, delete, evidence, sections, failures, key erasure) with actor, timestamp, operation and contract DID, followed by the entry summary and the integrity checks — DCS-IR-CSA-04, UC-07-03. The operations that name no single contract are returned to a caller who may read every contract.")
		Meta("dcs:requirements", "DCS-IR-CSA-04")
		Security(JWTAuth, func() {
			Scope("Auditor")
			Scope("Archive Manager")
		})
		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("did", String, "Optional archived contract DID filter")
			Attribute("justification", String, "Required audit justification", func() { Pattern(`\S`) })
			Required("justification")
		})

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		Error("forbidden", ErrorResult)
		Error("not_found", ErrorResult)
		HTTP(func() {
			Response("not_found", StatusNotFound)
			Response("forbidden", StatusForbidden)
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
