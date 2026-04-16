"""Shared HTTP and URL helpers for executable BDD scenarios."""

import requests


# URL builders

def contract_create_url(context) -> str:
    return f"{context.base_url}/contract/create"


def contract_update_url(context) -> str:
    return f"{context.base_url}/contract/update"


def contract_submit_url(context) -> str:
    return f"{context.base_url}/contract/submit"


def contract_negotiate_url(context) -> str:
    return f"{context.base_url}/contract/negotiate"


def contract_review_url(context) -> str:
    return f"{context.base_url}/contract/review"


def contract_approve_url(context) -> str:
    return f"{context.base_url}/contract/approve"


def contract_reject_url(context) -> str:
    return f"{context.base_url}/contract/reject"


def contract_retrieve_url(context) -> str:
    return f"{context.base_url}/contract/retrieve"


def contract_retrieve_by_id_url(context, did: str) -> str:
    return f"{context.base_url}/contract/retrieve/{did}"


def contract_verify_url(context) -> str:
    return f"{context.base_url}/contract/verify"


def template_create_url(context) -> str:
    return f"{context.base_url}/template/create"


def template_retrieve_by_id_url(context, did: str) -> str:
    return f"{context.base_url}/template/retrieve/{did}"


def template_retrieve_url(context) -> str:
    return f"{context.base_url}/template/retrieve"


def template_submit_url(context) -> str:
    return f"{context.base_url}/template/submit"


def template_update_url(context) -> str:
    return f"{context.base_url}/template/update"


def template_verify_url(context) -> str:
    return f"{context.base_url}/template/verify"


def template_approve_url(context) -> str:
    return f"{context.base_url}/template/approve"


def template_reject_url(context) -> str:
    return f"{context.base_url}/template/reject"


def template_register_url(context) -> str:
    return f"{context.base_url}/template/register"


def template_archive_url(context) -> str:
    return f"{context.base_url}/template/archive"


def template_audit_url(context) -> str:
    return f"{context.base_url}/template/audit"


def template_search_url(context) -> str:
    return f"{context.base_url}/template/search"


# HTTP helpers

def post_json(context, url: str, payload: dict, headers=None):
    h = headers if headers is not None else getattr(context, "headers", {})
    return requests.post(
        url,
        json=payload,
        headers=h,
        timeout=context.http_timeout_seconds,
    )


def put_json(context, url: str, payload: dict, headers=None):
    h = headers if headers is not None else getattr(context, "headers", {})
    return requests.put(
        url,
        json=payload,
        headers=h,
        timeout=context.http_timeout_seconds,
    )


def get_with_headers(context, url: str, headers=None):
    h = headers if headers is not None else getattr(context, "headers", {})
    return requests.get(
        url,
        headers=h,
        timeout=context.http_timeout_seconds,
    )
