import { describe, expect, it } from 'vitest'
import { activeNegotiations, isActiveNegotiation } from './active-negotiations'
import type { ContractNegotiation } from '@/models/contract/contract-negotiation'

function negotiation(version: number, decision?: 'ACCEPTED' | 'REJECTED' | 'CLOSED'): ContractNegotiation {
  return {
    id: `negotiation-${version}-${decision ?? 'pending'}`,
    change_request: {},
    created_by: 'Test Party',
    created_at: '2026-07-31T00:00:00Z',
    contract_version: version,
    negotiation_decisions: [{ negotiator: 'did:web:test', decision }],
  }
}

describe('active negotiations', () => {
  it('keeps a pending proposal visible after the contract version advances', () => {
    expect(isActiveNegotiation(negotiation(1), 2)).toBe(true)
  })

  it('hides a settled proposal from an older version', () => {
    expect(isActiveNegotiation(negotiation(1, 'ACCEPTED'), 2)).toBe(false)
  })

  it('keeps proposals for the current version visible after settlement', () => {
    expect(activeNegotiations([negotiation(2, 'ACCEPTED')], 2)).toHaveLength(1)
  })
})
