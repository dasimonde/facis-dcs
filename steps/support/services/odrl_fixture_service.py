"""Fixture builders for the ODRL-soundness BDD scenarios
(features/18_odrl_soundness).

These build canonical `dcs:documentStructure`-enveloped contract documents
(see backend/internal/base/validation/documentdata.go `isCanonicalEnvelope`)
carrying either:

  - the bare-Duty `dcs:policies` shape (flat array of
    `odrl:Duty`/`odrl:Permission`/`odrl:Prohibition` nodes, each with only an
    `odrl:constraint` — no `odrl:action`, no enclosing policy node, no
    parties/target), which structural validation must reject, or
  - the canonical ODRL 2.2 shape (docs/adr-6-odrl-profile-enforcement.md):
    one enclosing `odrl:Offer` while unsigned (its @id is its odrl:uid, `odrl:profile`
    declared),
    whose rules each carry exactly one `odrl:action` plus
    `odrl:assigner`/`odrl:assignee`/`odrl:target`.

A negotiable data point is declared once as a typed `dcs:ContractField` in
the top-level `dcs:contractFields` registry. It carries its `dcs:datatype`
(resolved from the field's SHACL shape), requiredness and — when set — its
`dcs:value`. The domain-level object in `dcs:contractData`, the human-readable
clause and the ODRL constraint's `odrl:leftOperand` all reference the same
field by a bare `{"@id"}`.

Both shapes constrain the SAME field (`urn:uuid:field-provider-country`, a
string, or `urn:uuid:field-provider-coverage`, a number) so the same fixture
family can drive the structure, enforcement, operator-matrix, and
bare-shape-rejection scenarios.
"""

FIELD_COUNTRY = "urn:uuid:field-provider-country"
FIELD_COVERAGE = "urn:uuid:field-provider-coverage"

# field name -> (@id, human dcs:label, xsd datatype resolved from its SHACL shape)
_FIELD_BY_NAME = {
    "country": (FIELD_COUNTRY, "Provider country", "xsd:string"),
    "coverage": (FIELD_COVERAGE, "Provider coverage", "xsd:decimal"),
}


def _contract_field_node(field_name: str, actual_value=None) -> dict:
    """Build one declared `dcs:ContractField` with its datatype, requiredness
    and optional negotiated value.
    """
    field_id, label, datatype = _FIELD_BY_NAME[field_name]
    node = {
        "@id": field_id,
        "@type": "dcs:ContractField",
        "dcs:label": label,
        "dcs:datatype": datatype,
        "dcs:shape": {
            "@id": f"https://w3id.org/facis/dcs/taxonomy/v1#field-provider-{field_name}"
        },
        "dcs:required": True,
    }
    if actual_value is not None:
        node["dcs:value"] = actual_value
    return node


def bare_duty_policies(field_name: str, operator: str, right_operand) -> list:
    """Bare `odrl:Duty` nodes each holding one constraint — no action, no
    parties/target, no enclosing policy node. Exists to prove structural
    validation rejects it.
    """
    field_id, _, _ = _FIELD_BY_NAME[field_name]
    return [
        {
            "@id": f"urn:uuid:policy-{field_name}-0",
            "@type": "odrl:Duty",
            "odrl:constraint": {
                "@type": "odrl:Constraint",
                "odrl:leftOperand": {"@id": field_id},
                "odrl:operator": {"@id": f"odrl:{operator}"},
                "odrl:rightOperand": right_operand,
            },
        }
    ]


def odrl_set_policies(
    contract_did: str,
    field_name: str,
    operator: str,
    right_operand,
    rule_type: str = "odrl:Duty",
) -> dict:
    """The canonical ODRL 2.2 shape: one enclosing `odrl:Offer` (`@id` anchored to
    contract DID), `odrl:profile` declared, rule carries exactly one
    `odrl:action` plus assigner/assignee/target. The constraint's
    `odrl:leftOperand` names the declared contract field's `@id`.
    """
    field_id, _, _ = _FIELD_BY_NAME[field_name]
    rule_bucket = {
        "odrl:Duty": "odrl:obligation",
        "odrl:Permission": "odrl:permission",
        "odrl:Prohibition": "odrl:prohibition",
    }[rule_type]
    rule = {
        "@id": f"urn:uuid:policy-{field_name}-0",
        "@type": rule_type,
        "odrl:action": {"@id": "dcs:provideCompliantValue"},
        "odrl:assigner": {"@id": "did:web:example.org%3A9001:bdd-provider-org"},
        "odrl:assignee": {"@id": "did:web:example.org%3A9002:bdd-customer-org"},
        "odrl:target": {"@id": contract_did},
        "dcs:prose": {"@id": "urn:uuid:block-clause-1"},
        "odrl:constraint": {
            "@type": "odrl:Constraint",
            "odrl:leftOperand": {"@id": field_id},
            "odrl:operator": {"@id": f"odrl:{operator}"},
            "odrl:rightOperand": right_operand,
        },
    }
    return {
        "@id": "urn:uuid:policy-set-1",
        "@type": "odrl:Offer",
        "odrl:profile": {"@id": "https://w3id.org/facis/dcs/ontology/v1/odrl-profile"},
        rule_bucket: [rule],
    }


def build_contract_document(contract_did: str, field_name: str, policies, actual_value) -> dict:
    """A full canonical `dcs:Contract` document (documentStructure +
    contractData + policies) suitable for a full replacement PUT to
    /contract/update while the contract is in DRAFT.

    `dcs:contractFields` declares the typed field and its value.
    `dcs:contractData` contains the domain-level provider object, whose
    country or coverage property references that field by `{"@id"}`. The
    clause and ODRL constraint name the same field `@id`.
    """
    field_id, _, _ = _FIELD_BY_NAME[field_name]
    return {
        "@context": {
            "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
            "odrl": "http://www.w3.org/ns/odrl/2/",
        },
        "@type": "dcs:Contract",
        "dcs:metadata": {
            "@type": "dcs:ContractMetadata",
            "dcs:title": "BDD ODRL Soundness Contract",
        },
        "dcs:documentStructure": {
            "@type": "dcs:DocumentStructure",
            "dcs:blocks": {
                "@list": [
                    {
                        "@id": "urn:uuid:block-clause-1",
                        "@type": "dcs:Clause",
                        "dcs:content": {
                            "@list": [
                                f"Provider {field_name}: ",
                                {"@id": field_id},
                            ]
                        },
                    }
                ]
            },
            "dcs:layout": {
                "@list": [
                    {
                        "@id": "urn:uuid:block-root",
                        "@type": "dcs:LayoutNode",
                        "dcs:isRoot": True,
                        "dcs:children": {"@list": [{"@id": "urn:uuid:block-clause-1"}]},
                    }
                ]
            },
        },
        "dcs:contractFields": [_contract_field_node(field_name, actual_value)],
        "dcs:contractData": [
            {
                "@id": "urn:uuid:object-provider",
                "@type": "dcs:Provider",
                f"dcs:{field_name}": {"@id": field_id},
            }
        ],
        "dcs:policies": policies,
    }


def extract_policy_rules(policies) -> list:
    """Flattens either policy shape into a plain list of rule dicts.

    - Bare shape: `policies` IS already the flat list of rules.
    - Canonical policy shape: `policies` is a single dict; rules live under
      `odrl:permission` / `odrl:prohibition` / `odrl:obligation`
      array properties.
    """
    if isinstance(policies, list):
        return [item for item in policies if isinstance(item, dict)]
    if isinstance(policies, dict):
        rules = []
        for key in ("odrl:permission", "odrl:prohibition", "odrl:obligation"):
            bucket = policies.get(key)
            if isinstance(bucket, list):
                rules.extend(item for item in bucket if isinstance(item, dict))
            elif isinstance(bucket, dict):
                rules.append(bucket)
        return rules
    return []
