import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import { useContractTemplatesStore } from '@/stores/contract-templates-store'
import { useContractsStore } from '@/stores/contracts-store'
import { useApprovalTaskStateFilterStore } from '@/stores/state-filter-store'
import { ApprovalTaskState } from '@/types/approval-task-state'
import { ContractState } from '@/types/contract-state'
import { TemplateState } from '@/types/contract-template-state'
import ApprovalTaskList from './ApprovalTaskList.vue'
import type { ContractApprovalTask } from '@/models/contract/contract-approval-task'
import type { ContractTemplateApprovalTask } from '@/models/contract-template/contract-template-approval-task'

type ApprovalTask = ContractTemplateApprovalTask | ContractApprovalTask

const templateTask = (did: string, state = ApprovalTaskState.open): ContractTemplateApprovalTask => ({
  type: 'template',
  did,
  version: 1,
  state,
  approver: 'did:web:approver.example',
  created_at: '2026-08-03T00:00:00Z',
})

const contractTask = (did: string, state = ApprovalTaskState.open): ContractApprovalTask => ({
  type: 'contract',
  did,
  contract_version: '1',
  state,
  approver: 'did:web:approver.example',
  created_at: '2026-08-03T00:00:00Z',
})

function mountList(
  tasks: ApprovalTask[],
  templateStates: Record<string, string> = {},
  contractStates: Record<string, string> = {},
  selectedState?: ApprovalTaskState,
) {
  const pinia = createPinia()
  setActivePinia(pinia)

  const templates = useContractTemplatesStore()
  templates.contractTemplates = Object.entries(templateStates).map(([did, state]) => ({
    did,
    name: `Template ${did}`,
    state,
  })) as never

  const contracts = useContractsStore()
  contracts.contracts = Object.entries(contractStates).map(([did, state]) => ({
    did,
    name: `Contract ${did}`,
    state,
  })) as never

  if (selectedState) {
    useApprovalTaskStateFilterStore().setFilter(selectedState)
  }

  return mount(ApprovalTaskList, { props: { tasks }, global: { plugins: [pinia] } })
}

describe('ApprovalTaskList readiness filter', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('shows open tasks for reviewed templates and contracts by default', () => {
    const templateDid = 'did:web:example:reviewed-template'
    const contractDid = 'did:web:example:reviewed-contract'
    const wrapper = mountList(
      [templateTask(templateDid), contractTask(contractDid)],
      { [templateDid]: TemplateState.reviewed },
      { [contractDid]: ContractState.reviewed },
    )

    const rows = wrapper.findAll('.list-row')
    expect(rows).toHaveLength(2)
    expect(wrapper.text()).toContain(templateDid)
    expect(wrapper.text()).toContain(contractDid)
  })

  it('hides open tasks whose parent is not reviewed and terminal tasks by default', () => {
    const draftTemplateDid = 'did:web:example:draft-template'
    const submittedContractDid = 'did:web:example:submitted-contract'
    const approvedTemplateDid = 'did:web:example:approved-template-task'
    const rejectedContractDid = 'did:web:example:rejected-contract-task'
    const wrapper = mountList(
      [
        templateTask(draftTemplateDid),
        contractTask(submittedContractDid),
        templateTask(approvedTemplateDid, ApprovalTaskState.approved),
        contractTask(rejectedContractDid, ApprovalTaskState.rejected),
      ],
      {
        [draftTemplateDid]: TemplateState.draft,
        [approvedTemplateDid]: TemplateState.reviewed,
      },
      {
        [submittedContractDid]: ContractState.submitted,
        [rejectedContractDid]: ContractState.reviewed,
      },
    )

    expect(wrapper.findAll('.list-row')).toHaveLength(0)
    expect(wrapper.text()).toContain('No approval tasks found.')
  })

  it('keeps an explicit open filter status-only and exposes pre-review tasks', () => {
    const templateDid = 'did:web:example:draft-template'
    const contractDid = 'did:web:example:submitted-contract'
    const wrapper = mountList(
      [templateTask(templateDid), contractTask(contractDid)],
      { [templateDid]: TemplateState.draft },
      { [contractDid]: ContractState.submitted },
      ApprovalTaskState.open,
    )

    expect(wrapper.findAll('.list-row')).toHaveLength(2)
    expect(wrapper.text()).toContain(templateDid)
    expect(wrapper.text()).toContain(contractDid)
  })

  it('keeps explicit terminal filters status-only', () => {
    const templateDid = 'did:web:example:approved-template-task'
    const wrapper = mountList(
      [templateTask(templateDid, ApprovalTaskState.approved)],
      { [templateDid]: TemplateState.draft },
      {},
      ApprovalTaskState.approved,
    )

    expect(wrapper.findAll('.list-row')).toHaveLength(1)
    expect(wrapper.text()).toContain(templateDid)
  })
})
