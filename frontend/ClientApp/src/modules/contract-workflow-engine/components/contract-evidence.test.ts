import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useContractEditorUiStore } from '@contract-workflow-engine/store/contractEditorUiStore'
import AuditView from './AuditView.vue'
import ContractEvidencePanel from './ContractEvidencePanel.vue'
import type { Contract } from '@/models/contract/contract'
import type { ContractEvidence } from '@/models/contract/contract-evidence'
import type { DcsBlock, DcsLayoutNode } from '@/models/dcs-jsonld'

const mocks = vi.hoisted(() => ({
  auth: { user: { roles: ['CONTRACT_MANAGER'] } },
  listEvidence: vi.fn(),
  audit: vi.fn(),
  storeEvidence: vi.fn(),
  retrieveEvidence: vi.fn(),
  downloadBlob: vi.fn(),
}))
vi.mock('vue-router', () => ({ useRoute: () => ({ params: { did: 'did:test:contract' } }) }))
vi.mock('@/stores/auth-store', () => ({ useAuthStore: () => mocks.auth }))
vi.mock('@/services/contract-workflow-service', () => ({ contractWorkflowService: mocks }))
vi.mock('@/utils/download-blob', () => ({ downloadBlob: mocks.downloadBlob }))
const contract: Contract = {
  did: 'did:test:contract',
  contract_version: 3,
  state: 'SIGNED',
  created_by: 'creator',
  created_at: '2026-10-16T00:00:00Z',
  updated_at: '2026-10-16T01:00:00Z',
}
const evidence: ContractEvidence = {
  did: contract.did,
  evidence_id: 'evidence-1',
  contract_version: 3,
  recorded_by: 'manager',
  recorded_at: contract.updated_at,
  filename: 'proof.bin',
  description: '<script>not executable</script>',
  media_type: 'application/octet-stream',
  sha256: 'hash',
}
function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((res) => {
    resolve = res
  })
  return { promise, resolve }
}

describe('evidence lifecycle', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.auth.user.roles = ['CONTRACT_MANAGER']
    mocks.listEvidence.mockResolvedValue([])
  })
  it.each(['AUDITOR', 'COMPLIANCE_OFFICER'])('allows %s to read without upload controls', async (role) => {
    mocks.auth.user.roles = [role]
    mocks.listEvidence.mockResolvedValue([evidence])
    const wrapper = mount(ContractEvidencePanel, { props: { contract } })
    await flushPromises()
    expect(wrapper.text()).toContain(evidence.description)
    expect(wrapper.find('script').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('Store evidence')
    expect(mocks.listEvidence).toHaveBeenCalledWith(contract.did)
  })
  it('validates file first, and cancel clears the unsaved form without POST', async () => {
    const wrapper = mount(ContractEvidencePanel, { props: { contract }, attachTo: document.body })
    await wrapper.get('button').trigger('click')
    await wrapper.get('form').trigger('submit')
    expect(wrapper.get('[role="alert"]').text()).toContain('non-empty evidence file')
    expect(document.activeElement).toBe(wrapper.get('input').element)
    await wrapper.get('textarea').setValue('Unsaved')
    await wrapper.get('form').trigger('keydown', { key: 'Escape' })
    expect(wrapper.find('form').exists()).toBe(false)
    expect(document.activeElement).toBe(wrapper.get('button').element)
    expect(mocks.storeEvidence).not.toHaveBeenCalled()
    wrapper.unmount()
  })
  it('clears previous content and ignores late list replies after DID changes', async () => {
    const late = deferred<ContractEvidence[]>()
    mocks.listEvidence.mockReturnValueOnce(late.promise).mockResolvedValueOnce([])
    const wrapper = mount(ContractEvidencePanel, { props: { contract } })
    await wrapper.setProps({ contract: { ...contract, did: 'did:test:new' } })
    await flushPromises()
    late.resolve([evidence])
    await flushPromises()
    expect(wrapper.text()).not.toContain('proof.bin')
    expect(wrapper.text()).toContain('No stored evidence')
  })
  it('offers retry for list errors and never downloads a late response after unmount', async () => {
    mocks.listEvidence.mockRejectedValueOnce(new Error('Unavailable')).mockResolvedValueOnce([evidence])
    const wrapper = mount(ContractEvidencePanel, { props: { contract } })
    await flushPromises()
    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    const late = deferred<ContractEvidence>()
    mocks.retrieveEvidence.mockReturnValue(late.promise)
    await wrapper.get('[aria-label="Download proof.bin"]').trigger('click')
    wrapper.unmount()
    late.resolve({ ...evidence, content: 'AAH/' })
    await flushPromises()
    expect(mocks.downloadBlob).not.toHaveBeenCalled()
  })
  it('shows unavailable evidence and permits retry without producing a download on failure', async () => {
    mocks.listEvidence.mockResolvedValue([evidence])
    mocks.retrieveEvidence
      .mockRejectedValueOnce(new Error('Evidence deleted'))
      .mockResolvedValueOnce({ ...evidence, content: 'AAH/' })
    const wrapper = mount(ContractEvidencePanel, { props: { contract } })
    await flushPromises()
    await wrapper.get('[aria-label="Download proof.bin"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('Evidence deleted')
    expect(mocks.downloadBlob).not.toHaveBeenCalled()
    await wrapper.get('[aria-label="Download proof.bin"]').trigger('click')
    await flushPromises()
    expect(mocks.downloadBlob).toHaveBeenCalledOnce()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
  })
})

it('renders contract evidence alongside the audit trail with block and layout context', async () => {
  const pinia = createPinia()
  setActivePinia(pinia)
  useContractEditorUiStore().setActiveTab('audit')
  mocks.listEvidence.mockResolvedValue([evidence])
  mocks.audit.mockResolvedValue([
    {
      id: 1,
      component: 'CONTRACT_WORKFLOW_ENGINE',
      event_type: 'CONTRACT_REJECTED',
      created_at: contract.updated_at,
      event_data: { annotations: [{ block_id: 'clause', comment: 'Clarify this term' }] },
    },
  ])
  const blocks: DcsBlock[] = [
    { '@id': 'clause', '@type': 'dcs:Clause', 'dcs:title': 'Delivery clause', 'dcs:content': '' },
  ]
  const layout: DcsLayoutNode[] = []
  const wrapper = mount(AuditView, { props: { contract, blocks, layout }, global: { plugins: [pinia] } })
  await flushPromises()
  expect(mocks.audit).not.toHaveBeenCalled()
  await wrapper.get('form input').setValue('Dispute review')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(mocks.audit).toHaveBeenCalledWith({ did: contract.did, justification: 'Dispute review' })
  expect(wrapper.findComponent(ContractEvidencePanel).exists()).toBe(true)
  expect(wrapper.text()).toContain('proof.bin')
  const audit = wrapper.findComponent({ name: 'ContractAuditList' })
  expect(audit.exists()).toBe(true)
  expect(audit.props('blocks')).toEqual(blocks)
  expect(audit.props('layout')).toEqual(layout)
  wrapper.unmount()
})

// DCS-IR-CWE-02: the contract creator populates evidence while creating the contract.
it('lets the contract creator store evidence', async () => {
  mocks.auth.user.roles = ['CONTRACT_CREATOR']
  mocks.listEvidence.mockResolvedValue([])
  const wrapper = mount(ContractEvidencePanel, { props: { contract: { ...contract, state: 'DRAFT' } } })
  await flushPromises()
  expect(wrapper.findAll('button').some((button) => button.text() === 'Store evidence')).toBe(true)
  mocks.auth.user.roles = ['CONTRACT_MANAGER']
  wrapper.unmount()
})

// DCS-IR-CWE-12: outside a draft only the manager stores evidence.
it('offers the contract creator no evidence upload once the contract left draft', async () => {
  mocks.auth.user.roles = ['CONTRACT_CREATOR']
  mocks.listEvidence.mockResolvedValue([])
  const wrapper = mount(ContractEvidencePanel, { props: { contract } })
  await flushPromises()
  expect(wrapper.findAll('button').some((button) => button.text() === 'Store evidence')).toBe(false)
  mocks.auth.user.roles = ['CONTRACT_MANAGER']
  wrapper.unmount()
})
