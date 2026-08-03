"""HTTP support for workflow-gate execution and PACM review."""

from __future__ import annotations

import requests

from steps.support.services.auth_service import AuthService


class WorkflowGateService:
    @staticmethod
    def review_url(context) -> str:
        return f"{context.base_url}/pac/workflow-gates/review"

    @staticmethod
    def read_url(context, run_id: str) -> str:
        return f"{context.base_url}/pac/workflow-gates/{run_id}"

    @classmethod
    def decide_review(
        cls,
        context,
        *,
        run_id: str,
        decision: str,
        justification: str,
        roles: list[str],
    ):
        return requests.post(
            cls.review_url(context),
            json={
                "run_id": run_id,
                "decision": decision,
                "justification": justification,
            },
            headers=AuthService.get_headers_for_roles(roles),
            timeout=context.http_timeout_seconds,
        )

    @classmethod
    def read_run(cls, context, run_id: str):
        return requests.get(
            cls.read_url(context, run_id),
            headers=AuthService.get_headers_for_roles(["Compliance Officer"]),
            timeout=context.http_timeout_seconds,
        )
