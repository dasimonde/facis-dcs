import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { router, ROUTES } from '@/router/router'
import { useAuthStore } from '@/stores/auth-store'
import ArchiveDashboardView from './ArchiveDashboardView.vue'
import type { UserRole } from '@/types/user-role'

/**
 * DCS-FR-CSA-24: the Auditor and the Compliance Officer inspect the archived
 * contracts, their compliance verdict, evidence and sections read-only.
 * DCS-FR-CSA-04: the expiring window is the configured one.
 */

const archive = vi.hoisted(() => ({
  retrieve: vi.fn(),
  erasureStatus: vi.fn(),
  compliance: vi.fn(),
  evidence: vi.fn(),
  sections: vi.fn(),
  detail: vi.fn(),
}))
const statistics = vi.hoisted(() => vi.fn())

vi.mock('@/composables/useContractChanges', () => ({ useContractChanges: vi.fn() }))
vi.mock('@/services/archive-service', () => ({ archiveService: archive }))
vi.mock('@/services/archive-statistics-service', () => ({ archiveStatisticsService: { statistics } }))

const entry = {
  did: 'did:web:example.com:contract:1',
  contract_version: 1,
  state: 'TERMINATED',
  created_by: 'did:web:example.com',
  created_at: '2026-10-01T00:00:00Z',
  updated_at: '2026-10-02T00:00:00Z',
}

async function mountDashboard(roles: UserRole[]) {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().user = { issuer: 'did:web:example.com', holder: 'user', roles }
  const wrapper = mount(ArchiveDashboardView, {
    global: { plugins: [pinia], stubs: { RouterLink: { template: '<a><slot /></a>' } } },
  })
  await flushPromises()
  return wrapper
}

describe('archive inspection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    archive.retrieve.mockResolvedValue([entry])
    archive.compliance.mockResolvedValue([
      {
        did: entry.did,
        contract_version: 1,
        flagged: true,
        checks: [{ rule: 'evidence_completeness', passed: false, reason: 'no TSA receipt' }],
      },
    ])
    archive.evidence.mockResolvedValue([
      {
        id: 'e-1',
        did: entry.did,
        name: 'delivery-note.pdf',
        media_type: 'application/pdf',
        content_hash: 'sha256:abc',
        size_bytes: 3,
        stored_by: 'Org A',
        stored_at: '2026-10-03T00:00:00Z',
        content: 'YWJj',
      },
    ])
    archive.sections.mockResolvedValue([])
    statistics.mockResolvedValue({
      archived_total: 1,
      archived_contracts: 1,
      deleted_total: 0,
      storage_bytes: 10,
      compliant_total: 0,
      flagged_total: 1,
      recent_actions: [],
      expiring_contracts: [],
      expiring_window_days: 14,
    })
  })

  it.each<UserRole>(['AUDITOR', 'COMPLIANCE_OFFICER'])(
    'shows the %s the entries, the compliance verdict, evidence and sections read-only',
    async (role) => {
      const wrapper = await mountDashboard([role])
      expect(archive.retrieve).toHaveBeenCalled()
      expect(wrapper.get(`[data-testid="archive-compliance-${entry.did}"]`).text()).toContain('Flagged')

      await wrapper.get(`[data-testid="archive-evidence-${entry.did}"]`).trigger('click')
      await flushPromises()
      expect(archive.evidence).toHaveBeenCalledWith(entry.did)
      expect(wrapper.get(`[data-testid="evidence-details-${entry.did}"]`).text()).toContain('delivery-note.pdf')

      await wrapper.get(`[data-testid="archive-sections-${entry.did}"]`).trigger('click')
      await flushPromises()
      expect(archive.sections).toHaveBeenCalledWith(entry.did)
      expect(wrapper.find('[data-testid="archive-section-store"]').exists()).toBe(false)

      for (const write of ['archive-store', `archive-annotate-${entry.did}`, `archive-delete-${entry.did}`]) {
        expect(wrapper.find(`[data-testid="${write}"]`).exists(), write).toBe(false)
      }
      expect(archive.erasureStatus).not.toHaveBeenCalled()
    },
  )

  it('opens the archive dashboard and detail routes to the Auditor and the Compliance Officer', () => {
    for (const name of [ROUTES.ARCHIVE.DASHBOARD, ROUTES.ARCHIVE.DETAIL]) {
      const roles = router.getRoutes().find((route) => route.name === name)?.meta.roles
      expect(roles, String(name)).toEqual(expect.arrayContaining(['AUDITOR', 'COMPLIANCE_OFFICER']))
    }
    // DCS-FR-PACM-05: the Auditor reads and exports the case files there.
    const investigation = router.getRoutes().find((route) => route.name === ROUTES.NON_COMPLIANCE.INVESTIGATION)
    expect(investigation?.meta.roles).toEqual(expect.arrayContaining(['AUDITOR', 'COMPLIANCE_OFFICER']))
  })

  // DCS-FR-PACM-04: the actors of the recent actions are audit-log content.
  it('shows the actors of the recent actions only after a justification', async () => {
    statistics.mockResolvedValueOnce({
      ...(await statistics()),
      recent_actions: [{ occurred_at: '2026-10-03T00:00:00Z', event_type: 'STORE_ARCHIVED_CONTRACT', did: entry.did }],
    })
    statistics.mockClear()
    const wrapper = await mountDashboard(['AUDITOR'])
    expect(statistics).toHaveBeenCalledWith(undefined)
    expect(wrapper.get('[data-testid="archive-recent-actions"]').text()).toContain('—')

    await wrapper.get('[data-testid="archive-actors-justification"]').setValue('quarterly review')
    await wrapper.get('[data-testid="archive-actors-form"]').trigger('submit')
    await flushPromises()
    expect(statistics).toHaveBeenLastCalledWith('quarterly review')
    expect(wrapper.find('[data-testid="archive-actors-form"]').exists()).toBe(false)
  })

  it('names the configured expiring window', async () => {
    const wrapper = await mountDashboard(['AUDITOR'])
    const expiring = wrapper.get('[data-testid="archive-expiring"]').text()
    expect(expiring).toContain('Expiring within 14 days')
    expect(expiring).toContain('within the next 14 days')
  })
})
