import { flushPromises, shallowMount } from '@vue/test-utils'
import { beforeEach, expect, it, vi } from 'vitest'
import ArchiveDashboardView from './ArchiveDashboardView.vue'

const mocks = vi.hoisted(() => ({
  retrieve: vi.fn(),
  erasureStatus: vi.fn(),
  statistics: vi.fn(),
  annotate: vi.fn(),
  addMessage: vi.fn(),
}))
vi.mock('@/services/archive-service', () => ({ archiveService: mocks }))
vi.mock('@/services/archive-statistics-service', () => ({ archiveStatisticsService: mocks }))
vi.mock('@/stores/auth-store', () => ({ useAuthStore: () => ({ user: { roles: ['ARCHIVE_MANAGER'] } }) }))
vi.mock('@/stores/error-store', () => ({ useErrorStore: () => ({ add: mocks.addMessage }) }))
vi.mock('@/router/router', () => ({ ROUTES: { ARCHIVE: { DETAIL: 'detail' }, AUDIT: { LIST: 'audit' } } }))
const entry = { did: 'did:test:archive', contract_version: 1, name: 'Example', state: 'ARCHIVED' }
async function openDialog(entries: object[] = [entry]) {
  mocks.retrieve.mockResolvedValue(entries)
  const wrapper = shallowMount(ArchiveDashboardView, {
    global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } },
  })
  await flushPromises()
  await wrapper.get(`[data-testid="archive-annotate-${entry.did}"]`).trigger('click')
  const dialog = wrapper.get('[data-testid="archive-annotation-dialog"]')
  expect((dialog.element as HTMLDialogElement).open).toBe(true)
  return { wrapper, dialog }
}
beforeEach(() => {
  vi.resetAllMocks()
  HTMLDialogElement.prototype.showModal = vi.fn(function (this: HTMLDialogElement) {
    this.open = true
  })
  HTMLDialogElement.prototype.close = vi.fn(function (this: HTMLDialogElement) {
    this.open = false
  })
  mocks.statistics.mockResolvedValue(null)
  mocks.erasureStatus.mockResolvedValue({ did: entry.did, local_status: 'live', peers: [] })
})
it.each([false, true])(
  'prefills both fields when present=%s and saves them with one request and a success message',
  async (present) => {
    const { wrapper, dialog } = await openDialog([
      { ...entry, ...(present ? { archive_summary: 'Original summary', archive_tags: ['original', 'qa'] } : {}) },
    ])
    expect((dialog.get('textarea').element as HTMLTextAreaElement).value).toBe(present ? 'Original summary' : '')
    expect((dialog.get('input').element as HTMLInputElement).value).toBe(present ? 'original, qa' : '')
    await dialog.get('textarea').setValue('  Revised summary  ')
    await dialog.get('input').setValue(' supply, renewed ')
    expect(mocks.annotate).not.toHaveBeenCalled()
    await dialog.get('form').trigger('submit')
    await flushPromises()
    expect(mocks.annotate).toHaveBeenCalledExactlyOnceWith(entry.did, 'Revised summary', ['supply', 'renewed'])
    expect(mocks.addMessage).toHaveBeenCalledExactlyOnceWith('Annotation saved.', 'info')
    expect((dialog.element as HTMLDialogElement).open).toBe(false)
    expect(mocks.retrieve).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  },
)
it('does not write when the dialog is canceled', async () => {
  const { wrapper, dialog } = await openDialog()
  await dialog.get('textarea').setValue('Summary')
  await dialog.get('button[type="button"]').trigger('click')
  await flushPromises()
  expect((dialog.element as HTMLDialogElement).open).toBe(false)
  expect(mocks.annotate).not.toHaveBeenCalled()
  expect(mocks.addMessage).not.toHaveBeenCalled()
  wrapper.unmount()
})
it('preserves omission semantics for empty summary and tags', async () => {
  const { wrapper, dialog } = await openDialog()
  await dialog.get('textarea').setValue('  ')
  await dialog.get('input').setValue(' , ')
  await dialog.get('form').trigger('submit')
  await flushPromises()
  expect(mocks.annotate).toHaveBeenCalledExactlyOnceWith(entry.did, undefined, undefined)
  wrapper.unmount()
})
it('keeps the dialog and its input open and shows the error when saving fails', async () => {
  mocks.annotate.mockRejectedValue(new Error('Annotation storage unavailable'))
  const { wrapper, dialog } = await openDialog()
  await dialog.get('textarea').setValue('Kept summary')
  await dialog.get('form').trigger('submit')
  await flushPromises()
  expect(mocks.annotate).toHaveBeenCalledOnce()
  expect((dialog.element as HTMLDialogElement).open).toBe(true)
  expect(dialog.get('[role="alert"]').text()).toBe('Annotation storage unavailable')
  expect((dialog.get('textarea').element as HTMLTextAreaElement).value).toBe('Kept summary')
  expect(mocks.addMessage).not.toHaveBeenCalled()
  wrapper.unmount()
})
