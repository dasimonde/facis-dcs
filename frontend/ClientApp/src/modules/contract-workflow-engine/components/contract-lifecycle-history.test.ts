import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useContractEditorUiStore } from '@contract-workflow-engine/store/contractEditorUiStore'
import ContractLifecycleHistory from './ContractLifecycleHistory.vue'

const mocks = vi.hoisted(() => ({ lifecycle: vi.fn() }))
vi.mock('vue-router', () => ({ useRoute: () => ({ params: { did: 'did:test:contract' } }) }))
vi.mock('@/services/contract-workflow-service', () => ({ contractWorkflowService: mocks }))

describe('contract lifecycle history', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('shows each transition with actor and timestamp', async () => {
    mocks.lifecycle.mockResolvedValue([
      { event_type: 'APPLIED_SIGNATURE', actor: 'Org B', occurred_at: '2026-10-01T12:00:00Z' },
      { event_type: 'CONTRACT_EXPIRED', occurred_at: '2026-10-02T12:00:00Z' },
      {
        event_type: 'SUBMIT_CONTRACT',
        actor: 'Org A',
        occurred_at: '2026-10-01T10:00:00Z',
        previous_state: 'SUBMITTED',
        new_state: 'REVIEWED',
      },
    ])
    useContractEditorUiStore().setActiveTab('history')

    const wrapper = mount(ContractLifecycleHistory)
    await flushPromises()

    expect(mocks.lifecycle).toHaveBeenCalledWith('did:test:contract')
    const items = wrapper.findAll('li').map((li) => li.text())
    expect(items).toHaveLength(3)
    expect(items[0]).toContain('Contract signed')
    expect(items[0]).toContain('By: Org B')
    expect(items[0]).toContain(new Date('2026-10-01T12:00:00Z').toLocaleString())
    expect(items[1]).toContain('Contract expired')
    expect(items[1]).not.toContain('By:')
    expect(items[2]).toContain('Review completed')
    expect(items[2]).toContain('By: Org A')
    expect(items[2]).toContain('Transition: Submitted → Reviewed')
  })

  it('loads nothing until the tab is opened', async () => {
    mount(ContractLifecycleHistory)
    await flushPromises()

    expect(mocks.lifecycle).not.toHaveBeenCalled()
  })

  it('reports a refused read', async () => {
    mocks.lifecycle.mockRejectedValue(new Error('not authorized to access this contract'))
    useContractEditorUiStore().setActiveTab('history')

    const wrapper = mount(ContractLifecycleHistory)
    await flushPromises()

    expect(wrapper.get('[role="alert"]').text()).toContain('not authorized')
  })
})
