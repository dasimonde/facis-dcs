# testWallet

A headless wallet+QTSP stand-in for local dev, CI, and BDD: it plays the
signatory's wallet across the standard OID4VP interface the DCS speaks
(login, Power of Attorney presentation, PID presentation, and the
Document-Retrieval signing ceremony), the same interface a real EUDI wallet
would use. Nothing DCS-specific crosses that boundary — swapping in a real
wallet is a configuration change, not a code change (see
[docs/adr-12-wallet-driven-signing.md](../docs/adr-12-wallet-driven-signing.md)).

## Setup

```bash
cd testWallet
pip install -r requirements.txt
python3 scripts/generate_keys.py --yes   # wallet.jwk, issuer-dev.jwk, wallet-ca (signing certs)
python3 scripts/check_status_list.py     # the issuer serves the list the credentials name
python3 scripts/issue_credentials.py     # PoA + self-issued PID credentials from credentials/*.template.json
```

## Dev PID workflow (ADR-20)

The remote EUDIPLO PID service is broken and is not a dependency of this
DCS. PID credentials for dev/test are **self-issued locally**:

- `scripts/issue_pid_credentials.py` self-signs a PID SD-JWT VC per
  `credentials/*.pid.template.json` (one `given_name`/`family_name`/... claim
  set each), using the SAME local signing primitives
  (`dcs_wallet.issuer.sign_credential_sd_jwt`) already used for the Power of
  Attorney credential — no remote call. It signs under
  `did:web:dev.example:issuer:poa` (`keys/issuer-dev.jwk`), the SAME dev
  issuer key/DID `backend/config/oid4vp/trust.dev.json` already trusts for
  PoA — reused, not a new identity, to keep dev key management to one
  keypair.
- **This is a dev-only substitution and must never be pointed at from a
  production trust store.** A production deployment configures
  `OID4VP_TRUST_DATA_PATH` at a real PID issuer registry's public keys
  instead — the DCS's verification code
  (`oid4vp.Verifier.VerifyPID`/`sdjwt.VerifyCredentialForPID`) is unchanged
  either way; only the trust anchor changes, and only via configuration.
- Self-issued PIDs carry a real status-list claim
  (`dcs_wallet.status_list.build_credential_status`), so the DCS's SM-18
  status check (`checkStatusList` in `VerifyPID`) runs for real. Revoke one
  for the negative test with `scripts/revoke_statuslist_index.py --credential
  credentials/<name>.pid.jwt`, which posts to the issuer's own admin endpoint
  and reads the bit back.

### Status list

Every credential here points at the ORCE issuer's own signed list,
`<ISSUER_BASE_URL>/status-list/1` (ADR-34). Each credential holds an index of
its own, allocated in `dcs_wallet/status_list.py`: the committed credential
files have a named entry each, identities a test mints get one derived from
the identity, and identities a scenario revokes are reserved by name. Sharing
a bit would mean revoking one credential revokes the other, so issuance
refuses a committed credential with no allocation of its own.

### Cert↔PID name alignment

The signing certificate the wallet uses for a ceremony
(`dcs_wallet/signer.py`'s `ensure_signing_material`) carries `GIVEN_NAME`/
`SURNAME` in its subject — the DCS's sole-control gate (ADR-20) checks these
against the ceremony's verified PID's `given_name`/`family_name`, mandatory
for QES and enabled by default for AES. **These must match** for a ceremony
to succeed:

- In BDD, the convention is `given_name = <signatory label>`,
  `family_name = "BDD-Testperson"` for every ceremony-driven scenario, and
  `ensure_signing_material` defaults to exactly that — no caller needs to
  pass names explicitly for the happy path.
- Using a real `*.pid.jwt` fixture (from `issue_pid_credentials.py`) instead,
  the certificate must be minted with that fixture's actual
  `given_name`/`family_name` (`ensure_signing_material(user, keys_dir,
  given_name=..., family_name=...)`, and the same override plumbed through
  `sign_pdf`/`sign_jades_payload`/`sign_via_document_retrieval`).
- To mint a **deliberately mismatched** certificate for the negative test,
  pass `given_name`/`family_name` that do NOT match the ceremony's PID, under
  a `user` label distinct from that identity's normal one (key/cert material
  is cached per `user`, so reusing the same label would return the cached,
  matching cert instead of minting a fresh mismatched one).

## Other scripts

- `scripts/issue_credentials.py` — issues PoA + PID credentials together
  from `credentials/*.template.json` / `*.pid.template.json`.
- `scripts/issue_vp_jwt.py`, `scripts/verify_sdjwt_locally.py`,
  `scripts/verify_statuslist_credentials.py` — presentation/verification
  debugging tools.
- `scripts/clean_dev_keys.sh` — wipe generated dev keys to start over.
- `demo_wallet.py` — interactive wallet demo, including against external
  third-party OpenID4VP verifiers for wallet-side testing (unrelated to any
  DCS backend dependency).

See [SDJWT_DEBUG_README.md](./SDJWT_DEBUG_README.md) for the SD-JWT+KB wire
format and lower-level presentation debugging.
