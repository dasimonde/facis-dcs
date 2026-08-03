import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import { useContractsStore } from '@/stores/contracts-store'
import NegotiationTaskList from './NegotiationTaskList.vue'
import type { ContractNegotiationTask } from '@/models/contract/contract-negotiation-task'

/**
 * The tab is task-driven, and a task now exists only because this instance
 * engaged with a negotiation round. Its default view is therefore the rounds
 * still awaiting an answer — read off the task's own state, not off the
 * contract's lifecycle.
 */

const task = (did: string, state: string): ContractNegotiationTask =>
  ({
    did,
    state,
    contract_version: 1,
    negotiator: 'did:web:local.example',
    created_at: '2026-07-31T00:00:00Z',
  }) as ContractNegotiationTask

function mountList(tasks: ContractNegotiationTask[], contractState = 'NEGOTIATION') {
  const pinia = createPinia()
  setActivePinia(pinia)
  const contracts = useContractsStore()
  contracts.contracts = tasks.map((entry) => ({
    did: entry.did,
    name: `Contract ${entry.did}`,
    state: contractState,
  })) as never
  return mount(NegotiationTaskList, { props: { tasks }, global: { plugins: [pinia] } })
}

describe('NegotiationTaskList default view', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('lists the rounds still awaiting a response and hides the ones already answered', () => {
    const wrapper = mountList([task('did:web:example:open', 'OPEN'), task('did:web:example:done', 'ACCEPTED')])

    const rows = wrapper.findAll('.list-row')
    expect(rows).toHaveLength(1)
    expect(rows[0]?.text()).toContain('did:web:example:open')
  })

  // The originator's own copy sits in OFFERED for as long as the counterparty
  // is deciding, while the task it has held since authoring stays open. A
  // default keyed on the CONTRACT's lifecycle hid exactly this row, so the
  // party that had work outstanding saw an empty tab.
  it('lists an open round whose contract is not itself in negotiation', () => {
    const wrapper = mountList([task('did:web:example:offered', 'OPEN')], 'OFFERED')

    expect(wrapper.findAll('.list-row')).toHaveLength(1)
  })

  it('says so plainly when no round is awaiting a response', () => {
    const wrapper = mountList([task('did:web:example:done', 'ACCEPTED')])

    expect(wrapper.findAll('.list-row')).toHaveLength(0)
    expect(wrapper.text()).toContain('No negotiation tasks found.')
  })
})
