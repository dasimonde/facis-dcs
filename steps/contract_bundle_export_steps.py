"""BDD steps for the hierarchy-family completeness of the contract bundle
export — GET /contract/export/{did} (DCS-FR-CWE-30, DCS-FR-TR-24).

Beyond the requested contract and its parents/ chain, the bundle packages
every other locally-known, requester-readable member of the hierarchy family
under related/<did>/ with the same per-contract entry structure. This module
only adds the related/ assertions: the bundle request, parent-chain and
bundle-manifest.json steps are reused from
steps/pdf_generation/dcs_bundle_export_steps.py, the hierarchy fixtures from
steps/template_management/dcs_contract_hierarchy_steps.py.
"""

from behave import then

from steps.pdf_generation.dcs_bundle_export_steps import _open_bundle_zip
from steps.support.services.contract_service import ContractService


@then('the contract bundle ZIP for "{name}" contains family member "{member_name}" under related/')
def step_then_bundle_contains_related_member(context, name, member_name):
    member_did, _ = ContractService._contract_data(context, member_name)
    zf = _open_bundle_zip(context)
    names = zf.namelist()

    prefix = f"related/{member_did}/"
    for required in (f"{prefix}contract.jsonld", f"{prefix}contract.pdf"):
        assert required in names, (
            f"Expected the contract bundle ZIP for '{name}' to contain family member "
            f"'{member_name}' entry '{required}' (FR-CWE-30 family completeness), "
            f"got entries: {names}"
        )
    # Related members are packaged flat — their lineage is derivable from
    # contract.jsonld's dcs:parentContract, never from a nested parent chain.
    nested = [n for n in names if n.startswith(f"{prefix}parents/")]
    assert not nested, (
        f"Related family member '{member_name}' must be packaged flat, found a nested "
        f"parent chain: {nested}"
    )
