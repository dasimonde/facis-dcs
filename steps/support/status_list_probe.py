"""Revoking a credential on the ORCE issuer, and proving a refusal came from
the bit that was set.

Revocation scenarios drive the issuer's own admin endpoint
(deployment/helm/charts/orce/flows-issuer/flow-admin.json), which writes the
persisted bitstring the signed list at <issuer base>/status-list/1 is built
from. That is the same list the credential names and the backend fetches, so
one bit is the whole difference between the two runs of an otherwise identical
presentation.

The hazard these helpers exist for: the backend refuses a presentation both
when the bit is set and when it cannot use the list at all — the issuer is
down, the list is signed by a key no anchor covers, its `sub` does not match
the credential's URI, the index is past the end of the served bitstring. Every
one of those produces a 4xx with the words "status list" in it, so an assertion
on "rejected", or on the word "status", passes just as happily when nothing was
ever revoked. Both sides are pinned instead:

  - before presenting: the served list is fetched twice and the bit must read
    0 then 1, which is only possible if the list loads AND the admin call took
    effect (dcs_wallet.status_list.revoke_and_prove);
  - after presenting: the refusal must name the index and call it revoked,
    which is the one message the backend produces for a set bit and for
    nothing else (statuslist_verify.go mapStatusListRejection).
"""

from __future__ import annotations

# statuslist_verify.go mapStatusListRejection — the only refusal that means
# "the bit for this index is set". Every other status-list refusal below is a
# list the verifier could not use, and says so instead of naming an index.
_REVOKED_MESSAGE = "credential status list index {index} is revoked"

# Every refusal the verifier can produce for a list it could not USE, quoted
# from backend/internal/auth/oid4vp/status/errors.go, status/policy.go and
# statuslist_verify.go. Each one is a scenario that proves nothing about
# revocation while looking exactly like a scenario that does.
#
# The assertion below does not depend on this being exhaustive: anything that
# is not the revoked message fails the scenario either way. What the list buys
# is the message — "the issuer is serving a list signed by a key you do not
# anchor" instead of "expected a rejection, got a rejection".
_LIST_UNUSABLE_MARKERS = (
    "status list retrieval failed",
    "status list signature verification failed",
    "status list is signed by an issuer other than the one it names",
    "status list subject does not match reference uri",
    "status list is not secured",
    "status list decoding failed",
    "status list decompression failed",
    "status list trust configuration is required",
    "unsupported status list media type",
    "status index out of range",
    "invalid status size",
    "wrong status list type",
    "status list verifier is not configured",
    "credential has no status reference",
    "credential status cannot be safely interpreted",
)


def revoke_credential_bit(context, claims) -> int:
    """Revoke the status-list index the credential's own claims name.

    Taking the index and the list URI out of the credential rather than
    recomputing them is what makes the scenario revoke the very credential it
    is about to present. Returns the index and stashes it on the context for
    assert_refused_for_the_revoked_bit.
    """
    from steps.support.services.auth_service import AuthService  # noqa: PLC0415

    AuthService._ensure_dcs_wallet_importable()
    from dcs_wallet.status_list import credential_status_from_claims, revoke_and_prove  # noqa: PLC0415

    parsed = credential_status_from_claims(claims)
    assert parsed, f"credential carries no status claim to revoke: {claims}"
    index, uri = parsed
    revoke_and_prove(index, status_list_uri=uri)
    context.revoked_status_index = index
    return index


def assert_refused_for_the_revoked_bit(context, response, what: str) -> None:
    """Assert `response` refuses `what` because the revoked index is set."""
    index = getattr(context, "revoked_status_index", None)
    assert index is not None, (
        f"no status-list index was revoked in this scenario, so a refusal of {what} "
        f"cannot be attributed to one"
    )
    assert response.status_code >= 400, (
        f"expected {what} to be refused after revoking status-list index {index}, "
        f"got {response.status_code}: {response.text}"
    )

    body = response.text
    if _REVOKED_MESSAGE.format(index=index) in body:
        return

    unusable = [marker for marker in _LIST_UNUSABLE_MARKERS if marker in body]
    assert not unusable, (
        f"{what} was refused because the status list could not be used ({', '.join(unusable)}), "
        f"not because index {index} is revoked — the list is the thing under test, so this "
        f"outcome proves nothing about revocation: {body}"
    )
    raise AssertionError(
        f"expected the refusal of {what} to name index {index} as revoked, got: {body}"
    )
