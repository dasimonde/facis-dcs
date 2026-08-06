import { mount, RouterLinkStub } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import { ROUTES } from '@/router/router'
import { useContractTemplatesStore } from '@/stores/contract-templates-store'
import { useContractsStore } from '@/stores/contracts-store'
import { useReviewTaskStateFilterStore } from '@/stores/state-filter-store'
import { ContractState } from '@/types/contract-state'
import { TemplateState } from '@/types/contract-template-state'
import { ReviewTaskState } from '@/types/review-task-state'
import ReviewTaskList from './ReviewTaskList.vue'
import type { ContractReviewTask } from '@/models/contract/contract-review-task'
import type { ContractTemplateReviewTask } from '@/models/contract-template/contract-template-review-task'

type ReviewTask = ContractTemplateReviewTask | ContractReviewTask

const templateTask = (did: string, state = ReviewTaskState.open): ContractTemplateReviewTask => ({
  type: 'template',
  did,
  version: 1,
  state,
  reviewer: 'did:web:reviewer.example',
  created_at: '2026-08-03T00:00:00Z',
})

const contractTask = (did: string, state = ReviewTaskState.open): ContractReviewTask => ({
  type: 'contract',
  did,
  contract_version: '1',
  state,
  reviewer: 'did:web:reviewer.example',
  created_at: '2026-08-03T00:00:00Z',
})

function mountList(
  tasks: ReviewTask[],
  templateStates: Record<string, string> = {},
  contractStates: Record<string, string> = {},
  selectedState?: ReviewTaskState,
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
    useReviewTaskStateFilterStore().setFilter(selectedState)
  }

  return mount(ReviewTaskList, {
    props: { tasks },
    global: { plugins: [pinia], stubs: { RouterLink: RouterLinkStub } },
  })
}

function viewTargets(wrapper: ReturnType<typeof mountList>) {
  return wrapper
    .findAllComponents(RouterLinkStub)
    .filter((link) => link.text() === 'View')
    .map((link) => (link.props('to') as { name: string }).name)
}

describe('ReviewTaskList readiness filter', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('shows open tasks for submitted templates and contracts by default', () => {
    const templateDid = 'did:web:example:submitted-template'
    const contractDid = 'did:web:example:submitted-contract'
    const wrapper = mountList(
      [templateTask(templateDid), contractTask(contractDid)],
      { [templateDid]: TemplateState.submitted },
      { [contractDid]: ContractState.submitted },
    )

    expect(wrapper.findAll('.list-row')).toHaveLength(2)
    expect(wrapper.text()).toContain(templateDid)
    expect(wrapper.text()).toContain(contractDid)
  })

  it('hides open tasks whose parent is not submitted and terminal tasks by default', () => {
    const rejectedTemplateDid = 'did:web:example:rejected-template'
    const draftContractDid = 'did:web:example:draft-contract'
    const approvedTemplateDid = 'did:web:example:approved-template-task'
    const rejectedContractDid = 'did:web:example:rejected-contract-task'
    const wrapper = mountList(
      [
        templateTask(rejectedTemplateDid),
        contractTask(draftContractDid),
        templateTask(approvedTemplateDid, ReviewTaskState.approved),
        contractTask(rejectedContractDid, ReviewTaskState.rejected),
      ],
      {
        [rejectedTemplateDid]: TemplateState.rejected,
        [approvedTemplateDid]: TemplateState.submitted,
      },
      {
        [draftContractDid]: ContractState.draft,
        [rejectedContractDid]: ContractState.submitted,
      },
    )

    expect(wrapper.findAll('.list-row')).toHaveLength(0)
    expect(wrapper.text()).toContain('No review tasks found.')
  })

  it('keeps an explicit open filter status-only and exposes non-ready tasks', () => {
    const templateDid = 'did:web:example:rejected-template'
    const contractDid = 'did:web:example:draft-contract'
    const wrapper = mountList(
      [templateTask(templateDid), contractTask(contractDid)],
      { [templateDid]: TemplateState.rejected },
      { [contractDid]: ContractState.draft },
      ReviewTaskState.open,
    )

    expect(wrapper.findAll('.list-row')).toHaveLength(2)
    expect(wrapper.text()).toContain(templateDid)
    expect(wrapper.text()).toContain(contractDid)
  })

  it('keeps explicit terminal filters status-only', () => {
    const templateDid = 'did:web:example:approved-template-task'
    const contractDid = 'did:web:example:rejected-contract-task'
    const approvedWrapper = mountList(
      [templateTask(templateDid, ReviewTaskState.approved)],
      { [templateDid]: TemplateState.draft },
      {},
      ReviewTaskState.approved,
    )
    const rejectedWrapper = mountList(
      [contractTask(contractDid, ReviewTaskState.rejected)],
      {},
      { [contractDid]: ContractState.draft },
      ReviewTaskState.rejected,
    )

    expect(approvedWrapper.findAll('.list-row')).toHaveLength(1)
    expect(approvedWrapper.text()).toContain(templateDid)
    expect(viewTargets(approvedWrapper)).toEqual([ROUTES.TEMPLATES.VIEW])
    expect(rejectedWrapper.findAll('.list-row')).toHaveLength(1)
    expect(rejectedWrapper.text()).toContain(contractDid)
    expect(viewTargets(rejectedWrapper)).toEqual([ROUTES.CONTRACTS.VIEW])
  })

  it('routes ready template and contract tasks to their review pages', () => {
    const templateDid = 'did:web:example:submitted-template'
    const contractDid = 'did:web:example:submitted-contract'
    const wrapper = mountList(
      [templateTask(templateDid), contractTask(contractDid)],
      { [templateDid]: TemplateState.submitted },
      { [contractDid]: ContractState.submitted },
    )

    expect(viewTargets(wrapper)).toEqual([ROUTES.TEMPLATES.REVIEW, ROUTES.CONTRACTS.REVIEW])
  })

  it('routes explicitly visible non-ready tasks to read-only views', () => {
    const templateDid = 'did:web:example:rejected-template'
    const contractDid = 'did:web:example:draft-contract'
    const openWrapper = mountList(
      [templateTask(templateDid), contractTask(contractDid)],
      { [templateDid]: TemplateState.rejected },
      { [contractDid]: ContractState.draft },
      ReviewTaskState.open,
    )

    expect(viewTargets(openWrapper)).toEqual([ROUTES.TEMPLATES.VIEW, ROUTES.CONTRACTS.VIEW])
  })
})
