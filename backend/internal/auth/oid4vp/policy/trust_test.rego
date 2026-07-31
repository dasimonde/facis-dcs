# Tests for the issuer trust authorization policy.
#
# Runs under `opa test internal/auth/oid4vp/policy` and, so it cannot be
# forgotten, in the Go suite too — TestTrustPolicyRegoTests executes this file.
package dcs.trust_test

import data.dcs.trust
import rego.v1

own_issuer := "did:web:a.example:issuer"

peer_issuer := "did:web:b.example:issuer"

# A deployment that grants its own issuer login+peer and nothing to the peer's.
demo_trust := {
	"peer_dynamic": false,
	"issuers": {own_issuer: {
		"purposes": ["login", "peer"],
		"organizations": ["did:web:a.example"],
		"mechanism": "did:web",
	}},
}

request(purpose, issuer, organization) := {
	"purpose": purpose,
	"issuer": issuer,
	"organization": organization,
	"trust": demo_trust,
}

test_granted_purpose_is_trusted if {
	trust.trusted with input as request("login", own_issuer, "")
	trust.trusted with input as request("peer", own_issuer, "")
}

# The credential from issuer B is refused at A's login because A's operator
# granted it nothing — not because the code says so.
test_unlisted_issuer_is_refused if {
	not trust.trusted with input as request("login", peer_issuer, "")
	not trust.trusted with input as request("peer", peer_issuer, "")
}

# An explicit entry is the operator's complete answer: withholding a purpose is
# a denial, not a fall-through to the dynamic path.
test_withheld_purpose_is_denied_not_defaulted if {
	not trust.trusted with input as request("pid", own_issuer, "")
	not trust.trusted with input as request("pid", own_issuer, "") with input.trust.peer_dynamic as true
}

test_issuer_attests_only_its_own_organizations if {
	trust.may_attest with input as request("peer", own_issuer, "did:web:a.example")
	not trust.may_attest with input as request("peer", own_issuer, "did:web:b.example")
}

# A request whose issuer entry names exactly these organizations.
entitled(organizations) := {
	"purpose": "peer",
	"issuer": own_issuer,
	"organization": "Acme Corp",
	"trust": {
		"peer_dynamic": false,
		"issuers": {own_issuer: {
			"purposes": ["peer"],
			"organizations": organizations,
			"mechanism": "did:web",
		}},
	},
}

# An issuer with no organizations may attest none, and an empty organization is
# never attestable — both fail closed.
test_organization_entitlement_fails_closed if {
	not trust.may_attest with input as request("peer", own_issuer, "")
	not trust.may_attest with input as entitled([])
	not trust.may_attest with input as entitled(["Someone Else"])
}

# The wildcard has to be written out; an absent list never means "any".
test_wildcard_must_be_explicit if {
	trust.may_attest with input as entitled(["*"])
	trust.may_attest with input as entitled(["Acme Corp"])
}

# Dynamic peer trust is opt-in, peer-only, and did:web-only.
test_dynamic_peer_trust_is_bounded if {
	not trust.trusted with input as request("peer", peer_issuer, "")
	trust.trusted with input as request("peer", peer_issuer, "") with input.trust.peer_dynamic as true
	not trust.trusted with input as request("login", peer_issuer, "") with input.trust.peer_dynamic as true
	not trust.trusted with input as request("peer", "https://b.example/issuer", "")
		with input.trust.peer_dynamic as true
}

# A dynamically trusted issuer speaks for its own authority and no other.
test_dynamic_peer_attests_only_its_own_authority if {
	trust.may_attest with input as request("peer", peer_issuer, "did:web:b.example")
		with input.trust.peer_dynamic as true
	not trust.may_attest with input as request("peer", peer_issuer, "did:web:a.example")
		with input.trust.peer_dynamic as true
}

# A denial says why: a policy that only answers false is one nobody can operate.
test_denial_carries_a_reason if {
	count(trust.reasons) > 0 with input as request("login", peer_issuer, "")
	count(trust.reasons) > 0 with input as request("pid", own_issuer, "")
}
