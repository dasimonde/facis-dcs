package design

import (
	. "goa.design/goa/v3/dsl"
)

var PDFVerifyResult = Type("PDFVerifyResult", func() {
	Description("Result of MR/HR hash consistency and C2PA provenance verification (DCS-FR-CWE-04, DCS-FR-CWE-05, DCS-OR-C2PA-006)")

	// MR/HR consistency (DCS-FR-CWE-04/05)
	Attribute("match", Boolean, "True when the stored PDF was generated from the embedded JSON-LD without alteration")
	// The three digests the match verdict is reached on, reported by pdf-core's
	// /verify. base_pdf_hash and stored_base_pdf_hash are equal exactly when the
	// stored document re-renders to its own bytes, so on a mismatch they name
	// which side diverged rather than leaving match=false unevidenced.
	Attribute("jsonld_hash", String, "SHA-256 hex of the machine-readable JSON-LD payload embedded in the stored PDF — the latest one on an amended document, i.e. the payload that governs its current visible state")
	Attribute("base_pdf_hash", String, "SHA-256 hex of the deterministic re-render produced from that payload: a bare recompile for a plain document, the replay of the last amendment hop for an incrementally updated one. Taken over the same COSE-zeroed normalization the comparison uses, since a fresh compile carries a fresh randomized claim signature.")
	Attribute("stored_base_pdf_hash", String, "SHA-256 hex of the stored bytes that re-render was compared against: the leading span of the stored PDF it must reproduce, excluding the append-only PAdES signature layers that legitimately follow, normalized the same way. Empty — like the other two — only where no digest could be taken at all: an artifact that failed authenticated decryption, or a verification that never reached the comparison. The discrepancy field names which.")

	// C2PA provenance validation (DCS-OR-C2PA-006). Both signature checks are
	// three-state rather than boolean: a check that could not be performed reports
	// so, and never borrows the vocabulary of one that passed.
	Attribute("c2pa_manifest_found", Boolean, "True when a C2PA JUMBF manifest was found in the PDF")
	Attribute("c2pa_signature_status", String, "C2PA COSE_Sign1 claim-signature verification status (DCS-OR-C2PA-006): 'valid' when every manifest's claim signature verified against its x5chain leaf and the assertions the signed claim commits to still hash to the recorded values, 'invalid' when one did not, 'not_available' when the PDF carries no manifest to check.")
	Attribute("vc_proof_status", String, "Embedded contract-lifecycle credential proof status: 'valid' when its Data Integrity proof verified against a key the credential's issuer publishes for assertions, 'invalid' when it did not, 'indeterminate' when the issuer could not be resolved, 'not_available' when the PDF carries no such credential. Never reports a pass for a credential that was only parsed.")
	Attribute("status_list_uri", String, "URI of the status list service queried for revocation check")
	Attribute("lifecycle_status", String, "Contract lifecycle state from the latest C2PA assertion (DCS-OR-C2PA-006 banner: draft, active, amended, suspended, terminated, expired, replaced)")
	Attribute("status_list_status", String, "Live revocation state queried from the XFSC status list service: active or revoked (DCS-OR-C2PA-006)")
	Attribute("status_list_check", String, "Named live status-list check result: passed, failed, or not_available (DCS-OR-C2PA-006)")
	Attribute("status_list_error", String, "Explicit failure reason when the live status-list check could not be completed")

	// PDF signature check (DCS-OR-C2PA-006). This is an independently named
	// check distinct from the C2PA COSE signature check: when the PDF carries
	// no PAdES signature, the verifier honestly reports "not_available" rather
	// than faking a passed PDF-signature verification.
	Attribute("pdf_signature_status", String, "PAdES/PDF signature verification status (DCS-OR-C2PA-006): 'not_available' when the PDF carries no PAdES signature, otherwise 'valid'/'invalid'. Never falsely reports a passed PDF signature check.")

	// Names WHY a verification failed, so a caller can tell failure classes
	// apart without inferring them from combinations of the booleans above.
	// 'artifact_not_authentic' is reachable only once artifacts are encrypted
	// at rest: the stored bytes fail authenticated decryption, so no claim can
	// be made about the content they were supposed to carry.
	Attribute("discrepancy", String, "Failure class when match is false: 'content_hash_mismatch' (manifest present, content differs from the embedded JSON-LD), 'artifact_not_authentic' (stored bytes failed authenticated decryption — altered or substituted at rest), or 'verification_failed' (any other check failure). Empty when match is true.")

	Required("match", "jsonld_hash", "base_pdf_hash", "stored_base_pdf_hash", "c2pa_manifest_found", "c2pa_signature_status", "vc_proof_status", "status_list_check", "pdf_signature_status")
})

// BundleExportRefusedError is returned when the FR-PACM-06 structural-integrity
// pre-flight check fails before zipping (FR-TR-26 behavior): a referenced
// component (e.g. an exported PDF) is missing or inconsistent, so the export is
// refused with a machine-readable findings list rather than shipping an
// incomplete ZIP.
var BundleExportRefusedError = Type("BundleExportRefusedError", func() {
	Description("Bundle export refused because the structural-integrity pre-flight check found missing/inconsistent components")
	Attribute("name", String, "Error name", func() {
		Meta("struct:error:name")
	})
	Attribute("message", String, "Human-readable summary of the refusal")
	Attribute("findings", ArrayOf(String), "The structural-integrity findings that caused the refusal")
	Required("name", "message", "findings")
})

// PDFGeneration Service  (/pdf/...)
var _ = Service("PDFGeneration", func() {
	Error("not_found", ErrorResult, "Contract or template not found")
	Error("internal_error", ErrorResult, "Internal server error")

	HTTP(func() {
		Response("not_found", StatusNotFound)
		Response("internal_error", StatusInternalServerError)
	})

	Description("PDF export and MR/HR hash verification for contracts and templates (DCS-FR-CWE-04, DCS-FR-CWE-05, DCS-OR-C2PA-001)")

	// export_contract_pdf — GET /pdf/export/contract/{did}
	Method("export_contract_pdf", func() {
		Description("Export a contract as a PDF/A-3 document with embedded JSON-LD and accumulated C2PA lifecycle assertions.")
		Meta("dcs:requirements", "DCS-FR-CWE-04", "DCS-FR-SM-27", "DCS-OR-C2PA-001")
		Security(JWTAuth, func() {
			Scope("Contract Manager")
			Scope("Contract Reviewer")
			Scope("Contract Creator")
			Scope("Contract Approver")
			Scope("Contract Observer")
			// The negotiate view offers Export PDF alongside every other
			// contract page, and a negotiator already reads the contract in full.
			Scope("Contract Negotiator")
		})
		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("did", String, "DID of the contract")
			Required("did")
		})
		HTTP(func() {
			GET("/pdf/export/contract/{did}")
			SkipResponseBodyEncodeDecode()
			Response(StatusOK, func() {
				ContentType("application/pdf")
			})
		})
	})

	// export_template_pdf — GET /pdf/export/template/{did}
	Method("export_template_pdf", func() {
		Description("Export a contract template as a PDF/A-3 document with embedded JSON-LD.")
		Meta("dcs:requirements", "DCS-FR-CWE-04", "DCS-FR-SM-27")
		Security(JWTAuth, func() {
			Scope("Template Manager")
			Scope("Template Reviewer")
			Scope("Template Creator")
			Scope("Template Approver")
		})
		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("did", String, "DID of the contract template")
			Required("did")
		})
		HTTP(func() {
			GET("/pdf/export/template/{did}")
			SkipResponseBodyEncodeDecode()
			Response(StatusOK, func() {
				ContentType("application/pdf")
			})
		})
	})

	// export_contract_bundle — GET /contract/export/{did}
	Method("export_contract_bundle", func() {
		Description("Export a contract as a ZIP bundle containing its JSON-LD, signed PDF, C2PA manifest store, extracted credentials, signature states, bundle manifest (with per-entry SHA-256), the parent chain upward under parents/, and every other locally-known member of its hierarchy family — descendants of the topmost locally-known ancestor, e.g. siblings — under related/ (FR-CWE-30). Family members held only by other instances are simply absent. Refuses with a findings list when a referenced component is missing (FR-TR-26/FR-PACM-06).")
		Meta("dcs:requirements", "DCS-FR-CWE-30", "DCS-FR-TR-26", "DCS-FR-PACM-06", "DCS-FR-CSA-18")
		Security(JWTAuth, func() {
			Scope("Contract Manager")
			Scope("Contract Reviewer")
			Scope("Contract Creator")
			Scope("Contract Approver")
			Scope("Contract Observer")
			// The negotiate view offers Export PDF alongside every other
			// contract page, and a negotiator already reads the contract in full.
			Scope("Contract Negotiator")
		})
		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("did", String, "DID of the contract")
			Required("did")
		})
		Result(func() {
			Attribute("content_type", String, "Media type of the response body (application/zip)")
		})
		Error("refused", BundleExportRefusedError, "Structural-integrity pre-flight refused the export")
		HTTP(func() {
			GET("/contract/export/{did}")
			SkipResponseBodyEncodeDecode()
			Response(StatusOK, func() {
				Header("content_type:Content-Type")
			})
			Response("refused", StatusUnprocessableEntity)
		})
	})

	// export_template_bundle — GET /template/export/{did}
	Method("export_template_bundle", func() {
		Description("Export a contract template as a ZIP bundle of flat artifacts: template JSON-LD, rendered PDF, and bundle manifest. No frame/parent chain directory (FR-TR-24/FR-TR-09 — templates are flat artifacts, no frame-type taxonomy).")
		Meta("dcs:requirements", "DCS-FR-TR-24", "DCS-FR-TR-09")
		Security(JWTAuth, func() {
			Scope("Template Manager")
			Scope("Template Reviewer")
			Scope("Template Creator")
			Scope("Template Approver")
		})
		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("did", String, "DID of the contract template")
			Required("did")
		})
		Result(func() {
			Attribute("content_type", String, "Media type of the response body (application/zip)")
		})
		Error("refused", BundleExportRefusedError, "Structural-integrity pre-flight refused the export")
		HTTP(func() {
			GET("/template/export/{did}")
			SkipResponseBodyEncodeDecode()
			Response(StatusOK, func() {
				Header("content_type:Content-Type")
			})
			Response("refused", StatusUnprocessableEntity)
		})
	})

	// verify_contract_pdf — GET /pdf/verify/contract/{did}
	Method("verify_contract_pdf", func() {
		Description("Verify MR/HR hash consistency for a contract: re-generates the base PDF from the embedded JSON-LD and compares SHA-256 hashes. If the contract's lifecycle state has advanced since the cached PDF was last generated, this transparently regenerates and re-caches a new C2PA-updated PDF (issuing a new provenance VC and re-uploading to IPFS) before comparing — i.e. this read endpoint can trigger a full PDF-generation write path. Requires that export_contract_pdf has been called at least once before; otherwise it errors. (DCS-FR-CWE-04, DCS-FR-CWE-05)")
		Meta("dcs:requirements", "DCS-FR-CWE-04", "DCS-FR-CWE-05", "DCS-FR-CSA-06")
		Security(JWTAuth, func() {
			Scope("Contract Manager")
			Scope("Contract Reviewer")
			Scope("Contract Approver")
			Scope("Contract Observer")
			Scope("Auditor")
		})
		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("did", String, "DID of the contract")
			Required("did")
		})
		Result(PDFVerifyResult)
		HTTP(func() {
			GET("/pdf/verify/contract/{did}")
			Response(StatusOK)
		})
	})

	// verify_template_pdf — GET /pdf/verify/template/{did}
	Method("verify_template_pdf", func() {
		Description("Verify MR/HR hash consistency for a contract template. Like verify_contract_pdf, this transparently regenerates and re-caches a new C2PA-updated PDF if the template's lifecycle state has advanced since the cached PDF was last generated, and requires that export_template_pdf has been called at least once before.")
		Meta("dcs:requirements", "DCS-FR-CWE-04", "DCS-FR-CWE-05")
		Security(JWTAuth, func() {
			Scope("Template Manager")
			Scope("Template Reviewer")
			Scope("Auditor")
		})
		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("did", String, "DID of the contract template")
			Required("did")
		})
		Result(PDFVerifyResult)
		HTTP(func() {
			GET("/pdf/verify/template/{did}")
			Response(StatusOK)
		})
	})
})
