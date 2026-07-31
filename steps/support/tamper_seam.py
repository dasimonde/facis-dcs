"""Shared helper for infra-mutation test seams used by the "inject a
tampered/stripped artifact" BDD scenarios in contract_format_review,
c2pa_conformance, and real_signing_vertical.

Why this exists: several verify-shaped backend endpoints (GET
/pdf/verify/contract/{did}, POST /signature/validate, GET
/c2pa/manifest/{contract_did}) always re-fetch the SERVER'S OWN stored PDF
from IPFS by the CID cached in the `contracts.pdf_ipfs_cid` column (see
backend/internal/pdfgeneration/query/verifycontract.go and
backend/internal/signingmanagement/db/pg/contractrepository.go's
FetchContractPDFBytes) — there is no upload-a-PDF-and-verify-it endpoint.
IPFS is content-addressed, so an existing CID's bytes can never be
overwritten in place; the seam is instead:

  1. Add the tampered/stripped bytes as a NEW CID, via `ipfs add` exec'd
     inside the shared in-cluster IPFS pod (see _ipfs_exec_prefix below).
  2. Point `contracts.pdf_ipfs_cid` at that new CID, via the existing
     `context.db` test-DB connection (see environment.py) — a direct
     Postgres connection is already an accepted test-only seam in this
     codebase (also used, read-only, by steps/peer_trust/
     dcs_trust_pdp_steps.py's `sync_fails` polling under ADR-19), preferred
     here over a second kubectl-exec-into-postgres seam.

This is a genuine black-box test target afterwards: the verify/validate
endpoints under test do not know or care that the CID was swapped by a test
seam rather than by the application's own PDF generation/signing code path
— they just fetch whatever is at `pdf_ipfs_cid` (or the equivalent column
for other tables) and check it, exactly as in production.

Restoration: the original CID value is captured before the swap and
restored via `context.add_cleanup`, which behave guarantees runs after the
scenario ends (pass OR fail) — satisfying the "restore any cluster state
you mutate, even on failure" rule without a manual try/finally in every step
that uses this seam. The restore function is also returned directly so a
step can invoke it PROACTIVELY mid-scenario (e.g. "I fix the inconsistency"
in contract_format_review.feature) — calling it twice (once explicitly,
once again as the cleanup safety net) is a harmless no-op UPDATE.
"""

from __future__ import annotations

import os
import shlex
import subprocess


class CidSwapHandle:
    """Result of swap_contract_pdf_cid: the new CID plus an idempotent
    restore() callable that points pdf_ipfs_cid back at the original value.
    """

    def __init__(self, new_cid: str, old_cid, restore):
        self.new_cid = new_cid
        self.old_cid = old_cid
        self.restore = restore


def _ipfs_exec_prefix() -> list:
    """Return the kubectl-exec argv prefix for running commands inside the
    shared IPFS pod.

    IPFS is a SINGLE instance shared across both BDD releases (see
    deployment/helm/values.bdd2.yml's ipfsClient.mfsBaseURL pointing at
    "dcs-ipfs" regardless of which DCS instance is calling) — there is no
    per-instance IPFS to disambiguate.

    Mirrors the BDD_HSMSIGN_EXEC convention established in
    tests/bdd/scripts/run_bdd_helm.sh: an env-var-provided exec prefix so
    this works identically in CI (GitHub Actions kind-in-docker) and local
    dev, without this Python code hardcoding a kubectl invocation. Hard-
    fails rather than silently skipping when unset (feedback_hard_fail_
    dependencies: never soft-fail a required external seam).
    """
    raw = os.environ.get("BDD_IPFS_EXEC")
    if not raw:
        raise RuntimeError(
            "BDD_IPFS_EXEC is not set — required for the IPFS CID-swap tamper "
            "seam. tests/bdd/scripts/run_bdd_helm.sh must export it (mirrors "
            "BDD_HSMSIGN_EXEC's convention). Hard-failing rather than silently "
            "skipping the seam."
        )
    return shlex.split(raw)


def ipfs_add_bytes(data: bytes) -> str:
    """Add `data` to the shared in-cluster IPFS node and return its CID.

    Content-addressed by design: this never overwrites any existing CID, it
    only ever adds new content under a new hash — nothing to restore for
    this half of the seam. The orphaned blob left pinned after a row-pointer
    restore is a harmless, permanent no-op on the cluster (same class of
    residue as any other BDD-created test fixture never garbage collected).
    """
    cmd = _ipfs_exec_prefix() + ["ipfs", "add", "-Q", "-"]
    proc = subprocess.run(cmd, input=data, capture_output=True, timeout=60)
    if proc.returncode != 0:
        raise RuntimeError(
            f"ipfs add failed (exit {proc.returncode}): "
            f"{proc.stderr.decode(errors='replace')}"
        )
    cid = proc.stdout.decode().strip()
    assert cid, (
        f"ipfs add produced no CID on stdout (stderr: "
        f"{proc.stderr.decode(errors='replace')})"
    )
    return cid


def ipfs_cat_bytes(cid: str) -> bytes:
    """Fetch the raw bytes stored under `cid` from the shared in-cluster IPFS
    node, exactly as an IPFS-level reader would see them — bypassing the DCS
    API entirely. Read-only counterpart of ipfs_add_bytes over the same
    BDD_IPFS_EXEC seam; used by the opaque-at-rest scenarios
    (features/26_encryption_at_rest/) to prove stored artifacts are
    ciphertext, not plaintext PDFs (DCS-NFR-SEC-14).
    """
    cmd = _ipfs_exec_prefix() + ["ipfs", "cat", cid]
    proc = subprocess.run(cmd, capture_output=True, timeout=60)
    if proc.returncode != 0:
        raise RuntimeError(
            f"ipfs cat {cid} failed (exit {proc.returncode}): "
            f"{proc.stderr.decode(errors='replace')}"
        )
    assert proc.stdout, f"ipfs cat {cid} returned no bytes"
    return proc.stdout


def swap_contract_pdf_cid(context, did: str, new_pdf_bytes: bytes) -> CidSwapHandle:
    """Point contracts.pdf_ipfs_cid at a NEW CID holding `new_pdf_bytes`, for
    `did`. Only the CID column is touched — pdf_c2pa_state/pdf_payload_hash/
    pdf_renderer_version are left as-is so the backend's cache-freshness
    checks (pdfState.C2PAState == currentC2PAState, PayloadHash comparisons
    in verifycontract.go/exportcontract.go) still see a "fresh" cache and
    fetch straight from the (now swapped) CID rather than silently
    regenerating over it.

    Returns a CidSwapHandle. A context.add_cleanup is registered
    automatically; the caller does not need to arrange restoration itself.
    """
    new_cid = ipfs_add_bytes(new_pdf_bytes)

    cursor = context.db.cursor()
    try:
        cursor.execute(
            "SELECT pdf_ipfs_cid FROM contracts WHERE did = %s", (did,)
        )
        row = cursor.fetchone()
        assert row is not None, f"no contracts row found for did={did!r}"
        original_cid = row[0]

        cursor.execute(
            "UPDATE contracts SET pdf_ipfs_cid = %s WHERE did = %s",
            (new_cid, did),
        )
        context.db.commit()
    except Exception:
        context.db.rollback()
        raise
    finally:
        cursor.close()

    def _restore():
        restore_cursor = context.db.cursor()
        try:
            restore_cursor.execute(
                "UPDATE contracts SET pdf_ipfs_cid = %s WHERE did = %s",
                (original_cid, did),
            )
            context.db.commit()
        except Exception:
            context.db.rollback()
            raise
        finally:
            restore_cursor.close()

    context.add_cleanup(_restore)
    return CidSwapHandle(new_cid=new_cid, old_cid=original_cid, restore=_restore)
