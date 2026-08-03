"""PDF generation API client helpers for BDD steps."""

import requests

from steps.support.api_client import (
    contract_export_pdf_url,
    contract_verify_pdf_url,
    template_export_pdf_url,
    template_verify_pdf_url,
)


class PDFService:
    """Wraps the PDF generation backend endpoints for test steps."""

    @staticmethod
    def export_contract_pdf(context, did: str, headers=None) -> requests.Response:
        h = headers if headers is not None else getattr(context, "headers", {})
        return requests.get(
            contract_export_pdf_url(context, did),
            headers=h,
            timeout=context.http_timeout_seconds,
        )

    @staticmethod
    def export_template_pdf(context, did: str) -> requests.Response:
        headers = getattr(context, "headers", {})
        return requests.get(
            template_export_pdf_url(context, did),
            headers=headers,
            timeout=context.http_timeout_seconds,
        )

    @staticmethod
    def verify_contract_pdf(context, did: str, headers=None) -> requests.Response:
        headers = headers if headers is not None else getattr(context, "headers", {})
        return requests.get(
            contract_verify_pdf_url(context, did),
            headers=headers,
            timeout=context.http_timeout_seconds,
        )

    @staticmethod
    def verify_template_pdf(context, did: str) -> requests.Response:
        headers = getattr(context, "headers", {})
        return requests.get(
            template_verify_pdf_url(context, did),
            headers=headers,
            timeout=context.http_timeout_seconds,
        )
