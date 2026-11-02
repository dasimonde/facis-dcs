import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { useAuthStore } from '@/stores/auth-store'
import DetailsEditor from './DetailsEditor.vue'
import TemplateProvenance from './TemplateProvenance.vue'
import type { ContractTemplateHistoryEntry } from '@/models/responses/template-response'
import type { UserRole } from '@/types/user-role'

/**
 * DCS-FR-TR-28: the Meta Data tab shows the template's provenance from its
 * stored history, one row per recorded state change, not only the current row.
 */

const DID = 'did:web:example.com:template:1'

const retrieveHistory = vi.fn<(did: string) => Promise<ContractTemplateHistoryEntry[]>>()
const retrieve = vi.fn(() => Promise.resolve({ contract_templates: [], approval_tasks: [], review_tasks: [] }))

vi.mock('@/services/contract-template-service', () => ({
  contractTemplateService: {
    retrieveHistory: (did: string) => retrieveHistory(did),
    retrieve: () => retrieve(),
  },
}))

function entry(version: number, state: string, updatedAt: string): ContractTemplateHistoryEntry {
  return {
    did: DID,
    version,
    state,
    template_type: 'CONTRACT_TEMPLATE',
    created_by: 'did:web:example.com:creator',
    created_at: '2026-07-01T08:00:00Z',
    updated_at: updatedAt,
  } as ContractTemplateHistoryEntry
}

async function mountWithDid(roles: UserRole[] = ['TEMPLATE_CREATOR'], component: object = TemplateProvenance) {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().user = { issuer: 'did:web:example.com:org', holder: 'user', roles }
  useDcsDraftStore().did = DID
  const wrapper = mount(component, { global: { plugins: [pinia] } })
  await flushPromises()
  return wrapper
}

describe('TemplateProvenance history', () => {
  beforeEach(() => {
    retrieveHistory.mockReset()
    retrieve.mockClear()
  })

  it('renders the history entries of the template, oldest first', async () => {
    retrieveHistory.mockResolvedValue([
      entry(1, 'SUBMITTED', '2026-07-02T10:00:00Z'),
      entry(1, 'DRAFT', '2026-07-01T09:00:00Z'),
    ])

    const wrapper = await mountWithDid()

    expect(retrieveHistory).toHaveBeenCalledWith(DID)
    const rows = wrapper.findAll('[data-testid="template-history-entry"]')
    expect(rows.map((row) => row.findAll('td').map((cell) => cell.text()))).toEqual([
      ['1', 'DRAFT', new Date('2026-07-01T09:00:00Z').toLocaleString(), 'did:web:example.com:creator'],
      ['1', 'SUBMITTED', new Date('2026-07-02T10:00:00Z').toLocaleString(), 'did:web:example.com:creator'],
    ])
  })

  it('says so when the history cannot be loaded', async () => {
    retrieveHistory.mockRejectedValue(new Error('403'))

    const wrapper = await mountWithDid()

    expect(wrapper.find('[data-testid="template-history"]').exists()).toBe(false)
    expect(wrapper.get('[role="alert"]').text()).toContain('could not be loaded')
  })

  // A Contract Creator sees the template it instantiates (embedded on
  // /contracts/new) but holds no role for the template list or history
  // endpoints, so the view does not call them.
  it('does not request the template history or list without a template role', async () => {
    const provenance = await mountWithDid(['CONTRACT_CREATOR'])
    const details = await mountWithDid(['CONTRACT_CREATOR'], DetailsEditor)

    expect(retrieveHistory).not.toHaveBeenCalled()
    expect(retrieve).not.toHaveBeenCalled()
    expect(provenance.find('[role="alert"]').exists()).toBe(false)
    expect(provenance.text()).not.toContain('Version history')
    expect(details.exists()).toBe(true)
  })

  it('requests the template list for a template role', async () => {
    await mountWithDid(['TEMPLATE_MANAGER'], DetailsEditor)

    expect(retrieve).toHaveBeenCalledTimes(1)
  })
})
