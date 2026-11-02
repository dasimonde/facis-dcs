import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuthStore } from '@/stores/auth-store'
import ArchiveDashboardView from './ArchiveDashboardView.vue'
import type { UserRole } from '@/types/user-role'

/**
 * DCS-FR-CSA-26: the Archive Manager archives a section of an archived
 * contract for the parties that may read it; each reader lists, opens and
 * downloads the archived sections of their organization.
 */

const archive = vi.hoisted(() => ({
  retrieve: vi.fn(),
  erasureStatus: vi.fn(),
  compliance: vi.fn(),
  detail: vi.fn(),
  sections: vi.fn(),
  storeSection: vi.fn(),
}))
const download = vi.hoisted(() => ({ downloadBlob: vi.fn() }))

vi.mock('@/composables/useContractChanges', () => ({ useContractChanges: vi.fn() }))
vi.mock('@/services/archive-service', () => ({ archiveService: archive }))
vi.mock('@/utils/download-blob', () => download)
vi.mock('@/services/archive-statistics-service', () => ({
  archiveStatisticsService: { statistics: vi.fn().mockResolvedValue(null) },
}))

const entry = {
  did: 'did:web:example.com:contract:1',
  contract_version: 2,
  state: 'SIGNED',
  created_by: 'Org A',
  created_at: '2026-10-01T00:00:00Z',
  updated_at: '2026-10-02T00:00:00Z',
}

const payment = { '@id': 'urn:c#payment', '@type': 'dcs:Clause', 'dcs:title': 'Payment' }
const archivedPayment = {
  id: 's-1',
  did: entry.did,
  contract_version: 2,
  section_id: 'urn:c#payment',
  parties: ['Org B'],
  content_hash: 'sha256:abc',
  stored_by: 'Org A',
  stored_at: '2026-10-03T00:00:00Z',
  content: payment,
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

describe('archive sections', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    archive.retrieve.mockResolvedValue([entry])
    archive.erasureStatus.mockResolvedValue({ did: entry.did, local_status: 'live', peers: [] })
    archive.compliance.mockResolvedValue([])
    archive.detail.mockResolvedValue({
      ...entry,
      contract_data: {
        'dcs:parties': [{ 'dcs:legalName': 'Org B' }, { 'dcs:legalName': 'Org C' }],
        'dcs:documentStructure': {
          'dcs:blocks': {
            '@list': [{ '@id': 'urn:c#terms', '@type': 'dcs:Section', 'dcs:title': 'Terms' }, payment],
          },
        },
      },
    })
    archive.sections.mockResolvedValue([])
    archive.storeSection.mockResolvedValue({ did: entry.did, kind: 'section', content_hash: 'sha256:abc' })
  })

  it('archives a section of the archived version for the chosen parties', async () => {
    const wrapper = await mountDashboard(['ARCHIVE_MANAGER'])
    await wrapper.get(`[data-testid="archive-sections-${entry.did}"]`).trigger('click')
    await flushPromises()
    expect(archive.detail).toHaveBeenCalledWith(entry.did, 2)

    const options = wrapper.findAll('[data-testid="archive-section-block"] option').map((option) => option.text())
    expect(options).toContain('Section: Terms')
    expect(options).toContain('Clause: Payment')
    expect(wrapper.find('[data-testid="archive-section-party-Org A"]').exists()).toBe(true)

    await wrapper.get('[data-testid="archive-section-block"]').setValue('urn:c#payment')
    await wrapper.get('[data-testid="archive-section-party-Org B"]').setValue(true)
    archive.sections.mockResolvedValue([archivedPayment])
    await wrapper.get('[data-testid="archive-section-store"]').trigger('submit')
    await flushPromises()

    expect(archive.storeSection).toHaveBeenCalledWith(entry.did, 2, 'urn:c#payment', ['Org B'])
    expect(wrapper.get('[data-testid="archive-section-urn:c#payment"]').text()).toContain('Clause: Payment')
  })

  it('lets a Contract Observer open and download the sections of their organization', async () => {
    archive.sections.mockResolvedValue([archivedPayment])
    const wrapper = await mountDashboard(['CONTRACT_OBSERVER'])
    await wrapper.get(`[data-testid="archive-sections-${entry.did}"]`).trigger('click')
    await flushPromises()

    expect(archive.sections).toHaveBeenCalledWith(entry.did)
    expect(archive.detail).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="archive-section-store"]').exists()).toBe(false)
    const row = wrapper.get('[data-testid="archive-section-urn:c#payment"]')
    expect(row.text()).toContain('Payment')

    const buttons = row.findAll('button')
    await buttons.find((button) => button.text() === 'Open')!.trigger('click')
    expect(wrapper.get('[data-testid="archive-section-content-urn:c#payment"]').text()).toContain(
      '"dcs:title": "Payment"',
    )
    await buttons.find((button) => button.text() === 'Download')!.trigger('click')
    expect(download.downloadBlob).toHaveBeenCalledWith(expect.any(Blob), 'section-v2-urn_c_payment.jsonld')
  })
})
