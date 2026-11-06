import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuthStore } from '@/stores/auth-store'
import ArchiveDashboardView from './ArchiveDashboardView.vue'

/**
 * The Archive Manager stores contracts and evidence (DCS-IR-CSA-02) and
 * terminates archived contracts (DCS-IR-CSA-03) from the dashboard.
 */

const archive = vi.hoisted(() => ({
  retrieve: vi.fn(),
  erasureStatus: vi.fn(),
  compliance: vi.fn(),
  storeContract: vi.fn(),
  storeEvidence: vi.fn(),
  evidence: vi.fn(),
  search: vi.fn(),
  annotate: vi.fn(),
  delete: vi.fn(),
}))
const workflow = vi.hoisted(() => ({ terminate: vi.fn() }))
const live = vi.hoisted(() => ({ onChange: vi.fn<(change: { did: string; event_type: string }) => void>() }))

vi.mock('@/composables/useContractChanges', () => ({
  useContractChanges: (fn: typeof live.onChange) => {
    live.onChange.mockImplementation(fn)
  },
}))

vi.mock('@/services/archive-service', () => ({ archiveService: archive }))
vi.mock('@/services/contract-workflow-service', () => ({ contractWorkflowService: workflow }))
vi.mock('@/services/archive-statistics-service', () => ({
  archiveStatisticsService: {
    statistics: vi.fn().mockResolvedValue({
      archived_total: 1,
      archived_contracts: 1,
      deleted_total: 0,
      storage_bytes: 10,
      compliant_total: 0,
      flagged_total: 1,
      recent_actions: [],
      expiring_contracts: [],
    }),
  },
}))

const signed = {
  did: 'did:web:example.com:contract:1',
  contract_version: 1,
  state: 'SIGNED',
  created_by: 'did:web:example.com',
  created_at: '2026-10-01T00:00:00Z',
  updated_at: '2026-10-02T00:00:00Z',
}

async function mountDashboard() {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().user = { issuer: 'did:web:example.com', holder: 'user', roles: ['ARCHIVE_MANAGER'] }
  const wrapper = mount(ArchiveDashboardView, {
    global: { plugins: [pinia], stubs: { RouterLink: { template: '<a><slot /></a>' } } },
  })
  await flushPromises()
  return wrapper
}

describe('archive dashboard actions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    archive.retrieve.mockResolvedValue([signed])
    archive.erasureStatus.mockResolvedValue({ did: signed.did, local_status: 'live', peers: [] })
    archive.compliance.mockResolvedValue([
      {
        did: signed.did,
        contract_version: 1,
        flagged: true,
        checks: [{ rule: 'evidence_completeness', passed: false, reason: 'no TSA receipt' }],
      },
    ])
  })

  it('archives a contract by DID', async () => {
    archive.storeContract.mockResolvedValue({
      did: signed.did,
      kind: 'contract',
      contract_version: 1,
      content_hash: 'h',
    })
    const wrapper = await mountDashboard()
    await wrapper.get('[data-testid="archive-store-did"]').setValue(signed.did)
    await wrapper.get('[data-testid="archive-store-contract"]').trigger('click')
    await flushPromises()
    expect(archive.storeContract).toHaveBeenCalledWith(signed.did)
  })

  it('flags a non-compliant entry', async () => {
    const wrapper = await mountDashboard()
    expect(wrapper.get(`[data-testid="archive-compliance-${signed.did}"]`).text()).toContain('Flagged')
  })

  it('offers termination only for a contract that can still end', async () => {
    archive.retrieve.mockResolvedValue([
      signed,
      { ...signed, did: 'did:web:example.com:contract:2', state: 'TERMINATED' },
    ])
    const wrapper = await mountDashboard()
    expect(wrapper.find(`[data-testid="archive-terminate-${signed.did}"]`).exists()).toBe(true)
    expect(wrapper.find('[data-testid="archive-terminate-did:web:example.com:contract:2"]').exists()).toBe(false)
  })

  // DCS-FR-CSA-17: only a contract out of force can be deleted from the archive.
  it('offers deletion only for a contract out of force', async () => {
    const outOfForce = ['TERMINATED', 'EXPIRED', 'REVOKED', 'REJECTED', 'WITHDRAWN'].map((state, index) => ({
      ...signed,
      did: `did:web:example.com:contract:gone-${index}`,
      state,
    }))
    const inForce = ['SIGNED', 'ACTIVE', 'APPROVED'].map((state, index) => ({
      ...signed,
      did: `did:web:example.com:contract:live-${index}`,
      state,
    }))
    archive.retrieve.mockResolvedValue([...inForce, ...outOfForce])
    const wrapper = await mountDashboard()
    for (const entry of outOfForce) {
      expect(wrapper.find(`[data-testid="archive-delete-${entry.did}"]`).exists(), entry.state).toBe(true)
    }
    for (const entry of inForce) {
      expect(wrapper.find(`[data-testid="archive-delete-${entry.did}"]`).exists(), entry.state).toBe(false)
    }
  })

  it('shows the termination reason, date and initiating role of a terminated contract (DCS-FR-CSA-16)', async () => {
    const terminated = {
      ...signed,
      did: 'did:web:example.com:contract:3',
      state: 'TERMINATED',
      termination_reason: 'breach of the SLA',
      terminated_at: '2026-10-20T08:00:00Z',
      termination_role: 'Archive Manager',
    }
    archive.retrieve.mockResolvedValue([signed, terminated])
    const wrapper = await mountDashboard()
    const record = wrapper.get(`[data-testid="archive-termination-${terminated.did}"]`).text()
    expect(record).toContain(new Date(terminated.terminated_at).toLocaleString())
    expect(record).toContain('by Archive Manager')
    expect(record).toContain('Reason: breach of the SLA')
    expect(wrapper.find(`[data-testid="archive-termination-${signed.did}"]`).exists()).toBe(false)
  })

  // DCS-FR-CSA-23: a change to a listed contract searches that contract again;
  // a change to another contract offers a reload.
  it('updates a changed entry in place and offers a reload for others', async () => {
    const wrapper = await mountDashboard()
    archive.search.mockResolvedValue([{ ...signed, state: 'TERMINATED' }])
    live.onChange({ did: signed.did, event_type: 'TERMINATE_CONTRACT' })
    await flushPromises()
    expect(archive.search).toHaveBeenCalledWith({ did: signed.did })
    expect(wrapper.get(`[data-testid="archive-entry-${signed.did}"]`).text()).toContain('TERMINATED')
    expect(archive.retrieve).toHaveBeenCalledTimes(1)

    live.onChange({ did: 'did:web:example.com:contract:other', event_type: 'STORE_ARCHIVED_CONTRACT' })
    await flushPromises()
    expect(wrapper.find('[data-testid="archive-entries-outdated"]').exists()).toBe(true)
    wrapper.unmount()
  })

  // DCS-FR-CSA-23: the Archive Manager terminates several contracts at once
  // with one reason.
  it('terminates the selected contracts', async () => {
    HTMLDialogElement.prototype.showModal = vi.fn()
    HTMLDialogElement.prototype.close = vi.fn()
    const second = { ...signed, did: 'did:web:example.com:contract:2' }
    archive.retrieve.mockResolvedValue([signed, second])
    workflow.terminate.mockResolvedValue({})
    const wrapper = await mountDashboard()
    await wrapper.get(`[data-testid="archive-select-${signed.did}"]`).setValue(true)
    await wrapper.get(`[data-testid="archive-select-${second.did}"]`).setValue(true)
    await wrapper.get('[data-testid="archive-terminate-selected"]').trigger('click')
    await flushPromises()
    const modals = document.querySelectorAll('dialog')
    const modal = modals[modals.length - 1]!
    const reason = modal.querySelector('textarea')!
    reason.value = 'retention reached'
    reason.dispatchEvent(new Event('input'))
    await flushPromises()
    modal.querySelector<HTMLButtonElement>('[data-testid="confirmation-confirm"]')!.click()
    await flushPromises()
    expect(workflow.terminate).toHaveBeenCalledTimes(2)
    expect(workflow.terminate).toHaveBeenCalledWith({
      did: second.did,
      reason: 'retention reached',
      updated_at: second.updated_at,
    })
    wrapper.unmount()
  })
})
