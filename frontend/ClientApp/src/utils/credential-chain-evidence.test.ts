import { describe, expect, it } from 'vitest'
import { poaRolesDisplay, signerDisplayName, summaryIssuerDisplay } from './credential-chain-evidence'

describe('credential-chain-evidence helpers', () => {
  it('prefers cert subject for display name', () => {
    expect(
      signerDisplayName({
        signer_did: 'did:jwk:abc',
        signer_cert_subject: 'DCS Signatory MAX',
      }),
    ).toBe('DCS Signatory MAX')
    expect(signerDisplayName({ signer_did: 'did:jwk:abc' })).toBe('did:jwk:abc')
  })

  it('returns signing-summary issuer without inventing values', () => {
    expect(summaryIssuerDisplay({ signer_did: 'x', summary_issuer: 'did:web:localhost%3A8991' })).toBe(
      'did:web:localhost%3A8991',
    )
    expect(summaryIssuerDisplay({ signer_did: 'x' })).toBeNull()
  })

  it('lists all PoA roles without inventing values', () => {
    expect(poaRolesDisplay({ signer_did: 'x', poa_roles: ['Contract Signer'] })).toBe('Contract Signer')
    expect(poaRolesDisplay({ signer_did: 'x', poa_roles: ['Contract Signer', 'Auditor'] })).toBe(
      'Contract Signer, Auditor',
    )
    expect(poaRolesDisplay({ signer_did: 'x' })).toBeNull()
  })
})
