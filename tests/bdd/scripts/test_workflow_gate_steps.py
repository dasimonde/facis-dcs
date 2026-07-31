from __future__ import annotations

import unittest
from types import SimpleNamespace
from unittest.mock import Mock, patch

from steps.semantic_hub import workflow_gate_steps


class WorkflowGateDeploymentTest(unittest.TestCase):
    def test_prepares_signed_contract_before_designating_deployment_target(self) -> None:
        context = SimpleNamespace(
            ceremony_ids={},
            pid_presentations={},
        )
        events = []

        def ceremony(_context, name, _field_name, _signatory):
            context.ceremony_ids[name] = "ceremony-1"
            context.pid_presentations[name] = {"subject_did": "did:signer:1"}
            events.append("ceremony")

        def sign(*_args, **_kwargs):
            events.append("sign")
            return Mock(status_code=200, text="ok")

        def designate(*_args, **_kwargs):
            events.append("designate")

        with (
            patch.object(
                workflow_gate_steps.OrceAuditControlService,
                "reset",
            ),
            patch.object(
                workflow_gate_steps.OrceAuditControlService,
                "set_mode",
            ),
            patch.object(
                workflow_gate_steps.ContractService,
                "_create_contract_in_draft",
            ),
            patch.object(workflow_gate_steps, "_advance_to_approved"),
            patch.object(workflow_gate_steps, "_run_full_ceremony", side_effect=ceremony),
            patch.object(workflow_gate_steps, "_apply_signature", side_effect=sign),
            patch.object(
                workflow_gate_steps.ContractService,
                "_local_peer_did",
                return_value="did:dcs:local",
            ),
            patch.object(
                workflow_gate_steps.ContractService,
                "_refresh_contract",
            ),
            patch.object(
                workflow_gate_steps,
                "_ensure_target_designated",
                side_effect=designate,
            ),
            patch.object(workflow_gate_steps, "_state", return_value="SIGNED"),
        ):
            workflow_gate_steps._prepare(context, "Deployment", "deployment")

        self.assertEqual(events, ["ceremony", "sign", "designate"])
        self.assertEqual(context.workflow_pre_states["Deployment"], "SIGNED")

    def test_requests_deployment_gate_through_explicit_deploy_endpoint(self) -> None:
        response = Mock(
            status_code=409,
            text="review",
            json=Mock(return_value={"gate_run_id": "run-1"}),
            headers={},
        )
        context = SimpleNamespace(base_url="http://dcs/api")

        with (
            patch.object(
                workflow_gate_steps.ContractService,
                "_contract_data",
                return_value=("did:contract:1", "2026-07-31T00:00:00Z"),
            ),
            patch.object(
                workflow_gate_steps.AuthService,
                "get_headers_for_roles",
                return_value={"Authorization": "Bearer manager"},
            ),
            patch.object(
                workflow_gate_steps,
                "post_json",
                return_value=response,
            ) as post,
            patch.object(workflow_gate_steps, "_apply_signature") as sign,
        ):
            actual = workflow_gate_steps._request_gate(
                context,
                "deployment",
                "Deployment",
            )

        self.assertIs(actual, response)
        sign.assert_not_called()
        post.assert_called_once_with(
            context,
            "http://dcs/api/contract/deploy",
            {
                "did": "did:contract:1",
                "updated_at": "2026-07-31T00:00:00Z",
            },
            headers={"Authorization": "Bearer manager"},
        )
        self.assertEqual(context.last_workflow_gate_run_id, "run-1")


if __name__ == "__main__":
    unittest.main()
