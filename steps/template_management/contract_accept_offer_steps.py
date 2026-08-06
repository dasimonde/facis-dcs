"""Steps for accepting an inbound offer as-is (POST /contract/accept-offer).

Accepting is what mints the accepting instance's negotiation task for the
offer's round; receiving the offer queues nothing. The authority to accept
comes from being the designated counterparty, so the origin — which already
holds a task from authoring the contract — is refused.
"""

from behave import when

from steps.support.api_client import contract_accept_offer_url, post_json
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService


@when('the negotiator accepts the offer for contract "{name}"')
def step_when_accept_offer(context, name):
    seed = getattr(context, "contract_seed_headers", None) or {}
    headers = seed.get(name) or getattr(context, "headers", None)
    ContractService._refresh_contract(context, name)
    did, updated_at = ContractService._contract_data(context, name)
    context.requests_response = post_json(
        context,
        contract_accept_offer_url(context),
        {
            "did": did,
            "updated_at": updated_at,
            "accepted_by": AuthService.username_for_roles(["Contract Creator"]),
        },
        headers=headers,
    )
    if context.requests_response.status_code == 200:
        ContractService._refresh_contract(context, name)
