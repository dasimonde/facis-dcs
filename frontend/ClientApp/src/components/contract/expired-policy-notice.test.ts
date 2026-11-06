import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import ExpiredPolicyNotice from './ExpiredPolicyNotice.vue'
import type { Contract, ExpirationPolicy } from '@/models/contract/contract'
import type { ContractState } from '@/types/contract-state'

function notice(state: ContractState, exp_policy?: ExpirationPolicy) {
  const contract = { did: 'did:web:example.com:contract', state, exp_policy } as Contract
  return mount(ExpiredPolicyNotice, { props: { contract } }).find('[data-testid="expired-policy-notice"]')
}

// DCS-FR-CWE-10: an expired contract names the workflow its policy configures.
describe('expired policy notice', () => {
  it.each<[ExpirationPolicy, string]>([
    ['RENEWAL', 'renews it'],
    ['TERMINATION', 'terminates it'],
    ['ARCHIVING', 'stay in the archive'],
  ])('names the next step for %s', (policy, step) => {
    const found = notice('EXPIRED', policy)
    expect(found.exists()).toBe(true)
    expect(found.text()).toContain(step)
  })

  it('stays hidden without a policy', () => {
    expect(notice('EXPIRED').exists()).toBe(false)
  })

  it('stays hidden before expiry', () => {
    expect(notice('ACTIVE', 'RENEWAL').exists()).toBe(false)
  })
})
