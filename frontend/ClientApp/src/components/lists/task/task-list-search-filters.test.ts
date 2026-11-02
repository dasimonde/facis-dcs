import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { expect, it, vi } from 'vitest'
import { useContractsStore } from '@/stores/contracts-store'
import { ContractState } from '@/types/contract-state'
import TaskListSearch from './TaskListSearch.vue'

const search = vi.hoisted(() => vi.fn().mockResolvedValue([]))
vi.mock('@/services/contract-workflow-service', () => ({ contractWorkflowService: { search } }))

// DCS-IR-CWE-07 and DCS-FR-CWE-13: the review search locates contracts by party,
// template and jurisdiction.
it.each([
  ['Party', 'party', 'did:web:b.example'],
  ['Template', 'template_did', 'did:web:template:1'],
  ['Jurisdiction', 'jurisdiction', 'Germany'],
])('searches contracts by %s', async (label, key, value) => {
  const pinia = createPinia()
  setActivePinia(pinia)
  useContractsStore().contracts = [{ did: 'did:web:c1', state: ContractState.submitted }] as never
  const wrapper = mount(TaskListSearch, {
    props: {
      tasks: [{ did: 'did:web:c1', type: 'contract' as const }],
      contractSearchDefaults: { state: [ContractState.submitted] },
    },
    global: { plugins: [pinia] },
  })

  // jsdom has no popover API.
  Object.assign(wrapper.get('#list-popover-search').element, { hidePopover: vi.fn() })
  const option = wrapper.findAll('#list-popover-search a').find((a) => a.text() === label)
  expect(option).toBeDefined()
  await option!.trigger('click')
  await wrapper.get('input').setValue(value)
  await wrapper.get('button:last-of-type').trigger('click')
  await flushPromises()

  expect(search).toHaveBeenLastCalledWith({ state: [ContractState.submitted], [key]: value })
  wrapper.unmount()
})
