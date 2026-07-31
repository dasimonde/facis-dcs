"""Read DCS-procured audit evidence from the request observed by ORCE.

POST /pac/audit returns the versioned executor result. The audit timeline
consumed by BDD assertions remains in the request's evidence envelope.
"""

from steps.support.services.orce_audit_control_service import OrceAuditControlService


class PacAuditEvidenceService:
    """Adapter from ORCE observations to legacy audit-trail entries."""

    _SCOPE_ALIASES = {
        "template": "templates",
        "templates": "templates",
        "contract_template_repository": "templates",
        "contract": "contracts",
        "contracts": "contracts",
        "contract_workflow_engine": "contracts",
        "archive": "archive",
        "contract_storage_archive": "archive",
        "signatures": "signatures",
        "signature_management": "signatures",
    }

    @staticmethod
    def _evidence_scope(scope: str) -> str:
        normalized = scope.strip()
        return PacAuditEvidenceService._SCOPE_ALIASES.get(
            normalized.lower(),
            normalized,
        )

    @staticmethod
    def observed_scope_results(
        context,
        scope: str,
        api_base: str | None = None,
    ) -> list[dict]:
        evidence_scope = PacAuditEvidenceService._evidence_scope(scope)
        results = []
        for observation in OrceAuditControlService.observations(
            context,
            "audit",
            api_base=api_base,
        ):
            request = (
                observation.get("request", observation)
                if isinstance(observation, dict)
                else {}
            )
            if request.get("scope") != evidence_scope:
                continue
            evidence = request.get("evidence") or {}
            for scope_result in evidence.get(evidence_scope) or []:
                if isinstance(scope_result, dict):
                    results.append(scope_result)
        return results

    @staticmethod
    def observed_audit_entries(
        context,
        scope: str,
        did: str | None = None,
        api_base: str | None = None,
    ) -> list[dict]:
        entries = [
            entry
            for scope_result in PacAuditEvidenceService.observed_scope_results(
                context,
                scope,
                api_base,
            )
            for entry in (scope_result.get("audit_trail") or [])
            if isinstance(entry, dict)
        ]
        if did is not None:
            entries = [entry for entry in entries if entry.get("did") == did]
        return entries
