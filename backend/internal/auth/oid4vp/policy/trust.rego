# Issuer trust authorization (ADR-31).
#
# This decides what an ALREADY-VERIFIED credential is allowed to do. Everything
# cryptographic — chain validation to an anchor, the leaf identifying its issuer,
# signature and holder binding, revocation — stays in Go, where it is typed and
# tested. Only the authorization question lives here, because that is the part a
# deployment changes: which issuers it trusts, for what, and for whose behalf.
#
# Input:  {"purpose": "login"|"peer"|"pid", "issuer": <iss>, "organization": <org>,
#         "anchored": whether Go verified this credential's x5c chain to the
#         anchors for this purpose, and the leaf named this issuer,
#         plus "trust": the trust document (issuers), verbatim.
package dcs.trust

import rego.v1

# An explicit entry is the operator's complete answer for this issuer. If it
# withholds the purpose that is a denial, not an invitation to fall through to
# the anchored path — otherwise purposes:["login"] would silently also grant
# peer to any issuer holding a certificate under the PoA CA, and no listed
# issuer could ever be denied it.
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
	anchored_credential
}

# A peer or a PID issuer is admitted by its certificate chain rather than by an
# entry (ADR-35). For peers, a federation whose membership is a file edited on
# every instance whenever a member joins is not a federation. For PID, nobody can
# anticipate who issues one — a holder may arrive with a PID from any national or
# qualified issuer — so the CA list is the only workable statement of which
# attestations of a person this deployment believes.
#
# `anchored` is Go's word that the chain verified to the CA list for this
# purpose (PoA or PID) and that the leaf named this issuer; the policy never
# inspects certificates itself.
#
# `login` is deliberately excluded, and is the reason the two questions stay
# apart: who may obtain a session HERE is local policy an operator states
# explicitly, naming their own organization's issuers. A certificate under the
# PoA CA admits a counterparty, never a session.
default anchored_credential := false

anchored_credential if {
	input.purpose in ["peer", "pid"]
	input.anchored == true
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

# An anchored peer issuer speaks for its own authority and no other, so the
# bound comes from the identifier rather than from configuration:
# did:web:example.com:issuer and https://example.com/issuer both attest
# did:web:example.com and nothing else.
#
# `anchored` is not re-checked here. This question is only ever asked about a
# credential that already verified (verify.go), and for an unlisted issuer the
# only way to have verified is the anchored path — `trusted` admits no other.
# Demanding the fact again would mean threading it through every caller to
# re-establish something the call itself implies.
may_attest if {
	not listed
	input.purpose == "peer"
	input.organization != ""
	input.organization == peer_authority
}

peer_authority := authority if {
	startswith(input.issuer, "did:web:")
	rest := trim_prefix(input.issuer, "did:web:")
	rest != ""
	authority := concat("", ["did:web:", split(rest, ":")[0]])
}

# The https form the issuers now put in `iss` (ADR-34) names the same authority.
# A did:web authority carrying a port is percent-encoded, so the host has to be
# encoded the same way or an issuer on a non-default port would attest an
# organization no did:web identifier can equal.
peer_authority := authority if {
	some scheme in ["https://", "http://"]
	startswith(input.issuer, scheme)
	rest := trim_prefix(input.issuer, scheme)
	rest != ""
	host := split(rest, "/")[0]
	host != ""
	authority := concat("", ["did:web:", replace(host, ":", "%3A")])
}

# Why a denial happened. A policy that only answers false is a policy nobody can
# operate: these are surfaced in the error the caller reports.
# An unlisted issuer fails for one of two reasons, and an operator cannot act on
# the difference unless the denial says which: a missing entry is a
# configuration edit, a chain that did not anchor is a PKI problem.
reasons contains sprintf("issuer %q is not listed, and login is granted by an entry only — this deployment names its own organization's login issuers", [input.issuer]) if {
	not listed
	input.purpose == "login"
}

reasons contains sprintf("issuer %q is not listed, and its credential carried no certificate chain reaching this deployment's %s CA list", [input.issuer, input.purpose]) if {
	not listed
	input.purpose != "login"
	not anchored_credential
}

reasons contains sprintf("issuer %q is anchored but may attest only %q, not %q", [input.issuer, peer_authority, input.organization]) if {
	not listed
	input.purpose == "peer"
	input.organization != ""
	not may_attest
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
