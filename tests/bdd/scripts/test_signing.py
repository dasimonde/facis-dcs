from __future__ import annotations

import unittest
from unittest.mock import Mock, patch

from steps.support.signing import _prepare_with_regeneration_retry


class SigningPrepareRetryTest(unittest.TestCase):
    def test_retries_only_pdf_regeneration_in_flight(self) -> None:
        transient = Mock(
            status_code=500,
            text=(
                '{"message":"the contract\'s PDF is still being regenerated; '
                'retry signing shortly: did:contract:1"}'
            ),
        )
        success = Mock(status_code=200, text="ok")

        with (
            patch(
                "steps.support.signing.post_json",
                side_effect=[transient, success],
            ) as post,
            patch(
                "steps.support.signing.time.monotonic",
                side_effect=[100.0, 100.0],
            ),
            patch("steps.support.signing.time.sleep") as sleep,
            patch.dict(
                "os.environ",
                {
                    "BDD_SIGNATURE_PREPARE_RETRY_SECONDS": "45",
                    "BDD_SIGNATURE_PREPARE_RETRY_INTERVAL_SECONDS": "1",
                },
            ),
        ):
            response = _prepare_with_regeneration_retry(
                object(),
                "http://dcs/signature/prepare",
                {"did": "did:contract:1"},
                {"Authorization": "Bearer test"},
            )

        self.assertIs(response, success)
        self.assertEqual(post.call_count, 2)
        sleep.assert_called_once_with(1.0)

    def test_returns_other_prepare_errors_without_retrying(self) -> None:
        rejected = Mock(
            status_code=500,
            text='{"message":"semantic workflow gate rejected the snapshot"}',
        )

        with (
            patch("steps.support.signing.post_json", return_value=rejected) as post,
            patch("steps.support.signing.time.sleep") as sleep,
        ):
            response = _prepare_with_regeneration_retry(
                object(),
                "http://dcs/signature/prepare",
                {"did": "did:contract:1"},
                {},
            )

        self.assertIs(response, rejected)
        post.assert_called_once()
        sleep.assert_not_called()

    def test_returns_last_transient_response_when_retry_window_is_exhausted(self) -> None:
        transient = Mock(
            status_code=500,
            text="the contract's PDF is still being regenerated; retry signing shortly",
        )

        with (
            patch("steps.support.signing.post_json", return_value=transient) as post,
            patch(
                "steps.support.signing.time.monotonic",
                side_effect=[100.0, 100.0],
            ),
            patch("steps.support.signing.time.sleep") as sleep,
            patch.dict(
                "os.environ",
                {"BDD_SIGNATURE_PREPARE_RETRY_SECONDS": "0"},
            ),
        ):
            response = _prepare_with_regeneration_retry(
                object(),
                "http://dcs/signature/prepare",
                {"did": "did:contract:1"},
                {},
            )

        self.assertIs(response, transient)
        post.assert_called_once()
        sleep.assert_not_called()


if __name__ == "__main__":
    unittest.main()
