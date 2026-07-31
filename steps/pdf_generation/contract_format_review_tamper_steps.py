"""Step definitions for the two contract_format_review.feature scenarios
that need a REAL, server-observable MR/HR (machine-readable/human-readable)
inconsistency: "System highlights inconsistencies between formats" and
"Fix inconsistency and re-validate".

Both scenarios are self-contained: unlike the other scenarios in this
feature file, they have no separate "contract ... exists" Given before the
inconsistency-inducing one, so `_induce_pdf_format_inconsistency` below
creates the contract itself.

Why a byte-flip (not a "different contract's PDF") induces the
inconsistency: GET /pdf/verify/contract/{did} (backend/internal/
pdfgeneration/query/verifycontract.go -> runVerify -> pdf-core POST /verify)
only checks the STORED PDF's own internal self-consistency — does its
embedded JSON-LD, recompiled, reproduce the visible PDF bytes — it never
cross-references the contract's live JSON-LD row in Postgres. Swapping in an
entirely different (but internally self-consistent) PDF would therefore
still verify as match=true; only actually corrupting the stored PDF's base
layer produces an observable MR/HR discrepancy from this endpoint. This is
the exact same seam as "Tampered PDF fails hash verification" in
contract_format_review.feature — reused here under different Given/Then
step text, and reused again for "Fix inconsistency and re-validate" (the
"fix" restores the original CID rather than re-exporting; see the docstring
on `_induce_pdf_format_inconsistency` for why re-export does not work here).
"""

from __future__ import annotations

from behave import given, then, when

from steps.support.services.contract_service import ContractService
from steps.support.services.pdf_service import PDFService
from steps.support.tamper_seam import swap_contract_pdf_cid


def _induce_pdf_format_inconsistency(context, name):
    """Create contract `name`, export its real PDF, corrupt a byte in the
    base layer, and swap it in as the contract's stored PDF via the
    CID-swap seam. Records the swap's restore() handle on
    context.pdf_inconsistency_restore[name] so "I fix the inconsistency"
    can invoke it directly (proactive repair), in addition to the automatic
    context.add_cleanup safety net the seam itself registers.

    Note on why "the fix" cannot be a plain re-export: export_contract_pdf
    (backend/internal/pdfgeneration/query/exportcontract.go) has a cache-hit
    fast path that serves the CACHED PDF as-is — fetched from whatever CID
    is currently in pdf_ipfs_cid — whenever the cached C2PA state AND
    payload hash both still match the contract's current state/JSON-LD.
    Neither changes when we swap the CID (we deliberately leave
    pdf_c2pa_state/pdf_payload_hash untouched, see tamper_seam.py), so a
    plain re-export would just re-serve the tampered bytes, not regenerate
    past them. The only real fix for content-addressed corruption is
    re-pointing at known-good content.
    """
    ContractService._create_contract_in_draft(context, name)
    did, _ = ContractService._contract_data(context, name)
    resp = PDFService.export_contract_pdf(context, did)
    assert resp.status_code == 200, (
        f"Failed to export baseline PDF for contract '{name}': "
        f"{resp.status_code} — {resp.text}"
    )
    raw = bytearray(resp.content)
    eof_pos = raw.find(b"%%EOF")
    assert eof_pos > 10, "exported PDF has no %%EOF marker to tamper before"
    raw[eof_pos - 5] ^= 0xFF
    tampered = bytes(raw)

    swap = swap_contract_pdf_cid(context, did, tampered)

    if not hasattr(context, "pdf_bytes"):
        context.pdf_bytes = {}
    context.pdf_bytes[name] = tampered

    if not hasattr(context, "pdf_inconsistency_restore"):
        context.pdf_inconsistency_restore = {}
    context.pdf_inconsistency_restore[name] = swap.restore
    context.pdf_inconsistency_contract = name
    return swap


# ---------------------------------------------------------------------------
# Given
# ---------------------------------------------------------------------------


@given('contract "{name}" has a formatting error')
def step_given_formatting_error(context, name):
    _induce_pdf_format_inconsistency(context, name)


@given('contract "{name}" has a detected inconsistency')
def step_given_detected_inconsistency(context, name):
    _induce_pdf_format_inconsistency(context, name)


# ---------------------------------------------------------------------------
# When
# ---------------------------------------------------------------------------


@when('I review both formats of contract "{name}"')
def step_when_review_both_formats(context, name):
    did, _ = ContractService._contract_data(context, name)
    context.requests_response = PDFService.verify_contract_pdf(context, did)
    if context.requests_response.status_code == 200:
        context.verify_result = context.requests_response.json()


@when("I fix the inconsistency")
def step_when_fix_inconsistency(context):
    name = getattr(context, "pdf_inconsistency_contract", None)
    assert name, (
        "No contract with a detected inconsistency is tracked in this scenario "
        '(expected a prior \'contract "..." has a detected inconsistency\' step)'
    )
    restore = getattr(context, "pdf_inconsistency_restore", {}).pop(name, None)
    assert restore, f"No inconsistency-restore handle recorded for contract '{name}'"
    restore()


@when('I re-validate contract "{name}"')
def step_when_revalidate_contract(context, name):
    did, _ = ContractService._contract_data(context, name)
    context.requests_response = PDFService.verify_contract_pdf(context, did)
    if context.requests_response.status_code == 200:
        context.verify_result = context.requests_response.json()


# ---------------------------------------------------------------------------
# Then
# ---------------------------------------------------------------------------


def _verify_result(context):
    result = getattr(context, "verify_result", None)
    if result is None and context.requests_response.status_code == 200:
        result = context.requests_response.json()
    assert result is not None, (
        f"No verification result available (status="
        f"{context.requests_response.status_code}, body="
        f"{context.requests_response.text[:2000]!r})"
    )
    return result


@then("the system highlights inconsistencies")
def step_then_system_highlights_inconsistencies(context):
    result = _verify_result(context)
    assert result.get("match") is False, (
        f"Expected the induced MR/HR formatting error to be reported as a "
        f"mismatch (match=false), got: {result}"
    )


@then("the specific discrepancies are identified")
def step_then_specific_discrepancies_identified(context):
    # Precision limit: PDFVerifyResult's jsonld_hash/base_pdf_hash/
    # stored_base_pdf_hash fields (backend/design/pdf_generation.go) are
    # declared but never populated by any code path today (grep
    # backend/internal/pdfgeneration -rn "JsonldHash" finds no assignment) —
    # they always serialize as empty strings, so asserting on them would
    # trivially "pass" without checking anything real. The one field the
    # current implementation actually differentiates by failure class is
    # c2pa_manifest_found: pdf-core's /verify reports HTTP 409 specifically
    # for "manifest present but content hash comparison failed" (a genuine
    # MR/HR discrepancy, the exact class this scenario induces), which
    # runVerify (backend/internal/pdfgeneration/query/common.go) maps to
    # c2pa_manifest_found=true even though match=false — distinguishing it
    # from other failure classes (manifest missing entirely, transport
    # error) where c2pa_manifest_found is false.
    #
    # Since artifacts are encrypted at rest, the CID-swap seam plants bytes the
    # instance never sealed, so verify cannot open them to look for a manifest
    # at all. That is itself a named class — `discrepancy`
    # ("artifact_not_authentic") — and it is a STRONGER statement than a hash
    # mismatch: the stored bytes are provably not the ones this instance wrote.
    # Either named class satisfies "the specific discrepancies are identified";
    # an undifferentiated failure does not.
    result = _verify_result(context)
    assert result.get("match") is False
    discrepancy = result.get("discrepancy")
    assert result.get("c2pa_manifest_found") is True or discrepancy in (
        "content_hash_mismatch",
        "artifact_not_authentic",
    ), (
        f"Expected the verify response to identify the SPECIFIC discrepancy "
        f"class (c2pa_manifest_found=true for a content hash mismatch, or "
        f"discrepancy naming the failure class) rather than a generic/"
        f"undifferentiated failure, got: {result}"
    )


@then("no inconsistencies are highlighted")
def step_then_no_inconsistencies_highlighted(context):
    result = _verify_result(context)
    assert result.get("match") is True, (
        f"Expected match=true after fixing the inconsistency, got: {result}"
    )


@then("both formats are synchronized")
def step_then_formats_synchronized(context):
    result = _verify_result(context)
    assert result.get("match") is True
    assert result.get("c2pa_manifest_found") is True, (
        f"Expected the manifest to be found and self-consistent after the fix, "
        f"got: {result}"
    )
