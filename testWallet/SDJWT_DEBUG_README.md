# SD-JWT+KB local debugging

`credentials/*.jwt` are complete SD-JWT+KB tokens generated from `credentials/*.template.json`.
There is no `presentations/` directory.

## OpenID4VP wire format (current)

`demo_wallet.py` now follows OpenID4VP 1.0 request-by-reference and direct_post:

- Fetch request object: `POST request_uri` with form body `wallet_nonce` + `wallet_metadata`
- Verify request JWT header/payload:
  - `typ == oauth-authz-req+jwt`
  - header `jwk` is present and verifies ES256 signature
  - payload `wallet_nonce` exactly echoes the sent nonce
  - `exp` is valid
- Submit presentation: `POST response_uri` with `application/x-www-form-urlencoded` fields:
  - `state=<request-object-state>`
  - `vp_token=<json object string>`

`vp_token` is not a bare SD-JWT string. It is a JSON object keyed by DCQL query id:

```json
{
  "dcs_poa_credential": [
    "<sd-jwt>~<disclosure>~<kb-jwt>"
  ]
}
```

## Clear entry points

Generate or refresh keys and trust list:

```bash
python3 testWallet/scripts/generate_keys.py --yes
```

Issue credentials from templates:

```bash
python3 testWallet/scripts/issue_credentials.py
```

Issue only one template:

```bash
python3 testWallet/scripts/issue_credentials.py --credential test
```

Issue one credential directly from roles, useful for feature tests:

```bash
python3 testWallet/scripts/issue_credentials.py \
  --name test \
  --organization "Acme Corp" \
  --roles "Contract Manager,Contract Signer"
```

## Issuer key resolution

The issuer JWT header carries the issuer's certificate chain, leaf first —
the same shape the ORCE credential issuer signs with:

```json
{
  "alg": "ES256",
  "typ": "dc+sd-jwt",
  "x5c": ["<leaf DER, base64>", "<dev root CA DER, base64>"]
}
```

DCS verifier logic for a `login` credential is (ADR-35):

```text
read the issuer x5c chain, take its leaf
check the leaf carries a key the trust entry for payload.iss pins
check the leaf names payload.iss (SAN URI, SAN DNS authority, or exact CN)
verify issuer signature with the leaf's key
read payload.cnf.jwk
verify Key Binding JWT with cnf.jwk
```

The chain is not trusted by itself, and for `login` no certificate authority is
consulted at all: the leaf must carry the key the operator wrote down, or a CA
in the trust list could introduce another login issuer.

## Claims shape

Visible issuer claims:

```json
{
  "iss": "http://localhost:30181",
  "sub": "did:jwk:...holder...",
  "vct": "urn:dcs:poa:v1",
  "iat": 1719129600,
  "exp": 1893456000,
  "cnf": {
    "jwk": {
      "kty": "EC",
      "crv": "P-256",
      "x": "...",
      "y": "..."
    }
  },
  "_sd": ["..."],
  "_sd_alg": "sha-256"
}
```

`organization` and `roles` are selectively disclosed.

## Regenerate credentials

Keep keys and templates, rewrite only `credentials/*.jwt`:

```bash
rm -f testWallet/credentials/*.jwt
python3 testWallet/scripts/issue_credentials.py
```

Regenerate keys, trust, and credentials:

```bash
python3 testWallet/scripts/generate_keys.py --regenerate --yes
python3 testWallet/scripts/issue_credentials.py
```

## Local verification

```bash
python3 testWallet/scripts/verify_sdjwt_locally.py \
  testWallet/credentials/test.jwt \
  --trust-path backend/config/oid4vp/trust.dev.json \
  --aud dcs-client \
  --nonce test-nonce
```

Expected result:

```text
issuer leaf pinned and names its issuer: OK
issuer signature: OK
key binding signature: OK
key binding sd_hash: OK
```

## sdjwt.co

Paste `testWallet/credentials/test.jwt` directly.

For manual verification inputs:

- `Signature(Input JWK to verify)`: the public key of the leaf in the credential's `x5c` header (`deployment/helm/charts/orce/pki-dev/issuer.key`)
- `Key Binding Signature(Input JWK to verify)`: `testWallet/keys/wallet.public.jwk`

Do not use the demo key pre-filled by sdjwt.co.
