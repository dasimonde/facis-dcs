import { shallowMount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { useAuthStore } from '@/stores/auth-store'
import NegotiateContractView from './NegotiateContractView.vue'
import type { UserRole } from '@/types/user-role'

/**
 * The counterparty's route into a received offer. Before this, the only forward
 * control on an OFFERED contract was "Change Proposal", which is disabled until
 * something has been edited — so an offer that needed no changes could not be
 * taken forward at all, and no negotiation task was ever minted for it.
 */

vi.mock(import('vue-router'), async (importOriginal) => ({
  ...(await importOriginal()),
  useRoute: () => ({ params: { did: 'did:web:example.com:contract' } }) as never,
}))

const LOCAL_PEER = 'did:web:counterparty.example'
const ORIGIN_PEER = 'did:web:origin.example'

const getLocalDIDFile = vi.fn(() => Promise.resolve({ id: LOCAL_PEER }))

vi.mock('@/services/did-service', () => ({
  getLocalDIDFile: (...args: unknown[]) => getLocalDIDFile(...(args as [])),
  resolveLocalInstanceDid: async () => (await getLocalDIDFile()).id,
}))

const contractDocument = {
  '@type': 'dcs:Contract',
  '@id': 'did:web:example.com:contract',
  'dcs:metadata': { '@type': 'dcs:ContractMetadata' },
  'dcs:documentStructure': {
    '@type': 'dcs:DocumentStructure',
    'dcs:blocks': { '@list': [] },
    'dcs:layout': { '@list': [] },
  },
  'dcs:contractFields': [],
  'dcs:contractData': [],
}

const acceptOffer = vi.fn(() => Promise.resolve({ did: 'did:web:example.com:contract' }))
const negotiate = vi.fn(() => Promise.resolve({ did: 'did:web:example.com:contract' }))

const offeredContract = (counterparty: string) => ({
  did: 'did:web:example.com:contract',
  name: 'Supply agreement',
  description: '',
  state: 'OFFERED',
  updated_at: '2026-07-31T00:00:00Z',
  contract_data: contractDocument,
  negotiations: [],
  responsible: {
    creator: ORIGIN_PEER,
    approvers: [ORIGIN_PEER],
    reviewers: [ORIGIN_PEER],
    negotiators: [ORIGIN_PEER],
    counterparty,
  },
})

vi.mock('@/services/contract-workflow-service', () => ({
  contractWorkflowService: {
    retrieveById: () => Promise.resolve(offeredContract(LOCAL_PEER)),
    retrieveNegotiationDraft: () => Promise.resolve(null),
    acceptOffer: (...args: unknown[]) => acceptOffer(...(args as [])),
    negotiate: (...args: unknown[]) => negotiate(...(args as [])),
  },
}))

async function mountOfferedContract(roles: UserRole[] = ['CONTRACT_NEGOTIATOR']) {
  const pinia = createPinia()
  const wrapper = shallowMount(NegotiateContractView, { global: { plugins: [pinia] } })
  setActivePinia(pinia)
  useAuthStore().user = { issuer: 'did:web:example.com:org', holder: 'user', roles }
  await nextTick()
  await nextTick()
  await nextTick()
  return wrapper
}

function buttonLabelled(wrapper: Awaited<ReturnType<typeof mountOfferedContract>>, label: string) {
  return wrapper.findAll('button').find((button) => button.text().includes(label))
}

describe('accepting an inbound offer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getLocalDIDFile.mockResolvedValue({ id: LOCAL_PEER })
  })

  it('offers an enabled Accept action on an OFFERED contract and hides Change Proposal', async () => {
    const wrapper = await mountOfferedContract()

    const accept = buttonLabelled(wrapper, 'Accept offer')
    expect(accept).toBeDefined()
    expect(accept?.attributes('disabled')).toBeUndefined()
    // Redlines wait until NEGOTIATION; OFFERED is accept-as-is (or wait) only.
    expect(buttonLabelled(wrapper, 'Change Proposal')).toBeUndefined()
  })

  it('accepts the offer as it stands, without a change request', async () => {
    const wrapper = await mountOfferedContract()

    await buttonLabelled(wrapper, 'Accept offer')?.trigger('click')
    await nextTick()

    expect(negotiate).not.toHaveBeenCalled()
    expect(acceptOffer).toHaveBeenCalledWith({
      did: 'did:web:example.com:contract',
      updated_at: '2026-07-31T00:00:00Z',
      accepted_by: 'did:web:example.com:org',
    })
  })

  // design accept_offer scopes Contract Creator, Contract Negotiator and
  // Contract Manager — the responder instance drives its inbound contracts as
  // the last of those and holds neither of the first two.
  it.each<UserRole>(['CONTRACT_NEGOTIATOR', 'CONTRACT_CREATOR', 'CONTRACT_MANAGER'])(
    'enables Accept offer for %s',
    async (role) => {
      const wrapper = await mountOfferedContract([role])

      expect(buttonLabelled(wrapper, 'Accept offer')?.attributes('disabled')).toBeUndefined()
    },
  )

  it('does not offer Accept to a role the endpoint refuses', async () => {
    const wrapper = await mountOfferedContract(['CONTRACT_OBSERVER'])

    expect(buttonLabelled(wrapper, 'Accept offer')?.attributes('disabled')).toBeDefined()
    expect(buttonLabelled(wrapper, 'Change Proposal')).toBeUndefined()
  })

  it('does not offer Submit before the offer has been accepted', async () => {
    const wrapper = await mountOfferedContract()

    // Submit settles a negotiation round; an offer nobody has entered has none.
    expect(buttonLabelled(wrapper, 'Submit')).toBeUndefined()
  })

  it('hides Accept offer on the origin instance that sent the offer', async () => {
    getLocalDIDFile.mockResolvedValue({ id: ORIGIN_PEER })
    const wrapper = await mountOfferedContract(['CONTRACT_CREATOR'])

    expect(buttonLabelled(wrapper, 'Accept offer')).toBeUndefined()
  })
})
