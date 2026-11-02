import { flushPromises, mount, shallowMount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ContractState } from '@/types/contract-state'
import ContractHistoryDiffView from './ContractHistoryDiffView.vue'

/**
 * Each version in the history picker names the actor whose save replaced it,
 * so an edit is attributed to the editor and not to the contract's creator.
 */

const history = vi.hoisted(() => ({ items: [] as unknown[] }))
const defaultHistory = () => [
  {
    id: 'h1',
    did: 'did:web:example.com:contract',
    contract_version: 1,
    state: 'DRAFT',
    created_by: 'Contract Creator',
    created_at: '2026-10-27T08:00:00Z',
    updated_at: '2026-10-27T08:00:00Z',
    modified_by: 'Editing User',
    modified_at: '2026-10-28T09:30:00Z',
  },
  {
    id: 'h2',
    did: 'did:web:example.com:contract',
    contract_version: 1,
    state: 'DRAFT',
    created_by: 'Contract Creator',
    created_at: '2026-10-27T08:00:00Z',
    updated_at: '2026-10-26T08:00:00Z',
  },
]
vi.mock('@/services/contract-workflow-service', () => ({
  contractWorkflowService: { retrieveHistoryByDid: () => Promise.resolve(history.items) },
}))

describe('ContractHistoryDiffView', () => {
  beforeEach(() => {
    history.items = defaultHistory()
  })

  it('labels each version with the actor of the save that replaced it', async () => {
    const wrapper = shallowMount(ContractHistoryDiffView, {
      props: { contractDid: 'did:web:example.com:contract', contractState: ContractState.draft },
    })
    await flushPromises()

    const labels = wrapper.findAll('#diff-left-pick option').map((option) => option.text())
    expect(labels).toHaveLength(2)
    expect(labels[0]).toContain('replaced by Editing User at')
    expect(labels[0]).not.toContain('Contract Creator')
    expect(labels[1]).not.toContain('replaced by')
  })

  // DCS-FR-CWE-08: an old version is offered for rollback.
  it('restores the version picked on the left', async () => {
    const wrapper = shallowMount(ContractHistoryDiffView, {
      props: {
        contractDid: 'did:web:example.com:contract',
        contractState: ContractState.draft,
        currentContractData: {} as never,
        restorable: true,
      },
    })
    await flushPromises()

    await wrapper.get('[data-testid="restore-history-version"]').trigger('click')

    const restored = wrapper.emitted('restore')?.[0]?.[0] as { updated_at: string }
    expect(restored.updated_at).toBe('2026-10-27T08:00:00Z')
  })

  it.each([false, true])('offers a restore only when restorable is %s', async (restorable) => {
    const wrapper = shallowMount(ContractHistoryDiffView, {
      props: {
        contractDid: 'did:web:example.com:contract',
        contractState: ContractState.submitted,
        currentContractData: {} as never,
        restorable,
      },
    })
    await flushPromises()
    expect(wrapper.find('[data-testid="restore-history-version"]').exists()).toBe(restorable)
  })

  // Several saves snapshot one contract version; each stays a version of its own.
  it('offers every history entry of one contract version', async () => {
    history.items = defaultHistory().map((item) => ({ ...item, updated_at: '2026-10-27T08:00:00Z' }))
    const wrapper = shallowMount(ContractHistoryDiffView, {
      props: { contractDid: 'did:web:example.com:contract', contractState: ContractState.draft },
    })
    await flushPromises()
    const values = wrapper.findAll('#diff-left-pick option').map((option) => option.attributes('value'))
    expect(new Set(values).size).toBe(2)
  })

  // DCS-FR-CWE-17, DCS-IR-CWE-04: two versions are compared side by side.
  it('compares the details of two versions', async () => {
    history.items = defaultHistory().map((item, index) => ({
      ...item,
      name: index === 0 ? 'Newer name' : 'Older name',
    }))
    const wrapper = mount(ContractHistoryDiffView, {
      props: { contractDid: 'did:web:example.com:contract', contractState: ContractState.submitted },
    })
    await flushPromises()
    const text = wrapper.text()
    expect(text).toContain('Newer name')
    expect(text).toContain('Older name')
  })
})
