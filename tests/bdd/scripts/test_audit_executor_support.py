from types import SimpleNamespace
from unittest import TestCase
from unittest.mock import Mock, patch

from steps.support.services.orce_audit_control_service import OrceAuditControlService


class AuditExecutorSupportTests(TestCase):
    @patch.object(OrceAuditControlService, "observations")
    def test_evidence_groups_reads_the_latest_executor_request(self, observations):
        observations.return_value = [
            {"request": {"evidence": {"contracts": [{"did": "old"}]}}},
            {"request": {"evidence": {"contracts": [{"did": "current"}, "ignored"]}}},
        ]

        groups = OrceAuditControlService.evidence_groups(object(), "contracts")

        self.assertEqual(groups, [{"did": "current"}])

    @patch.object(OrceAuditControlService, "observations", return_value=[])
    def test_evidence_groups_requires_an_observed_request(self, _observations):
        with self.assertRaisesRegex(AssertionError, "Expected an ORCE audit observation"):
            OrceAuditControlService.evidence_groups(object(), "contracts")

    @patch("steps.support.services.orce_audit_control_service.time.sleep")
    @patch("steps.support.services.orce_audit_control_service.requests.post")
    def test_direct_executor_post_retries_transient_route_404(self, post, _sleep):
        post.side_effect = [Mock(status_code=404), Mock(status_code=200)]
        context = SimpleNamespace(base_url="http://dcs.test", http_timeout_seconds=1)

        response = OrceAuditControlService.post_executor(context, {"contract_version": "v1"})

        self.assertEqual(response.status_code, 200)
        self.assertEqual(post.call_count, 2)


if __name__ == "__main__":
    import unittest

    unittest.main()
