"""Step definitions for real_signing_vertical.feature's evidence-tamper
scenario ("Corrupting the signature-evidence attachment invalidates the
embedded-PID cross-check"), using the IPFS CID-swap seam
(steps/support/tamper_seam.py) to get a corrupted, server-STORED signed PDF
the same way contract_format_review.feature's "Tampered PDF fails hash
verification" does.

--- Why this corrupts the evidence stream bytes IN PLACE rather than
deleting the attachment ---

pdf-core embeds the signing evidence (SD-JWT VC + KB-JWT PID presentation +
ContractSigningSummaryCredential) as a PDF embedded-file object BEFORE
applying the PAdES signature (embed-first-sign-second — pdf-core/compiler/
signing_evidence.go's EmbedSigningEvidence), so its bytes fall inside the
PAdES signature's /ByteRange-covered region. Actually DELETING the
attachment (shrinking the file) would shift every subsequent byte offset,
corrupting the PDF's own xref table and /ByteRange fields — the PDF would
stop parsing altogether, which is a different (uninteresting) failure mode.
Flipping bytes strictly WITHIN the embedded-file stream's data (same
length, same offsets, found via the same filespec/EF/object/stream-marker
walk as pdf-core's own ExtractSigningEvidence in that file) keeps the PDF
otherwise well-formed while genuinely destroying the evidence content —
exactly the "mutate bytes inside the signed ByteRange" seam this scenario
needs.

--- Why the observable signal is /signature/validate's PID cross-check,
not a "PAdES signature is now invalid" claim ---

No endpoint reachable from this black-box harness cryptographically
re-verifies the PAdES CMS signature over its /ByteRange (grepping pdf-core
confirms no such check exists — see compiler/compiler_verify.go, which only
does C2PA page-content-coverage checks). Moreover pdf-core's own /verify
(reached via GET /pdf/verify/contract/{did} and POST /signature/validate's
"Document integrity check") treats the entire PAdES-signed span — including
the evidence attachment — as an OPAQUE, unchecked suffix by design (see
compiler/update.go's VerifyIncrementalUpdate docstring: "a PAdES signature
and its signing-evidence attachment may sit, opaque to this check, between
any two updates"). So corrupting the evidence bytes is invisible to both of
those checks. The ONE place this codebase actually re-derives meaning from
the evidence attachment is backend/internal/signingmanagement/query/
validate.go's crossCheckEmbeddedPID, which extracts it via pdf-core's
POST /evidence/extract and JSON-decodes it to re-verify the embedded PID
presentation. Corrupting the evidence bytes makes that JSON decode fail,
which crossCheckEmbeddedPID surfaces as a genuine, real, distinctly-worded
validation finding ("Embedded signing evidence is missing the PID
presentation") in place of its normal positive finding ("Embedded PID
presentation re-verified and cross-checked against the signature record").
That finding — a real behavioral difference this codebase's actual code
produces — is what this scenario asserts on, rather than a fabricated
"PAdES cryptographic signature invalid" claim this harness cannot honestly
prove.
"""

from __future__ import annotations

from behave import then, when

from steps.support.api_client import signature_validate_url, post_json
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService
from steps.support.tamper_seam import swap_contract_pdf_cid


def _pdf_bytes_for(context, name) -> bytes:
    pdf_bytes = getattr(context, "pdf_bytes", {}).get(name)
    assert pdf_bytes, f"no exported/signed PDF bytes recorded for contract '{name}'"
    return pdf_bytes


def _corrupt_signing_evidence_stream(pdf_bytes: bytes) -> bytes:
    """Flip every byte of the signing-evidence embedded-file stream, found
    via the exact same filespec -> /EF -> object -> "stream\\n"..."\\nendstream"
    walk as pdf-core's ExtractSigningEvidence (pdf-core/compiler/
    signing_evidence.go), so the byte range located here is provably the
    same one pdf-core itself would extract — not a heuristic guess.
    """
    spec_marker = b"/F (signing-evidence.json)"
    spec_pos = pdf_bytes.rfind(spec_marker)
    assert spec_pos != -1, "no signing-evidence.json filespec found in the signed PDF"

    ef_marker = b"/EF << /F "
    ef_pos = pdf_bytes.find(ef_marker, spec_pos)
    assert ef_pos != -1, "signing-evidence.json filespec is missing its /EF reference"
    ef_pos += len(ef_marker)

    ref_end = pdf_bytes.find(b" 0 R", ef_pos)
    assert ref_end != -1, "signing-evidence.json object reference is malformed"
    obj_id = int(pdf_bytes[ef_pos:ref_end].strip())

    obj_marker = f"{obj_id} 0 obj".encode()
    obj_pos = pdf_bytes.rfind(obj_marker)
    assert obj_pos != -1, f"signing-evidence.json object {obj_id} not found"

    stream_marker = b"stream\n"
    marker_pos = pdf_bytes.find(stream_marker, obj_pos)
    assert marker_pos != -1, "signing-evidence.json stream start not found"
    stream_start = marker_pos + len(stream_marker)

    stream_end = pdf_bytes.find(b"\nendstream", stream_start)
    assert stream_end != -1, "signing-evidence.json stream end not found"
    assert stream_end > stream_start, "signing-evidence.json stream is empty — nothing to corrupt"

    mutated = bytearray(pdf_bytes)
    for i in range(stream_start, stream_end):
        mutated[i] ^= 0xFF
    return bytes(mutated)


@when('the signature-evidence attachment for contract "{name}" is corrupted on the server-stored PDF')
def step_when_corrupt_signature_evidence(context, name):
    did, _ = ContractService._contract_data(context, name)
    signed_pdf = _pdf_bytes_for(context, name)
    corrupted = _corrupt_signing_evidence_stream(signed_pdf)
    swap_contract_pdf_cid(context, did, corrupted)
    context.pdf_bytes[name] = corrupted


@then('the signature validation findings for contract "{name}" report the embedded signing evidence as invalid')
def step_then_evidence_reported_invalid(context, name):
    did, _ = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    resp = post_json(context, signature_validate_url(context), {"did": did}, headers=manager_h)
    assert resp.status_code == 200, (
        f"/signature/validate failed for contract '{name}': {resp.status_code} {resp.text}"
    )
    findings = resp.json().get("findings") or []
    body_text = " ".join(findings).lower()
    assert "evidence" in body_text and (
        "missing" in body_text or "invalid" in body_text or "failed" in body_text or "mismatch" in body_text
    ), (
        f"Expected /signature/validate to report the corrupted signing evidence as "
        f"invalid/missing (distinct from its normal positive 'PID presentation "
        f"re-verified' finding), got findings: {findings}"
    )
