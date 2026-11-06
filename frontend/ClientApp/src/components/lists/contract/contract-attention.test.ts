import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import ContractListView from '@/views/contract/ContractListView.vue'

const http = vi.hoisted(() => ({ get: vi.fn() }))

vi.mock('@/api/http', () => ({ default: http }))
vi.mock('@contract-workflow-engine/composables/useContractPermissions', () => ({
  useContractPermissions: () => ({ isCreator: false }),
}))

const ContractListStub = defineComponent({ emits: ['change'], template: '<div />' })

// DCS-FR-CSA-23: the owner sees the contracts that need attention above the
// contract list, and a live contract change reloads them.
describe('contract attention', () => {
  it('lists the contracts that need attention and reloads on a change', async () => {
    http.get.mockResolvedValue({
      data: [
        { did: 'did:c:1', name: 'Supply', state: 'ACTIVE', reason: 'renewal_due', exp_date: '2026-11-10T00:00:00Z' },
        { did: 'did:c:2', state: 'TERMINATED', reason: 'terminated', terminated_at: '2026-10-30T00:00:00Z' },
      ],
    })
    const wrapper = mount(ContractListView, {
      global: {
        stubs: { RouterLink: { template: '<a><slot /></a>' }, ContractList: ContractListStub },
        mocks: { $route: { meta: { name: 'Contracts' } } },
      },
    })
    await flushPromises()

    expect(http.get).toHaveBeenCalledWith('/contract/attention')
    expect(wrapper.get('[data-testid="contract-attention-did:c:1"]').text()).toBe('Renewal due')
    expect(wrapper.get('[data-testid="contract-attention-did:c:2"]').text()).toBe('Terminated')
    expect(wrapper.text()).toContain('Supply')

    http.get.mockResolvedValue({ data: [] })
    wrapper.getComponent(ContractListStub).vm.$emit('change', { did: 'did:c:1', event_type: 'RENEW_CONTRACT' })
    await flushPromises()
    expect(http.get).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-testid="contract-attention"]').exists()).toBe(false)
  })
})
