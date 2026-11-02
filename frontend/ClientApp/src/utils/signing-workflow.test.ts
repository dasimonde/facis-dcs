import { describe, expect, it } from 'vitest'
import { buildContractDocument } from '@template-repository/store/dcsDraftStore'
import { signingWorkflow } from './signing-workflow'
import type { DcsContractData } from '@/models/dcs-jsonld'

describe('signing workflow (DCS-FR-SM-13)', () => {
  it('reads the signing order and the deadline from the contract document', () => {
    const workflow = signingWorkflow({
      'dcs:signatureFields': [
        { 'dcs:signatoryName': 'did:web:b', 'dcs:signingOrder': 2 },
        { 'dcs:signatoryName': 'did:web:a', 'dcs:signingOrder': { '@value': '1', '@type': 'xsd:integer' } },
        { 'dcs:signatoryName': 'did:web:c' },
      ],
      'dcs:signingDeadline': { '@value': '2026-11-30', '@type': 'xsd:date' },
    })
    expect(workflow.order).toEqual([
      { field: 'did:web:a', position: 1 },
      { field: 'did:web:b', position: 2 },
    ])
    expect(workflow.deadline?.toISOString()).toBe('2026-11-30T23:59:59.999Z')
  })

  it('keeps the signing deadline when the editor rebuilds the contract document', () => {
    const storedDocument = {
      '@type': 'dcs:Contract',
      'dcs:signingDeadline': { '@value': '2026-11-30T12:00:00Z', '@type': 'xsd:dateTime' },
    } as unknown as DcsContractData
    const rebuilt = buildContractDocument({
      documentId: 'did:web:a:contract:1',
      storedDocument,
      blocks: [],
      layout: [],
      contractFields: [],
      contractData: [],
      policies: [],
      semanticConditionValues: [],
    })
    expect(rebuilt['dcs:signingDeadline']).toEqual(storedDocument['dcs:signingDeadline'])
  })
})
