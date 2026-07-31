from __future__ import annotations

import unittest
from unittest.mock import ANY, patch

from steps.support.services.orce_audit_control_service import OrceAuditControlService
from steps.support.services.pac_audit_evidence_service import PacAuditEvidenceService


class PacAuditEvidenceServiceTest(unittest.TestCase):
    def test_reads_only_matching_scope_from_observed_request_evidence(self) -> None:
        observations = [
            {
                "request": {
                    "scope": "contracts",
                    "evidence": {
                        "contracts": [
                            {
                                "audit_trail": [
                                    {"did": "did:contract:1", "event_type": "MATCH"},
                                    {"did": "did:contract:2", "event_type": "OTHER"},
                                ]
                            }
                        ]
                    },
                }
            },
            {
                "request": {
                    "scope": "SYSTEM",
                    "evidence": {
                        "SYSTEM": [
                            {
                                "audit_trail": [
                                    {"did": "did:contract:1", "event_type": "WRONG_SCOPE"}
                                ]
                            }
                        ]
                    },
                }
            },
        ]

        with patch.object(
            OrceAuditControlService,
            "observations",
            return_value=observations,
        ) as observed:
            entries = PacAuditEvidenceService.observed_audit_entries(
                object(),
                "contracts",
                "did:contract:1",
                api_base="http://dcs-b.localhost:18080/api",
            )

        observed.assert_called_once_with(
            ANY,
            "audit",
            api_base="http://dcs-b.localhost:18080/api",
        )
        self.assertEqual(
            entries,
            [{"did": "did:contract:1", "event_type": "MATCH"}],
        )

    def test_ignores_malformed_observations_and_entries(self) -> None:
        observations = [
            "not-an-observation",
            {"request": {"scope": "contracts", "evidence": None}},
            {
                "scope": "contracts",
                "evidence": {
                    "contracts": [
                        {"audit_trail": [None, "not-an-entry", {"event_type": "MATCH"}]}
                    ]
                },
            },
        ]

        with patch.object(
            OrceAuditControlService,
            "observations",
            return_value=observations,
        ):
            entries = PacAuditEvidenceService.observed_audit_entries(
                object(), "contracts"
            )

        self.assertEqual(entries, [{"event_type": "MATCH"}])

    def test_normalizes_component_scope_to_backend_evidence_scope(self) -> None:
        observations = [
            {
                "request": {
                    "scope": "archive",
                    "evidence": {
                        "archive": [
                            {
                                "audit_trail": [
                                    {
                                        "did": "did:contract:archive",
                                        "event_type": "ARCHIVED",
                                    }
                                ]
                            }
                        ]
                    },
                }
            }
        ]

        with patch.object(
            OrceAuditControlService,
            "observations",
            return_value=observations,
        ):
            entries = PacAuditEvidenceService.observed_audit_entries(
                object(),
                "CONTRACT_STORAGE_ARCHIVE",
                "did:contract:archive",
            )

        self.assertEqual(
            entries,
            [{"did": "did:contract:archive", "event_type": "ARCHIVED"}],
        )

    def test_reads_timeline_from_versioned_executor_result(self) -> None:
        payload = {
            "contract_version": "v1",
            "timeline": [
                {"did": "did:contract:1", "event_type": "MATCH"},
                {"did": "did:contract:2", "event_type": "OTHER"},
                None,
                "not-an-entry",
            ],
        }

        entries = PacAuditEvidenceService.result_audit_entries(
            payload,
            "did:contract:1",
        )

        self.assertEqual(
            entries,
            [{"did": "did:contract:1", "event_type": "MATCH"}],
        )

    def test_rejects_legacy_or_malformed_executor_results(self) -> None:
        self.assertEqual(
            PacAuditEvidenceService.result_audit_entries(
                [{"audit_trail": [{"event_type": "LEGACY"}]}]
            ),
            [],
        )
        self.assertEqual(
            PacAuditEvidenceService.result_audit_entries({"timeline": "invalid"}),
            [],
        )


if __name__ == "__main__":
    unittest.main()
