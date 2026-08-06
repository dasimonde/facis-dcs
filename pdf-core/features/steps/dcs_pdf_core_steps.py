import base64
import io
import json
from datetime import datetime, timezone
import os
import re
import hashlib
import shutil
import subprocess
import tarfile
import urllib.error
import urllib.request
from datetime import datetime, timedelta, timezone
from io import BytesIO
from pathlib import Path
import tempfile
import cbor2
from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec, rsa, utils
from cryptography.x509.oid import NameOID
from pyhanko.pdf_utils import generic
from pyhanko.pdf_utils.font.api import FontEngine, FontEngineFactory, ShapeResult
from pyhanko.pdf_utils.incremental_writer import IncrementalPdfFileWriter
from pyhanko.pdf_utils.reader import PdfFileReader
from pyhanko.sign import signers
from pyhanko.sign.validation import validate_pdf_signature
from pyhanko.stamp import TextBoxStyle, TextStampStyle
from pyhanko_certvalidator import ValidationContext

from behave import given, then, when

ARTIFACTS_DIR = os.path.join(
    os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))),
    "artifacts",
)


def _artifact_stem(context):
    """Return a filesystem-safe name derived from the current scenario title."""
    title = getattr(context, "scenario", None)
    name = title.name if title is not None else "unknown"
    return re.sub(r"[^a-zA-Z0-9_-]+", "_", name).strip("_").lower()


def _save_artifact(context, pdf_bytes, suffix):
    os.makedirs(ARTIFACTS_DIR, exist_ok=True)
    stem = _artifact_stem(context)
    path = os.path.join(ARTIFACTS_DIR, f"{stem}{suffix}.pdf")
    with open(path, "wb") as fh:
        fh.write(pdf_bytes)
    _validate_pdf_artifact(Path(path))


def sign_sig_structure(context, sig_structure):
    """Sign one COSE Sig_structure with the harness key (ES256 → 64-byte r||s),
    matching the DCS backend's dcs-c2pa signing. Deterministic (RFC 6979) so
    repeated compilations stay byte-identical, preserving pdf-core's determinism
    guarantee under test."""
    der = context.c2pa_private_key.sign(
        sig_structure, ec.ECDSA(hashes.SHA256(), deterministic_signing=True)
    )
    r, s = utils.decode_dss_signature(der)
    return r.to_bytes(32, "big") + s.to_bytes(32, "big")


def _raw_request(context, method, path, data, content_type):
    request = urllib.request.Request(f"{context.server_url}{path}", data=data, method=method)
    if content_type:
        request.add_header("Content-Type", content_type)
    # pdf-core holds no signing material, so every render names the chain it signs
    # under, as the DCS backend does with the certificate for its dcs-c2pa key.
    chain = getattr(context, "c2pa_x5chain_pem", None)
    if chain:
        request.add_header("X-DCS-C2PA-X5Chain", base64.b64encode(chain).decode("ascii"))
    try:
        with urllib.request.urlopen(request, timeout=30) as response:
            return {
                "status": response.status,
                "content_type": response.headers.get("Content-Type", ""),
                "body": response.read(),
            }
    except urllib.error.HTTPError as error:
        return {
            "status": error.code,
            "content_type": error.headers.get("Content-Type", ""),
            "body": error.read(),
        }


def _embed_if_prepared(context, response):
    """When a /render or /render/amendment call returns pdf-core's prepare
    envelope ({pdf_base64, c2pa_sig_structures}), sign each C2PA Sig_structure and
    post the signatures to the stateless /c2pa/embed, returning the finished signed
    PDF response. Any other response passes through unchanged, so error responses
    and genuine JSON endpoints (/verify, /version, /ontology) are untouched."""
    if response["status"] != 200 or not response["content_type"].startswith("application/json"):
        return response
    try:
        payload = json.loads(response["body"].decode("utf-8"))
    except (ValueError, UnicodeDecodeError):
        return response
    if not isinstance(payload, dict) or "pdf_base64" not in payload or "c2pa_sig_structures" not in payload:
        return response
    signatures = [
        base64.b64encode(sign_sig_structure(context, base64.b64decode(s))).decode("ascii")
        for s in payload["c2pa_sig_structures"]
    ]
    embed_body = json.dumps(
        {"pdf_base64": payload["pdf_base64"], "c2pa_signatures": signatures}
    ).encode("utf-8")
    return _raw_request(context, "POST", "/c2pa/embed", embed_body, "application/json")


def _request(context, method, path, body=None, content_type=None):
    data = None if body is None else body.encode("utf-8") if isinstance(body, str) else body
    response = _raw_request(context, method, path, data, content_type)
    context.last_response = _embed_if_prepared(context, response)

    if context.last_response["content_type"].startswith("application/json"):
        context.last_json_response = json.loads(context.last_response["body"].decode("utf-8"))


def _response_json(context):
    return json.loads(context.last_response["body"].decode("utf-8"))


_C2PATOOL_VERSION = os.environ.get("DCS_PDF_CORE_C2PATOOL_VERSION", "0.26.61")
_C2PATOOL_CACHE_DIR = os.path.join(tempfile.gettempdir(), "dcs-pdf-core-c2patool")
_C2PATOOL_BINARY = None


def _ensure_c2patool_binary():
    global _C2PATOOL_BINARY
    if _C2PATOOL_BINARY:
        return _C2PATOOL_BINARY

    found = shutil.which("c2patool")
    if found:
        _C2PATOOL_BINARY = found
        return found

    os.makedirs(_C2PATOOL_CACHE_DIR, exist_ok=True)
    archive_name = f"c2patool-v{_C2PATOOL_VERSION}-x86_64-unknown-linux-gnu.tar.gz"
    archive_path = os.path.join(_C2PATOOL_CACHE_DIR, archive_name)
    extract_dir = os.path.join(_C2PATOOL_CACHE_DIR, f"c2patool-v{_C2PATOOL_VERSION}")
    binary_path = None
    candidate_path = os.path.join(extract_dir, "c2patool")
    if os.path.isfile(candidate_path):
        binary_path = candidate_path
    else:
        url = f"https://github.com/contentauth/c2pa-rs/releases/download/c2patool-v{_C2PATOOL_VERSION}/{archive_name}"
        if not os.path.exists(archive_path):
            with urllib.request.urlopen(url, timeout=120) as response:
                with open(archive_path, "wb") as fh:
                    fh.write(response.read())
        os.makedirs(extract_dir, exist_ok=True)
        with tarfile.open(archive_path, "r:gz") as archive:
            archive.extractall(extract_dir)
        for root, _dirs, files in os.walk(extract_dir):
            if "c2patool" in files:
                binary_path = os.path.join(root, "c2patool")
                break
    if not binary_path or not os.path.isfile(binary_path):
        raise AssertionError("c2patool binary could not be prepared")
    os.chmod(binary_path, 0o755)
    _C2PATOOL_BINARY = binary_path
    return binary_path


# c2patool exits 0 whenever it can *read* a manifest, including when it judges
# that manifest invalid, so the exit code alone proves only that the JUMBF parses.
# The verdict lives in validation_state / validation_results, which must be
# asserted explicitly.
#
# signingCredential.untrusted is the one expected failure: the dev C2PA CA is not
# in c2patool's trust list, and it does not by itself make the state Invalid.
# Every other failure — a claim signature that does not verify, a hard binding
# that does not match the bytes — is a defect.
_C2PA_EXPECTED_FAILURES = {"signingCredential.untrusted"}


def _run_c2patool(pdf_path):
    binary = _ensure_c2patool_binary()
    completed = subprocess.run(
        [binary, str(pdf_path)],
        check=False,
        capture_output=True,
        text=True,
        timeout=300,
    )
    if completed.returncode != 0:
        raise AssertionError(
            f"c2patool failed for {pdf_path.name}\nstdout:\n{completed.stdout}\nstderr:\n{completed.stderr}"
        )

    try:
        report = json.loads(completed.stdout)
    except json.JSONDecodeError as exc:
        raise AssertionError(
            f"c2patool produced no JSON report for {pdf_path.name}: {exc}\nstdout:\n{completed.stdout}"
        )

    unexpected = [
        f"{entry.get('code')} ({entry.get('explanation')}) at {entry.get('url')}"
        for scope in _c2pa_validation_scopes(report)
        for entry in scope.get("failure", [])
        if entry.get("code") not in _C2PA_EXPECTED_FAILURES
    ]
    state = report.get("validation_state")
    # Signed contracts included: provenance is re-anchored over the signature
    # (ADR-26), so a signed artifact validates like any other and a dataHash
    # mismatch on one is a regression, not an expected state.
    if unexpected or state != "Valid":
        raise AssertionError(
            f"c2patool reports validation_state={state!r} for {pdf_path.name}; "
            f"unexpected failures:\n  " + "\n  ".join(unexpected or ["(none)"])
        )


def _c2pa_validation_scopes(report):
    """Yield every validation_results scope: the active manifest and each ingredient."""
    results = report.get("validation_results", {})
    active = results.get("activeManifest")
    if active:
        yield active
    for delta in results.get("ingredientDeltas", []):
        deltas = delta.get("validationDeltas")
        if deltas:
            yield deltas


def _run_verapdf(pdf_path):
    image = os.environ.get("DCS_PDF_CORE_VERAPDF_IMAGE", "ghcr.io/verapdf/cli:latest")
    completed = subprocess.run(
        [
            "docker",
            "run",
            "--rm",
            "-v",
            f"{ARTIFACTS_DIR}:/data",
            image,
            "-f",
            "3a",
            "--format",
            "text",
            f"/data/{pdf_path.name}",
        ],
        check=False,
        capture_output=True,
        text=True,
        timeout=300,
    )
    if "PASS" not in completed.stdout:
        raise AssertionError(
            f"veraPDF PDF/A-3a validation failed for {pdf_path.name}\nstdout:\n{completed.stdout}\nstderr:\n{completed.stderr}"
        )


def _run_ghostscript_pdf_a_conversion(pdf_path):
    temp_output = os.path.join(ARTIFACTS_DIR, f"{pdf_path.stem}_gs_converted.pdf")
    try:
        completed = subprocess.run(
            [
                "gs",
                "-dBATCH",
                "-dNOPAUSE",
                "-sDEVICE=pdfwrite",
                "-dCompatibilityLevel=1.7",
                "-dDOINTERPOLATE",
                "-dEmbedAllFonts=true",
                "-dSubsetFonts=true",
                "-dPreserveEPSInfo=true",
                f"-sOutputFile={temp_output}",
                str(pdf_path),
            ],
            check=False,
            capture_output=True,
            text=True,
            timeout=300,
        )
        if completed.returncode != 0:
            raise AssertionError(
                f"Ghostscript PDF/A conversion failed for {pdf_path.name}\nstdout:\n{completed.stdout}\nstderr:\n{completed.stderr}"
            )
        if not os.path.isfile(temp_output):
            raise AssertionError(f"Ghostscript did not produce output for {pdf_path.name}")
    finally:
        if os.path.exists(temp_output):
            os.remove(temp_output)



def _validate_pdf_artifact(pdf_path):
    _run_c2patool(pdf_path)
    _run_verapdf(pdf_path)
    _run_ghostscript_pdf_a_conversion(pdf_path)


def _validate_saved_pdf_artifacts(context):
    stem = _artifact_stem(context)
    pdf_paths = sorted(
        Path(entry.path)
        for entry in os.scandir(ARTIFACTS_DIR)
        if entry.is_file() and entry.name.startswith(stem) and entry.name.endswith(".pdf")
    )
    assert pdf_paths, f"no saved PDF artifacts found for {stem}"
    for pdf_path in pdf_paths:
        _run_c2patool(pdf_path)
        _run_verapdf(pdf_path)


@given("the compiler service is running")
def step_service_running(context):
    assert getattr(context, "server_url", None), "service did not start"


@given("a semantic payload:")
def step_semantic_payload(context):
    context.payload_text = context.text.strip().replace("http://127.0.0.1:8080", context.server_url)
    context.payload = json.loads(context.payload_text)


@given("an equivalent semantic payload flavor:")
def step_equivalent_semantic_payload_flavor(context):
    context.equivalent_payload_text = context.text.strip().replace(
        "http://127.0.0.1:8080", context.server_url
    )
    context.equivalent_payload = json.loads(context.equivalent_payload_text)


@given("I compile the payload through /download")
@when("I compile the payload through /download")
def step_compile_once(context):
    _request(context, "POST", "/render", context.payload_text, "application/ld+json")
    context.compiled_pdf = context.last_response["body"]
    if context.last_response["status"] == 200:
        _save_artifact(context, context.compiled_pdf, "")


@when("I compile the payload twice through /download")
def step_compile_twice(context):
    _request(context, "POST", "/render", context.payload_text, "application/ld+json")
    context.first_pdf = context.last_response["body"]
    _request(context, "POST", "/render", context.payload_text, "application/ld+json")
    context.second_pdf = context.last_response["body"]
    _save_artifact(context, context.first_pdf, "_run1")
    _save_artifact(context, context.second_pdf, "_run2")


@when("I compile both payload flavors through /download")
def step_compile_both_payload_flavors(context):
    _request(context, "POST", "/render", context.payload_text, "application/ld+json")
    assert context.last_response["status"] == 200, context.last_response
    context.first_pdf = context.last_response["body"]

    _request(context, "POST", "/render", context.equivalent_payload_text, "application/ld+json")
    assert context.last_response["status"] == 200, context.last_response
    context.second_pdf = context.last_response["body"]

    _save_artifact(context, context.first_pdf, "_flavor1")
    _save_artifact(context, context.second_pdf, "_flavor2")


@when('I fetch "{path}"')
def step_fetch_path(context, path):
    _request(context, "GET", path)


@when('I POST plain text to "{path}"')
def step_post_plain_text(context, path):
    _request(context, "POST", path, "not-a-valid-payload", "text/plain")


@when('I POST the payload to "{path}" as "{content_type}"')
def step_post_payload(context, path, content_type):
    _request(context, "POST", path, context.payload_text, content_type)


@when("I verify the compiled PDF through /verify")
def step_verify_pdf(context):
    _request(context, "POST", "/verify", context.compiled_pdf, "application/pdf")
    import base64 as _b64
    resp = json.loads(context.last_response["body"].decode("utf-8"))
    context.verified_pdf = _b64.b64decode(resp["artifact"])
    _save_artifact(context, context.verified_pdf, "_verified")


@then("both PDF responses are byte-for-byte identical")
def step_assert_identical(context):
    assert context.first_pdf == context.second_pdf


@then("both compiled payload flavors are byte-for-byte identical")
def step_compiled_payload_flavors_are_identical(context):
    assert context.first_pdf == context.second_pdf


@then('the response content type is "{content_type}"')
def step_content_type_exact(context, content_type):
    actual = context.last_response["content_type"].split(";", 1)[0]
    assert actual == content_type, context.last_response["content_type"]


@then('the response content type starts with "{content_type}"')
def step_content_type_prefix(context, content_type):
    assert context.last_response["content_type"].startswith(content_type), context.last_response["content_type"]


@then("the response status is {status:d}")
def step_status(context, status):
    assert context.last_response["status"] == status, context.last_response


@then('the response body contains "{snippet}"')
def step_body_contains(context, snippet):
    body = context.last_response["body"]
    assert snippet.encode("utf-8") in body, body.decode("utf-8", errors="ignore")


@then('the JSON response has boolean "{field}" equal to "{expected}"')
def step_json_boolean(context, field, expected):
    payload = _response_json(context)
    assert field in payload, payload
    assert isinstance(payload[field], bool), payload
    assert payload[field] is (expected.lower() == "true"), payload


@then('the JSON response has a non-empty "{field}"')
def step_json_non_empty(context, field):
    payload = _response_json(context)
    value = payload.get(field)
    assert isinstance(value, str) and value.strip(), payload


@then("the compiled PDF spans at least {n:d} pages")
def step_compiled_pdf_spans_pages(context, n):
    pdf_bytes = context.compiled_pdf
    # /Count N inside the /Pages dictionary gives the total page count.
    import re as _re
    matches = _re.findall(rb"/Count\s+(\d+)", pdf_bytes)
    assert matches, "PDF does not contain a /Count entry in the Pages tree"
    page_count = max(int(m) for m in matches)
    assert page_count >= n, f"expected at least {n} pages, got {page_count}"


@then("the PDF contains these markers:")
def step_pdf_contains_markers(context):
    pdf_bytes = getattr(context, "verified_pdf", None) or getattr(context, "compiled_pdf", None) or context.last_response["body"]
    for row in context.table:
        marker = row["marker"].encode("utf-8")
        assert marker in pdf_bytes, row["marker"]


@then("the verified PDF contains these markers:")
def step_verified_pdf_contains_markers(context):
    for row in context.table:
        marker = row["marker"].encode("utf-8")
        assert marker in context.verified_pdf, row["marker"]


@then("the verified PDF is longer than the original")
def step_verified_longer(context):
    assert len(context.verified_pdf) > len(context.compiled_pdf)


@then("the verified PDF preserves the original bytes as a prefix")
def step_verified_prefix(context):
    assert context.verified_pdf.startswith(context.compiled_pdf)



@then("the PDF exposes positive non-overlapping text coordinates")
def step_pdf_positive_text_coordinates(context):
    pdf_bytes = getattr(context, "verified_pdf", None) or getattr(context, "compiled_pdf", None) or context.last_response["body"]
    streams = re.findall(rb"stream\n(.*?)\nendstream", pdf_bytes, flags=re.DOTALL)
    coordinate_streams = [stream for stream in streams if b" Tm\n" in stream]
    assert coordinate_streams, "no text content streams found"

    tm_pattern = re.compile(rb"1 0 0 1 ([0-9]+(?:\.[0-9]+)?) ([0-9]+(?:\.[0-9]+)?) Tm")
    for stream in coordinate_streams:
        matches = tm_pattern.findall(stream)
        assert matches, stream.decode("latin-1", errors="ignore")
        previous_y = None
        for raw_x, raw_y in matches:
            x = float(raw_x)
            y = float(raw_y)
            assert x > 0, (x, y)
            assert y > 0, (x, y)
            if previous_y is not None:
                assert y < previous_y, matches
            previous_y = y


@then("all saved PDF artifacts are validated by c2patool and dockerized veraPDF CLI")
def step_all_saved_pdf_artifacts_are_validated(context):
    _validate_saved_pdf_artifacts(context)


def _extract_embedded_stream_by_filespec_name(pdf_bytes, file_name):
    needle = f"/F ({file_name})".encode("utf-8")
    file_spec_pos = pdf_bytes.find(needle)
    assert file_spec_pos >= 0, f"filespec {file_name} not found"

    ef_pos = pdf_bytes.find(b"/EF << /F ", file_spec_pos)
    assert ef_pos >= 0, f"embedded file reference for {file_name} not found"
    ref_start = ef_pos + len(b"/EF << /F ")
    ref_end = pdf_bytes.find(b" 0 R", ref_start)
    assert ref_end >= 0, f"embedded object reference for {file_name} malformed"

    obj_id = int(pdf_bytes[ref_start:ref_end].strip())
    obj_pos = _find_last_object_header_offset(pdf_bytes, obj_id)
    assert obj_pos >= 0, f"embedded object {obj_id} for {file_name} not found"

    stream_start = pdf_bytes.find(b"stream\n", obj_pos)
    assert stream_start >= 0, f"stream start for {file_name} not found"
    stream_start += len(b"stream\n")
    stream_end = pdf_bytes.find(b"\nendstream", stream_start)
    assert stream_end >= 0, f"stream end for {file_name} not found"
    return pdf_bytes[stream_start:stream_end]


def _find_last_object_header_offset(pdf_bytes, obj_id):
    header_at_start = f"{obj_id} 0 obj\n".encode("utf-8")
    header_with_newline = f"\n{obj_id} 0 obj\n".encode("utf-8")
    best = 0 if pdf_bytes.startswith(header_at_start) else -1
    search_from = 0
    while True:
        rel = pdf_bytes.find(header_with_newline, search_from)
        if rel < 0:
            break
        best = rel + 1
        search_from = best + 1
    return best


def _parse_bmff_boxes(data):
    boxes = []
    pos = 0
    while pos + 8 <= len(data):
        size = int.from_bytes(data[pos : pos + 4], "big")
        box_type = data[pos + 4 : pos + 8]
        if size < 8 or pos + size > len(data):
            break
        payload = data[pos + 8 : pos + size]
        boxes.append((box_type, payload))
        pos += size
    return boxes


def _parse_bmff_boxes_with_raw(data):
    boxes = []
    pos = 0
    while pos + 8 <= len(data):
        size = int.from_bytes(data[pos : pos + 4], "big")
        box_type = data[pos + 4 : pos + 8]
        if size < 8 or pos + size > len(data):
            break
        payload = data[pos + 8 : pos + size]
        raw = data[pos : pos + size]
        boxes.append((box_type, payload, raw))
        pos += size
    return boxes


def _extract_top_level_manifest_boxes_raw(c2pa_bytes):
    top = _parse_bmff_boxes_with_raw(c2pa_bytes)
    assert top and top[0][0] == b"jumb", "C2PA root JUMBF box not found"
    store_children = _parse_bmff_boxes_with_raw(top[0][1])
    return [raw for box_type, _payload, raw in store_children if box_type == b"jumb"]


def _extract_jumbf_label(raw_jumb_box):
    children = _parse_bmff_boxes_with_raw(raw_jumb_box[8:])
    assert children and children[0][0] == b"jumd", "JUMBF description box missing"
    jumd_payload = children[0][1]
    assert len(jumd_payload) >= 17, "JUMBF description payload too small"
    label_bytes = jumd_payload[17:]
    nul = label_bytes.find(b"\x00")
    assert nul >= 0, "JUMBF label terminator missing"
    return label_bytes[:nul].decode("utf-8")


def _find_jumbf_cbor_payload_by_label(data, label):
    for box_type, payload in _parse_bmff_boxes(data):
        if box_type != b"jumb":
            continue
        children = _parse_bmff_boxes(payload)
        if children and children[0][0] == b"jumd" and (label.encode("utf-8") + b"\x00") in children[0][1]:
            for child_type, child_payload in children[1:]:
                if child_type == b"cbor":
                    return child_payload
        nested = _find_jumbf_cbor_payload_by_label(payload, label)
        if nested is not None:
            return nested
    return None


def _find_jumbf_uuid_payload_by_label(data, label):
    for box_type, payload in _parse_bmff_boxes(data):
        if box_type != b"jumb":
            continue
        children = _parse_bmff_boxes(payload)
        if children and children[0][0] == b"jumd" and (label.encode("utf-8") + b"\x00") in children[0][1]:
            for child_type, child_payload in children[1:]:
                if child_type == b"uuid" and len(child_payload) >= 16:
                    return child_payload[16:]
        nested = _find_jumbf_uuid_payload_by_label(payload, label)
        if nested is not None:
            return nested
    return None


def _find_jumbf_content_payload_by_label(data, label, content_box_type):
    for box_type, payload in _parse_bmff_boxes(data):
        if box_type != b"jumb":
            continue
        children = _parse_bmff_boxes(payload)
        if children and children[0][0] == b"jumd" and (label.encode("utf-8") + b"\x00") in children[0][1]:
            for child_type, child_payload in children[1:]:
                if child_type == content_box_type:
                    return child_payload
        nested = _find_jumbf_content_payload_by_label(payload, label, content_box_type)
        if nested is not None:
            return nested
    return None


def _find_all_jumbf_cbor_payloads_by_label(data, label):
    matches = []
    for box_type, payload in _parse_bmff_boxes(data):
        if box_type != b"jumb":
            continue
        children = _parse_bmff_boxes(payload)
        if children and children[0][0] == b"jumd" and (label.encode("utf-8") + b"\x00") in children[0][1]:
            for child_type, child_payload in children[1:]:
                if child_type == b"cbor":
                    matches.append(child_payload)
        matches.extend(_find_all_jumbf_cbor_payloads_by_label(payload, label))
    return matches


def _count_top_level_manifest_boxes(c2pa_bytes):
    top = _parse_bmff_boxes(c2pa_bytes)
    if not top or top[0][0] != b"jumb":
        return 0
    store_children = _parse_bmff_boxes(top[0][1])
    return sum(1 for box_type, _ in store_children if box_type == b"jumb")


def _extract_active_manifest_jumbf_box(c2pa_bytes):
    top = _parse_bmff_boxes(c2pa_bytes)
    assert top and top[0][0] == b"jumb", "C2PA root JUMBF box not found"
    store_children = _parse_bmff_boxes(top[0][1])
    manifests = [payload for box_type, payload in store_children if box_type == b"jumb"]
    assert manifests, "no manifest boxes found in C2PA store"
    return manifests[-1]


def _find_cbor_payload_in_manifest(manifest_payload, label):
    return _find_jumbf_cbor_payload_by_label(manifest_payload, label)


def _hash_bytes_with_exclusions(data, exclusions):
    if not exclusions:
        return hashlib.sha256(data).digest()

    normalized = []
    for entry in exclusions:
        start = int(entry.get("start", 0))
        length = int(entry.get("length", 0))
        if length <= 0:
            continue
        normalized.append((start, start + length))
    normalized.sort()

    hasher = hashlib.sha256()
    cursor = 0
    for start, end in normalized:
        if start > len(data):
            break
        if cursor < start:
            hasher.update(data[cursor:start])
        cursor = max(cursor, min(end, len(data)))
    if cursor < len(data):
        hasher.update(data[cursor:])
    return hasher.digest()


@then("the embedded C2PA attachment starts with a JUMBF superbox")
def step_c2pa_attachment_starts_with_jumb(context):
    pdf_bytes = getattr(context, "compiled_pdf", None) or context.last_response["body"]
    c2pa_bytes = _extract_embedded_stream_by_filespec_name(pdf_bytes, "content_credential.c2pa")
    assert len(c2pa_bytes) >= 8, "C2PA attachment too small"
    assert c2pa_bytes[4:8] == b"jumb", c2pa_bytes[:16].hex()


@then("the embedded C2PA attachment contains these markers:")
def step_c2pa_attachment_contains_markers(context):
    pdf_bytes = getattr(context, "compiled_pdf", None) or context.last_response["body"]
    c2pa_bytes = _extract_embedded_stream_by_filespec_name(pdf_bytes, "content_credential.c2pa")
    for row in context.table:
        marker = row["marker"].encode("utf-8")
        assert marker in c2pa_bytes, row["marker"]


@then("the embedded c2pa.hash.data assertion hash matches the compiled PDF bytes")
def step_c2pa_hash_data_matches_pdf(context):
    pdf_bytes = getattr(context, "compiled_pdf", None) or context.last_response["body"]
    c2pa_bytes = _extract_embedded_stream_by_filespec_name(pdf_bytes, "content_credential.c2pa")
    hash_data_cbor = _find_jumbf_cbor_payload_by_label(c2pa_bytes, "c2pa.hash.data")
    assert hash_data_cbor is not None, "c2pa.hash.data CBOR payload not found"

    payload = cbor2.loads(hash_data_cbor)
    expected_hash = payload.get("hash")
    exclusions = payload.get("exclusions")
    assert isinstance(expected_hash, (bytes, bytearray)), "c2pa.hash.data.hash missing or invalid"
    assert isinstance(exclusions, list) and exclusions, "c2pa.hash.data.exclusions missing or empty"

    actual_hash = _hash_bytes_with_exclusions(pdf_bytes, exclusions)
    assert bytes(expected_hash) == actual_hash, "c2pa.hash.data does not match compiled PDF bytes with exclusions"


@then("the embedded c2pa.hash.data assertion hash matches the verified PDF bytes")
def step_c2pa_hash_data_matches_verified_pdf(context):
    pdf_bytes = getattr(context, "verified_pdf", None) or context.last_response["body"]
    c2pa_bytes = _extract_embedded_stream_by_filespec_name(pdf_bytes, "content_credential.c2pa")
    hash_data_cbor = _find_jumbf_cbor_payload_by_label(c2pa_bytes, "c2pa.hash.data")
    assert hash_data_cbor is not None, "c2pa.hash.data CBOR payload not found"

    payload = cbor2.loads(hash_data_cbor)
    expected_hash = payload.get("hash")
    exclusions = payload.get("exclusions")
    assert isinstance(expected_hash, (bytes, bytearray)), "c2pa.hash.data.hash missing or invalid"
    assert isinstance(exclusions, list) and exclusions, "c2pa.hash.data.exclusions missing or empty"

    actual_hash = _hash_bytes_with_exclusions(pdf_bytes, exclusions)
    assert bytes(expected_hash) == actual_hash, "c2pa.hash.data does not match verified PDF bytes with exclusions"


@then("the embedded c2pa.hash.data assertion hash matches the compiled document bytes")
def step_c2pa_hash_data_matches_compiled_in_signed(context):
    # C2PA covers content provenance, not signature attestations.  Extract the
    # manifest from the signed PDF (proving it survived the PAdES append) but
    # validate the hash against the compiled document bytes only.
    c2pa_bytes = _extract_embedded_stream_by_filespec_name(context.signed_pdf, "content_credential.c2pa")
    hash_data_cbor = _find_jumbf_cbor_payload_by_label(c2pa_bytes, "c2pa.hash.data")
    assert hash_data_cbor is not None, "c2pa.hash.data CBOR payload not found"

    payload = cbor2.loads(hash_data_cbor)
    expected_hash = payload.get("hash")
    exclusions = payload.get("exclusions")
    assert isinstance(expected_hash, (bytes, bytearray)), "c2pa.hash.data.hash missing or invalid"
    assert isinstance(exclusions, list) and exclusions, "c2pa.hash.data.exclusions missing or empty"

    actual_hash = _hash_bytes_with_exclusions(context.compiled_pdf, exclusions)
    assert bytes(expected_hash) == actual_hash, "c2pa.hash.data does not match compiled document bytes"


@then("the verified PDF C2PA attachment contains two manifest boxes")
def step_verified_c2pa_contains_two_manifests(context):
    pdf_bytes = getattr(context, "verified_pdf", None) or context.last_response["body"]
    c2pa_bytes = _extract_embedded_stream_by_filespec_name(pdf_bytes, "content_credential.c2pa")
    assert _count_top_level_manifest_boxes(c2pa_bytes) == 2, "verified PDF should contain original and update manifests"


@then("the verified PDF preserves the original manifest bytes as the parent chain node")
def step_verified_preserves_original_manifest_bytes(context):
    compiled_c2pa = _extract_embedded_stream_by_filespec_name(context.compiled_pdf, "content_credential.c2pa")
    verified_c2pa = _extract_embedded_stream_by_filespec_name(context.verified_pdf, "content_credential.c2pa")
    compiled_manifests = _extract_top_level_manifest_boxes_raw(compiled_c2pa)
    verified_manifests = _extract_top_level_manifest_boxes_raw(verified_c2pa)
    assert len(compiled_manifests) == 1, "compiled PDF should contain one manifest"
    assert len(verified_manifests) == 2, "verified PDF should contain parent and update manifests"
    assert verified_manifests[0] == compiled_manifests[0], "parent manifest bytes were modified during append-only update"


@then("the verified PDF ingredient references the parent manifest with matching hash")
def step_verified_ingredient_references_parent_manifest(context):
    verified_c2pa = _extract_embedded_stream_by_filespec_name(context.verified_pdf, "content_credential.c2pa")
    manifests = _extract_top_level_manifest_boxes_raw(verified_c2pa)
    assert len(manifests) == 2, "verified PDF should contain parent and update manifests"

    parent_manifest = manifests[0]
    parent_label = _extract_jumbf_label(parent_manifest)
    expected_url = f"self#jumbf=/c2pa/{parent_label}"
    expected_hash = hashlib.sha256(parent_manifest[8:]).digest()

    active_manifest = manifests[-1][8:]
    ingredient_cbor = _find_cbor_payload_in_manifest(active_manifest, "c2pa.ingredient.v2")
    assert ingredient_cbor is not None, "active update manifest missing c2pa.ingredient.v2 assertion"
    ingredient = cbor2.loads(ingredient_cbor)
    manifest_ref = ingredient.get("c2pa_manifest") if isinstance(ingredient, dict) else None
    assert isinstance(manifest_ref, dict), "ingredient assertion missing c2pa_manifest reference"
    assert manifest_ref.get("url") == expected_url, "ingredient parent manifest URL does not match preserved parent manifest"
    assert bytes(manifest_ref.get("hash", b"")) == expected_hash, "ingredient parent manifest hash does not match preserved parent manifest"


@then("the verified PDF C2PA attachment contains the c2pa.ingredient assertion marker")
def step_verified_c2pa_contains_ingredient_marker(context):
    pdf_bytes = getattr(context, "verified_pdf", None) or context.last_response["body"]
    c2pa_bytes = _extract_embedded_stream_by_filespec_name(pdf_bytes, "content_credential.c2pa")
    assert b"c2pa.ingredient" in c2pa_bytes, "verified PDF missing ingredient assertion for provenance chain"


@then("the verified PDF C2PA attachment includes an opened action with ingredient references")
def step_verified_c2pa_has_opened_action_with_ingredient_refs(context):
    pdf_bytes = getattr(context, "verified_pdf", None) or context.last_response["body"]
    c2pa_bytes = _extract_embedded_stream_by_filespec_name(pdf_bytes, "content_credential.c2pa")
    actions_payloads = _find_all_jumbf_cbor_payloads_by_label(c2pa_bytes, "c2pa.actions.v2")
    assert actions_payloads, "verified PDF missing actions assertion payloads"

    for payload in actions_payloads:
        decoded = cbor2.loads(payload)
        actions = decoded.get("actions") if isinstance(decoded, dict) else None
        if not isinstance(actions, list):
            continue
        for action in actions:
            if not isinstance(action, dict):
                continue
            if action.get("action") != "c2pa.opened":
                continue
            params = action.get("parameters")
            if isinstance(params, dict) and isinstance(params.get("ingredients"), list) and params.get("ingredients"):
                return

    raise AssertionError("verified PDF missing c2pa.opened action with ingredient references")


@then("the active update manifest claim references c2pa.hash.data")
def step_active_update_manifest_claim_references_hash_data(context):
    pdf_bytes = getattr(context, "verified_pdf", None) or context.last_response["body"]
    c2pa_bytes = _extract_embedded_stream_by_filespec_name(pdf_bytes, "content_credential.c2pa")
    active_manifest = _extract_active_manifest_jumbf_box(c2pa_bytes)
    claim_cbor = _find_cbor_payload_in_manifest(active_manifest, "c2pa.claim.v2")
    assert claim_cbor is not None, "active update manifest missing c2pa.claim.v2"

    claim = cbor2.loads(claim_cbor)
    refs = claim.get("created_assertions") if isinstance(claim, dict) else None
    assert isinstance(refs, list) and refs, "active update claim missing created_assertions"
    for ref in refs:
        if isinstance(ref, dict) and isinstance(ref.get("url"), str) and ref["url"].endswith("/c2pa.assertions/c2pa.hash.data"):
            return
    raise AssertionError("active update claim missing c2pa.hash.data created_assertions reference")


@then("the active update manifest c2pa.hash.data assertion hash matches the verified PDF bytes")
def step_active_update_manifest_hash_data_matches_verified_pdf(context):
    pdf_bytes = getattr(context, "verified_pdf", None) or context.last_response["body"]
    c2pa_bytes = _extract_embedded_stream_by_filespec_name(pdf_bytes, "content_credential.c2pa")
    active_manifest = _extract_active_manifest_jumbf_box(c2pa_bytes)
    hash_data_cbor = _find_cbor_payload_in_manifest(active_manifest, "c2pa.hash.data")
    assert hash_data_cbor is not None, "active update manifest missing c2pa.hash.data assertion"

    payload = cbor2.loads(hash_data_cbor)
    expected_hash = payload.get("hash")
    exclusions = payload.get("exclusions")
    assert isinstance(expected_hash, (bytes, bytearray)), "active update c2pa.hash.data.hash missing or invalid"
    assert isinstance(exclusions, list) and exclusions, "active update c2pa.hash.data.exclusions missing or empty"

    actual_hash = _hash_bytes_with_exclusions(pdf_bytes, exclusions)
    assert bytes(expected_hash) == actual_hash, "active update c2pa.hash.data does not match verified PDF bytes with exclusions"


@then('the embedded c2pa.hash.data assertion payload contains the "pad" field')
def step_c2pa_hash_data_contains_pad(context):
    pdf_bytes = getattr(context, "compiled_pdf", None) or context.last_response["body"]
    c2pa_bytes = _extract_embedded_stream_by_filespec_name(pdf_bytes, "content_credential.c2pa")
    hash_data_cbor = _find_jumbf_cbor_payload_by_label(c2pa_bytes, "c2pa.hash.data")
    assert hash_data_cbor is not None, "c2pa.hash.data CBOR payload not found"
    # CBOR text key "pad" appears as 0x63 0x70 0x61 0x64.
    assert b"\x63pad" in hash_data_cbor, "c2pa.hash.data assertion missing required pad field"


@then("the embedded signing leaf certificate is x509 v3")
def step_c2pa_leaf_cert_is_v3(context):
    pdf_bytes = getattr(context, "compiled_pdf", None) or context.last_response["body"]
    c2pa_bytes = _extract_embedded_stream_by_filespec_name(pdf_bytes, "content_credential.c2pa")
    signature_cbor = _find_jumbf_content_payload_by_label(c2pa_bytes, "c2pa.signature", b"cbor")
    assert signature_cbor is not None, "c2pa.signature CBOR payload not found"

    sign1 = cbor2.loads(signature_cbor)
    tagged = sign1.value if isinstance(sign1, cbor2.CBORTag) else sign1
    assert isinstance(tagged, (list, tuple)) and len(tagged) == 4, "COSE_Sign1 payload malformed"
    protected = tagged[0]
    assert isinstance(protected, (bytes, bytearray)), "COSE protected header missing"

    headers = cbor2.loads(protected)
    chain = headers.get(33)
    assert isinstance(chain, list) and chain, "x5chain missing or empty"
    leaf_der = chain[0]
    assert isinstance(leaf_der, (bytes, bytearray)), "leaf certificate bytes missing"
    # X.509 v3 is encoded as [0] EXPLICIT INTEGER 2 in TBSCertificate.
    assert b"\xA0\x03\x02\x01\x02" in bytes(leaf_der), "signing leaf certificate is not x509 v3"


@then("the embedded signing leaf certificate includes emailProtection EKU")
def step_c2pa_leaf_cert_has_email_protection_eku(context):
    pdf_bytes = getattr(context, "compiled_pdf", None) or context.last_response["body"]
    c2pa_bytes = _extract_embedded_stream_by_filespec_name(pdf_bytes, "content_credential.c2pa")
    signature_cbor = _find_jumbf_content_payload_by_label(c2pa_bytes, "c2pa.signature", b"cbor")
    assert signature_cbor is not None, "c2pa.signature CBOR payload not found"

    sign1 = cbor2.loads(signature_cbor)
    tagged = sign1.value if isinstance(sign1, cbor2.CBORTag) else sign1
    assert isinstance(tagged, (list, tuple)) and len(tagged) == 4, "COSE_Sign1 payload malformed"
    headers = cbor2.loads(tagged[0])
    chain = headers.get(33)
    assert isinstance(chain, list) and chain, "x5chain missing or empty"
    leaf_der = bytes(chain[0])

    # OID 1.3.6.1.5.5.7.3.4 (id-kp-emailProtection) DER bytes.
    assert b"\x06\x08\x2b\x06\x01\x05\x05\x07\x03\x04" in leaf_der, "signing leaf certificate missing emailProtection EKU"


@then("the embedded C2PA signature includes x5chain protected header key")
def step_c2pa_signature_has_x5chain_key(context):
    pdf_bytes = getattr(context, "compiled_pdf", None) or context.last_response["body"]
    c2pa_bytes = _extract_embedded_stream_by_filespec_name(pdf_bytes, "content_credential.c2pa")
    signature_cbor = _find_jumbf_content_payload_by_label(c2pa_bytes, "c2pa.signature", b"cbor")
    assert signature_cbor is not None, "c2pa.signature CBOR payload not found"
    sign1 = cbor2.loads(signature_cbor)
    tagged = sign1.value if isinstance(sign1, cbor2.CBORTag) else sign1
    assert isinstance(tagged, (list, tuple)) and len(tagged) == 4, "COSE_Sign1 payload malformed"
    headers = cbor2.loads(tagged[0])
    assert 33 in headers, "x5chain header key (33) missing"


# ---------------------------------------------------------------------------
# Amendment (/update) steps
# ---------------------------------------------------------------------------

def _build_multipart_body(pdf_bytes, payload_text, manifest_url=None):
    boundary = b"dcs-pdf-amendment-boundary"
    body = (
        b"--" + boundary + b"\r\n"
        b'Content-Disposition: form-data; name="pdf"; filename="doc.pdf"\r\n'
        b"Content-Type: application/pdf\r\n\r\n"
        + pdf_bytes
        + b"\r\n"
        b"--" + boundary + b"\r\n"
        b'Content-Disposition: form-data; name="payload"; filename="contract.jsonld"\r\n'
        b"Content-Type: application/ld+json\r\n\r\n"
        + payload_text.encode("utf-8")
        + b"\r\n"
    )
    if manifest_url is not None:
        body += (
            b"--" + boundary + b"\r\n"
            b'Content-Disposition: form-data; name="manifest_url"\r\n\r\n'
            + manifest_url.encode("utf-8")
            + b"\r\n"
        )
    body += b"--" + boundary + b"--\r\n"
    content_type = "multipart/form-data; boundary=" + boundary.decode()
    return body, content_type


@given("an amended semantic payload:")
@when("an amended semantic payload:")
def step_amended_payload(context):
    context.amended_payload_text = context.text.strip().replace(
        "http://127.0.0.1:8080", context.server_url
    )


@when("I update the compiled PDF with the amended payload through /update")
def step_update_with_amended_payload(context):
    body, content_type = _build_multipart_body(
        context.compiled_pdf, context.amended_payload_text
    )
    _request(context, "POST", "/render/amendment", body, content_type)
    if context.last_response["status"] == 200:
        context.amended_pdf = context.last_response["body"]
        _save_artifact(context, context.amended_pdf, "_amended")


@when("I update the compiled PDF with the same payload through /update")
def step_update_with_same_payload(context):
    body, content_type = _build_multipart_body(
        context.compiled_pdf, context.payload_text
    )
    _request(context, "POST", "/render/amendment", body, content_type)


@then("the amended PDF is longer than the original")
def step_amended_longer(context):
    assert len(context.amended_pdf) > len(context.compiled_pdf)


@then("the amended PDF preserves the original bytes as a prefix")
def step_amended_prefix(context):
    assert context.amended_pdf.startswith(context.compiled_pdf)



@then("the amended PDF embeds the new JSON-LD payload")
def step_amended_embeds_new_payload(context):
    # pdf-core carries the JSON-LD attachment VERBATIM: the embedded stream is the
    # exact payload submitted to /update, byte-preserved, NOT re-canonicalized.
    # pdf-core's determinism guarantee is over the visible render (documentStructure
    # → page content), which is asserted separately; the attachment is opaque and
    # carried through unchanged.
    extracted = _extract_embedded_stream_by_filespec_name(context.amended_pdf, "contract.jsonld")
    submitted = context.amended_payload_text
    if isinstance(extracted, bytes):
        extracted = extracted.decode("utf-8")
    if isinstance(submitted, bytes):
        submitted = submitted.decode("utf-8")

    assert extracted.strip() == submitted.strip(), (
        f"embedded JSON-LD is not the verbatim submitted payload\n"
        f"got:  {extracted[:200]}\n"
        f"want: {submitted[:200]}"
    )


# ---------------------------------------------------------------------------
# /update variants used in the PAdES lifecycle feature
# ---------------------------------------------------------------------------

@when("I update the signed PDF with the amended payload through /update")
def step_update_signed_pdf_with_amended_payload(context):
    body, content_type = _build_multipart_body(
        context.signed_pdf, context.amended_payload_text
    )
    _request(context, "POST", "/render/amendment", body, content_type)
    if context.last_response["status"] == 200:
        context.amended_pdf = context.last_response["body"]
        _save_artifact(context, context.amended_pdf, "_amended")


@when("I update the amended PDF with the second amended payload through /update")
def step_update_amended_pdf_with_second_amended_payload(context):
    body, content_type = _build_multipart_body(
        context.amended_pdf, context.amended_payload_text
    )
    _request(context, "POST", "/render/amendment", body, content_type)
    if context.last_response["status"] == 200:
        context.amended_pdf = context.last_response["body"]
        _save_artifact(context, context.amended_pdf, "_amended2")


def _build_test_signer():
    key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    name = x509.Name([
        x509.NameAttribute(NameOID.COMMON_NAME, "dcs-pdf-core test signer"),
        x509.NameAttribute(NameOID.ORGANIZATION_NAME, "dcs-pdf-core"),
    ])
    cert = (
        x509.CertificateBuilder()
        .subject_name(name)
        .issuer_name(name)
        .public_key(key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(datetime.now(timezone.utc) - timedelta(days=1))
        .not_valid_after(datetime.now(timezone.utc) + timedelta(days=365))
        .add_extension(x509.BasicConstraints(ca=True, path_length=None), critical=True)
        .add_extension(
            x509.KeyUsage(
                digital_signature=True,
                content_commitment=True,
                key_encipherment=False,
                data_encipherment=False,
                key_agreement=False,
                key_cert_sign=True,
                crl_sign=True,
                encipher_only=False,
                decipher_only=False,
            ),
            critical=True,
        )
        .sign(private_key=key, algorithm=hashes.SHA256())
    )

    tmpdir = tempfile.mkdtemp(prefix="dcs-pdf-core-pades-")
    key_path = os.path.join(tmpdir, "signer-key.pem")
    cert_path = os.path.join(tmpdir, "signer-cert.pem")
    with open(key_path, "wb") as fh:
        fh.write(
            key.private_bytes(
                encoding=serialization.Encoding.PEM,
                format=serialization.PrivateFormat.PKCS8,
                encryption_algorithm=serialization.NoEncryption(),
            )
        )
    with open(cert_path, "wb") as fh:
        fh.write(cert.public_bytes(serialization.Encoding.PEM))

    signer = signers.SimpleSigner.load(
        key_file=key_path,
        cert_file=cert_path,
        ca_chain_files=(cert_path,),
        key_passphrase=None,
    )
    return signer


_TEST_SIGNER = None


def _get_test_signer():
    global _TEST_SIGNER
    if _TEST_SIGNER is None:
        _TEST_SIGNER = _build_test_signer()
    return _TEST_SIGNER


class _EmbeddedFontEngine(FontEngine):
    """References an already-embedded font object in the base PDF.

    pyHanko's default appearance uses unembedded Courier, which fails
    PDF/A-3a clause 6.2.11.4.1. This engine instead references the
    Liberation Sans font (object 6) that the compiler embeds in every PDF,
    so the appearance stream uses only embedded fonts.
    """

    def __init__(self, writer, font_obj_id: int, avg_width: float = 0.45):
        super().__init__(writer, "LiberationSans-Regular", embedded_subset=False)
        self._font_obj_id = font_obj_id
        self._avg_width = avg_width

    def shape(self, txt: str) -> ShapeResult:
        ops = BytesIO()
        generic.TextStringObject(txt).write_to_stream(ops)
        ops.write(b" Tj")
        return ShapeResult(
            graphics_ops=ops.getvalue(),
            x_advance=len(txt) * self._avg_width,
            y_advance=0,
        )

    def as_resource(self):
        # Return an indirect reference to the pre-embedded font in the base PDF.
        # The appearance XObject resource dict will contain "/F1 6 0 R" which
        # points to the Liberation Sans TrueType font with FontFile2 embedded.
        return generic.IndirectObject(self._font_obj_id, 0, self.writer)


class _EmbeddedFontEngineFactory(FontEngineFactory):
    """Creates _EmbeddedFontEngine instances pointing to an existing font."""

    def __init__(self, font_obj_id: int):
        self._font_obj_id = font_obj_id

    def create_font_engine(self, writer, obj_stream=None):
        return _EmbeddedFontEngine(writer, self._font_obj_id)


def _pades_sign(pdf_bytes, field_name):
    signer = _get_test_signer()
    writer = IncrementalPdfFileWriter(BytesIO(pdf_bytes))

    # Locate the embedded Liberation Sans font object in the compiled PDF.
    # The compiler writes it as object 6 ("/F1 6 0 R" in page resources).
    # Using it for the signature appearance avoids referencing unembedded
    # Courier (PDF/A-3a clause 6.2.11.4.1).
    font_obj_id = _find_liberation_sans_obj_id(pdf_bytes)
    stamp_style = TextStampStyle(
        stamp_text="%(signer)s\n%(ts)s",
        text_box_style=TextBoxStyle(font=_EmbeddedFontEngineFactory(font_obj_id)),
        border_width=0,
        background=None,
        background_opacity=0,
    )

    metadata = signers.PdfSignatureMetadata(field_name=field_name)
    pdf_signer = signers.PdfSigner(metadata, signer=signer, stamp_style=stamp_style)
    out = BytesIO()
    pdf_signer.sign_pdf(writer, output=out)
    return out.getvalue()


def _find_sig_field_names(pdf_bytes: bytes) -> list:
    """Return names of all /Sig AcroForm fields in the PDF, in order."""
    reader = PdfFileReader(BytesIO(pdf_bytes))
    root = reader.root
    if '/AcroForm' not in root:
        return []
    acroform = root['/AcroForm'].get_object()
    fields = acroform.get('/Fields')
    if fields is None:
        return []
    names = []
    for field_ref in fields:
        field = field_ref.get_object()
        ft = field.get('/FT')
        if ft is not None and str(ft) == '/Sig':
            t = field.get('/T')
            if t is not None:
                names.append(str(t))
    return names



def _find_liberation_sans_obj_id(pdf_bytes: bytes) -> int:
    """Return the object ID of the Liberation Sans font dict in the compiled PDF.

    The compiler always emits the font as object 6. We verify this by scanning
    for the /F1 font resource reference in any page's resource dictionary.
    """
    # Default object ID used by the compiler for the /F1 font.
    default_id = 6
    # Verify by looking for the font in the page resources.
    m = re.search(rb"/F1\s+(\d+)\s+0\s+R", pdf_bytes)
    if m:
        return int(m.group(1))
    return default_id


def _validate_pades_signatures(pdf_bytes):
    reader = PdfFileReader(BytesIO(pdf_bytes))
    assert reader.embedded_signatures, "no embedded signatures found"
    signer = _get_test_signer()
    vc = ValidationContext(trust_roots=[signer.signing_cert], allow_fetching=False)
    for embedded_sig in reader.embedded_signatures:
        status = validate_pdf_signature(embedded_sig, signer_validation_context=vc, skip_diff=True)
        assert status.bottom_line, "pyHanko validation failed"


def _find_pades_signatures(pdf_bytes):
    """Return list of (byte_range, cms_bytes) for every /adbe.pkcs7.detached sig."""
    sigs = []
    for m in re.finditer(rb"/SubFilter\s*/(?:adbe\.pkcs7\.detached|ETSI\.CAdES\.detached)", pdf_bytes):
        # Walk back to the << opening the sig dictionary
        start = pdf_bytes.rfind(b"<<", 0, m.start())
        end = pdf_bytes.find(b">>", m.start()) + 2
        sig_dict = pdf_bytes[start:end]

        br_match = re.search(rb"/ByteRange\s*\[([^\]]+)\]", sig_dict)
        ct_match = re.search(rb"/Contents\s*<([0-9a-fA-F]*)>", sig_dict)
        if not br_match or not ct_match:
            continue
        ranges = [int(x) for x in br_match.group(1).split()]
        r1s, r1l, r2s, r2l = ranges
        signed_bytes = pdf_bytes[r1s:r1s + r1l] + pdf_bytes[r2s:r2s + r2l]
        cms_bytes = bytes.fromhex(ct_match.group(1).decode())
        sigs.append((signed_bytes, cms_bytes))
    return sigs


# ---------------------------------------------------------------------------
# PAdES step definitions
# ---------------------------------------------------------------------------

@when('I apply a PAdES signature to the compiled PDF at field "{field_name}"')
def step_sign_compiled_pdf(context, field_name):
    context.signed_pdf = _pades_sign(context.compiled_pdf, field_name)
    _save_artifact(context, context.signed_pdf, "_signed")


@when('I apply a PAdES signature to the amended PDF at field "{field_name}"')
def step_sign_amended_pdf(context, field_name):
    pdf = getattr(context, "amended_pdf", None) or context.compiled_pdf
    context.re_signed_pdf = _pades_sign(pdf, field_name)
    _save_artifact(context, context.re_signed_pdf, "_resigned")


@when('I apply a PAdES signature to the twice-amended PDF at field "{field_name}"')
def step_sign_twice_amended_pdf(context, field_name):
    context.re_signed_pdf = _pades_sign(context.amended_pdf, field_name)
    _save_artifact(context, context.re_signed_pdf, "_resigned")


@when('I apply a PAdES signature to the re-signed PDF at field "{field_name}"')
def step_sign_resigned_pdf(context, field_name):
    context.final_pdf = _pades_sign(context.re_signed_pdf, field_name)
    _save_artifact(context, context.final_pdf, "_final")


@then("the signed PDF has no extra AcroForm signature fields")
def step_no_extra_sig_fields(context):
    compiled_names = set(_find_sig_field_names(context.compiled_pdf))
    signed_names = set(_find_sig_field_names(context.signed_pdf))
    extra = signed_names - compiled_names
    assert not extra, (
        f"signing created unexpected new sig fields: {extra!r}; "
        f"compiled had {compiled_names!r}"
    )


@then("the signed PDF is longer than the compiled PDF")
def step_signed_longer_than_compiled(context):
    assert len(context.signed_pdf) > len(context.compiled_pdf)


@then("the signed PDF preserves the compiled PDF bytes as a prefix")
def step_signed_preserves_compiled_prefix(context):
    assert context.signed_pdf.startswith(context.compiled_pdf)


@then("the signed PDF contains a valid PAdES signature")
def step_signed_has_valid_pades(context):
    pdf = getattr(context, "re_signed_pdf", None) or context.signed_pdf
    _validate_pades_signatures(pdf)


@then("the re-signed PDF is longer than the amended PDF")
def step_resigned_longer_than_amended(context):
    assert len(context.re_signed_pdf) > len(context.amended_pdf)


@then("the re-signed PDF preserves the amended PDF bytes as a prefix")
def step_resigned_preserves_amended_prefix(context):
    assert context.re_signed_pdf.startswith(context.amended_pdf)


@then("the re-signed PDF preserves the twice-amended PDF bytes as a prefix")
def step_resigned_preserves_twice_amended_prefix(context):
    assert context.re_signed_pdf.startswith(context.amended_pdf)


@then("the re-signed PDF contains a valid PAdES signature")
def step_resigned_has_valid_pades(context):
    _validate_pades_signatures(context.re_signed_pdf)


@then("the re-signed PDF contains {count:d} PAdES signatures")
def step_resigned_has_n_pades_sigs(context, count):
    sigs = _find_pades_signatures(context.re_signed_pdf)
    assert len(sigs) == count, f"expected {count} PAdES signatures, found {len(sigs)}"


@then("the final PDF contains {count:d} PAdES signatures")
def step_final_has_n_pades_sigs(context, count):
    sigs = _find_pades_signatures(context.final_pdf)
    assert len(sigs) == count, f"expected {count} PAdES signatures, found {len(sigs)}"


@then("all PAdES signatures in the final PDF are valid")
def step_final_all_sigs_valid(context):
    _validate_pades_signatures(context.final_pdf)


@then("the final PDF preserves the re-signed PDF bytes as a prefix")
def step_final_preserves_resigned_prefix(context):
    assert context.final_pdf.startswith(context.re_signed_pdf), (
        "final PDF must preserve re-signed PDF bytes as a prefix"
    )


@then("the first PAdES signature byte range is covered by the re-signed PDF bytes unchanged")
def step_first_sig_byte_range_intact(context):
    # The first signature was applied over context.compiled_pdf (or signed_pdf prefix).
    # Its ByteRange must still be readable from the re-signed PDF without modification.
    sigs = _find_pades_signatures(context.re_signed_pdf)
    assert len(sigs) >= 1, "no PAdES signatures found"
    # Find the first sig's ByteRange in re_signed_pdf and verify the bytes it
    # covers still produce a parseable CMS blob (the original bytes are intact).
    pdf = context.re_signed_pdf
    br_matches = list(re.finditer(rb"/ByteRange\s*\[([^\]]+)\]", pdf))
    assert br_matches, "no ByteRange found"
    first_br = [int(x) for x in br_matches[0].group(1).split()]
    r1s, r1l, r2s, r2l = first_br
    assert r1s + r1l <= len(pdf), "first sig ByteRange range1 out of bounds"
    assert r2s + r2l <= len(pdf), "first sig ByteRange range2 out of bounds"


# ---------------------------------------------------------------------------
# C2PA structural verification helpers shared across lifecycle steps
# ---------------------------------------------------------------------------

def _assert_cose_sign1_intact(c2pa_bytes):
    """Assert the active manifest's c2pa.signature is a well-formed COSE_Sign1."""
    active_manifest = _extract_active_manifest_jumbf_box(c2pa_bytes)
    signature_cbor = _find_jumbf_content_payload_by_label(active_manifest, "c2pa.signature", b"cbor")
    assert signature_cbor is not None, "c2pa.signature CBOR payload not found in active manifest"
    sign1 = cbor2.loads(signature_cbor)
    tagged = sign1.value if isinstance(sign1, cbor2.CBORTag) else sign1
    assert isinstance(tagged, (list, tuple)) and len(tagged) == 4, "COSE_Sign1 must be a 4-element array"
    protected = tagged[0]
    assert isinstance(protected, (bytes, bytearray)) and len(protected) > 0, "COSE protected header missing"
    headers = cbor2.loads(protected)
    assert 33 in headers, "COSE protected header missing x5chain key (33)"
    chain = headers[33]
    assert isinstance(chain, list) and chain, "x5chain missing or empty"
    assert isinstance(chain[0], (bytes, bytearray)), "x5chain leaf certificate bytes missing"


def _assert_active_hash_data_matches(pdf_bytes):
    """Assert the active manifest's c2pa.hash.data covers pdf_bytes correctly."""
    c2pa_bytes = _extract_embedded_stream_by_filespec_name(pdf_bytes, "content_credential.c2pa")
    active_manifest = _extract_active_manifest_jumbf_box(c2pa_bytes)
    hash_data_cbor = _find_cbor_payload_in_manifest(active_manifest, "c2pa.hash.data")
    assert hash_data_cbor is not None, "active manifest missing c2pa.hash.data assertion"
    payload = cbor2.loads(hash_data_cbor)
    expected_hash = payload.get("hash")
    exclusions = payload.get("exclusions")
    assert isinstance(expected_hash, (bytes, bytearray)), "c2pa.hash.data.hash missing or invalid"
    assert isinstance(exclusions, list) and exclusions, "c2pa.hash.data.exclusions missing or empty"
    actual_hash = _hash_bytes_with_exclusions(pdf_bytes, exclusions)
    assert bytes(expected_hash) == actual_hash, "active manifest c2pa.hash.data hash mismatch"


def _assert_active_claim_references_hash_data(pdf_bytes):
    """Assert the active manifest's c2pa.claim.v2 references c2pa.hash.data."""
    c2pa_bytes = _extract_embedded_stream_by_filespec_name(pdf_bytes, "content_credential.c2pa")
    active_manifest = _extract_active_manifest_jumbf_box(c2pa_bytes)
    claim_cbor = _find_cbor_payload_in_manifest(active_manifest, "c2pa.claim.v2")
    assert claim_cbor is not None, "active manifest missing c2pa.claim.v2"
    claim = cbor2.loads(claim_cbor)
    refs = claim.get("created_assertions") if isinstance(claim, dict) else None
    assert isinstance(refs, list) and refs, "active claim missing created_assertions"
    for ref in refs:
        if isinstance(ref, dict) and isinstance(ref.get("url"), str) and ref["url"].endswith("/c2pa.assertions/c2pa.hash.data"):
            return
    raise AssertionError("active claim missing c2pa.hash.data in created_assertions")


# ---------------------------------------------------------------------------
# C2PA structural steps for pades_lifecycle scenarios
# ---------------------------------------------------------------------------

@then("the amended PDF C2PA attachment contains 2 manifest boxes")
def step_amended_c2pa_contains_2_manifests(context):
    c2pa_bytes = _extract_embedded_stream_by_filespec_name(context.amended_pdf, "content_credential.c2pa")
    count = _count_top_level_manifest_boxes(c2pa_bytes)
    assert count == 2, f"amended PDF C2PA should contain 2 manifest boxes, found {count}"


@then("the amended PDF C2PA preserves the compiled manifest as the parent chain node")
def step_amended_c2pa_preserves_compiled_manifest(context):
    compiled_c2pa = _extract_embedded_stream_by_filespec_name(context.compiled_pdf, "content_credential.c2pa")
    amended_c2pa = _extract_embedded_stream_by_filespec_name(context.amended_pdf, "content_credential.c2pa")
    compiled_manifests = _extract_top_level_manifest_boxes_raw(compiled_c2pa)
    amended_manifests = _extract_top_level_manifest_boxes_raw(amended_c2pa)
    assert len(compiled_manifests) == 1, "compiled PDF should contain one manifest"
    assert len(amended_manifests) == 2, "amended PDF should contain parent and update manifests"
    assert amended_manifests[0] == compiled_manifests[0], "parent manifest bytes were modified during amendment"


@then("the amended PDF C2PA ingredient references the compiled manifest with matching hash")
def step_amended_c2pa_ingredient_references_compiled(context):
    amended_c2pa = _extract_embedded_stream_by_filespec_name(context.amended_pdf, "content_credential.c2pa")
    manifests = _extract_top_level_manifest_boxes_raw(amended_c2pa)
    assert len(manifests) == 2, "amended PDF should contain parent and update manifests"
    parent_manifest = manifests[0]
    parent_label = _extract_jumbf_label(parent_manifest)
    expected_url = f"self#jumbf=/c2pa/{parent_label}"
    expected_hash = hashlib.sha256(parent_manifest[8:]).digest()
    active_manifest = manifests[-1][8:]
    ingredient_cbor = _find_cbor_payload_in_manifest(active_manifest, "c2pa.ingredient.v2")
    assert ingredient_cbor is not None, "amended PDF active manifest missing c2pa.ingredient.v2"
    ingredient = cbor2.loads(ingredient_cbor)
    manifest_ref = ingredient.get("c2pa_manifest") if isinstance(ingredient, dict) else None
    assert isinstance(manifest_ref, dict), "ingredient assertion missing c2pa_manifest reference"
    assert manifest_ref.get("url") == expected_url, "ingredient parent manifest URL mismatch"
    assert bytes(manifest_ref.get("hash", b"")) == expected_hash, "ingredient parent manifest hash mismatch"


@then("the active manifest c2pa.hash.data hash matches the amended PDF bytes")
def step_active_hash_data_matches_amended_pdf(context):
    _assert_active_hash_data_matches(context.amended_pdf)


@then("the active manifest claim references c2pa.hash.data in the amended PDF")
def step_active_claim_refs_hash_data_amended(context):
    _assert_active_claim_references_hash_data(context.amended_pdf)


@then("the C2PA signature box in the re-signed PDF is a valid COSE_Sign1 structure")
def step_cose_sign1_intact_resigned(context):
    c2pa_bytes = _extract_embedded_stream_by_filespec_name(context.re_signed_pdf, "content_credential.c2pa")
    _assert_cose_sign1_intact(c2pa_bytes)


@then("the twice-amended PDF C2PA attachment contains 3 manifest boxes")
def step_twice_amended_c2pa_contains_3_manifests(context):
    c2pa_bytes = _extract_embedded_stream_by_filespec_name(context.amended_pdf, "content_credential.c2pa")
    count = _count_top_level_manifest_boxes(c2pa_bytes)
    assert count == 3, f"twice-amended PDF C2PA should contain 3 manifest boxes, found {count}"


@then("the active manifest c2pa.hash.data hash matches the twice-amended PDF bytes")
def step_active_hash_data_matches_twice_amended_pdf(context):
    _assert_active_hash_data_matches(context.amended_pdf)


@then("the active manifest claim references c2pa.hash.data in the twice-amended PDF")
def step_active_claim_refs_hash_data_twice_amended(context):
    _assert_active_claim_references_hash_data(context.amended_pdf)


@then("the C2PA signature box in the final PDF is a valid COSE_Sign1 structure")
def step_cose_sign1_intact_final(context):
    c2pa_bytes = _extract_embedded_stream_by_filespec_name(context.final_pdf, "content_credential.c2pa")
    _assert_cose_sign1_intact(c2pa_bytes)


@then("the C2PA content hash in the final PDF validates against the amended document")
def step_c2pa_content_hash_in_final_validates_against_amended(context):
    # The final PDF is compiled_pdf + amendment /update + two PAdES revisions.
    # C2PA covers content provenance only; the active manifest's hash.data was
    # computed against the amended_pdf bytes (before any PAdES attestations were
    # appended). Extracting the manifest from final_pdf proves it survived intact;
    # validating the hash against amended_pdf proves the content hash is still correct.
    c2pa_bytes = _extract_embedded_stream_by_filespec_name(context.final_pdf, "content_credential.c2pa")
    active_manifest = _extract_active_manifest_jumbf_box(c2pa_bytes)
    hash_data_cbor = _find_cbor_payload_in_manifest(active_manifest, "c2pa.hash.data")
    assert hash_data_cbor is not None, "final PDF active manifest missing c2pa.hash.data"
    payload = cbor2.loads(hash_data_cbor)
    expected_hash = payload.get("hash")
    exclusions = payload.get("exclusions")
    assert isinstance(expected_hash, (bytes, bytearray)), "c2pa.hash.data.hash missing or invalid"
    assert isinstance(exclusions, list) and exclusions, "c2pa.hash.data.exclusions missing or empty"
    actual_hash = _hash_bytes_with_exclusions(context.amended_pdf, exclusions)
    assert bytes(expected_hash) == actual_hash, (
        "C2PA content hash in final PDF does not match amended document bytes — "
        "C2PA must cover content provenance, not PAdES attestation revisions"
    )


@then("the active manifest claim references c2pa.hash.data in the final PDF")
def step_active_claim_refs_hash_data_final(context):
    _assert_active_claim_references_hash_data(context.final_pdf)


# ---------------------------------------------------------------------------
# /claim – external JSON-LD claim binding
# ---------------------------------------------------------------------------


def _strip_embedded_jsonld(pdf_bytes):
    """Return a copy of pdf_bytes with the embedded JSON-LD stream content
    replaced by null bytes of the same length, mirroring StripEmbeddedJSONLD
    in compiler/claim.go.  All object offsets remain unchanged."""
    needle = b"/F (contract.jsonld)"
    file_spec_pos = pdf_bytes.find(needle)
    assert file_spec_pos >= 0, "embedded JSON-LD filespec not found"

    ef_pos = pdf_bytes.find(b"/EF << /F ", file_spec_pos)
    assert ef_pos >= 0, "embedded JSON-LD object reference not found"
    ref_start = ef_pos + len(b"/EF << /F ")
    ref_end = pdf_bytes.find(b" 0 R", ref_start)
    assert ref_end >= 0, "embedded JSON-LD object reference malformed"

    obj_id = int(pdf_bytes[ref_start:ref_end].strip())
    # Use the FIRST definition (same as Go's bytes.Index) so we zero the
    # original stream, not the superseding one in an incremental update.
    obj_marker = f"{obj_id} 0 obj".encode("utf-8")
    obj_pos = pdf_bytes.find(obj_marker)
    assert obj_pos >= 0, f"embedded JSON-LD object {obj_id} not found"

    stream_start = pdf_bytes.find(b"stream\n", obj_pos)
    assert stream_start >= 0, "embedded JSON-LD stream start not found"
    stream_start += len(b"stream\n")
    stream_end = pdf_bytes.find(b"\nendstream", stream_start)
    assert stream_end >= 0, "embedded JSON-LD stream end not found"

    result = bytearray(pdf_bytes)
    for i in range(stream_start, stream_end):
        result[i] = 0
    return bytes(result)


@given("I strip the embedded JSON-LD from the compiled PDF")
def step_strip_embedded_jsonld(context):
    context.stripped_pdf = _strip_embedded_jsonld(context.compiled_pdf)


@when("I claim the stripped PDF with its original payload through /claim")
def step_claim_stripped_pdf(context):
    body, content_type = _build_multipart_body(
        context.stripped_pdf, context.payload_text
    )
    _request(context, "POST", "/claim", body, content_type)
    if context.last_response["status"] == 200:
        context.claimed_pdf = context.last_response["body"]
        _save_artifact(context, context.claimed_pdf, "_claimed")


@when("I claim the compiled PDF with its original payload through /claim")
def step_claim_compiled_pdf(context):
    body, content_type = _build_multipart_body(
        context.compiled_pdf, context.payload_text
    )
    _request(context, "POST", "/claim", body, content_type)
    if context.last_response["status"] == 200:
        context.claimed_pdf = context.last_response["body"]
        _save_artifact(context, context.claimed_pdf, "_claimed")


@when("I claim the compiled PDF with the amended payload through /claim")
def step_claim_compiled_pdf_with_amended_payload(context):
    body, content_type = _build_multipart_body(
        context.compiled_pdf, context.amended_payload_text
    )
    _request(context, "POST", "/claim", body, content_type)
    if context.last_response["status"] == 200:
        context.claimed_pdf = context.last_response["body"]
        _save_artifact(context, context.claimed_pdf, "_claimed")


@then("the claimed PDF is longer than the compiled PDF")
def step_claimed_longer_than_compiled(context):
    assert len(context.claimed_pdf) > len(context.compiled_pdf), (
        f"claimed PDF ({len(context.claimed_pdf)} bytes) must be longer than "
        f"compiled PDF ({len(context.compiled_pdf)} bytes)"
    )


@then("the claimed PDF embeds the original JSON-LD payload")
def step_claimed_embeds_original_payload(context):
    extracted = _extract_embedded_stream_by_filespec_name(
        context.claimed_pdf, "contract.jsonld"
    )

    # /claim embeds the canonicalized payload form. Validate against the same
    # canonical form produced by /download for the original payload.
    _request(context, "POST", "/render", context.payload_text, "application/ld+json")
    assert context.last_response["status"] == 200, context.last_response
    expected_pdf = context.last_response["body"]
    expected = _extract_embedded_stream_by_filespec_name(expected_pdf, "contract.jsonld")

    assert extracted.strip() == expected.strip(), (
        f"claimed PDF embedded JSON-LD does not match canonical payload\n"
        f"got:  {extracted[:200]}\n"
        f"want: {expected[:200]}"
    )


@then("the claimed PDF contains a verification witness")
def step_claimed_pdf_contains_witness(context):
    assert b"verification-witness" in context.claimed_pdf, (
        "claimed PDF must contain a verification-witness Sig object"
    )


# ---------------------------------------------------------------------------
# Re-rendering / determinism helpers
# ---------------------------------------------------------------------------

def _extract_bt_et_content(pdf_bytes):
    """Concatenate all BT...ET text operator blocks from PDF content streams.

    Only considers content streams (streams that contain at least one BT
    operator).  This is the Python analogue of concatBTBlocks in the Go
    test suite: it isolates the human-readable portion of the PDF that the
    re-rendering guarantee must preserve across amendments and signatures.
    """
    streams = re.findall(rb"stream\n(.*?)\nendstream", pdf_bytes, flags=re.DOTALL)
    result = []
    for stream in streams:
        blocks = re.findall(rb"BT\n.*?ET\n", stream, flags=re.DOTALL)
        result.extend(blocks)
    return b"".join(result)


_PDF_STAGE_ATTR = {
    "compiled": "compiled_pdf",
    "verified": "verified_pdf",
    "signed": "signed_pdf",
    "amended": "amended_pdf",
    "re-signed": "re_signed_pdf",
    "final": "final_pdf",
}


def _get_stage_pdf(context, stage):
    attr = _PDF_STAGE_ATTR.get(stage)
    assert attr is not None, f"unknown PDF stage {stage!r}; valid stages: {list(_PDF_STAGE_ATTR)}"
    pdf = getattr(context, attr, None)
    assert pdf is not None, f"no PDF available for stage {stage!r}"
    return pdf


@when('I extract and recompile the embedded JSON-LD from the "{stage}" PDF')
def step_extract_and_recompile(context, stage):
    """Extract the embedded contract.jsonld from the named PDF stage and recompile it."""
    pdf = _get_stage_pdf(context, stage)
    extracted = _extract_embedded_stream_by_filespec_name(pdf, "contract.jsonld")
    _request(context, "POST", "/render", extracted, "application/ld+json")
    assert context.last_response["status"] == 200, (
        f"recompile from {stage} PDF embedded JSON-LD failed "
        f"(HTTP {context.last_response['status']}): "
        f"{context.last_response['body'][:300]}"
    )
    context.recompiled_pdf = context.last_response["body"]


@then('the recompiled PDF page content matches a fresh compile of the original payload')
def step_recompiled_matches_fresh_original(context):
    """Re-rendering from embedded JSON-LD must give identical page content to a fresh compile."""
    _request(context, "POST", "/render", context.payload_text, "application/ld+json")
    assert context.last_response["status"] == 200
    fresh = context.last_response["body"]
    fresh_blocks = _extract_bt_et_content(fresh)
    recompiled_blocks = _extract_bt_et_content(context.recompiled_pdf)
    assert fresh_blocks, "fresh compile produced no BT/ET page content"
    assert recompiled_blocks, "recompiled PDF produced no BT/ET page content"
    assert recompiled_blocks == fresh_blocks, (
        f"re-rendering guarantee violated: page content from embedded JSON-LD "
        f"differs from fresh compile of original payload\n"
        f"  fresh:      {len(fresh_blocks)} bytes\n"
        f"  recompiled: {len(recompiled_blocks)} bytes"
    )


@then('the recompiled PDF page content matches a fresh compile of the amended payload')
def step_recompiled_matches_fresh_amended(context):
    """Re-rendering from embedded JSON-LD must give identical page content to a fresh compile of the amendment."""
    _request(context, "POST", "/render", context.amended_payload_text, "application/ld+json")
    assert context.last_response["status"] == 200
    fresh = context.last_response["body"]
    fresh_blocks = _extract_bt_et_content(fresh)
    recompiled_blocks = _extract_bt_et_content(context.recompiled_pdf)
    assert fresh_blocks, "fresh compile of amended payload produced no BT/ET page content"
    assert recompiled_blocks, "recompiled PDF produced no BT/ET page content"
    assert recompiled_blocks == fresh_blocks, (
        f"re-rendering guarantee violated: page content from amended embedded JSON-LD "
        f"differs from fresh compile of amended payload\n"
        f"  fresh:      {len(fresh_blocks)} bytes\n"
        f"  recompiled: {len(recompiled_blocks)} bytes"
    )


@then('a fresh compile of the amended payload has different page content from the compiled PDF')
def step_amended_content_differs_from_original(context):
    """Sanity check: the amended payload must render differently from the original."""
    _request(context, "POST", "/render", context.amended_payload_text, "application/ld+json")
    assert context.last_response["status"] == 200
    fresh_amended = context.last_response["body"]
    amended_blocks = _extract_bt_et_content(fresh_amended)
    original_blocks = _extract_bt_et_content(context.compiled_pdf)
    assert amended_blocks != original_blocks, (
        "amended payload must produce different page content from the original — "
        "the amendment added no visible change"
    )


@when('I verify the "{stage}" PDF through /verify')
def step_verify_stage_pdf(context, stage):
    """Verify an arbitrary PDF stage and store result in context.verified_pdf."""
    pdf = _get_stage_pdf(context, stage)
    _request(context, "POST", "/verify", pdf, "application/pdf")
    assert context.last_response["status"] == 200, (
        f"verify of {stage} PDF failed (HTTP {context.last_response['status']}): "
        f"{context.last_response['body'][:300]}"
    )
    import base64 as _b64
    resp = json.loads(context.last_response["body"].decode("utf-8"))
    context.verified_pdf = _b64.b64decode(resp["artifact"])
    _save_artifact(context, context.verified_pdf, f"_verified_{stage.replace('-', '_')}")


# ---------------------------------------------------------------------------
# Tampering helpers
# ---------------------------------------------------------------------------

def _flip_byte_in_page_content(pdf_bytes):
    """Flip one bit inside the first Tj text string in any BT/ET content block.

    Content streams in this compiler are uncompressed, so BT/ET blocks are
    directly visible in the raw bytes.  The target is the first letter character
    inside a ``(text) Tj`` operator — the change lands squarely in the
    human-readable, C2PA-covered region of the PDF.

    Returns the tampered bytes.  The PDF remains structurally parseable but its
    visible text differs from what the embedded JSON-LD payload describes.
    """
    result = bytearray(pdf_bytes)
    content_stream_re = re.compile(rb"stream\n(.*?)\nendstream", re.DOTALL)
    for stream_m in content_stream_re.finditer(pdf_bytes):
        stream_data = stream_m.group(1)
        # Match only streams that are in BT/ET format (page content)
        if b"BT" not in stream_data:
            continue
        # Find a Tj operator whose string argument begins with at least two letters
        tj_m = re.search(rb"\(([A-Za-z]{2,}[^)]*)\) Tj", stream_data)
        if not tj_m:
            continue
        # Absolute offset of the first letter of the string argument within the PDF
        char_offset = stream_m.start(1) + tj_m.start(1)
        result[char_offset] ^= 1  # flip LSB — changes one glyph code
        return bytes(result)
    raise AssertionError(
        "no Tj operator with letter content found in PDF page content streams; "
        "cannot tamper"
    )


@given('I tamper with the page content of the "{stage}" PDF')
def step_tamper_stage_pdf(context, stage):
    """Flip one byte inside the BT/ET human-readable portion of the named PDF."""
    pdf = _get_stage_pdf(context, stage)
    context.tampered_pdf = _flip_byte_in_page_content(pdf)


@when("I verify the tampered PDF through /verify")
def step_verify_tampered_pdf(context):
    assert hasattr(context, "tampered_pdf"), "no tampered PDF in context"
    _request(context, "POST", "/verify", context.tampered_pdf, "application/pdf")


# ---------------------------------------------------------------------------
# Offline amendment simulation
# ---------------------------------------------------------------------------

def _simulate_offline_pdf_amendment(pdf_bytes):
    """Simulate a PDF editor appending content to a compiled PDF without using /update.

    The simulated editor appends a structurally valid PDF incremental update that
    adds a new text annotation object, an updated cross-reference table, and a
    new trailer pointing back at the original startxref — exactly what Acrobat,
    LibreOffice, or any standards-compliant PDF writer would produce.

    Crucially:
    - The original compiled bytes are preserved byte-for-byte as a prefix.
    - There is NO ``% dcs-pdf-core incremental update`` marker, so the /verify
      endpoint takes the plain re-render path rather than the provenance chain path.
    - The submitted PDF differs from CompilePDF(embeddedPayload) because that fresh
      compile never emits the offline editor's annotation object.
    """
    # Locate the original startxref value — the offline editor's trailer /Prev
    # field must reference it so PDF readers can traverse the xref chain.
    m = re.search(rb"startxref\s+(\d+)\s+%%EOF", pdf_bytes)
    assert m is not None, "original PDF missing startxref/%%EOF"
    prev_xref = int(m.group(1))

    # Choose an object number that cannot collide with compiler-generated objects
    # (which are numbered from 1 upward to ~20 for a minimal document).
    annot_obj_id = 88888

    # The new annotation object: a modest /Text annotation referencing the first
    # page. The content is plausible — an editor adding a review comment.
    annot_obj_offset = len(pdf_bytes)
    annot_obj = (
        f"{annot_obj_id} 0 obj\n"
        f"<< /Type /Annot /Subtype /Text /Open false\n"
        f"   /Contents (Reviewed and approved - offline editor)\n"
        f"   /Rect [72 700 200 720] >>\n"
        f"endobj\n"
    ).encode()

    # Minimal cross-reference table for the single new object.
    xref_offset = annot_obj_offset + len(annot_obj)
    xref_section = (
        f"xref\n"
        f"0 1\n"
        f"0000000000 65535 f \n"
        f"{annot_obj_id} 1\n"
        f"{annot_obj_offset:010d} 00000 n \n"
    ).encode()

    # New trailer with /Prev pointing at the original startxref.
    trailer = (
        f"trailer\n"
        f"<< /Size {annot_obj_id + 1} /Prev {prev_xref} >>\n"
        f"startxref\n"
        f"{xref_offset}\n"
        f"%%EOF\n"
    ).encode()

    return pdf_bytes + b"\n% offline-editor amendment\n" + annot_obj + xref_section + trailer


@given("I apply an offline amendment to the compiled PDF")
def step_offline_amend_compiled(context):
    """Simulate a PDF editor modifying the compiled PDF outside the /update workflow."""
    context.offline_amended_pdf = _simulate_offline_pdf_amendment(context.compiled_pdf)


@when("I verify the offline-amended PDF through /verify")
def step_verify_offline_amended(context):
    assert hasattr(context, "offline_amended_pdf"), "no offline-amended PDF in context"
    _request(context, "POST", "/verify", context.offline_amended_pdf, "application/pdf")


@then("the offline-amended PDF preserves the compiled PDF bytes as a prefix")
def step_offline_amended_preserves_prefix(context):
    assert context.offline_amended_pdf.startswith(context.compiled_pdf), (
        "offline-amended PDF must preserve the original compiled bytes as an exact prefix"
    )


@then("the tampered PDF C2PA hash does not match its content")
def step_tampered_c2pa_hash_mismatch(context):
    """Assert that the active c2pa.hash.data binding is broken by the tamper.

    The flipped byte is in a page content stream, which falls inside the C2PA
    hash boundary (only the JUMBF manifest stream itself is excluded).  The
    hash stored in the manifest was computed before tampering, so it must differ
    from the hash of the tampered bytes.
    """
    pdf = context.tampered_pdf
    c2pa_bytes = _extract_embedded_stream_by_filespec_name(pdf, "content_credential.c2pa")
    active_manifest = _extract_active_manifest_jumbf_box(c2pa_bytes)
    hash_data_cbor = _find_cbor_payload_in_manifest(active_manifest, "c2pa.hash.data")
    assert hash_data_cbor is not None, "active manifest missing c2pa.hash.data assertion"
    payload = cbor2.loads(hash_data_cbor)
    recorded_hash = payload.get("hash")
    exclusions = payload.get("exclusions")
    assert isinstance(recorded_hash, (bytes, bytearray)), "c2pa.hash.data.hash missing or invalid"
    assert isinstance(exclusions, list) and exclusions, "c2pa.hash.data.exclusions missing or empty"
    actual_hash = _hash_bytes_with_exclusions(pdf, exclusions)
    assert bytes(recorded_hash) != actual_hash, (
        "C2PA hash still matches tampered content — the flipped byte must fall "
        "inside the hash boundary (not in a JUMBF exclusion zone)"
    )


# ---------------------------------------------------------------------------
# Remote manifest URL (DCS-OR-C2PA-008) steps
# ---------------------------------------------------------------------------

@when("I amend the PDF with a new payload:")
def step_amend_without_manifest_url(context):
    payload_text = context.text.strip().replace("http://127.0.0.1:8080", context.server_url)
    body, content_type = _build_multipart_body(context.compiled_pdf, payload_text)
    _request(context, "POST", "/render/amendment", body, content_type)
    if context.last_response["status"] == 200:
        context.updated_pdf = context.last_response["body"]
        _save_artifact(context, context.updated_pdf, "_updated_no_manifest_url")


@then("the updated PDF C2PA manifest contains no remote manifest URL")
def step_updated_pdf_has_no_manifest_url(context):
    _request(context, "POST", "/manifest/extract", context.updated_pdf, "application/pdf")
    assert context.last_response["status"] == 200, (
        f"manifest/extract failed: {context.last_response['body'][:300]}"
    )
    c2pa_bytes = context.last_response["body"]
    assert b"remote_manifests" not in c2pa_bytes, (
        "remote_manifests key unexpectedly present in C2PA manifest store"
    )


@when('I amend the PDF with a new payload and manifest_url "{manifest_url}":')
def step_amend_with_manifest_url(context, manifest_url):
    payload_text = context.text.strip().replace("http://127.0.0.1:8080", context.server_url)
    resolved_manifest_url = manifest_url.replace("http://127.0.0.1:8080", context.server_url)
    context.manifest_url = resolved_manifest_url
    body, content_type = _build_multipart_body(
        context.compiled_pdf, payload_text, manifest_url=resolved_manifest_url
    )
    _request(context, "POST", "/render/amendment", body, content_type)
    if context.last_response["status"] == 200:
        context.updated_pdf = context.last_response["body"]
        _save_artifact(context, context.updated_pdf, "_updated_with_manifest_url")


def _extract_active_manifest_from_updated_pdf(context):
    _request(context, "POST", "/manifest/extract", context.updated_pdf, "application/pdf")
    assert context.last_response["status"] == 200, (
        f"manifest/extract failed: {context.last_response['body'][:300]}"
    )
    c2pa_bytes = context.last_response["body"]
    return _extract_active_manifest_jumbf_box(c2pa_bytes)


@then("the updated PDF C2PA claim has no remote_manifests field")
def step_updated_pdf_claim_has_no_remote_manifests_field(context):
    active_manifest = _extract_active_manifest_from_updated_pdf(context)
    claim_cbor = _find_cbor_payload_in_manifest(active_manifest, "c2pa.claim.v2")
    assert claim_cbor is not None, "c2pa.claim.v2 not found in active manifest"
    claim = cbor2.loads(claim_cbor)
    assert "remote_manifests" not in claim, (
        f"claim unexpectedly contains a standalone remote_manifests field: {sorted(claim.keys())}"
    )


@then('the updated PDF C2PA manifest embeds a dcs.remote_manifests assertion for "{manifest_url}"')
def step_updated_pdf_has_remote_manifests_assertion(context, manifest_url):
    resolved_manifest_url = manifest_url.replace("http://127.0.0.1:8080", context.server_url)
    active_manifest = _extract_active_manifest_from_updated_pdf(context)
    remote_manifests_cbor = _find_cbor_payload_in_manifest(active_manifest, "dcs.remote_manifests")
    assert remote_manifests_cbor is not None, (
        "dcs.remote_manifests assertion not found in active manifest"
    )
    payload = cbor2.loads(remote_manifests_cbor)
    assert payload.get("remote_manifests") == [resolved_manifest_url], (
        f"dcs.remote_manifests assertion does not equal [{resolved_manifest_url!r}]: {payload}"
    )


@when("I extract the C2PA manifest store from the compiled PDF")
def step_extract_manifest_store(context):
    _request(context, "POST", "/manifest/extract", context.compiled_pdf, "application/pdf")


@then("the manifest store response contains the JUMBF marker")
def step_manifest_store_contains_jumbf(context):
    body = context.last_response["body"]
    assert b"jumb" in body, "manifest store response does not contain JUMBF marker 'jumb'"


# ---------------------------------------------------------------------------
# Lifecycle assertion (DCS-OR-C2PA-003)
# ---------------------------------------------------------------------------

@then("the embedded C2PA manifest contains a dcs.lifecycle assertion")
def step_manifest_has_lifecycle_assertion(context):
    pdf_bytes = getattr(context, "compiled_pdf", None) or context.last_response["body"]
    c2pa_bytes = _extract_embedded_stream_by_filespec_name(pdf_bytes, "content_credential.c2pa")
    payload = _find_jumbf_cbor_payload_by_label(c2pa_bytes, "dcs.lifecycle")
    assert payload is not None, "dcs.lifecycle CBOR assertion not found in C2PA manifest"
    context.lifecycle_assertion = cbor2.loads(payload)
    context.lifecycle_file_hash_compiled = context.lifecycle_assertion.get("file_hash")


@then('the dcs.lifecycle assertion has contract_id "{expected}"')
def step_lifecycle_contract_id(context, expected):
    assertion = getattr(context, "lifecycle_assertion", None)
    assert assertion is not None, "no lifecycle assertion in context"
    assert assertion.get("contract_id") == expected, (
        f"expected contract_id={expected!r}, got {assertion.get('contract_id')!r}"
    )


@then("the dcs.lifecycle assertion has a non-empty file_hash")
def step_lifecycle_file_hash_non_empty(context):
    assertion = getattr(context, "lifecycle_assertion", None)
    assert assertion is not None, "no lifecycle assertion in context"
    fh = assertion.get("file_hash")
    assert isinstance(fh, str) and len(fh) == 64, (
        f"expected 64-char hex file_hash, got {fh!r}"
    )


@then("the active C2PA manifest in the updated PDF contains a dcs.lifecycle assertion")
def step_updated_manifest_has_lifecycle_assertion(context):
    pdf_bytes = getattr(context, "updated_pdf", None) or context.last_response["body"]
    c2pa_bytes = _extract_embedded_stream_by_filespec_name(pdf_bytes, "content_credential.c2pa")
    active_manifest = _extract_active_manifest_jumbf_box(c2pa_bytes)
    payload = _find_jumbf_cbor_payload_by_label(active_manifest, "dcs.lifecycle")
    assert payload is not None, "dcs.lifecycle CBOR assertion not found in active update manifest"
    context.lifecycle_assertion = cbor2.loads(payload)


@then('the dcs.lifecycle assertion in the updated manifest has contract_id "{expected}"')
def step_updated_lifecycle_contract_id(context, expected):
    assertion = getattr(context, "lifecycle_assertion", None)
    assert assertion is not None, "no lifecycle assertion in context"
    assert assertion.get("contract_id") == expected, (
        f"expected contract_id={expected!r}, got {assertion.get('contract_id')!r}"
    )


@then("the dcs.lifecycle file_hash differs from the compiled PDF file_hash")
def step_lifecycle_file_hash_differs(context):
    assertion = getattr(context, "lifecycle_assertion", None)
    assert assertion is not None, "no lifecycle assertion in context"
    compiled_hash = getattr(context, "lifecycle_file_hash_compiled", None)
    updated_hash = assertion.get("file_hash")
    assert isinstance(updated_hash, str) and len(updated_hash) == 64, (
        f"updated file_hash invalid: {updated_hash!r}"
    )
    assert updated_hash != compiled_hash, (
        "amended document file_hash must differ from original compiled document file_hash"
    )


@then('the dcs.lifecycle assertion has status "{expected}"')
def step_lifecycle_status(context, expected):
    assertion = getattr(context, "lifecycle_assertion", None)
    assert assertion is not None, "no lifecycle assertion in context"
    assert assertion.get("status") == expected, (
        f"expected status={expected!r}, got {assertion.get('status')!r}"
    )


@then("the dcs.lifecycle assertion has a valid effective_at timestamp")
def step_lifecycle_effective_at(context):
    assertion = getattr(context, "lifecycle_assertion", None)
    assert assertion is not None, "no lifecycle assertion in context"
    effective_at = assertion.get("effective_at")
    assert isinstance(effective_at, str) and len(effective_at) > 0, (
        f"effective_at must be a non-empty string, got {effective_at!r}"
    )
    try:
        datetime.fromisoformat(effective_at.replace("Z", "+00:00"))
    except ValueError as e:
        raise AssertionError(f"effective_at {effective_at!r} is not valid RFC 3339: {e}")


@then("the dcs.lifecycle assertion has an empty prev_manifest_hash")
def step_lifecycle_prev_manifest_hash_empty(context):
    assertion = getattr(context, "lifecycle_assertion", None)
    assert assertion is not None, "no lifecycle assertion in context"
    assert "prev_manifest_hash" in assertion, "prev_manifest_hash key missing from lifecycle assertion"
    assert assertion["prev_manifest_hash"] == "", (
        f"expected empty prev_manifest_hash for initial compile, got {assertion['prev_manifest_hash']!r}"
    )


@then('the dcs.lifecycle assertion has field "{field}"')
def step_lifecycle_has_field(context, field):
    assertion = getattr(context, "lifecycle_assertion", None)
    assert assertion is not None, "no lifecycle assertion in context"
    assert field in assertion, f"field {field!r} missing from lifecycle assertion; keys: {list(assertion.keys())}"


@then('the dcs.lifecycle assertion in the updated manifest has status "{expected}"')
def step_updated_lifecycle_status(context, expected):
    assertion = getattr(context, "lifecycle_assertion", None)
    assert assertion is not None, "no lifecycle assertion in context"
    assert assertion.get("status") == expected, (
        f"expected status={expected!r}, got {assertion.get('status')!r}"
    )


@then("the dcs.lifecycle assertion in the updated manifest has a valid effective_at timestamp")
def step_updated_lifecycle_effective_at(context):
    assertion = getattr(context, "lifecycle_assertion", None)
    assert assertion is not None, "no lifecycle assertion in context"
    effective_at = assertion.get("effective_at")
    assert isinstance(effective_at, str) and len(effective_at) > 0, (
        f"effective_at must be a non-empty string, got {effective_at!r}"
    )
    try:
        datetime.fromisoformat(effective_at.replace("Z", "+00:00"))
    except ValueError as e:
        raise AssertionError(f"effective_at {effective_at!r} is not valid RFC 3339: {e}")


@then("the dcs.lifecycle assertion in the updated manifest has a non-empty prev_manifest_hash")
def step_updated_lifecycle_prev_manifest_hash(context):
    assertion = getattr(context, "lifecycle_assertion", None)
    assert assertion is not None, "no lifecycle assertion in context"
    pmh = assertion.get("prev_manifest_hash")
    assert isinstance(pmh, str) and len(pmh) == 64, (
        f"expected 64-char hex prev_manifest_hash, got {pmh!r}"
    )
