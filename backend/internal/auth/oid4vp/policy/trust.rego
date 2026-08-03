# Issuer trust authorization (ADR-31).
#
# This decides what an ALREADY-VERIFIED credential is allowed to do. Everything
# cryptographic — chain validation to an anchor, the leaf identifying its issuer,
# signature and holder binding, revocation — stays in Go, where it is typed and
# tested. Only the authorization question lives here, because that is the part a
# deployment changes: which issuers it trusts, for what, and for whose behalf.
#
# Input:  {"purpose": "login"|"peer"|"pid", "issuer": <iss>, "organization": <org>,
#         plus "trust": the trust document (issuers, peer_dynamic), verbatim.
package dcs.trust

import rego.v1

# An explicit entry is the operator's complete answer for this issuer. If it
# withholds the purpose that is a denial, not an invitation to fall through to
# the dynamic path — otherwise purposes:["login"] would silently also grant
# peer, and no did:web issuer could ever be denied it.
entry := input.trust.issuers[input.issuer]

listed if {
	_ = entry
}

default trusted := false

trusted if {
	input.purpose in entry.purposes
}

trusted if {
	not listed
	dynamic_peer
}

# Only did:web qualifies for dynamic trust: the identifier has to resolve to a
# document this instance can fetch, or there is nothing to verify against.
# `login` is deliberately excluded — who may obtain a session here is local
# policy an operator states explicitly.
default dynamic_peer := false

dynamic_peer if {
	input.purpose == "peer"
	input.trust.peer_dynamic == true
	startswith(input.issuer, "did:web:")
}

# An issuer may only attest organizations its own entry names. The empty case
# fails closed: an issuer with no organizations may attest none.
default may_attest := false

may_attest if {
	input.organization != ""
	some allowed in entry.organizations
	allowed == input.organization
}

# "*" is the explicit wildcard, for an issuer that IS the tenant registry of its
# deployment. It has to be written out — treating an absent list as "any" is how
# an issuer silently gains the right to speak for a party nobody granted it.
may_attest if {
	input.organization != ""
	some allowed in entry.organizations
	allowed == "*"
}

# A dynamically trusted peer issuer speaks for its own authority and no other,
# so the bound comes from the identifier rather than from configuration:
# did:web:example.com:issuer may attest did:web:example.com.
may_attest if {
	not listed
	input.trust.peer_dynamic == true
	input.organization != ""
	input.organization == peer_authority
}

peer_authority := authority if {
	startswith(input.issuer, "did:web:")
	rest := trim_prefix(input.issuer, "did:web:")
	rest != ""
	authority := concat("", ["did:web:", split(rest, ":")[0]])
}

# Why a denial happened. A policy that only answers false is a policy nobody can
# operate: these are surfaced in the error the caller reports.
reasons contains sprintf("issuer %q is not listed and does not qualify for dynamic peer trust", [input.issuer]) if {
	not listed
	not dynamic_peer
}

reasons contains sprintf("issuer %q is listed but not granted %q (granted: %v)", [input.issuer, input.purpose, entry.purposes]) if {
	listed
	not input.purpose in entry.purposes
}

reasons contains sprintf("issuer %q may not attest organization %q (entitled: %v)", [input.issuer, input.organization, entry.organizations]) if {
	listed
	input.organization != ""
	not may_attest
}
