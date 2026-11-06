import { flushPromises, mount, RouterLinkStub, shallowMount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, expect, it, vi } from 'vitest'
import { defineComponent, reactive } from 'vue'
import TemplatePreview from '@template-repository/components/builder-editor/preview/TemplatePreview.vue'
import ContractListItem from '@/components/lists/contract/ContractListItem.vue'
import { useAuthStore } from '@/stores/auth-store'
import { ContractState } from '@/types/contract-state'
import ApproveContractView from './ApproveContractView.vue'
import type { Contract } from '@/models/contract/contract'

const mocks = vi.hoisted(() => ({ retrieve: vi.fn(), approve: vi.fn(), submit: vi.fn(), reviewFindings: vi.fn() }))
const route = reactive({ params: { did: 'did:test:c1' } })
vi.mock('vue-router', async (original) => ({
  ...(await original<typeof import('vue-router')>()),
  useRoute: () => route,
}))
vi.mock('@/services/did-service', () => ({ resolveLocalInstanceDid: () => Promise.resolve('local') }))
vi.mock('@/services/contract-workflow-service', () => ({
  contractWorkflowService: {
    retrieveById: mocks.retrieve,
    approve: mocks.approve,
    submit: mocks.submit,
    reviewFindings: mocks.reviewFindings,
  },
}))

const revealed = vi.hoisted(() => ({ options: [] as unknown[] }))
const ConfirmationModalStub = defineComponent({
  setup(_, { expose }) {
    expose({
      reveal: (options: unknown) => {
        revealed.options.push(options)
        return Promise.resolve({ isCanceled: false, data: ' clause 4 checked ' })
      },
    })
    return () => null
  },
})

const reviewed = (): Contract => ({
  did: 'did:test:c1',
  name: 'C1',
  contract_version: 1,
  state: ContractState.reviewed,
  created_by: 'org',
  created_at: '',
  updated_at: '2026-10-30T12:00:00Z',
})

function setup(roles: string[]) {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().user = { issuer: 'org', holder: 'user', roles } as never
  return pinia
}

beforeEach(() => {
  vi.resetAllMocks()
  mocks.retrieve.mockResolvedValue(reviewed())
  mocks.approve.mockResolvedValue({})
  mocks.submit.mockResolvedValue({})
  mocks.reviewFindings.mockResolvedValue([])
})

async function mountApprove() {
  const pinia = setup(['CONTRACT_APPROVER'])
  const wrapper = shallowMount(ApproveContractView, {
    global: { plugins: [pinia], stubs: { ConfirmationModal: ConfirmationModalStub } },
  })
  await flushPromises()
  return wrapper
}

const button = (wrapper: Awaited<ReturnType<typeof mountApprove>>, label: string) =>
  wrapper.findAll('button').find((b) => b.text().includes(label))!

// DCS-FR-CWE-15: the approver's comment travels with the approval.
it('sends the approver comment with the approval', async () => {
  const wrapper = await mountApprove()
  await button(wrapper, 'Approve').trigger('click')
  await flushPromises()
  expect(mocks.approve).toHaveBeenCalledWith({
    did: 'did:test:c1',
    updated_at: '2026-10-30T12:00:00Z',
    decision_notes: ['clause 4 checked'],
  })
  // The optional comment does not turn the approval decision into a "Submit".
  expect(revealed.options[revealed.options.length - 1]).toMatchObject({ confirmLabel: 'Confirm' })
  wrapper.unmount()
})

// DCS-IR-CWE-09: the approver can send the contract back for review.
it('resubmits a reviewed contract for review', async () => {
  const wrapper = await mountApprove()
  await button(wrapper, 'Resubmit').trigger('click')
  await flushPromises()
  expect(mocks.submit).toHaveBeenCalledWith({
    did: 'did:test:c1',
    updated_at: '2026-10-30T12:00:00Z',
    comments: ['clause 4 checked'],
  })
  wrapper.unmount()
})

// DCS-FR-CWE-15: a rejected contract is back in draft; its creator edits it.
it('offers the creator to edit a rejected contract', () => {
  const pinia = setup(['CONTRACT_CREATOR'])
  const wrapper = mount(ContractListItem, {
    props: { contract: { ...reviewed(), state: ContractState.rejected } },
    global: { plugins: [pinia], stubs: { RouterLink: RouterLinkStub } },
  })
  expect(wrapper.findAllComponents(RouterLinkStub).some((link) => link.text() === 'Edit')).toBe(true)
})

// DCS-IR-CWE-08, DCS-FR-CWE-25: the approver inspects the reviewer findings before deciding.
it("a plain approver sees the reviewer's comments and highlighted blocks", async () => {
  mocks.reviewFindings.mockResolvedValue([
    {
      submitted_by: 'org',
      occurred_at: '2026-10-30T11:00:00Z',
      contract_version: 1,
      decision: 'APPROVAL',
      comments: ['Liability cap checked'],
      annotations: [{ block_id: 'clause-7', comment: 'Penalty wording is unusual' }],
    },
  ])
  const wrapper = await mountApprove()
  expect(mocks.reviewFindings).toHaveBeenCalledWith('did:test:c1')
  const findings = wrapper.get('[data-testid="reviewer-findings"]').text()
  expect(findings).toContain('Liability cap checked')
  expect(findings).toContain('Penalty wording is unusual')
  expect(wrapper.findComponent(TemplatePreview).props('highlightedBlockIds')).toEqual(['clause-7'])
  expect(wrapper.findComponent({ name: 'NegotiationList' }).props('disabled')).toBe(true)
  expect(wrapper.findComponent({ name: 'ContractHistoryDiffView' }).exists()).toBe(true)
  wrapper.unmount()
})
