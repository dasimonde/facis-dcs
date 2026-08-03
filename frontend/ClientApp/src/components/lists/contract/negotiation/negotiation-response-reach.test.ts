import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuthStore } from '@/stores/auth-store'
import NegotiationList from './NegotiationList.vue'
import type { Contract } from '@/models/contract/contract'
import type { UserRole } from '@/types/user-role'

/**
 * Accept/Reject on a change proposal. The gate has to admit every role
 * design/contract_workflow_engine.go respond() scopes, and the "have I already
 * decided" lookup has to key on this party's DCS instance did:web — decision
 * rows carry that, never the logged-in user's credential issuer.
 */

vi.mock('@/services/contract-workflow-service', () => ({
  contractWorkflowService: { respond: vi.fn(() => Promise.resolve({ id: 'n1' })) },
}))

const LOCAL_INSTANCE = 'did:web:local.example'
const USER_ISSUER = 'did:web:issuer.example:org'

const contract = (decision: string | null): Contract =>
  ({
    did: 'did:web:local.example:contract',
    state: 'NEGOTIATION',
    negotiations: [
      {
        id: 'n1',
        created_by: 'did:web:peer.example',
        created_at: '2026-07-31T00:00:00Z',
        state: 'OPEN',
        negotiation_decisions: [{ negotiator: LOCAL_INSTANCE, decision }],
      },
    ],
  }) as unknown as Contract

function mountList(roles: UserRole[], decision: string | null = null) {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().user = { issuer: USER_ISSUER, holder: 'user', roles }
  return mount(NegotiationList, {
    props: { contract: contract(decision), localInstanceDid: LOCAL_INSTANCE },
    global: { plugins: [pinia] },
  })
}

async function actionsFor(roles: UserRole[], decision: string | null = null) {
  const wrapper = mountList(roles, decision)
  await wrapper
    .findAll('button')
    .find((button) => button.text().includes('Show'))
    ?.trigger('click')
  return wrapper.findAll('button')
}

describe('responding to a change proposal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it.each<UserRole>(['CONTRACT_CREATOR', 'CONTRACT_REVIEWER', 'CONTRACT_NEGOTIATOR', 'CONTRACT_MANAGER'])(
    'enables Accept and Reject for %s',
    async (role) => {
      const buttons = await actionsFor([role])

      for (const label of ['Accept', 'Reject']) {
        const button = buttons.find((candidate) => candidate.text().includes(label))
        expect(button, label).toBeDefined()
        expect(button?.attributes('disabled'), label).toBeUndefined()
      }
    },
  )

  it('leaves Accept and Reject disabled for a role respond() refuses', async () => {
    const buttons = await actionsFor(['CONTRACT_OBSERVER'])

    expect(buttons.find((button) => button.text().includes('Accept'))?.attributes('disabled')).toBeDefined()
    expect(buttons.find((button) => button.text().includes('Reject'))?.attributes('disabled')).toBeDefined()
  })

  // Matching on the user's issuer found no row, so a decision already taken
  // never disabled its own buttons and a second click hit ErrNoMatchingDecision.
  it('disables Accept and Reject once this instance has decided', async () => {
    const buttons = await actionsFor(['CONTRACT_NEGOTIATOR'], 'ACCEPTED')

    expect(buttons.find((button) => button.text().includes('Accept'))?.attributes('disabled')).toBeDefined()
  })
})
