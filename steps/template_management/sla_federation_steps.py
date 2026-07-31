"""BDD steps for the SLA contract federation demonstrator
(features/27_sla_contract_federation).

One authored artefact — features/fixtures/sla_hosting_template.jsonld, a
Managed Kubernetes Hosting SLA between a Provider on instance A and a Customer
on instance B — travels the whole federation path: authored and registered on
A, published to the Federated Catalogue, found and imported by B, approved and
registered under B's OWN workflow, instantiated into a contract, negotiated
back and forth between the two instances, and signed on both.

What the pack measures is that A's AUTHORED ENFORCEMENT SEMANTICS survive that
journey. The ODRL the template declares — its rules, its nested logical
constraint tree, its duty consequence, and its negotiated boundaries (right
operands that reference a contract field rather than a literal) — must arrive
on B byte-equivalent in structure, and must then bind instance B, which never
saw instance A's code, exactly as it binds A. The load-bearing case is the
refusal: a counter-offer of a committed availability below the template's own
`gteq 99.5` floor is refused at the approval gate on BOTH instances.

Fixture coupling
----------------
The fixture is loaded by path and every structural number the scenarios assert
(rule counts per bucket, constraint-tree depth, consequence count, field-bound
right operands, field count) is COMPUTED from it rather than hardcoded here —
the feature file states the expected numbers, and this module only counts. A
fixture change therefore surfaces as a feature-file diff, not as a silent pass.

The only fixture knowledge baked in is _FIELD_FILL_DEFAULTS: a plausible
negotiated value per field, keyed by the field @id's FRAGMENT (the part after
'#'), which survives the rebase contract creation applies to every template
node IRI (validation.rebaseIDText: `<templateID>#field-x` -> `<contractID>
#field-x`). A value the field's own dcs:valueConstraint does not admit is
replaced by one it does, an unknown fragment falls back to a datatype default,
and every fill is checked against both that declared constraint and the
literal ODRL constraints that bind it, so a fixture that grows a field this
table does not know still fails loudly and specifically rather than
mysteriously at the submission gate.

Domain library
--------------
The template declares TWO shapes graphs: the canonical DCS envelope, which
every instance seeds at startup, and `facis-sla-hosting`, the hosting domain —
the asset classes in a kind="shapes" entry and the flat commercial fields in a
kind="ontology" one. That library is published at RUNTIME, never seeded
(features/fixtures/facis-sla-hosting-{shapes,ontology}.ttl), because importing
a vocabulary is exactly the seam a second domain enters DCS through. Nothing
resolves a declared shapes graph until a document is submitted, so an instance
that has not installed it authors and imports the template happily and then
refuses the contract drawn from it — which is why _install_sla_domain_library
runs on every instance this pack touches, before anything is authored on it.

Identity
--------
Most steps authenticate with the suite-shared role tokens. The negotiating
participant on each instance does NOT: /contract/submit's DRAFT/OFFERED branch
requires the submitter to be the contract's own creator (submit.go:118), and
HasOpenNegotiationDecisions excludes only a decision the CALLER itself
authored — so the create, the offer, the counter-offer and the submit that
closes the round have to come from one and the same participant. A BDD identity
is exactly (roles, organization), so each instance gets its own dedicated
organization for that participant, keeping this pack's negotiation rounds out
of the suite-shared role tokens whose state other scenarios depend on. This is
the same reasoning (and the same shape) as
steps/peer_trust/dcs_peer_trust_steps.py's _B_COUNTERPARTY_ORG.

That dedicated organization is also why the create names its parties. Read
authorization matches the caller's organization against the creating one and
against the organizations the document declares under dcs:parties — the create
request's `parties` list (query/contract/querybyid.go CallerMayReadContract).
Instance B is the ORIGINATOR here, so its copy gets no relief from the
received-copy rule that lets instance A's reviewer and approver read theirs,
and its readers are two organizations rather than one: the negotiating
participant above, and the suite-shared one the reviewer, the approver, the
manager and the peer-trust signing steps this pack reuses all present. Naming
both is what a real creator does for every organization entitled to see the
contract.
"""

import json
import time
from contextlib import contextmanager
from datetime import date, timedelta
from pathlib import Path
from urllib.parse import urlparse, parse_qs

import requests as _requests
from behave import given, step, then

from steps.support.api_client import (
    catalogue_template_retrieve_by_id_url,
    catalogue_template_search_url,
    contract_create_url,
    contract_retrieve_by_id_url,
    contract_update_url,
    get_with_headers,
    hub_shapes_anchors,
    post_json,
    put_json,
    template_approve_url,
    template_create_url,
    template_publish_url,
    template_register_url,
    template_retrieve_by_id_url,
    template_submit_url,
    template_verify_url,
)
from steps.support.services.auth_service import AuthService


@contextmanager
def _as_instance(context, base_url):
    """The peer-trust pack's instance switch (steps/peer_trust/
    dcs_peer_trust_steps.py), imported lazily: that module pulls in the
    deployment, signing and state-machine step packs, several of which live
    under steps.template_management — importing it while this package's own
    __init__ is still running would close a cycle."""
    from steps.peer_trust.dcs_peer_trust_steps import _as_instance as _peer_as_instance  # noqa: PLC0415

    with _peer_as_instance(context, base_url):
        yield


# ---------------------------------------------------------------------------
# Fixture
# ---------------------------------------------------------------------------

# steps/template_management/<this file>.py -> repository root
_REPO_ROOT = Path(__file__).resolve().parents[2]
_FIXTURE_PATH = _REPO_ROOT / "features" / "fixtures" / "sla_hosting_template.jsonld"

# The hosting domain the template declares in sh:shapesGraph, published to the
# hub at runtime (see the module docstring). The version is pinned rather than
# assigned: the document names ?version=1, and ADR-8 has that number resolve
# the same graph on every hub that holds it.
_SLA_LIBRARY_NAME = "facis-sla-hosting"
_SLA_LIBRARY_VERSION = 1
_SLA_LIBRARY_ENTRIES = (
    ("shapes", _REPO_ROOT / "features" / "fixtures" / "facis-sla-hosting-shapes.ttl"),
    ("ontology", _REPO_ROOT / "features" / "fixtures" / "facis-sla-hosting-ontology.ttl"),
)


def _fixture() -> dict:
    assert _FIXTURE_PATH.is_file(), (
        f"the SLA hosting template fixture is missing at {_FIXTURE_PATH} — this pack federates "
        "one authored artefact and has nothing to federate without it"
    )
    with _FIXTURE_PATH.open(encoding="utf-8") as handle:
        document = json.load(handle)
    # Tolerate either a bare template document or one wrapped in the
    # /template/create payload envelope, so the fixture can gain metadata
    # without breaking every scenario here.
    if isinstance(document, dict) and isinstance(document.get("template_data"), dict):
        return document["template_data"]
    return document


# ---------------------------------------------------------------------------
# JSON-LD / ODRL structure helpers
#
# Everything below reads the COMPACT form the API stores and returns. Keys are
# matched by local name so a document that names ODRL terms by full IRI (an
# expanded producer) is read the same way as one using the `odrl:` prefix.
# ---------------------------------------------------------------------------

_LOGICAL_OPERANDS = ("and", "or", "xone", "andSequence")


def _local(name: str) -> str:
    """The local name of a compact term or a full IRI (`odrl:and`, `and`,
    `http://www.w3.org/ns/odrl/2/and` all yield `and`)."""
    for separator in ("#", "/", ":"):
        if separator in name:
            name = name.rsplit(separator, 1)[-1]
    return name


def _get(node: dict, term: str):
    if not isinstance(node, dict):
        return None
    for key, value in node.items():
        if _local(key) == term:
            return value
    return None


def _as_list(value) -> list:
    if value is None:
        return []
    if isinstance(value, dict) and "@list" in value:
        inner = value["@list"]
        return inner if isinstance(inner, list) else [inner]
    if isinstance(value, list):
        return value
    return [value]


def _reference_id(value):
    """The @id of a JSON-LD reference node; a value node or a literal is not a
    reference."""
    if isinstance(value, dict) and "@id" in value and "@value" not in value:
        return value["@id"]
    return None


def _policies(document: dict) -> dict:
    policies = _get(document, "policies")
    assert isinstance(policies, dict), (
        f"expected ONE enclosing ODRL policy node on the document, got: {policies!r}"
    )
    return policies


def _bucket_rules(policies: dict, bucket: str) -> list:
    return [rule for rule in _as_list(_get(policies, bucket)) if isinstance(rule, dict)]


def _logical_children(constraint: dict):
    """The child constraints of a logical constraint (odrl:and/or/xone/
    andSequence), or None when the node is an atomic leaf."""
    for operand in _LOGICAL_OPERANDS:
        children = _get(constraint, operand)
        if children is not None:
            return [child for child in _as_list(children) if isinstance(child, dict)]
    return None


def _constraint_nesting(constraint: dict) -> int:
    children = _logical_children(constraint)
    if not children:
        return 1
    return 1 + max(_constraint_nesting(child) for child in children)


def _constraint_leaves(constraint: dict) -> list:
    children = _logical_children(constraint)
    if not children:
        return [constraint]
    leaves = []
    for child in children:
        leaves.extend(_constraint_leaves(child))
    return leaves


def _constraint_bearing_nodes(node: dict) -> list:
    """A rule and, recursively, the duties hanging off it and the consequences
    hanging off those (ODRL IM §2.5) — every node that may carry constraints."""
    nodes = [node]
    for nested_term in ("duty", "consequence", "obligation", "remedy"):
        for nested in _as_list(_get(node, nested_term)):
            if isinstance(nested, dict):
                nodes.extend(_constraint_bearing_nodes(nested))
    return nodes


def _all_rules(policies: dict) -> list:
    rules = []
    for bucket in ("permission", "prohibition", "obligation"):
        for rule in _bucket_rules(policies, bucket):
            rules.extend(_constraint_bearing_nodes(rule))
    return rules


def _odrl_metrics(document: dict) -> dict:
    """The structural facts the federation must preserve. `depth` counts the
    rule as level 1 and each further level of logical-constraint nesting as one
    more, so a rule holding an odrl:or of atomic constraints is depth 3."""
    policies = _policies(document)
    field_ids = {field["@id"] for field in _contract_fields(document) if field.get("@id")}
    metrics = {
        "permission": len(_bucket_rules(policies, "permission")),
        "prohibition": len(_bucket_rules(policies, "prohibition")),
        "obligation": len(_bucket_rules(policies, "obligation")),
        "depth": 0,
        "consequences": 0,
        "field_bound_right_operands": 0,
        "actions": [],
    }
    for node in _all_rules(policies):
        action = _reference_id(_get(node, "action"))
        if action:
            metrics["actions"].append(action)
        metrics["consequences"] += len(
            [c for c in _as_list(_get(node, "consequence")) if isinstance(c, dict)]
        )
        for constraint in _as_list(_get(node, "constraint")):
            if not isinstance(constraint, dict):
                continue
            metrics["depth"] = max(metrics["depth"], 1 + _constraint_nesting(constraint))
            for leaf in _constraint_leaves(constraint):
                right = _get(leaf, "rightOperand")
                for candidate in _as_list(right):
                    if _reference_id(candidate) in field_ids:
                        metrics["field_bound_right_operands"] += 1
    metrics["actions"] = sorted(metrics["actions"])
    return metrics


def _rules_constraining_field(document: dict, field_id: str) -> list:
    """The @ids of every rule/duty/consequence node whose constraint subtree
    names `field_id` — the identities the ODRL audit puts in its finding
    messages (`ODRL policy "<id>" violated: …`)."""
    matching = []
    for node in _all_rules(_policies(document)):
        for constraint in _as_list(_get(node, "constraint")):
            if not isinstance(constraint, dict):
                continue
            for leaf in _constraint_leaves(constraint):
                operands = [_reference_id(_get(leaf, "leftOperand"))]
                operands.extend(_reference_id(v) for v in _as_list(_get(leaf, "rightOperand")))
                if field_id in operands and node.get("@id"):
                    matching.append(node["@id"])
    return sorted(set(matching))


# ---------------------------------------------------------------------------
# Contract fields
# ---------------------------------------------------------------------------

# One plausible negotiated value per field, keyed by the field @id's fragment
# (see the module docstring on why the fragment, not the whole IRI). Values are
# what the two parties would actually agree; the constraint check below proves
# they clear the template's own floors rather than assuming it.
_FIELD_FILL_DEFAULTS = {
    "field-governing-law": "DE",
    "field-provisioned-nodes": "8",
    "field-node-class": "GENERAL_PURPOSE",
    "field-service-region": "EMEA",
    "field-contract-end-date": (date.today() + timedelta(days=365)).isoformat(),
    "field-service-credit-rate": "5.0",
    "field-committed-availability": "99.9",
    "field-measurement-window": "P1M",
    "field-monthly-fee": "12500.00",
    "field-currency": "EUR",
    "field-billing-period": "P1M",
    "field-incident-response-minutes": "30",
    "field-required-signature-level": "AES",
}

_DATATYPE_FALLBACKS = {
    "integer": "1",
    "int": "1",
    "long": "1",
    "decimal": "1.0",
    "double": "1.0",
    "float": "1.0",
    "boolean": "true",
    "date": (date.today() + timedelta(days=365)).isoformat(),
    "dateTime": (date.today() + timedelta(days=365)).isoformat() + "T00:00:00Z",
    "duration": "P1M",
    "anyURI": "https://example.org/bdd-sla",
}


def _contract_fields(document: dict) -> list:
    return [field for field in _as_list(_get(document, "contractFields")) if isinstance(field, dict)]


def _fragment(field_id: str) -> str:
    return field_id.rsplit("#", 1)[-1] if "#" in field_id else field_id


def _field_by_fragment(document: dict, fragment_substring: str) -> dict:
    """The single contract field whose @id fragment contains
    `fragment_substring` (matched case-insensitively, label as a fallback) —
    how a scenario names 'the committed availability' without hardcoding the
    contract-rebased IRI it ends up with."""
    needle = fragment_substring.lower()
    fields = _contract_fields(document)
    matching = [f for f in fields if needle in _fragment(str(f.get("@id", ""))).lower()]
    if not matching:
        matching = [f for f in fields if needle in str(f.get("dcs:label", "")).lower()]
    assert len(matching) == 1, (
        f"expected exactly one contract field matching {fragment_substring!r}, got "
        f"{[f.get('@id') for f in matching]} out of {[f.get('@id') for f in fields]}"
    )
    return matching[0]


def _literal_value(raw):
    """The lexical value of a field value or ODRL right operand, whether it is
    a bare literal or a JSON-LD value node."""
    if isinstance(raw, dict):
        if "@value" in raw:
            return raw["@value"]
        return None
    return raw


def _as_number(raw):
    try:
        return float(str(_literal_value(raw)))
    except (TypeError, ValueError):
        return None


def _declared_constraint(field: dict) -> dict:
    """The field's own dcs:valueConstraint — the enum, bounds and pattern the
    hub vocabulary it was picked from declares, carried on the field itself."""
    constraint = field.get("dcs:valueConstraint")
    return constraint if isinstance(constraint, dict) else {}


def _allowed_values(field: dict) -> list:
    return [str(value) for value in (_declared_constraint(field).get("allowedValues") or []) if str(value)]


def _fill_value(field: dict) -> str:
    """The value to negotiate into a field: the table's, when the field's own
    declared constraint admits it, and otherwise one the constraint does admit.

    Deriving rather than trusting the table is what keeps a fixture whose
    vocabulary tightens — an enum gaining a value, a bound moving — from
    failing at the submission gate under a SHACL message about a shapes graph
    instead of here."""
    fragment = _fragment(str(field.get("@id", "")))
    preferred = _FIELD_FILL_DEFAULTS.get(fragment)
    allowed = _allowed_values(field)
    if allowed:
        return preferred if preferred in allowed else allowed[0]
    if preferred is not None:
        return _within_declared_bounds(field, preferred)
    datatype = _local(str(field.get("dcs:datatype") or "xsd:string"))
    fallback = _DATATYPE_FALLBACKS.get(datatype)
    if fallback is None:
        return f"BDD SLA {fragment}"
    return _within_declared_bounds(field, fallback)


def _within_declared_bounds(field: dict, value: str) -> str:
    """`value` clamped into the field's declared numeric bounds; returned
    verbatim when the field declares none, the value is not a number, or it
    already lies inside them."""
    constraint = _declared_constraint(field)
    number = _as_number(value)
    if number is None:
        return value
    clamped = number
    minimum, maximum = _as_number(constraint.get("min")), _as_number(constraint.get("max"))
    if minimum is not None and clamped < minimum:
        clamped = minimum
    if maximum is not None and clamped > maximum:
        clamped = maximum
    if clamped == number:
        return value
    if _local(str(field.get("dcs:datatype") or "")) in ("integer", "int", "long"):
        return str(int(clamped))
    return str(clamped)


def _fill_open_fields(document: dict) -> int:
    """Give every contract field without a value its negotiated value, as a
    typed literal carrying the field's own declared datatype."""
    filled = 0
    for field in _contract_fields(document):
        if _literal_value(field.get("dcs:value")) not in (None, ""):
            continue
        field["dcs:value"] = {
            "@value": _fill_value(field),
            "@type": field.get("dcs:datatype", "xsd:string"),
        }
        filled += 1
    return filled


def _assert_declared_constraints_satisfied(document: dict):
    """Check every filled field against its OWN dcs:valueConstraint — the enum
    and the bounds the hub vocabulary declares, which the same shapes graph
    enforces at the submission gate (ADR-23).

    Diagnosis, like the ODRL check below: a value the vocabulary does not admit
    fails here naming the field and the enum, rather than arriving as a SHACL
    report about a shapes graph three steps later."""
    for field in _contract_fields(document):
        value = _literal_value(field.get("dcs:value"))
        if value in (None, ""):
            continue
        identity = field.get("@id")
        allowed = _allowed_values(field)
        assert not allowed or str(value) in allowed, (
            f"the value filled into contract field {identity!r} ({value!r}) is not one the field's "
            f"own declared vocabulary admits ({allowed}) — the SLA fill table in "
            "steps/template_management/sla_federation_steps.py needs one that is"
        )
        constraint = _declared_constraint(field)
        number = _as_number(value)
        minimum, maximum = _as_number(constraint.get("min")), _as_number(constraint.get("max"))
        if number is None:
            continue
        assert minimum is None or number >= minimum, (
            f"the value filled into contract field {identity!r} ({number}) is below the minimum "
            f"{minimum} the field itself declares"
        )
        assert maximum is None or number <= maximum, (
            f"the value filled into contract field {identity!r} ({number}) is above the maximum "
            f"{maximum} the field itself declares"
        )


def _assert_literal_constraints_satisfied(document: dict):
    """Check every atomic constraint that binds a filled contract field
    against a LITERAL right operand — the same comparisons the approval gate
    runs (ValidateContractPolicySatisfaction). Constraints whose left operand
    is an ODRL context operand (odrl:percentage, odrl:spatial, …) are deferred
    to use-time by the engine and are not checked here.

    Its job is diagnosis: if the fixture grows a floor this module's fill
    table does not clear, the scenario fails HERE, naming the field and the
    floor, instead of at a later approval that looks like a federation bug.
    """
    values = {
        field["@id"]: _literal_value(field.get("dcs:value"))
        for field in _contract_fields(document)
        if field.get("@id")
    }
    for node in _all_rules(_policies(document)):
        for constraint in _as_list(_get(node, "constraint")):
            if not isinstance(constraint, dict):
                continue
            for leaf in _constraint_leaves(constraint):
                left = _reference_id(_get(leaf, "leftOperand"))
                if left not in values:
                    continue
                right_raw = _get(leaf, "rightOperand")
                if _reference_id(right_raw) is not None or isinstance(right_raw, list):
                    continue  # a negotiated boundary or a set operand, not a literal floor
                operator = _local(str(_reference_id(_get(leaf, "operator")) or ""))
                actual, expected = _as_number(values[left]), _as_number(right_raw)
                if actual is None or expected is None:
                    continue
                comparisons = {
                    "gteq": actual >= expected,
                    "gt": actual > expected,
                    "lteq": actual <= expected,
                    "lt": actual < expected,
                    "eq": actual == expected,
                    "neq": actual != expected,
                }
                if operator in comparisons and not comparisons[operator]:
                    raise AssertionError(
                        f"the value filled into contract field {left!r} ({actual}) does not satisfy "
                        f"the template's own {operator} {expected} constraint on rule "
                        f"{node.get('@id')!r} — the SLA fill table in "
                        "steps/template_management/sla_federation_steps.py needs a value for "
                        f"{_fragment(left)!r} that clears it"
                    )


# ---------------------------------------------------------------------------
# Instance-scoped access
# ---------------------------------------------------------------------------

# The participant that creates, offers, counter-offers and settles on each
# instance. One identity per instance (see the module docstring).
_PARTY_ROLES = ["Contract Creator", "Contract Manager", "Contract Negotiator"]
_PARTY_ORG = {
    "A": "BDD SLA Federation Provider",
    "B": "BDD SLA Federation Customer",
}

# The organizations named as parties when the contract is created — the
# organizations entitled to read it. On a real instance this is one company:
# the creator, the reviewer and the approver are colleagues, and one
# organization claim covers them all. A BDD identity is (roles, organization),
# so here they are two — the negotiating participant's dedicated one and the
# suite-shared one every other role token presents — and both are declared,
# because a company that reads the contract is a company the creator declares.
_READING_ORGANIZATIONS = [_PARTY_ORG["B"], AuthService.DEFAULT_ORGANIZATION]


def _base(context, label: str) -> str:
    label = label.strip().upper()
    assert label in ("A", "B"), f"unknown instance label {label!r} — use A or B"
    base_url = context.base_url_a if label == "A" else context.base_url_b
    assert base_url, (
        "instance A and B base URLs are not set — this scenario needs the Given step "
        '"instance A and instance B are both running and trust each other" first'
    )
    return base_url


def _headers(context, label: str, roles, organization=None) -> dict:
    return AuthService.get_headers_for_roles(
        roles, api_base=_base(context, label), organization=organization
    )


def _party_headers(context, label: str) -> dict:
    return _headers(context, label, _PARTY_ROLES, organization=_PARTY_ORG[label.strip().upper()])


def _sla(context) -> dict:
    if not hasattr(context, "sla") or context.sla is None:
        context.sla = {}
    return context.sla


def _template_did(context, label: str) -> str:
    key = "template_did_a" if label.strip().upper() == "A" else "template_did_b"
    did = _sla(context).get(key)
    assert did, f"no SLA template recorded for instance {label} yet"
    return did


def _template_body(context, label: str, did: str = None) -> dict:
    label = label.strip().upper()
    with _as_instance(context, _base(context, label)):
        headers = _headers(context, label, ["Template Manager"])
        resp = get_with_headers(
            context, template_retrieve_by_id_url(context, did or _template_did(context, label)),
            headers=headers,
        )
    assert resp.status_code == 200, f"could not read the SLA template on instance {label}: {resp.text}"
    return resp.json()


def _template_data(context, label: str) -> dict:
    data = _template_body(context, label).get("template_data")
    assert isinstance(data, dict), f"instance {label}'s SLA template carries no template_data"
    return data


def _contract_did(context) -> str:
    did = _sla(context).get("contract_did")
    assert did, "no SLA contract has been instantiated yet"
    return did


def _contract_body(context, label: str) -> dict:
    label = label.strip().upper()
    with _as_instance(context, _base(context, label)):
        headers = _headers(context, label, ["Contract Manager"])
        resp = get_with_headers(
            context, contract_retrieve_by_id_url(context, _contract_did(context)), headers=headers
        )
    assert resp.status_code == 200, (
        f"could not read the SLA contract on instance {label}: {resp.status_code} {resp.text}"
    )
    return resp.json()


def _contract_data(context, label: str) -> dict:
    data = _contract_body(context, label).get("contract_data")
    assert isinstance(data, dict), f"the SLA contract on instance {label} carries no contract_data"
    return data


def _contract_state(context, label: str) -> str:
    return str(_contract_body(context, label).get("state", "")).upper()


def _post_with_fresh_updated_at(context, label: str, path: str, payload: dict, headers: dict, what: str):
    """POST a mutating contract command to `label`, re-reading updated_at
    immediately beforehand. Both copies move without their own instance
    acting — the peer's ships land asynchronously — so a value read a step
    earlier can already be behind the lost-update guard; a guard refusal is
    answered by re-reading rather than by failing on a race under nobody's
    test. The final response is returned unasserted: refusal is the expected
    outcome for some callers."""
    base_url = _base(context, label)
    did = _contract_did(context)
    last = None
    for _ in range(4):
        with _as_instance(context, base_url):
            retrieve = get_with_headers(
                context, contract_retrieve_by_id_url(context, did), headers=headers
            )
            assert retrieve.status_code == 200, (
                f"could not read {did} on instance {label}: {retrieve.text}"
            )
            body = dict(payload, did=did, updated_at=retrieve.json().get("updated_at"))
            last = post_json(context, f"{base_url}{path}", body, headers=headers)
        if last.status_code == 200 or "updated elsewhere" not in last.text.lower():
            return last
        time.sleep(2)
    return last


def _shapes_anchor_identity(document: dict) -> list:
    """The (name, version) pairs of every hub shapes anchor a document pins.
    semantichub.AnchorURL emits /semantic/shapes/{name}?version=N, so a pin's
    identity is that pair — not the absolute URL, whose host is naturally the
    instance that produced the document and therefore differs between the two
    copies by design (ADR-8: a document stays anchored to the shapes its
    AUTHOR pinned, it is never re-anchored to the importer's own hub)."""
    anchors = hub_shapes_anchors(document)
    assert anchors, f"the document carries no hub sh:shapesGraph anchor: {document.get('sh:shapesGraph')!r}"
    return sorted(_anchor_identity(anchor) for anchor in anchors)


def _anchor_identity(anchor) -> tuple:
    """One hub anchor's (name, version) — see _shapes_anchor_identity."""
    assert isinstance(anchor, str) and anchor, f"not a hub anchor: {anchor!r}"
    parsed = urlparse(anchor)
    name = parsed.path.rstrip("/").rsplit("/", 1)[-1]
    version = (parse_qs(parsed.query).get("version") or [None])[0]
    return name, version


def _install_sla_domain_library(context, base_url: str):
    """Publish the hosting domain to `base_url`'s Semantic Hub: the shapes
    graph the template declares in sh:shapesGraph and the ontology its flat
    commercial fields come from, both under _SLA_LIBRARY_NAME at the version
    the document pins.

    Idempotent, and it has to be: hub versions are immutable rows in a
    run-durable database, so the second scenario of the pack — and every later
    CI run against the same instance — finds version 1 already there and
    installs nothing.
    """
    headers = AuthService.get_headers_for_roles(
        ["Template Manager"], api_base=base_url, timeout=context.http_timeout_seconds
    )
    for kind, path in _SLA_LIBRARY_ENTRIES:
        assert path.is_file(), (
            f"the SLA hosting {kind} fixture is missing at {path} — the template declares "
            f"{_SLA_LIBRARY_NAME} in sh:shapesGraph and no contract drawn from it can be "
            "submitted until that library is published"
        )
        installed = _requests.get(
            f"{base_url}/semantic/schema/retrieve",
            params={"name": _SLA_LIBRARY_NAME, "kind": kind, "version": _SLA_LIBRARY_VERSION},
            timeout=context.http_timeout_seconds,
        )
        if installed.status_code == 200:
            continue
        resp = post_json(
            context,
            f"{base_url}/semantic/schema/register",
            {
                "name": _SLA_LIBRARY_NAME,
                "kind": kind,
                "media_type": "text/turtle",
                "content": path.read_text(encoding="utf-8"),
                "version": _SLA_LIBRARY_VERSION,
                "activate": True,
            },
            headers=headers,
        )
        # A concurrent install of the same immutable version is the outcome
        # this wanted anyway; anything else is a real failure.
        assert resp.status_code == 200 or "already registered" in resp.text.lower(), (
            f"publishing the SLA hosting {kind} library to {base_url} failed: "
            f"{resp.status_code} {resp.text}"
        )


# ---------------------------------------------------------------------------
# Given / When — authoring and publishing on instance A
# ---------------------------------------------------------------------------


def _drive_template_to_registered(context, label: str, did: str):
    """Creator submit -> reviewer verify -> reviewer forward -> approver
    approve -> manager register, all on one instance."""
    label = label.strip().upper()
    base_url = _base(context, label)

    def _post(url_builder, extra, headers, what):
        """Re-read updated_at immediately before each call and retry the
        lost-update guard. The genesis PDF render create triggers is
        asynchronous and bumps updated_at shortly afterwards, so a value read
        even one call earlier can already be stale (same race
        TemplateService.do_submit retries)."""
        last = None
        for _ in range(6):
            with _as_instance(context, base_url):
                current = get_with_headers(
                    context, template_retrieve_by_id_url(context, did), headers=headers
                )
                assert current.status_code == 200, (
                    f"could not read the SLA template on instance {label}: {current.text}"
                )
                payload = dict(extra, did=did, updated_at=current.json().get("updated_at"))
                last = post_json(context, url_builder(context), payload, headers=headers)
            if last.status_code == 200 or "updated elsewhere" not in last.text.lower():
                break
            time.sleep(1)
        assert last.status_code == 200, (
            f"{what} failed for the SLA template on instance {label}: {last.status_code} {last.text}"
        )
        return last

    creator_h = _headers(context, label, ["Template Creator"])
    reviewer_h = _headers(context, label, ["Template Reviewer"])
    approver_h = _headers(context, label, ["Template Approver"])
    manager_h = _headers(context, label, ["Template Manager"])

    _post(
        template_submit_url,
        {
            "reviewers": [AuthService.username_for_roles(["Template Reviewer"])],
            "approver": AuthService.username_for_roles(["Template Approver"]),
        },
        creator_h,
        "creator submit",
    )
    _post(template_verify_url, {}, reviewer_h, "verify")
    _post(
        template_submit_url,
        {
            "approver": AuthService.username_for_roles(["Template Approver"]),
            "forward_to": "approval",
        },
        reviewer_h,
        "reviewer forward-to-approval",
    )
    _post(template_approve_url, {}, approver_h, "approve")
    _post(template_register_url, {}, manager_h, "register")


@given("the SLA hosting template is authored and registered on instance A")
def step_given_sla_template_registered_on_a(context):
    sla = _sla(context)
    # Timestamped: the Federated Catalogue is a shared, run-durable store, so a
    # fixed name would collide with every previous run's entry and make the
    # search assertion meaningless.
    name = f"Managed Kubernetes Hosting SLA {int(time.time())}"
    _install_sla_domain_library(context, _base(context, "A"))
    fixture = _fixture()
    metadata = _get(fixture, "metadata") or {}
    description = metadata.get("dcs:description") or "Federated SLA demonstrator template"

    creator_h = _headers(context, "A", ["Template Creator"])
    with _as_instance(context, _base(context, "A")):
        resp = post_json(
            context,
            template_create_url(context),
            {
                "template_type": "CONTRACT_TEMPLATE",
                "name": name,
                "description": description,
                "template_data": fixture,
            },
            headers=creator_h,
        )
    assert resp.status_code == 200, (
        f"creating the SLA hosting template on instance A failed: {resp.status_code} {resp.text}"
    )
    did = resp.json().get("did")
    assert did, f"the template create response carries no did: {resp.text}"

    _drive_template_to_registered(context, "A", did)
    sla["template_name"] = name
    sla["template_did_a"] = did
    sla["template_version_a"] = _template_body(context, "A", did).get("version") or 1


@step("instance A publishes the SLA template to the Federated Catalogue")
def step_when_publish_sla_template(context):
    sla = _sla(context)
    manager_h = _headers(context, "A", ["Template Manager"])
    resp = None
    for _ in range(6):
        body = _template_body(context, "A")
        with _as_instance(context, _base(context, "A")):
            resp = post_json(
                context,
                template_publish_url(context),
                {"did": sla["template_did_a"], "updated_at": body.get("updated_at")},
                headers=manager_h,
            )
        if resp.status_code == 200 or "updated elsewhere" not in resp.text.lower():
            break
        time.sleep(1)
    context.requests_response = resp
    assert resp.status_code == 200, (
        f"publishing the SLA template to the Federated Catalogue failed: {resp.status_code} {resp.text}"
    )
    sla["template_version_a"] = _template_body(context, "A").get("version") or sla["template_version_a"]


@step("instance B searches the Federated Catalogue for the SLA template")
def step_when_b_searches_catalogue(context):
    with _as_instance(context, _base(context, "B")):
        context.requests_response = _requests.get(
            catalogue_template_search_url(context),
            params={"name": _sla(context)["template_name"], "offset": 0, "limit": 100},
            headers=_headers(context, "B", ["Contract Creator"]),
            timeout=context.http_timeout_seconds,
        )


@then("the catalogue search result on instance B includes the SLA template")
def step_then_b_search_includes_sla(context):
    resp = context.requests_response
    assert resp.status_code == 200, f"the catalogue search on instance B failed: {resp.status_code} {resp.text}"
    items = resp.json().get("items") or []
    entry = next((i for i in items if i.get("did") == _sla(context)["template_did_a"]), None)
    assert entry is not None, (
        f"expected instance B's Federated Catalogue search to return instance A's SLA template "
        f"({_sla(context)['template_did_a']}), got: {[i.get('did') for i in items]}"
    )
    _sla(context)["catalogue_version"] = entry.get("version") or _sla(context)["template_version_a"]


@then("the catalogued SLA entry carries the ODRL Offer and the sh:shapesGraph pin")
def step_then_catalogue_entry_carries_odrl_and_pin(context):
    sla = _sla(context)
    with _as_instance(context, _base(context, "B")):
        resp = _requests.get(
            catalogue_template_retrieve_by_id_url(context, sla["template_did_a"]),
            params={"version": sla.get("catalogue_version") or sla["template_version_a"]},
            headers=_headers(context, "B", ["Contract Creator"]),
            timeout=context.http_timeout_seconds,
        )
    assert resp.status_code == 200, (
        f"reading the catalogued SLA entry from instance B failed: {resp.status_code} {resp.text}"
    )
    template_data = resp.json().get("template_data")
    assert isinstance(template_data, dict), (
        f"the catalogued SLA entry carries no template_data: {resp.text}"
    )
    policies = _policies(template_data)
    assert _local(str(policies.get("@type"))) == "Offer", (
        "an unsigned template's policy set is an odrl:Offer (its parties and negotiated boundaries "
        f"are still open), got @type {policies.get('@type')!r}"
    )
    pins = _shapes_anchor_identity(template_data)
    assert all(name and version for name, version in pins), (
        "the catalogued SLA entry's sh:shapesGraph pin names no shapes name/version: "
        f"{template_data.get('sh:shapesGraph')!r}"
    )
    sla["catalogued_template_data"] = template_data


# ---------------------------------------------------------------------------
# When / Then — importing on instance B
# ---------------------------------------------------------------------------


@step("instance B registers the catalogued SLA template into its own repository")
def step_when_b_registers_catalogued_template(context):
    sla = _sla(context)
    # Content crosses the catalogue; the vocabulary it is written against does
    # not. Instance B installs the same library under the same version before
    # taking the template on, or the contract it later draws from it names a
    # shapes graph its own hub cannot resolve.
    _install_sla_domain_library(context, _base(context, "B"))
    manager_h = _headers(context, "B", ["Template Manager"])
    with _as_instance(context, _base(context, "B")):
        resp = post_json(
            context,
            template_register_url(context),
            {
                "did": sla["template_did_a"],
                "version": int(sla.get("catalogue_version") or sla["template_version_a"]),
            },
            headers=manager_h,
        )
    context.requests_response = resp
    assert resp.status_code == 200, (
        "registering instance A's catalogued SLA template into instance B's own repository failed: "
        f"{resp.status_code} {resp.text}"
    )
    imported_did = resp.json().get("did")
    assert imported_did, f"the register response carries no did: {resp.text}"
    # An import mints a BRAND-NEW local DID (register.go's not-found branch);
    # sharing A's DID would make the two repositories one.
    assert imported_did != sla["template_did_a"], (
        "expected the imported template to be a new local identity on instance B, but it reused "
        f"instance A's DID {imported_did}"
    )
    sla["template_did_b"] = imported_did


@then('the imported SLA template on instance B is in "{state}" status')
def step_then_imported_template_state(context, state):
    actual = str(_template_body(context, "B").get("state", ""))
    assert actual.upper() == state.strip().upper(), (
        f"expected instance B's imported SLA template to be {state!r} — a template imported from "
        f"the catalogue lands in Draft and runs B's OWN approval (register.go) — got {actual!r}"
    )


@then(
    "the imported SLA template declares {permissions:d} permissions, {prohibitions:d} prohibitions "
    "and {obligations:d} obligations"
)
def step_then_imported_rule_counts(context, permissions, prohibitions, obligations):
    imported = _odrl_metrics(_template_data(context, "B"))
    authored = _odrl_metrics(_template_data(context, "A"))
    expected = {"permission": permissions, "prohibition": prohibitions, "obligation": obligations}
    for bucket, count in expected.items():
        assert imported[bucket] == count, (
            f"expected {count} {bucket} rules on instance B's imported SLA template, got "
            f"{imported[bucket]}"
        )
        assert authored[bucket] == count, (
            f"expected {count} {bucket} rules on instance A's authored SLA template, got "
            f"{authored[bucket]} — the fixture and the feature file disagree"
        )
    assert imported["actions"] == authored["actions"], (
        "the imported template's rule actions differ from the authored ones: "
        f"{imported['actions']} vs {authored['actions']}"
    )


@then("the imported SLA template carries a logical constraint tree of depth {depth:d}")
def step_then_imported_constraint_depth(context, depth):
    imported = _odrl_metrics(_template_data(context, "B"))["depth"]
    authored = _odrl_metrics(_template_data(context, "A"))["depth"]
    assert imported == depth, (
        f"expected the imported SLA template's deepest constraint tree to be {depth} levels "
        f"(rule -> constraint -> nested constraint), got {imported}"
    )
    assert authored == depth, (
        f"instance A's own template's deepest constraint tree is {authored}, not {depth} — the "
        "fixture and the feature file disagree"
    )


@then("the imported SLA template carries {count:d} duty consequence")
@then("the imported SLA template carries {count:d} duty consequences")
def step_then_imported_consequences(context, count):
    imported = _odrl_metrics(_template_data(context, "B"))["consequences"]
    assert imported == count, (
        f"expected {count} odrl:consequence node(s) — the duty that falls due when the primary duty "
        f"is not fulfilled — on the imported SLA template, got {imported}"
    )


@then("the imported SLA template carries {count:d} field-bound right operands")
def step_then_imported_field_bound_operands(context, count):
    imported = _odrl_metrics(_template_data(context, "B"))
    authored = _odrl_metrics(_template_data(context, "A"))
    assert imported["field_bound_right_operands"] == count, (
        f"expected {count} negotiated boundaries (an odrl:rightOperand referencing a declared "
        f"dcs:ContractField rather than a literal) on the imported SLA template, got "
        f"{imported['field_bound_right_operands']}"
    )
    assert authored["field_bound_right_operands"] == count, (
        "instance A's own template declares "
        f"{authored['field_bound_right_operands']} negotiated boundaries, not {count} — the fixture "
        "and the feature file disagree"
    )


@then(
    "the SLA contract on instance {label} pins an effective bundle covering every shapes graph it declares"
)
def step_then_contract_bundle_covers_its_declarations(context, label):
    """The workflow gate refuses a transition whose immutable snapshot it
    cannot build, and it builds that snapshot from these two properties
    (backend/internal/processauditandcompliance/workflowgate). A contract drawn
    from an IMPORTED template is the case that breaks them apart: its template
    declares the upstream author's shape library beside the canonical DCS graph,
    so sh:shapesGraph is a LIST and one of its anchors is served by the
    publishing instance. Both are compared by hub entry name and version, never
    by URL — the host is the instance that produced the document."""
    document = _contract_data(context, label)
    declared = [_anchor_identity(anchor) for anchor in hub_shapes_anchors(document)]
    assert declared, (
        f"the SLA contract on instance {label} declares no hub sh:shapesGraph anchor: "
        f"{document.get('sh:shapesGraph')!r}"
    )
    effective_anchors = document.get("dcs:effectiveShapes")
    assert isinstance(effective_anchors, list) and effective_anchors, (
        f"the SLA contract on instance {label} pins no dcs:effectiveShapes bundle, so no workflow "
        f"transition can be gated: {effective_anchors!r}"
    )
    effective = [
        _anchor_identity(entry.get("@id") if isinstance(entry, dict) else entry)
        for entry in effective_anchors
    ]
    assert declared[0] == effective[0], (
        f"the canonical shapes graph the contract declares first ({declared[0]}) is not the one its "
        f"effective bundle leads with ({effective[0]}) on instance {label}"
    )
    missing = [anchor for anchor in declared if anchor not in effective]
    assert not missing, (
        f"the SLA contract on instance {label} declares shapes graphs its effective bundle does not "
        f"carry: {missing}; bundle={effective}"
    )
    # The upstream author's library is the reason the array form exists.
    assert any(name == _SLA_LIBRARY_NAME for name, _version in declared), (
        f"the contract lost the {_SLA_LIBRARY_NAME!r} library its imported template declared: {declared}"
    )


@then("the imported SLA template pins the same sh:shapesGraph name and version as instance A's")
def step_then_imported_shapes_pin_identical(context):
    imported = _shapes_anchor_identity(_template_data(context, "B"))
    authored = _shapes_anchor_identity(_template_data(context, "A"))
    assert imported == authored, (
        "the imported template must stay anchored to the SAME shapes graph name and version the "
        f"authoring instance pinned (ADR-8), got {imported} on B vs {authored} on A"
    )


@step("instance B drives the imported SLA template through its own approval and registers it")
def step_when_b_approves_and_registers_imported_template(context):
    _drive_template_to_registered(context, "B", _template_did(context, "B"))


# ---------------------------------------------------------------------------
# When / Then — the contract
# ---------------------------------------------------------------------------


@step("instance B instantiates an SLA contract from its imported template with instance A as counterparty")
def step_when_b_instantiates_contract(context):
    party_h = _party_headers(context, "B")
    with _as_instance(context, _base(context, "B")):
        resp = post_json(
            context,
            contract_create_url(context),
            {
                "template_did": _template_did(context, "B"),
                "counterparty": context.peer_did_a,
                # Who may READ this contract on instance B (see the module
                # docstring): the negotiating participant's own organization
                # and the one every other role token here presents. B is the
                # originator, so nothing else grants its reviewer, approver or
                # signer access to the copy they are asked to act on.
                "parties": _READING_ORGANIZATIONS,
            },
            headers=party_h,
        )
    assert resp.status_code == 200, (
        f"creating the SLA contract on instance B failed: {resp.status_code} {resp.text}"
    )
    did = resp.json().get("did")
    assert did, f"the contract create response carries no did: {resp.text}"
    _sla(context)["contract_did"] = did
    # The peer-trust pack's cross-instance signing and replication steps read
    # this attribute; setting it lets this pack reuse them verbatim rather than
    # re-implementing a signing ceremony.
    context.cross_instance_contract_did = did
    context.cross_instance_creator_headers = party_h


@step("instance B fills every open field of the SLA contract")
def step_when_b_fills_fields(context):
    """The genesis PDF render triggered by contract creation is asynchronous
    and moves updated_at, so the fill re-reads the document (and with it the
    lock token) on every attempt rather than racing it."""
    party_h = _party_headers(context, "B")
    resp = None
    for _ in range(6):
        body = _contract_body(context, "B")
        document = body.get("contract_data")
        filled = _fill_open_fields(document)
        assert filled, "the SLA contract already carried a value for every field — nothing to negotiate"
        _assert_declared_constraints_satisfied(document)
        _assert_literal_constraints_satisfied(document)
        with _as_instance(context, _base(context, "B")):
            resp = put_json(
                context,
                contract_update_url(context),
                {
                    "did": _contract_did(context),
                    "updated_at": body.get("updated_at"),
                    "contract_data": document,
                },
                headers=party_h,
            )
        if resp.status_code == 200 or "updated elsewhere" not in resp.text.lower():
            break
        time.sleep(2)
    context.requests_response = resp
    assert resp.status_code == 200, (
        f"filling the SLA contract's fields on instance B failed: {resp.status_code} {resp.text}"
    )


@then("all {count:d} contract fields of the SLA contract carry a value on instance {label}")
def step_then_all_fields_filled(context, count, label):
    fields = _contract_fields(_contract_data(context, label))
    assert len(fields) == count, (
        f"expected the SLA contract on instance {label} to declare {count} contract fields, got "
        f"{len(fields)}: {[f.get('@id') for f in fields]}"
    )
    open_fields = [f.get("@id") for f in fields if _literal_value(f.get("dcs:value")) in (None, "")]
    assert not open_fields, (
        f"expected every contract field to carry an agreed value on instance {label}; still open: "
        f"{open_fields}"
    )


@step("instance B offers the SLA contract to instance A")
def step_when_b_offers(context):
    party_h = _party_headers(context, "B")
    resp = _post_with_fresh_updated_at(
        context, "B", "/contract/offer", {}, party_h, "offer"
    )
    context.requests_response = resp
    assert resp.status_code == 200, (
        f"offering the SLA contract from instance B failed: {resp.status_code} {resp.text}"
    )


@then("the SLA contract is in state {expected} on instance {label}")
def step_then_sla_state_now(context, expected, label):
    actual = _contract_state(context, label)
    assert actual == expected.strip().upper(), (
        f"expected the SLA contract to be {expected} on instance {label}, got {actual!r}"
    )


@step("the SLA contract appears on instance {label} in state {expected} within a few seconds")
def step_then_sla_state_on_instance(context, label, expected):
    expected = expected.strip().upper()
    deadline = time.monotonic() + 90
    actual = None
    while time.monotonic() < deadline:
        try:
            actual = _contract_state(context, label)
        except AssertionError:
            actual = None  # not replicated yet
        if actual == expected:
            return
        time.sleep(2)
    raise AssertionError(
        f"expected the SLA contract to reach state {expected} on instance {label} within a few "
        f"seconds of the peer ship, last observed: {actual!r}"
    )


@then("instance A's copy of the SLA contract carries the identical ODRL Offer")
def step_then_a_copy_identical_odrl(context):
    on_a = _odrl_metrics(_contract_data(context, "A"))
    on_b = _odrl_metrics(_contract_data(context, "B"))
    assert on_a == on_b, (
        "the ODRL on instance A's received copy differs from the one instance B shipped:\n"
        f"  A: {on_a}\n  B: {on_b}"
    )


@then("the SLA contract's ODRL is structurally intact on instance {label}")
def step_then_odrl_intact(context, label):
    """Negotiation moves VALUES; it must never move the policy structure. The
    authored template on instance A is the reference — the rule counts, the
    actions, the constraint-tree depth, the consequence, and the number of
    field-bound right operands must all still match it, and every field-bound
    right operand must still resolve to a field that carries an agreed
    value."""
    document = _contract_data(context, label)
    actual = _odrl_metrics(document)
    reference = _odrl_metrics(_template_data(context, "A"))
    for key in ("permission", "prohibition", "obligation", "depth", "consequences",
                "field_bound_right_operands", "actions"):
        assert actual[key] == reference[key], (
            f"the SLA contract's ODRL on instance {label} no longer matches the authored template: "
            f"{key} is {actual[key]!r}, the template declares {reference[key]!r}"
        )
    values = {
        field["@id"]: _literal_value(field.get("dcs:value"))
        for field in _contract_fields(document)
        if field.get("@id")
    }
    for node in _all_rules(_policies(document)):
        for constraint in _as_list(_get(node, "constraint")):
            if not isinstance(constraint, dict):
                continue
            for leaf in _constraint_leaves(constraint):
                for candidate in _as_list(_get(leaf, "rightOperand")):
                    boundary = _reference_id(candidate)
                    if boundary is None or boundary not in values:
                        continue
                    assert values[boundary] not in (None, ""), (
                        f"the negotiated boundary {boundary!r} referenced by rule "
                        f"{node.get('@id')!r} no longer resolves to an agreed value on instance "
                        f"{label}"
                    )


# ---------------------------------------------------------------------------
# When / Then — the negotiation ping-pong
# ---------------------------------------------------------------------------


@step('instance {label} counter-offers a committed availability of "{value}" on the SLA contract')
def step_when_counter_offer_availability(context, label, value):
    """A structured redline: the counter-offer replaces the contract document
    with one differing in a single negotiated value, which negotiate.go applies
    immediately and the regenerator re-ships to the peer as a fresh PDF
    (ADR-13). The whole document has to travel, so it is read from the copy
    being negotiated rather than assembled here."""
    label = label.strip().upper()
    document = _contract_data(context, label)
    field = _field_by_fragment(document, "committed-availability")
    field["dcs:value"] = {"@value": value, "@type": field.get("dcs:datatype", "xsd:decimal")}
    party_h = _party_headers(context, label)
    resp = _post_with_fresh_updated_at(
        context,
        label,
        "/contract/negotiate",
        {
            "negotiated_by": AuthService.username_for_roles(_PARTY_ROLES),
            "change_request": {"contract_data": document},
        },
        party_h,
        "counter-offer",
    )
    context.requests_response = resp
    assert resp.status_code == 200, (
        f"instance {label}'s counter-offer of committed availability {value} was refused: "
        f"{resp.status_code} {resp.text}"
    )


@then('the SLA contract shows a committed availability of "{value}" on instance {label} within a few seconds')
def step_then_availability_value_on_instance(context, label, value):
    deadline = time.monotonic() + 120
    actual = None
    while time.monotonic() < deadline:
        document = _contract_data(context, label)
        actual = _literal_value(_field_by_fragment(document, "committed-availability").get("dcs:value"))
        if actual is not None and str(actual).strip() == value:
            return
        time.sleep(3)
    raise AssertionError(
        f"expected the counter-offered committed availability {value!r} to reach instance {label} "
        f"over the PDF exchange within a few seconds, last observed: {actual!r}"
    )


@step("instance {label} drives its copy of the SLA contract to REVIEWED")
def step_when_drive_to_reviewed(context, label):
    """Each instance settles its OWN copy (ADR-13): the negotiating
    participant submits until the round closes, then the local reviewer
    forwards it to approval. Neither step consults the counterparty — which is
    the point of the approval assertion that follows, since the ODRL gate is
    the first thing on this path that does.
    """
    label = label.strip().upper()
    party_h = _party_headers(context, label)
    # OFFERED and NEGOTIATION both leave via /contract/submit; the first submit
    # from OFFERED opens the round, and each further one folds the round into a
    # new contract version until none is open (submit.go). Driving on the state
    # rather than a fixed count keeps this correct when a peer ship bumps the
    # version in between.
    for _ in range(5):
        state = _contract_state(context, label)
        if state not in ("OFFERED", "NEGOTIATION"):
            break
        response = _post_with_fresh_updated_at(context, label, "/contract/submit", {}, party_h, "submit")
        assert response.status_code == 200, (
            f"settling the SLA contract on instance {label} (state {state}) failed: "
            f"{response.status_code} {response.text}"
        )
    state = _contract_state(context, label)
    assert state == "SUBMITTED", (
        f"expected instance {label}'s copy to close its negotiation round and reach SUBMITTED, got "
        f"{state!r}"
    )

    reviewer_h = _headers(context, label, ["Contract Reviewer"])
    response = _post_with_fresh_updated_at(
        context, label, "/contract/submit", {"forward_to": "approval"}, reviewer_h,
        "reviewer forward-to-approval",
    )
    assert response.status_code == 200, (
        f"forwarding the SLA contract to approval on instance {label} failed: "
        f"{response.status_code} {response.text}"
    )
    assert _contract_state(context, label) == "REVIEWED", (
        f"expected instance {label}'s copy to be REVIEWED, got {_contract_state(context, label)!r}"
    )


@step("instance {label} attempts to approve the SLA contract")
def step_when_attempt_approve(context, label):
    label = label.strip().upper()
    approver_h = _headers(context, label, ["Contract Approver"])
    # Attempts, not asserts: whether this is granted or refused is the point of
    # the Then step that follows. The lost-update retry inside the helper keeps
    # a stale lock token from masquerading as the policy refusal.
    context.requests_response = _post_with_fresh_updated_at(
        context, label, "/contract/approve", {}, approver_h, "approve"
    )


@then("the approval on instance {label} succeeds")
def step_then_approval_succeeds(context, label):
    resp = context.requests_response
    assert resp.status_code == 200, (
        f"expected instance {label} to approve the settled SLA contract, got {resp.status_code}: "
        f"{resp.text}"
    )


@then("the approval on instance {label} is refused naming the availability constraint")
def step_then_approval_refused_naming_availability(context, label):
    """The refusal must be the ODRL gate's, not any other failure that happens
    to return 4xx: the message has to name a rule that actually constrains the
    committed-availability field (the audit reports `ODRL policy "<rule @id>"
    violated: …`). Reading the rule identities off the STORED contract is what
    makes this independent of the fixture's authored IRIs — contract creation
    rebases every one of them onto the contract's own DID."""
    label = label.strip().upper()
    resp = context.requests_response
    assert resp.status_code != 200, (
        f"expected instance {label} to REFUSE approval of a committed availability below the "
        f"template's own floor, but the approval succeeded: {resp.text}"
    )
    document = _contract_data(context, label)
    field_id = _field_by_fragment(document, "committed-availability")["@id"]
    candidates = _rules_constraining_field(document, field_id) + [field_id]
    message = resp.text
    assert any(candidate in message for candidate in candidates), (
        f"expected instance {label}'s refusal to name the availability constraint (one of "
        f"{candidates}), got: {message}"
    )


@then("renegotiating the signed SLA contract on instance {label} is refused")
def step_then_renegotiation_refused_after_signing(context, label):
    """A redline after signing cannot be honoured: the document would move on
    while the artefact stays the counterparty's signed PDF.

    TWO gates forbid it and which one answers depends on where the copy stands.
    A copy still in OFFERED/NEGOTIATION whose artefact already carries the
    counterparty's signature is stopped by command.ErrAgreementSettled ("this
    agreement is settled; a signed contract cannot be renegotiated"). A copy
    that has itself reached SIGNED never gets that far: the state machine
    declares no EventNegotiate edge out of SIGNED (transition.go), so
    ValidateTransition refuses first. This scenario reaches the second case,
    and the assertion accepts either rather than pretending to exercise a gate
    the ordering does not reach — the requirement under test is that a signed
    agreement is not renegotiable, not which of its two guards spoke.
    """
    label = label.strip().upper()
    document = _contract_data(context, label)
    field = _field_by_fragment(document, "committed-availability")
    field["dcs:value"] = {"@value": "99.6", "@type": field.get("dcs:datatype", "xsd:decimal")}
    resp = _post_with_fresh_updated_at(
        context,
        label,
        "/contract/negotiate",
        {
            "negotiated_by": AuthService.username_for_roles(_PARTY_ROLES),
            "change_request": {"contract_data": document},
        },
        _party_headers(context, label),
        "post-signature redline",
    )
    context.requests_response = resp
    assert resp.status_code != 200, (
        f"expected a redline against the signed SLA agreement on instance {label} to be refused, "
        f"got {resp.status_code}: {resp.text}"
    )
    message = resp.text.lower()
    assert "settled" in message or "transition" in message, (
        "expected the refusal to name either the settled agreement or the refused state "
        f"transition, got: {resp.text}"
    )


@then("the sealed policy of the SLA contract on instance {label} is an odrl:Agreement")
def step_then_sealed_as_agreement(context, label):
    """The ODRL policy lifecycle: a template and an unsigned contract carry an
    odrl:Offer (parties and boundaries still open); the first signature seals
    it into the odrl:Agreement the signatures actually bind."""
    policies = _policies(_contract_data(context, label))
    assert _local(str(policies.get("@type"))) == "Agreement", (
        f"expected the signed SLA contract's policy set on instance {label} to be sealed as an "
        f"odrl:Agreement, got @type {policies.get('@type')!r}"
    )


# ---------------------------------------------------------------------------
# Single-instance: the SLA in operation (no Federated Catalogue)
# ---------------------------------------------------------------------------


@given('a filled SLA contract "{name}" exists on this instance')
def step_given_single_instance_sla_contract(context, name):
    """The same authored SLA, instantiated on ONE instance so the deployment
    and KPI packs' existing steps can drive it. It is registered under `name`
    in ContractService's stores, which is the handle every one of those steps
    resolves a contract by."""
    from steps.support.services.contract_service import ContractService  # noqa: PLC0415

    _install_sla_domain_library(context, context.base_url)
    creator_h = AuthService.get_headers_for_roles(["Template Creator"])
    resp = post_json(
        context,
        template_create_url(context),
        {
            "template_type": "CONTRACT_TEMPLATE",
            "name": f"{name} Template {int(time.time())}",
            "description": "Federated SLA demonstrator, single-instance operation",
            "template_data": _fixture(),
        },
        headers=creator_h,
    )
    assert resp.status_code == 200, f"creating the SLA template failed: {resp.status_code} {resp.text}"
    template_did = resp.json().get("did")

    # Reuse the two-instance driver against this single instance by pointing
    # both instance slots at it: every call it makes reads context.base_url.
    context.base_url_a = context.base_url
    context.base_url_b = context.base_url
    _drive_template_to_registered(context, "A", template_did)

    contract_creator_h = AuthService.get_headers_for_roles(["Contract Creator"])
    create = post_json(
        context, contract_create_url(context), {"template_did": template_did}, headers=contract_creator_h
    )
    assert create.status_code == 200, f"creating the SLA contract failed: {create.status_code} {create.text}"
    contract_did = create.json().get("did")

    retrieve = get_with_headers(
        context, contract_retrieve_by_id_url(context, contract_did), headers=contract_creator_h
    )
    assert retrieve.status_code == 200, retrieve.text
    document = retrieve.json().get("contract_data")
    _fill_open_fields(document)
    _assert_declared_constraints_satisfied(document)
    _assert_literal_constraints_satisfied(document)
    update = put_json(
        context,
        contract_update_url(context),
        {
            "did": contract_did,
            "updated_at": retrieve.json().get("updated_at"),
            "contract_data": document,
        },
        headers=contract_creator_h,
    )
    assert update.status_code == 200, (
        f"filling the SLA contract's fields failed: {update.status_code} {update.text}"
    )

    ContractService._ensure_store(context, "contract_dids", {})
    ContractService._ensure_store(context, "contract_updated_at", {})
    ContractService._ensure_store(context, "contract_seed_headers", {})
    context.contract_dids[name] = contract_did
    context.contract_seed_headers[name] = contract_creator_h
    ContractService._refresh_contract(context, name)
    _sla(context)["single_instance_contract_did"] = contract_did


@step('the target reports an availability KPI value "{value}" for contract "{name}"')
def step_when_target_reports_availability_kpi(context, value, name):
    """The metric IS the committed-availability field's node IRI, which is how
    EvaluateKPIViolation binds a reported measurement to the ODRL constraint
    that governs it (DCS-FR-CWE-09) — a label would bind to nothing."""
    from steps.contract_deployment.dcs_contract_deployment_steps import (  # noqa: PLC0415
        step_when_target_reports_kpi,
    )
    from steps.support.services.contract_service import ContractService  # noqa: PLC0415

    did, _ = ContractService._contract_data(context, name)
    headers = context.contract_seed_headers.get(name)
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=headers)
    assert retrieve.status_code == 200, retrieve.text
    field = _field_by_fragment(retrieve.json().get("contract_data") or {}, "committed-availability")
    step_when_target_reports_kpi(context, field["@id"], value, name)


@step('the committed availability of contract "{name}" is "{value}"')
def step_then_committed_availability_is(context, name, value):
    from steps.support.services.contract_service import ContractService  # noqa: PLC0415

    did, _ = ContractService._contract_data(context, name)
    headers = context.contract_seed_headers.get(name)
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=headers)
    assert retrieve.status_code == 200, retrieve.text
    field = _field_by_fragment(retrieve.json().get("contract_data") or {}, "committed-availability")
    actual = _literal_value(field.get("dcs:value"))
    assert str(actual).strip() == value, (
        f"expected the committed availability of contract {name!r} to be {value!r}, got {actual!r}"
    )


@then('the contract detail for "{name}" shows a KPI violation flag for its committed availability')
def step_then_kpi_violation_on_availability(context, name):
    from steps.contract_deployment.dcs_contract_deployment_steps import (  # noqa: PLC0415
        step_then_contract_detail_shows_kpi_violation,
    )
    from steps.support.services.contract_service import ContractService  # noqa: PLC0415

    did, _ = ContractService._contract_data(context, name)
    headers = context.contract_seed_headers.get(name)
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=headers)
    assert retrieve.status_code == 200, retrieve.text
    field = _field_by_fragment(retrieve.json().get("contract_data") or {}, "committed-availability")
    step_then_contract_detail_shows_kpi_violation(context, name, field["@id"])
