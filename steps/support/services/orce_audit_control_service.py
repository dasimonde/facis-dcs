"""Shared control/observation client for the bundled ORCE BDD flow."""

from __future__ import annotations

import os
import time

import requests

from steps.support.api_client import origin_url


class OrceAuditControlService:
    """Use the existing audit-executor test seam for all PAC executor modes."""

    @staticmethod
    def base_url(context) -> str:
        configured = os.getenv("BDD_ORCE_AUDIT_CONTROL_URL", "").strip()
        return configured.rstrip("/") if configured else (
            f"{origin_url(context.base_url)}/orce/audit-executor/test"
        )

    @staticmethod
    def executor_url(context) -> str:
        configured = os.getenv("BDD_ORCE_AUDIT_EXECUTOR_URL", "").strip()
        return configured or f"{origin_url(context.base_url)}/orce/audit/run"

    @classmethod
    def post_executor(cls, context, payload: dict):
        """Wait for the flow route after an ORCE pod replacement."""
        deadline = time.monotonic() + 90
        while True:
            try:
                response = requests.post(
                    cls.executor_url(context),
                    json=payload,
                    timeout=context.http_timeout_seconds,
                )
                if response.status_code != 404 or time.monotonic() >= deadline:
                    return response
            except (requests.ConnectionError, requests.Timeout):
                if time.monotonic() >= deadline:
                    raise
            time.sleep(2)

    @classmethod
    def request(cls, context, path: str, payload: dict | None = None):
        url = f"{cls.base_url(context)}{path}"
        deadline = time.monotonic() + 90
        while True:
            try:
                if payload is None:
                    response = requests.get(
                        url,
                        timeout=context.http_timeout_seconds,
                    )
                else:
                    response = requests.post(
                        url,
                        json=payload,
                        timeout=context.http_timeout_seconds,
                    )
                # Kubernetes marks the replacement pod ready before Node-RED
                # has necessarily registered every flow route. A 404 directly
                # after an ORCE restart is therefore a transient readiness
                # state, just like a refused connection.
                if response.status_code != 404 or time.monotonic() >= deadline:
                    break
            except (requests.ConnectionError, requests.Timeout):
                if time.monotonic() >= deadline:
                    raise
            time.sleep(2)
        assert response.status_code in (200, 204), (
            f"ORCE audit control endpoint {url} failed: "
            f"{response.status_code} {response.text}"
        )
        return response

    @classmethod
    def reset(cls, context, channel: str) -> None:
        cls.request(context, "/reset", {"channel": channel})

    @classmethod
    def set_mode(cls, context, channel: str, mode: str) -> None:
        cls.request(context, "/mode", {"channel": channel, "mode": mode})

    @classmethod
    def observations(cls, context, channel: str) -> list[dict]:
        body = cls.request(context, f"/requests?channel={channel}").json()
        observations = body.get("requests") if isinstance(body, dict) else body
        assert isinstance(observations, list), (
            f"Expected ORCE {channel} observations, got {body!r}"
        )
        return observations

    @classmethod
    def evidence_groups(cls, context, evidence_scope: str, channel: str = "audit") -> list[dict]:
        """Return the DCS-procured timeline groups sent to the executor.

        POST /pac/audit returns the external executor result. The audit trail
        used as input remains observable at the ORCE test seam and must not be
        inferred from that result response.
        """
        observations = cls.observations(context, channel)
        assert observations, f"Expected an ORCE {channel} observation"
        observation = observations[-1]
        request = observation.get("request", observation) if isinstance(observation, dict) else {}
        evidence = request.get("evidence") or {}
        groups = evidence.get(evidence_scope) or []
        assert isinstance(groups, list), (
            f"Expected {evidence_scope!r} evidence groups in the ORCE request, got {groups!r}"
        )
        return [group for group in groups if isinstance(group, dict)]

    @classmethod
    def status(cls, context, channel: str) -> dict:
        body = cls.request(context, f"/status?channel={channel}").json()
        assert isinstance(body, dict), f"Expected ORCE {channel} status, got {body!r}"
        return body

    @classmethod
    def run_once(cls, context, channel: str) -> None:
        cls.request(context, "/run-once", {"channel": channel})
