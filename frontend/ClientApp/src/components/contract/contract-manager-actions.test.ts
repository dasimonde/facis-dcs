import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it, vi } from 'vitest'
import { useAuthStore } from '@/stores/auth-store'
import ContractManagerActions from './ContractManagerActions.vue'
import type { Contract } from '@/models/contract/contract'
import type { ContractState } from '@/types/contract-state'
import type { UserRole } from '@/types/user-role'

/**
 * The off-ramps a contract has from the list and detail pages. WITHDRAW and
 * RENEW are full backend actions (design withdraw/renew, transition.go
 * EventWithdraw, command/renew.go renewableStates) that no control reached.
 */

vi.mock(import('vue-router'), async (importOriginal) => ({
  ...(await importOriginal()),
  useRouter: () => ({ push: vi.fn(), go: vi.fn(), back: vi.fn() }) as never,
}))

vi.mock('@/services/contract-workflow-service', () => ({
  contractWorkflowService: {},
}))

function mountActions(roles: UserRole[], state: ContractState) {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().user = { issuer: 'did:web:example.com:org', holder: 'user', roles }
  const contract = { did: 'did:web:example.com:contract', state, updated_at: '2026-07-31T00:00:00Z' } as Contract
  return mount(ContractManagerActions, { props: { contract }, global: { plugins: [pinia] } })
}

const labels = (wrapper: ReturnType<typeof mountActions>) => wrapper.findAll('button').map((button) => button.text())

describe('contract off-ramps', () => {
  // transition.go allows EventWithdraw from exactly these four and refuses it
  // once APPROVED; design withdraw() scopes Contract Creator.
  it.each<ContractState>(['OFFERED', 'NEGOTIATION', 'SUBMITTED', 'REVIEWED'])(
    'offers Withdraw to the creator in %s',
    (state) => {
      expect(labels(mountActions(['CONTRACT_CREATOR'], state))).toContain('Withdraw')
    },
  )

  it('does not offer Withdraw once the contract is approved', () => {
    expect(labels(mountActions(['CONTRACT_CREATOR'], 'APPROVED'))).not.toContain('Withdraw')
  })

  it('does not offer Withdraw to a manager, whom the endpoint does not scope', () => {
    expect(labels(mountActions(['CONTRACT_MANAGER'], 'OFFERED'))).not.toContain('Withdraw')
  })

  // command/renew.go renewableStates; design renew() scopes Contract Manager.
  it.each<ContractState>(['APPROVED', 'SIGNED', 'ACTIVE', 'TERMINATED', 'EXPIRED'])(
    'offers Renew to the manager in %s',
    (state) => {
      expect(labels(mountActions(['CONTRACT_MANAGER'], state))).toContain('Renew')
    },
  )

  it('does not offer Renew before the contract is approved', () => {
    expect(labels(mountActions(['CONTRACT_MANAGER'], 'NEGOTIATION'))).not.toContain('Renew')
  })

  // The responder instance drives its inbound contracts as Contract Manager,
  // and this button is the only route into the negotiate view from OFFERED.
  it('lets a manager open a received offer', () => {
    expect(labels(mountActions(['CONTRACT_MANAGER'], 'OFFERED'))).toContain('Review offer')
  })
})
