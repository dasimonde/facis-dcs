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

The issuer JWT header embeds only the issuer public key material:

```json
{
  "alg": "ES256",
  "typ": "dc+sd-jwt",
  "jwk": {
    "kty": "EC",
    "crv": "P-256",
    "x": "...",
    "y": "..."
  }
}
```

DCS verifier logic should be:

```text
read issuer header.jwk
check header.jwk matches the trusted issuer public key in trust.dev.json
verify issuer signature with header.jwk
read payload.cnf.jwk
verify Key Binding JWT with cnf.jwk
```

`header.jwk` is not trusted by itself. It is only a public-key candidate and must match the trust list first.

## Claims shape

Visible issuer claims:

```json
{
  "iss": "did:web:dev.example:issuer:poa",
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
  --trust-path testWallet/trust.dev.json \
  --aud dcs-client \
  --nonce test-nonce
```

Expected result:

```text
issuer header.jwk trusted: OK
issuer signature: OK
key binding signature: OK
key binding sd_hash: OK
```

## sdjwt.co

Paste `testWallet/credentials/test.jwt` directly.

For manual verification inputs:

- `Signature(Input JWK to verify)`: `testWallet/keys/issuer-dev.public.jwk`
- `Key Binding Signature(Input JWK to verify)`: `testWallet/keys/wallet.public.jwk`

Do not use the demo key pre-filled by sdjwt.co.
