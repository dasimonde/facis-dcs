"""Step definitions for the non-tamper scenarios in
features/03_contract_creation/contract_format_review.feature: viewing a
contract in machine-/human-readable format, the synchronized (MR+HR) view,
version-tag consistency across exports, positive structural validation, and
the unauthorized-role denial. The three tamper-detection scenarios in that
feature are out of scope here (owned elsewhere).

Reuses steps/pdf_generation/pdf_service.py (PDFService) for PDF export/verify
— see steps/pdf_generation/pdf_steps.py for the already-passing PDF
export/verify scenarios this borrows conventions from.
"""

from behave import given, then, when

from steps.support.api_client import (
    contract_negotiate_url,
    contract_retrieve_by_id_url,
    contract_submit_url,
    get_with_headers,
    post_json,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService
from steps.support.services.pdf_service import PDFService
from steps.pdf_generation.pdf_steps import _utf16be
from steps.template_management.contract_workflow_steps import _negotiation_id_for_change


# ---------------------------------------------------------------------------
# Given helpers
# ---------------------------------------------------------------------------


# NOTE (registration order is load-bearing): the versioned step below MUST
# be registered BEFORE the generic 'contract "{name}" exists' step. behave's
# step registry raises AmbiguousStep at load time when a newly registered
# step's literal text is matched by an ALREADY-registered pattern — and the
# generic pattern's {name} placeholder matches the literal text
# 'contract "{name}" with version "{version}" exists' (parse captures
# '<name>" with version "<version>' as the name). Registering the more
# specific step first avoids both the load-time AmbiguousStep and any
# runtime shadowing (behave returns the first matching step in registration
# order, so versioned scenario text hits the versioned step).
@given('contract "{name}" with version "{version}" exists')
def step_given_contract_with_version(context, name, version):
    # contract_version is an integer bumped by the negotiation merge in
    # submit.go (Negotiation-state branch), not a semver "X.Y" string — reach
    # the requested integer by driving that many accepted negotiation
    # rounds. "2.0" -> 2 rounds -> contract_version 2 (starts at 1).
    target_version = int(float(version))
    rounds_needed = max(target_version - 1, 0)

    ContractService._create_contract_in_negotiation(context, name)
    creator_h = context.contract_seed_headers[name]
    responder_h = AuthService.get_headers_for_roles(["Contract Reviewer"], organization="TechVendor Inc")

    for i in range(rounds_needed):
        did, updated_at = ContractService._contract_data(context, name)
        # Structured change request: submit's merge pass unmarshals accepted
        # change requests into negotiationmerging.ChangeRequest and a bare
        # string 500s there; select the entry by content because the
        # negotiations list order is random (Go map iteration, see
        # contract_workflow_steps._negotiation_id_for_change).
        change = {"description": f"Round {i + 1}: version bump edit"}
        resp = post_json(
            context,
            contract_negotiate_url(context),
            {"did": did, "updated_at": updated_at,
             "negotiated_by": AuthService.username_for_roles(["Contract Manager"]),
             "change_request": change},
            headers=creator_h,
        )
        assert resp.status_code == 200, f"negotiate failed: {resp.status_code} {resp.text}"
        refreshed = ContractService._refresh_contract(context, name)
        negotiation_id = _negotiation_id_for_change(refreshed, change)

        resp = post_json(
            context,
            f"{context.base_url}/contract/respond",
            {"id": str(negotiation_id), "did": did, "action_flag": "ACCEPTING",
             "rejected_by": "", "rejection_reason": ""},
            headers=responder_h,
        )
        assert resp.status_code == 200, f"accept failed: {resp.status_code} {resp.text}"
        ContractService._refresh_contract(context, name)

        did, updated_at = ContractService._contract_data(context, name)
        submit_resp = post_json(
            context,
            contract_submit_url(context),
            ContractService._contract_submit_payload(context, did, updated_at),
            headers=creator_h,
        )
        assert submit_resp.status_code == 200, f"submit/merge failed: {submit_resp.status_code} {submit_resp.text}"
        ContractService._refresh_contract(context, name)

    did, _ = ContractService._contract_data(context, name)
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=creator_h)
    assert retrieve.status_code == 200, retrieve.text
    actual_version = retrieve.json().get("contract_version")
    assert actual_version == target_version, (
        f"Expected contract_version {target_version}, got {actual_version}"
    )
    context.expected_contract_version = target_version


@given('contract "{name}" exists')
def step_given_contract_exists(context, name):
    ContractService._create_contract_in_draft(context, name)


@given('contract "{name}" has machine-readable representation')
def step_given_contract_has_mr_representation(context, name):
    ContractService._create_contract_in_draft(context, name)


# ---------------------------------------------------------------------------
# When steps
# ---------------------------------------------------------------------------


@when('I view contract "{name}" in machine-readable format')
def step_when_view_mr_format(context, name):
    did, _ = ContractService._contract_data(context, name)
    context.requests_response = get_with_headers(context, contract_retrieve_by_id_url(context, did))


@when('I view contract "{name}" in human-readable format')
def step_when_view_hr_format(context, name):
    did, _ = ContractService._contract_data(context, name)
    context.requests_response = PDFService.export_contract_pdf(context, did)


@when('I request synchronized view of contract "{name}"')
def step_when_request_synchronized_view(context, name):
    did, _ = ContractService._contract_data(context, name)
    context.mr_response = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert context.mr_response.status_code == 200, context.mr_response.text
    context.hr_response = PDFService.export_contract_pdf(context, did)
    assert context.hr_response.status_code == 200, context.hr_response.text
    context.verify_response = PDFService.verify_contract_pdf(context, did)
    # requests_response drives the generic "get http 200" style assertions if
    # any scenario adds one later; the synchronized view is the MR retrieve.
    context.requests_response = context.mr_response


@when('I export contract "{name}" in both formats')
def step_when_export_both_formats(context, name):
    did, _ = ContractService._contract_data(context, name)
    context.mr_response = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert context.mr_response.status_code == 200, context.mr_response.text
    context.hr_response = PDFService.export_contract_pdf(context, did)
    assert context.hr_response.status_code == 200, context.hr_response.text
    context.verify_response = PDFService.verify_contract_pdf(context, did)
    context.requests_response = context.mr_response


@when("I validate the machine-readable structure")
def step_when_validate_mr_structure(context):
    name = "Service Agreement"
    did, _ = ContractService._contract_data(context, name)
    context.requests_response = get_with_headers(context, contract_retrieve_by_id_url(context, did))


@when('I attempt to access the synchronized view of contract "{name}"')
def step_when_attempt_synchronized_view(context, name):
    did, _ = ContractService._contract_data(context, name)
    export_resp = PDFService.export_contract_pdf(context, did)
    if export_resp.status_code != 200:
        context.requests_response = export_resp
        return
    context.requests_response = PDFService.verify_contract_pdf(context, did)


# ---------------------------------------------------------------------------
# Then assertions
# ---------------------------------------------------------------------------


@then("the JSON-LD or XML representation is displayed")
def step_then_jsonld_or_xml_displayed(context):
    assert context.requests_response.status_code == 200, context.requests_response.text
    contract_data = context.requests_response.json().get("contract_data")
    assert isinstance(contract_data, dict) and contract_data, (
        f"Expected a JSON-LD contract_data object, got: {contract_data}"
    )
    assert "@context" in contract_data, f"Expected '@context' in contract_data: {contract_data}"


@then("the structure is valid")
def step_then_mr_structure_valid(context):
    contract_data = context.requests_response.json().get("contract_data")
    assert isinstance(contract_data, dict)
    assert "@type" in contract_data, f"Expected '@type' in contract_data: {contract_data}"
    assert "dcs:documentStructure" in contract_data, (
        f"Expected 'dcs:documentStructure' in contract_data: {contract_data}"
    )


@then("the PDF or document view is displayed")
def step_then_pdf_or_doc_displayed(context):
    assert context.requests_response.status_code == 200, (
        f"Expected 200 from PDF export, got {context.requests_response.status_code}: "
        f"{context.requests_response.text}"
    )
    assert context.requests_response.content[:4] == b"%PDF", (
        "Response body does not start with PDF magic bytes (%PDF)"
    )


@then("the content is readable")
def step_then_pdf_content_readable(context):
    pdf_bytes = context.requests_response.content
    assert len(pdf_bytes) > 500, f"Exported PDF is suspiciously small ({len(pdf_bytes)} bytes)"
    assert b"contract.jsonld" in pdf_bytes or _utf16be(b"contract.jsonld") in pdf_bytes, (
        "Exported PDF does not appear to carry the contract's content attachment"
    )


@then("both machine-readable and human-readable views are rendered")
def step_then_both_views_rendered(context):
    assert context.mr_response.status_code == 200, context.mr_response.text
    assert context.hr_response.status_code == 200, context.hr_response.text
    assert context.hr_response.content[:4] == b"%PDF"
    assert context.mr_response.json().get("contract_data")


@then("both formats are derived from the same source")
def step_then_same_source(context):
    pdf_bytes = context.hr_response.content
    assert b"contract.jsonld" in pdf_bytes or _utf16be(b"contract.jsonld") in pdf_bytes, (
        "Exported PDF does not embed the machine-readable (contract.jsonld) source"
    )


@then("both formats have matching content hashes")
def step_then_matching_content_hashes(context):
    assert context.verify_response.status_code == 200, context.verify_response.text
    result = context.verify_response.json()
    assert result.get("match") is True, f"Expected MR/HR hash match, got: {result}"
    assert "jsonld_hash" in result and "base_pdf_hash" in result, (
        f"Expected jsonld_hash/base_pdf_hash in verify result: {result}"
    )


@then('the machine-readable export has version tag "{tag}"')
def step_then_mr_version_tag(context, tag):
    expected = getattr(context, "expected_contract_version", None)
    assert expected is not None, "No expected contract_version recorded by the Given step"
    actual = context.mr_response.json().get("contract_version")
    assert actual == expected, f"Expected machine-readable contract_version {expected}, got {actual}"


@then('the human-readable export has version tag "{tag}"')
def step_then_hr_version_tag(context, tag):
    # The PDF export itself carries no separate literal version string; its
    # "version tag" is proven via hash-for-hash equivalence with the current
    # (already version-checked) machine-readable source.
    assert context.verify_response.status_code == 200, context.verify_response.text
    assert context.verify_response.json().get("match") is True, (
        f"Expected the human-readable export to match the current version's source: "
        f"{context.verify_response.json()}"
    )


@then("both exports are consistent")
def step_then_exports_consistent(context):
    assert context.verify_response.status_code == 200, context.verify_response.text
    assert context.verify_response.json().get("match") is True


@then("the schema validation passes")
def step_then_schema_validation_passes(context):
    assert context.requests_response.status_code == 200, context.requests_response.text
    contract_data = context.requests_response.json().get("contract_data")
    assert isinstance(contract_data, dict) and contract_data


@then("required fields are present")
def step_then_required_fields_present(context):
    contract_data = context.requests_response.json().get("contract_data")
    for field in ("@context", "@type", "dcs:documentStructure"):
        assert field in contract_data, f"Expected required field '{field}' in contract_data: {contract_data}"


@then("data types are correct")
def step_then_data_types_correct(context):
    contract_data = context.requests_response.json().get("contract_data")
    assert isinstance(contract_data.get("@type"), str), (
        f"Expected '@type' to be a string, got: {contract_data.get('@type')!r}"
    )
    assert isinstance(contract_data.get("dcs:documentStructure"), dict), (
        f"Expected 'dcs:documentStructure' to be an object, got: {contract_data.get('dcs:documentStructure')!r}"
    )
