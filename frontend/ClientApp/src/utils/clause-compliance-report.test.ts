import { describe, expect, it } from 'vitest'
import {
  buildClauseComplianceReport,
  contentFindingsFromPacTimeline,
  extractClausesFromContractData,
  parseApplicableChecks,
  partitionFindingsByClause,
} from './clause-compliance-report'

describe('parseApplicableChecks', () => {
  it('extracts label and taxonomy id for hover', () => {
    const amount = 'https://w3id.org/facis/dcs/taxonomy/v1#field-contract-payment-amount'
    const currency = 'https://w3id.org/facis/dcs/taxonomy/v1#field-contract-payment-currency'
    expect(parseApplicableChecks(`Payment Amount (${amount}), Payment Currency (${currency})`)).toEqual([
      { label: 'Payment Amount', id: amount },
      { label: 'Payment Currency', id: currency },
    ])
  })

  it('keeps bare shape names and status tokens', () => {
    expect(parseApplicableChecks('PaymentClauseShape')).toEqual([{ label: 'PaymentClauseShape' }])
    expect(parseApplicableChecks('None')).toEqual([{ label: 'None' }])
    expect(parseApplicableChecks('Not established')).toEqual([{ label: 'Not established' }])
  })
})

describe('extractClausesFromContractData', () => {
  it('lists dcs:Clause blocks by @id', () => {
    const clauses = extractClausesFromContractData({
      'dcs:documentStructure': {
        'dcs:blocks': {
          '@list': [
            { '@id': 'urn:uuid:c1', '@type': 'dcs:Clause', 'dcs:title': 'Availability' },
            { '@id': 'urn:uuid:s1', '@type': 'dcs:Section', 'dcs:title': 'Section' },
          ],
        },
      },
    })
    expect(clauses).toEqual([{ id: 'urn:uuid:c1', title: 'Availability' }])
  })
})

describe('partitionFindingsByClause', () => {
  const clauses = [
    { id: 'urn:uuid:c1', title: 'A' },
    { id: 'urn:uuid:c2', title: 'B' },
  ]

  it('attributes only when exactly one clause @id appears', () => {
    const { byClause, unassigned } = partitionFindingsByClause(
      [
        { message: 'Focus node urn:uuid:c1 failed availability' },
        { message: 'Global policy breach' },
        { message: 'Conflict urn:uuid:c1 vs urn:uuid:c2' },
      ],
      clauses,
    )
    expect(byClause.get('urn:uuid:c1')).toHaveLength(1)
    expect(byClause.get('urn:uuid:c2')).toEqual([])
    expect(unassigned).toHaveLength(2)
  })
})

describe('contentFindingsFromPacTimeline', () => {
  it('reads CONTRACT_CONTENT_POLICY_AUDIT_FINDING event_data', () => {
    const findings = contentFindingsFromPacTimeline([
      {
        event_type: 'CONTRACT_CONTENT_POLICY_AUDIT_FINDING',
        event_data: {
          message: 'bad value on urn:uuid:c1',
          path: 'availability',
          fieldIri: 'https://example/availability',
          ruleId: 'r1',
          severity: 'error',
        },
      },
    ])
    expect(findings).toEqual([
      {
        rule_id: 'r1',
        severity: 'error',
        message: 'bad value on urn:uuid:c1',
        path: 'availability',
        field_iri: 'https://example/availability',
        result: undefined,
      },
    ])
  })
})

describe('buildClauseComplianceReport', () => {
  it('keeps role-gated unavailability reasons', () => {
    const report = buildClauseComplianceReport({
      clauses: [],
      findings: [],
      clausesAvailable: false,
      findingsAvailable: false,
      clausesUnavailableReason: 'needs retrieve',
      findingsUnavailableReason: 'needs pac',
    })
    expect(report.clauses_unavailable_reason).toBe('needs retrieve')
    expect(report.findings_unavailable_reason).toBe('needs pac')
  })
})
