import unittest
from types import SimpleNamespace
from unittest.mock import patch

from steps.support.services.contract_service import ContractService


class ContractTransitionRetryTests(unittest.TestCase):
    def setUp(self):
        self.context = SimpleNamespace(base_url="http://dcs.test/api")

    @patch("steps.support.services.contract_service.time.sleep")
    @patch("steps.support.services.contract_service.post_json")
    @patch("steps.support.services.contract_service.get_with_headers")
    def test_retries_explicit_optimistic_lock_conflict(
        self, retrieve, post, sleep
    ):
        retrieve.side_effect = [
            SimpleNamespace(status_code=200, text="", json=lambda: {"updated_at": "v1"}),
            SimpleNamespace(status_code=200, text="", json=lambda: {"updated_at": "v2"}),
        ]
        post.side_effect = [
            SimpleNamespace(status_code=500, text="contract was updated elsewhere"),
            SimpleNamespace(status_code=200, text="ok"),
        ]

        response = ContractService.post_transition_with_current_version(
            self.context,
            "did:example:contract",
            "http://dcs.test/api/contract/terminate",
            lambda updated_at: {"updated_at": updated_at},
            {"Authorization": "Bearer token"},
        )

        self.assertEqual(response.status_code, 200)
        self.assertEqual(
            [call.args[2] for call in post.call_args_list],
            [{"updated_at": "v1"}, {"updated_at": "v2"}],
        )
        sleep.assert_called_once_with(0.5)

    @patch("steps.support.services.contract_service.time.sleep")
    @patch("steps.support.services.contract_service.post_json")
    @patch("steps.support.services.contract_service.get_with_headers")
    def test_does_not_retry_other_transition_failures(self, retrieve, post, sleep):
        retrieve.return_value = SimpleNamespace(
            status_code=200, text="", json=lambda: {"updated_at": "v1"}
        )
        post.return_value = SimpleNamespace(status_code=422, text="invalid transition")

        response = ContractService.post_transition_with_current_version(
            self.context,
            "did:example:contract",
            "http://dcs.test/api/contract/terminate",
            lambda updated_at: {"updated_at": updated_at},
            {},
        )

        self.assertEqual(response.status_code, 422)
        self.assertEqual(post.call_count, 1)
        sleep.assert_not_called()


if __name__ == "__main__":
    unittest.main()
