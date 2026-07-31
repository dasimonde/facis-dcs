#!/usr/bin/env python3
"""Independently verify a DCS-produced contract PDF is a real, conformant
artifact — PDF/A-3a (veraPDF) and a valid C2PA manifest (c2patool / c2pa-rs) —
using the SAME external validators pdf-core runs in its own suite
(pdf-core/features/steps/dcs_pdf_core_steps.py). The Playwright vertical shells
out to this at every artifact-producing hop so "it exports a PDF" becomes "it
exports the verifiable artifact we promise".

    python3 verify_artifact.py <pdf-path> [--lifecycle proposed|agreed|executed]

Exits non-zero with a diagnostic on any failure. c2patool is downloaded and
cached on first use; veraPDF runs via its official CLI image (docker).
"""

from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys
import tarfile
import tempfile
import urllib.request

_C2PATOOL_VERSION = os.environ.get("DCS_C2PATOOL_VERSION", "0.26.61")
_C2PATOOL_CACHE_DIR = os.path.join(tempfile.gettempdir(), "dcs-c2patool")
_VERAPDF_IMAGE = os.environ.get("DCS_VERAPDF_IMAGE", "ghcr.io/verapdf/cli:latest")


def _ensure_c2patool() -> str:
    found = shutil.which("c2patool")
    if found:
        return found
    os.makedirs(_C2PATOOL_CACHE_DIR, exist_ok=True)
    archive_name = f"c2patool-v{_C2PATOOL_VERSION}-x86_64-unknown-linux-gnu.tar.gz"
    archive_path = os.path.join(_C2PATOOL_CACHE_DIR, archive_name)
    extract_dir = os.path.join(_C2PATOOL_CACHE_DIR, f"c2patool-v{_C2PATOOL_VERSION}")
    if not os.path.isfile(os.path.join(extract_dir, "c2patool")):
        url = (
            "https://github.com/contentauth/c2pa-rs/releases/download/"
            f"c2patool-v{_C2PATOOL_VERSION}/{archive_name}"
        )
        if not os.path.exists(archive_path):
            with urllib.request.urlopen(url, timeout=120) as resp, open(archive_path, "wb") as fh:
                fh.write(resp.read())
        os.makedirs(extract_dir, exist_ok=True)
        with tarfile.open(archive_path, "r:gz") as archive:
            archive.extractall(extract_dir)
    for root, _dirs, files in os.walk(extract_dir):
        if "c2patool" in files:
            binary = os.path.join(root, "c2patool")
            os.chmod(binary, 0o755)
            return binary
    raise SystemExit("c2patool binary could not be prepared")


def _verapdf(pdf_path: str) -> None:
    artifacts_dir = os.path.dirname(os.path.abspath(pdf_path)) or "."
    name = os.path.basename(pdf_path)
    completed = subprocess.run(
        ["docker", "run", "--rm", "-v", f"{artifacts_dir}:/data", _VERAPDF_IMAGE,
         "-f", "3a", "--format", "text", f"/data/{name}"],
        check=False, capture_output=True, text=True, timeout=300,
    )
    if "PASS" not in completed.stdout:
        # The terse text format reports only PASS/FAIL, which says nothing about
        # WHICH clause the artifact violates. Re-run for the failed-rule detail
        # so a conformance regression is actionable rather than just "FAIL 3a".
        detail = subprocess.run(
            ["docker", "run", "--rm", "-v", f"{artifacts_dir}:/data", _VERAPDF_IMAGE,
             "-f", "3a", "--format", "xml", f"/data/{name}"],
            check=False, capture_output=True, text=True, timeout=300,
        )
        failed = []
        for line in detail.stdout.splitlines():
            stripped = line.strip()
            if "<rule" in stripped and 'status="failed"' in stripped:
                failed.append(stripped[:300])
            elif "<description>" in stripped or "<errorMessage>" in stripped:
                failed.append(stripped[:300])
        raise SystemExit(
            f"veraPDF PDF/A-3a validation FAILED for {name}\n"
            f"stdout:\n{completed.stdout}\nstderr:\n{completed.stderr}\n"
            f"failed rules:\n  " + ("\n  ".join(failed[:40]) or "(no rule detail parsed)")
        )


def _c2patool(pdf_path: str, lifecycle: str | None) -> None:
    binary = _ensure_c2patool()
    completed = subprocess.run(
        [binary, pdf_path, "--detailed"],
        check=False, capture_output=True, text=True, timeout=300,
    )
    if completed.returncode != 0:
        raise SystemExit(
            f"c2patool (c2pa-rs) rejected the C2PA manifest in {os.path.basename(pdf_path)}\n"
            f"stdout:\n{completed.stdout}\nstderr:\n{completed.stderr}"
        )
    if lifecycle:
        # The DCS stamps the SRS lifecycle state as a C2PA assertion; require the
        # expected banner (proposed/agreed/executed) to appear in the validated
        # manifest so negotiation/settle/sign transitions are provable on the
        # artifact itself, not just the DCS's own state column.
        if lifecycle.lower() not in completed.stdout.lower():
            raise SystemExit(
                f"C2PA manifest for {os.path.basename(pdf_path)} does not carry the expected "
                f"lifecycle '{lifecycle}'.\nc2patool output:\n{completed.stdout}"
            )


def _extract_embedded_jsonld(pdf: bytes) -> bytes:
    """Return the embedded contract.jsonld byte stream (the latest, superseding
    definition for an incrementally updated PDF), mirroring pdf-core's own
    extractJSONLDStream. The EmbeddedFile stream is uncompressed JSON-LD."""
    fs_pos = pdf.find(b"/F (contract.jsonld)")
    if fs_pos < 0:
        raise SystemExit("embedded JSON-LD filespec not found")
    ef = pdf.find(b"/EF << /F ", fs_pos)
    if ef < 0:
        raise SystemExit("embedded JSON-LD object reference not found")
    ef += len(b"/EF << /F ")
    ref_end = pdf.find(b" 0 R", ef)
    if ref_end < 0:
        raise SystemExit("embedded JSON-LD object reference malformed")
    obj_id = int(pdf[ef:ref_end].strip())
    obj_pos = pdf.rfind(b"%d 0 obj" % obj_id)
    if obj_pos < 0:
        raise SystemExit("embedded JSON-LD object not found")
    start = pdf.find(b"stream", obj_pos)
    if start < 0:
        raise SystemExit("embedded JSON-LD stream start not found")
    start += len(b"stream")
    if pdf[start:start + 2] == b"\r\n":
        start += 2
    elif pdf[start:start + 1] in (b"\n", b"\r"):
        start += 1
    end = pdf.find(b"endstream", start)
    if end < 0:
        raise SystemExit("embedded JSON-LD stream end not found")
    data = pdf[start:end]
    if data.endswith(b"\r\n"):
        return data[:-2]
    if data.endswith(b"\n") or data.endswith(b"\r"):
        return data[:-1]
    return data


def main() -> int:
    parser = argparse.ArgumentParser(description="Verify a DCS contract PDF (PDF/A-3a + C2PA).")
    parser.add_argument("pdf")
    parser.add_argument("--lifecycle", default=None,
                        help="expected C2PA lifecycle banner: proposed|agreed|executed")
    parser.add_argument("--dump-jsonld", default=None,
                        help="also write the PDF's embedded contract JSON-LD payload to this path")
    parser.add_argument("--extract-only", action="store_true",
                        help="skip veraPDF/c2patool and only extract the embedded JSON-LD (use with --dump-jsonld)")
    args = parser.parse_args()
    if not os.path.isfile(args.pdf):
        raise SystemExit(f"no such PDF: {args.pdf}")
    with open(args.pdf, "rb") as fh:
        pdf_bytes = fh.read()
    if pdf_bytes[:5] != b"%PDF-":
        raise SystemExit(f"{args.pdf} is not a PDF")
    if not args.extract_only:
        _verapdf(args.pdf)
        _c2patool(args.pdf, args.lifecycle)
    if args.dump_jsonld:
        payload = _extract_embedded_jsonld(pdf_bytes)
        out_dir = os.path.dirname(os.path.abspath(args.dump_jsonld))
        os.makedirs(out_dir, exist_ok=True)
        with open(args.dump_jsonld, "wb") as fh:
            fh.write(payload)
    if args.extract_only:
        print(json.dumps({"pdf": os.path.basename(args.pdf),
                          "extracted_jsonld": bool(args.dump_jsonld)}))
        return 0
    print(json.dumps({"pdf": os.path.basename(args.pdf), "pdfa3a": "PASS",
                      "c2pa": "VALID", "lifecycle": args.lifecycle or "n/a"}))
    return 0


if __name__ == "__main__":
    sys.exit(main())
