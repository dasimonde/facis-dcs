"""HTTP transport for access-attempt audit specifications."""

import requests

from steps.support.api_client import pac_access_attempts_url, pac_audit_url, pac_report_url, post_json


class AccessAttemptService:
    @staticmethod
    def read(context, headers, **params):
        return requests.get(
            pac_access_attempts_url(context), params=params, headers=headers,
            timeout=context.http_timeout_seconds,
        )

    @staticmethod
    def denied_request(context, action, headers, payload):
        if action == "audit":
            return post_json(context, pac_audit_url(context), payload, headers=headers)
        assert action == "report", action
        return requests.get(
            pac_report_url(context), params={**payload, "format": "json"}, headers=headers,
            timeout=context.http_timeout_seconds,
        )
