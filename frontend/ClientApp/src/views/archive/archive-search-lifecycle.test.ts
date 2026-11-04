import { flushPromises, shallowMount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ArchiveDashboardView from './ArchiveDashboardView.vue'

const mocks = vi.hoisted(() => ({
  retrieve: vi.fn(),
  search: vi.fn(),
  erasureStatus: vi.fn(),
  statistics: vi.fn(),
  roles: ['ARCHIVE_MANAGER'],
}))
vi.mock('@/services/archive-service', () => ({ archiveService: mocks }))
vi.mock('@/services/archive-statistics-service', () => ({ archiveStatisticsService: mocks }))
vi.mock('@/stores/auth-store', () => ({ useAuthStore: () => ({ user: { roles: mocks.roles } }) }))
vi.mock('@/stores/error-store', () => ({ useErrorStore: () => ({ add: vi.fn() }) }))
vi.mock('@/router/router', () => ({ ROUTES: { ARCHIVE: { DETAIL: 'detail' }, AUDIT: { LIST: 'audit' } } }))
const entry = { did: 'did:test:archive', contract_version: 2, name: 'Current entry', state: 'SIGNED' }
const live = { did: entry.did, local_status: 'live', peers: [] }
const shredded = { ...live, local_status: 'shredded' }
function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((yes) => {
    resolve = yes
  })
  return { promise, resolve }
}
function render() {
  return shallowMount(ArchiveDashboardView, { global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
}
describe('archive search and erasure lifecycle', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    mocks.roles = ['ARCHIVE_MANAGER']
    mocks.statistics.mockResolvedValue(null)
    mocks.retrieve.mockResolvedValue([entry])
    mocks.search.mockResolvedValue([entry])
    mocks.erasureStatus.mockResolvedValue(live)
  })
  it.each([entry.did, 'did:test:other'])(
    'reset clears expanded data and ignores old erasure results for %s',
    async (did) => {
      const wrapper = render()
      await flushPromises()
      await wrapper.get(`[data-testid="erasure-badge-${entry.did}"]`).trigger('click')
      expect(wrapper.find(`[data-testid="erasure-details-${entry.did}"]`).exists()).toBe(true)
      const old = deferred<typeof live>()
      mocks.erasureStatus.mockReturnValueOnce(old.promise).mockResolvedValueOnce({ ...shredded, did })
      await wrapper.get('form').trigger('submit')
      await flushPromises()
      expect(wrapper.find(`[data-testid="erasure-details-${entry.did}"]`).exists()).toBe(false)
      expect(wrapper.find(`[data-testid="erasure-badge-${entry.did}"]`).exists()).toBe(false)
      mocks.retrieve.mockResolvedValueOnce([{ ...entry, did }])
      const reset = wrapper.findAll('button').find((button) => button.text() === 'Reset search')
      await reset?.trigger('click')
      await flushPromises()
      expect(wrapper.get(`[data-testid="erasure-badge-${did}"]`).text()).toBe('Keys destroyed')
      old.resolve(live)
      await flushPromises()
      expect(wrapper.get(`[data-testid="erasure-badge-${did}"]`).text()).toBe('Keys destroyed')
      wrapper.unmount()
    },
  )
  it('reports failed search, retries, and ignores old entry replies after a reset', async () => {
    mocks.retrieve.mockRejectedValueOnce(new Error('Archive unavailable'))
    const wrapper = render()
    await flushPromises()
    expect(wrapper.get('[data-testid="archive-entries-error"]').text()).toContain('Archive unavailable')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.find('[data-testid="archive-entries-error"]').exists()).toBe(false)
    const old = deferred<(typeof entry)[]>()
    mocks.retrieve.mockReturnValueOnce(old.promise).mockResolvedValueOnce([{ ...entry, name: 'Newest entry' }])
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    await wrapper
      .findAll('button')
      .find((button) => button.text() === 'Reset search')
      ?.trigger('click')
    await flushPromises()
    old.resolve([{ ...entry, name: 'Stale entry' }])
    await flushPromises()
    expect(wrapper.text()).toContain('Newest entry')
    expect(wrapper.text()).not.toContain('Stale entry')
    expect(wrapper.find('[data-testid="archive-entries-error"]').exists()).toBe(false)
    wrapper.unmount()
  })
})

describe('archive layout and filters', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    mocks.roles = ['ARCHIVE_MANAGER']
    mocks.statistics.mockResolvedValue(null)
    mocks.retrieve.mockResolvedValue([entry])
    mocks.search.mockResolvedValue([entry])
    mocks.erasureStatus.mockResolvedValue(live)
  })
  it.each(['ARCHIVE_MANAGER', 'CONTRACT_OBSERVER'])(
    'preserves and submits all filters, resets hidden values for %s',
    async (role) => {
      mocks.roles = [role]
      const wrapper = render()
      await flushPromises()
      const toggle = wrapper.get('[aria-controls="archive-additional-filters"]')
      const additional = wrapper.get('#archive-additional-filters')
      expect(additional.isVisible()).toBe(false)
      await toggle.trigger('click')
      const inputs = additional.findAll('input')
      for (const [index, value] of ['did:test:archive', 'party', 'tag', '2', '2026-10-01', '2026-10-31'].entries()) {
        await inputs[index]!.setValue(value)
      }
      await wrapper.get('input[placeholder="Search by contract name"]').setValue('Archived')
      await wrapper.get('select').setValue('SIGNED')
      expect(toggle.text()).toContain('6')
      await toggle.trigger('click')
      expect(additional.isVisible()).toBe(false)
      await wrapper.get('form').trigger('submit')
      await flushPromises()
      expect(mocks.search).toHaveBeenLastCalledWith({
        name: 'Archived',
        state: 'SIGNED',
        did: 'did:test:archive',
        party: 'party',
        tag: 'tag',
        contract_version: 2,
        valid_from: '2026-10-01T00:00:00.000Z',
        valid_until: '2026-10-31T00:00:00.000Z',
      })
      await toggle.trigger('click')
      expect((inputs[0]!.element as HTMLInputElement).value).toBe('did:test:archive')
      await toggle.trigger('click')
      await wrapper
        .findAll('button')
        .find((button) => button.text() === 'Reset search')!
        .trigger('click')
      await flushPromises()
      expect(mocks.retrieve).toHaveBeenCalledTimes(2)
      expect(toggle.text()).not.toContain('7')
      expect(inputs.every((input) => (input.element as HTMLInputElement).value === '')).toBe(true)
      wrapper.unmount()
    },
  )
  it.each(['ARCHIVE_MANAGER', 'CONTRACT_OBSERVER', 'AUDITOR'])('keeps role-specific sections for %s', async (role) => {
    mocks.roles = [role]
    mocks.statistics.mockResolvedValue({
      archived_total: 1,
      archived_contracts: 1,
      storage_bytes: 1024,
      compliant_total: 1,
      flagged_total: 0,
      deleted_total: 0,
      expiring_contracts: [],
      recent_actions: [],
    })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.find('[data-testid="archive-entries"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="archive-statistics"]').exists()).toBe(role !== 'CONTRACT_OBSERVER')
    expect(mocks.retrieve).toHaveBeenCalledTimes(1)
    expect(mocks.statistics).toHaveBeenCalledTimes(role === 'CONTRACT_OBSERVER' ? 0 : 1)
    expect(mocks.erasureStatus).toHaveBeenCalledTimes(role === 'ARCHIVE_MANAGER' ? 1 : 0)
    wrapper.unmount()
  })
  it('shows pending, empty and retry results without stale content', async () => {
    const pending = deferred<(typeof entry)[]>()
    mocks.retrieve.mockReturnValueOnce(pending.promise)
    const wrapper = render()
    await flushPromises()
    expect(wrapper.get('[role="status"]').text()).toContain('Loading archived contracts')
    expect(wrapper.find('[data-testid="archive-entries-empty"]').exists()).toBe(false)
    pending.resolve([])
    await flushPromises()
    expect(wrapper.get('[data-testid="archive-entries-empty"]').text()).toBe('No archived contracts.')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain(entry.name)
    expect(wrapper.find('[data-testid="archive-entries-empty"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
