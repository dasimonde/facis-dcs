import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuthStore } from '@/stores/auth-store'
import ActionHistoryPanel from './ActionHistoryPanel.vue'
import AuditView from './AuditView.vue'
import type { UserRole } from '@/types/user-role'

/**
 * DCS-FR-PACM-05: the Compliance Officer exports the audit report. DCS-FR-PACM-01:
 * the Auditor verifies the checkpoint inclusion proof of an audit entry.
 */

const auditing = vi.hoisted(() => ({ audit: vi.fn(), report: vi.fn(), checkpointProof: vi.fn() }))
vi.mock('@/utils/download-blob', () => ({ downloadBlob: vi.fn() }))
vi.mock('vue-router', () => ({ useRoute: () => ({ query: {} }) }))
vi.mock('@/services/archive-service', () => ({ archiveService: { erasureStatus: vi.fn().mockResolvedValue(null) } }))
vi.mock('@/services/auditing-service', () => ({ auditingService: auditing }))
vi.mock('@/services/contract-workflow-service', () => ({ contractWorkflowService: { audit: vi.fn() } }))
vi.mock('@/services/contract-template-service', () => ({ contractTemplateService: { audit: vi.fn() } }))

// A proof computed by backend/internal/base/merkle.go: the last of five leaves.
const proof = {
  entry_cid: 'bafy-entry',
  leaf_hash: '0fd6aa6f31ae10f035920ee19bf4ba406757e3219706d7b8fac65cc6eb6ab919',
  leaf_index: 4,
  siblings: ['4ba812d5a68fbe28906e6995fc01d2114990ac5eba53598dfd910ff01695db54'],
  head: {
    seq: 7,
    root: '48ba578022b004753b4466952e75c45806df915fcdd13b06acba3ebce2598647',
    leaf_count: 5,
    created_at: '2026-10-30T10:00:00Z',
  },
}

function render(roles: UserRole[]) {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().user = { issuer: 'org', holder: 'user', roles }
  return mount(AuditView, {
    global: {
      plugins: [pinia],
      mocks: { $route: { meta: { name: 'Audit' } } },
      stubs: { ActionHistoryPanel: true, AccessAttempts: true },
    },
  })
}

describe('audit report and checkpoint proof', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    auditing.report.mockResolvedValue({ bytes: new ArrayBuffer(1), contentType: 'application/json', filename: 'r' })
  })

  it('lets the Compliance Officer export the audit report of the archive', async () => {
    const wrapper = render(['COMPLIANCE_OFFICER'])
    await flushPromises()
    const section = wrapper.get('[data-testid="audit-report-export"]')
    expect(section.findAll('#select-report-scope option').map((option) => option.attributes('value'))).toEqual([
      'contracts',
      'archive',
    ])
    expect(wrapper.get('[data-testid="audit-report-export-json"]').attributes('disabled')).toBeDefined()
    await wrapper.get('#select-report-scope').setValue('archive')
    await wrapper.get('#input-report-justification').setValue('regulator request')
    await wrapper.get('[data-testid="audit-report-export-json"]').trigger('click')
    await flushPromises()
    expect(auditing.report).toHaveBeenCalledWith({
      scope: 'archive',
      format: 'json',
      did: undefined,
      justification: 'regulator request',
    })
    wrapper.unmount()
  })

  it('does not offer the Auditor the report-only section', async () => {
    const wrapper = render(['AUDITOR'])
    await flushPromises()
    expect(wrapper.find('[data-testid="audit-report-export"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('verifies the inclusion proof of an audit entry for the Auditor', async () => {
    auditing.checkpointProof.mockResolvedValue(proof)
    const wrapper = render(['AUDITOR'])
    await flushPromises()
    await wrapper.get('[data-testid="checkpoint-proof-cid"]').setValue('bafy-entry')
    await wrapper.get('[data-testid="checkpoint-proof"] form').trigger('submit')
    await flushPromises()
    expect(auditing.checkpointProof).toHaveBeenCalledWith('bafy-entry')
    expect(wrapper.get('[data-testid="checkpoint-proof-verdict"]').text()).toBe(
      'Proof verified against the checkpoint root',
    )
    expect(wrapper.get('[data-testid="checkpoint-proof-result"]').text()).toContain(proof.head.root)

    auditing.checkpointProof.mockResolvedValue({ ...proof, head: { ...proof.head, root: proof.leaf_hash } })
    await wrapper.get('[data-testid="checkpoint-proof"] form').trigger('submit')
    await flushPromises()
    expect(wrapper.get('[data-testid="checkpoint-proof-verdict"]').text()).toBe(
      'Proof does not match the checkpoint root',
    )
    wrapper.unmount()
  })

  it('opens the action history on the DID of a drill-down', () => {
    const wrapper = mount(ActionHistoryPanel, { props: { initialDid: 'did:web:example.com:contract:1' } })
    expect((wrapper.get('#input-action-history-did').element as HTMLInputElement).value).toBe(
      'did:web:example.com:contract:1',
    )
  })
})
