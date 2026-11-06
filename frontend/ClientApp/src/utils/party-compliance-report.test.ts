import { describe, expect, it } from 'vitest'
import {
  buildPartyComplianceReport,
  findingMentionsSigner,
  partitionFindingsBySigner,
  partyAssessment,
} from './party-compliance-report'
import type { SignatureViewItem } from '@/services/signature-management-service'

function sig(partial: Partial<SignatureViewItem> & Pick<SignatureViewItem, 'signer_did'>): SignatureViewItem {
  return {
    credential_type: 'AES',
    status: 'SIGNED',
    format: 'PAdES',
    ...partial,
  }
}

describe('findingMentionsSigner', () => {
  it('matches signer DID case-insensitively', () => {
    expect(findingMentionsSigner('Signer did:jwk:ABC has a revoked signature', 'did:jwk:abc')).toBe(true)
    expect(findingMentionsSigner('Compliance checks passed', 'did:jwk:abc')).toBe(false)
  })
})

describe('partitionFindingsBySigner', () => {
  const a = sig({ signer_did: 'did:jwk:a' })
  const b = sig({ signer_did: 'did:jwk:b' })

  it('attributes findings that name exactly one signer', () => {
    const { bySigner, unassigned } = partitionFindingsBySigner(
      [
        'Signer did:jwk:a has a revoked signature',
        'Compliance checks passed',
        'No active signed credentials satisfy compliance checks',
      ],
      [a, b],
    )
    expect(bySigner.get('did:jwk:a')).toEqual(['Signer did:jwk:a has a revoked signature'])
    expect(bySigner.get('did:jwk:b')).toEqual([])
    expect(unassigned).toEqual(['Compliance checks passed', 'No active signed credentials satisfy compliance checks'])
  })

  it('keeps ambiguous multi-DID findings at contract level', () => {
    const { unassigned } = partitionFindingsBySigner(['Conflict between did:jwk:a and did:jwk:b'], [a, b])
    expect(unassigned).toEqual(['Conflict between did:jwk:a and did:jwk:b'])
  })
})

describe('partyAssessment', () => {
  it('does not treat SIGNED alone as compliant', () => {
    const assessment = partyAssessment(sig({ signer_did: 'did:jwk:a', status: 'SIGNED', credential_type: 'AES' }), [])
    expect(assessment.toLowerCase()).not.toContain('compliant')
    expect(assessment).toMatch(/meets requirement/i)
    expect(assessment).toMatch(/no party-specific findings/i)
  })

  it('flags revoked and below-requirement levels', () => {
    expect(partyAssessment(sig({ signer_did: 'x', status: 'REVOKED' }), []).toLowerCase()).toContain('revoked')
    expect(
      partyAssessment(sig({ signer_did: 'x', credential_type: 'SES', required_credential_type: 'AES' }), []),
    ).toMatch(/below requirement/i)
  })
})

describe('buildPartyComplianceReport', () => {
  it('builds only local signatures and keeps unassigned findings contract-level', () => {
    const report = buildPartyComplianceReport(
      [
        sig({
          signer_did: 'did:jwk:a',
          signer_cert_subject: 'DCS Signatory MAX',
          poa_organization: 'did:web:localhost%3A8991',
          poa_roles: ['Contract Signer'],
          signed_at: '2026-10-13T13:39:00Z',
        }),
      ],
      ['Signer did:jwk:a uses unknown credential type X', 'Compliance checks passed'],
      [],
    )
    expect(report.scope_note).toMatch(/current DCS instance/)
    expect(report.parties).toHaveLength(1)
    expect(report.parties[0]!.signer).toBe('DCS Signatory MAX')
    expect(report.parties[0]!.organization).toBe('did:web:localhost%3A8991')
    expect(report.parties[0]!.authorized_role).toBe('Contract Signer')
    expect(report.parties[0]!.non_conformance).toEqual(['Signer did:jwk:a uses unknown credential type X'])
    expect(report.contract_findings).toEqual(['Compliance checks passed'])
  })
})
