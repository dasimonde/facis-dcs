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
    def status(cls, context, channel: str) -> dict:
        body = cls.request(context, f"/status?channel={channel}").json()
        assert isinstance(body, dict), f"Expected ORCE {channel} status, got {body!r}"
        return body

    @classmethod
    def run_once(cls, context, channel: str) -> None:
        cls.request(context, "/run-once", {"channel": channel})
