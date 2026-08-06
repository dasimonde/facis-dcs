# ADR-34: A status list is signed by the issuer whose credential it governs

Status: Accepted (2026-07-31). Removes the eclipse-xfsc statuslist-service from
this project's own deployments. The XFSC status-list *mechanism* is retained on
the verifying side, because a third-party issuer may legitimately serve one.

## Context

A status list decides whether a credential is revoked. It is the one artifact a
verifier consults *after* a credential has otherwise verified, and the only
thing standing between a withdrawn authorisation and a signature accepted under
it. Everything about how it is served therefore has to survive the same
scrutiny as the credential itself.

The eclipse-xfsc statuslist-service was deployed as this project's status-list
provider. Two properties of it, one configurational and one structural, make it
unsuitable.

**It served unsigned lists here.** The service can sign — its README calls JWT
the default output — but only by delegating to a separate component: *"Optional:
Signer Service (in case for signed results)"*, reached over NATS
(`STATUSLIST_SIGNER_URL`, `STATUSLIST_SIGNER_TOPIC`; `topics.signer` in our
chart). No deployment here wired one — for the reason the next point gives —
and the service also offers a raw unsigned output, so what it served was
`application/json` —
`{tenantId, listId, list}` — an envelope with no signature at all. The verifier
was given an `xfscAllowUnsignedFallback` flag to accept it, enabled in the BDD
stack *and* on both live instances. An unsigned status list asserts a
revocation status that nobody vouched for, and anyone able to answer the URL can
write it: a network position, a DNS answer, or a misrouted ingress is enough to
un-revoke a credential. No conformant wallet or verifier can accept that, and
neither should ours.

**It cannot express an x5c chain.** This is a property of the code, not of its
configuration. `internal/api/messaging.go` builds the protected header as a map
with exactly one entry:

    var ph = make(map[string]interface{})
    ph["kid"] = did + "#" + key

`x5c` appears nowhere in the repository. There is no option to enable and no
field to populate: a certificate chain cannot reach the header the service
signs. The `kid` it does emit is a DID fragment, so resolving it means resolving
a DID — a third key-resolution model beside our certificate chains and bundled
JWKS.

The same function shows the signed path is not merely certificate-less but
unusable as written: `iat` is set with `UnixMilli()` where RFC 7519 defines
seconds, and `exp` is `time.Now().Add(time.Duration(time.Now().Year()))`, which
adds the year *as nanoseconds* — every signed token expires roughly two
microseconds after it is issued. A conformant verifier refuses them as expired.
That is the likely reason no deployment ever wired the signer: the unsigned
output was the only one that worked, and the fallback kept anyone from noticing.

The same README states the service *"should not be directly public"*. A status
list is fetched by every verifier that checks a credential, so a component whose
own documentation says it should not be publicly reachable is a poor fit for the
one artifact that must be.

The PKI this project demonstrates is certificate-based. A PID credential is
identified by an `x5c` chain verified to a trust anchor (ADR-31); the demo PID
issuer and the login issuer both sign that way; `iss` is an https URI a
Subject Alternative Name attests. A status list whose key is identified by `kid`
cannot participate in that chain of reasoning: the verifier ends up trusting the
credential by certificate and its revocation status by a bundled key, with no
relationship between the two. The status list is exactly where that relationship
matters most.

Both failures were invisible while the fallback existed. It absorbed every
signature and retrieval error, so no deployment ever discovered that its status
lists were unsigned, and no test ever exercised a list signed by an x5c issuer —
the BDD PID credentials point their status URI at a third party's service, and
the login credentials pointed at the unsigned one. The first x5c-signed status
list this system ever verified was in production, where it failed and blocked
every signing ceremony.

## Decision

**A status list is served by the issuer that issued the credential, signed with
the same key, and identified the same way.**

1. **This project operates no statuslist-service.** The subchart, its database,
   its ingress routes and its dev seeding are removed. Our own status lists are
   served by the issuer's own flow, which signs ES256 with `typ:
   statuslist+jwt`, carries the issuer's `x5c` chain, sets `iss` to the issuer's
   https URI, persists its bits, and exposes an `/admin` endpoint so a list stays
   adjustable — which is what makes a revocation test prove anything.

2. **An unsigned status list is never accepted.** Not by configuration, not in
   development, not behind a flag. The unsigned code path and the flag that
   reached it are deleted rather than defaulted off, so no deployment can
   re-enable it and no future reader can mistake it for a supported mode.

3. **A status list is verified the way the credential is.** When the token
   carries an `x5c` header its chain is verified against the configured trust
   anchors, and the leaf must identify the issuer the token names — the same
   binding a credential is held to, without which any certificate under any
   configured anchor could publish any issuer's revocation status. A token
   without a chain still resolves against a bundled JWKS.

4. **The XFSC mechanism stays supported on the verifying side.** A counterparty
   or third-party issuer may serve a `kid`-identified list, and after (2) and (3)
   we check its signature properly instead of falling back. We simply stop
   operating one.

## Consequences

Development, CI and production converge on one status-list model for the first
time. That is the point: the defect above reached the demo instances because CI
exercised a different one, so a signed-list bug could not fail in CI.

The trust anchors an instance mounts must cover **every** issuer whose signature
it verifies, which is more than one. Each ORCE issuer mints its own root at
runtime, and they all carry the subject `CN = FACIS Demo Root CA` while having
different keys — a bundle holding only one silently refuses the others, and
every log line names the same CA. Anchors are therefore assembled from what each
issuer is actually serving and compared by fingerprint, never by subject
(`tmp/redeploy/build-x5c-anchors.py`). Mounting a single root is what made a
correctly-signed login list unverifiable while looking correctly configured.

Anchor assembly happens after the issuers are serving, since a root minted at
runtime cannot be read before its issuer is up. A deployment that re-mints a root
— a wiped volume, a fresh install — needs its anchors rebuilt; until then
verification fails closed, which is the correct direction to fail but gives an
error naming only the CA's subject.

Revocation scenarios move to the issuer's `/admin`. A revocation test has to
distinguish a set bit from a list that failed to load: with the fallback gone,
both refuse the credential, and only one of them proves anything.

Six tests in `internal/auth/oid4vp` passed only because the fallback rescued
their unsigned fixtures. They serve a signed list now. Their previous state is
the clearest statement of the problem this ADR closes: the suite was green
because the fixture avoided the case.

## Alternatives considered

**Wire the Signer Service into the statuslist-service.** Documented upstream and
closes the unsigned hole. Rejected on the source: the tokens it produces expire
microseconds after issuance (`exp` adds the year as nanoseconds) and carry a DID
`kid` and no chain, so they are neither verifiable by a conformant verifier nor
expressible in this project's PKI. Making it work would mean patching upstream's
signing path and running an additional component, to reach a position the
issuer's own flow already occupies.

**Keep the fallback for development only.** The smallest change, and it keeps
CI green today. Rejected outright: it is the arrangement that hid both defects.
A development stack that accepts what production must refuse cannot test
production's behaviour, and the difference surfaces where it is most expensive.

**Have the DCS serve status lists for its own issuers.** It already holds an HSM
key and could sign them. Rejected: a status list is the issuer's statement about
credentials it issued, and the relying party issuing it is the same conflation
ADR-31 removed for PID issuance. The issuer signs its own.

## Scope note: the demo issuers

The ORCE issuers this project deploys are stand-ins. They mint their own root at
runtime, hold no legally meaningful key, and a real deployment replaces them with
the counterparty's own credential issuance — the DCS decides whether to trust an
issuer (ADR-31), it does not operate one.

Properties of those stand-ins are therefore out of scope here and are not
defects to fix: the unauthenticated /pki/reissue endpoint, the unauthenticated
/admin revocation endpoints enabled in the shared values base, and the issuer
minting a leaf for whatever base URL a request carries. None survives contact
with a real issuer, and the guard that stops a real deployment inheriting demo
material already exists — DCS_ALLOW_DEV_TRUST is set in values.bdd.yml alone.

What is NOT in that category, and remains a defect: no status-list handler binds
the list's issuer to the issuer of the credential it governs. All three discard
the credential argument (handler.IETFToken, handler.XFSC, handler.W3CBitstring
take `_ status.VerifiedCredential`). That is this project's verification logic,
not the stand-in's key hygiene: it lets any trusted issuer publish revocation
status for any other issuer's credential, and it is the sentence this ADR is
named after. Replacing the demo issuers does not fix it.

## Amendment: the DCS serves the status lists for the credentials it issues

The decision above is a rule about WHO signs, and applying it consistently means
the DCS hosts a status list of its own.

The ORCE issuer issues PID and PoA credentials, so it serves their status list.
The DCS issues the contract lifecycle credential embedded in every generated PDF
and the signature evidence / signing summary VC embedded in a contract's PDF —
it mints them, embeds them, and asserts them. By the same rule, the DCS serves
their status list, signed with its own HSM key and carrying its own certificate
chain (the hsm-provision job already produces c2pa-x5chain.pem beside the signing
key, so this needs no new key ceremony).

This is NOT a reversal of the alternative rejected above. That rejection is about
the DCS serving status lists for the ORCE ISSUERS' credentials — a relying party
publishing revocation for credentials it did not issue, which is the
attests-to-itself conflation ADR-31 removed. Serving lists for credentials the
DCS itself issued is that same principle applied, not an exception to it.

Consequence, and the reason this is written down now: the XFSC statuslist-service
CANNOT be removed until this lands. The contract lifecycle credential still
allocates against it (OCMWStatusListPublisher, /v1/tenants/<tenant>/status/<n>)
and its revocation status is read back UNSIGNED by ReadUnsignedStatusList
(pdfgeneration/provenance/status_list.go), consumed by
signingmanagement/query/verify.go and pdfgeneration/query/common.go — the C2PA
provenance verification path. Deleting the subchart first would pass every unit
test and break PDF verification, because nothing asserts that credential's
revocation status end to end.

Order is therefore: DCS serves its own signed list -> lifecycle and signature
evidence credentials point at it -> the XFSC service has no consumers -> the
subchart, its database, ingress and seeding are deleted.
