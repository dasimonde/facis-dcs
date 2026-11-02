import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { reactive } from 'vue'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import ArchiveDetailView from './ArchiveDetailView.vue'
import type { Contract } from '@/models/contract/contract'

const detail = vi.hoisted(() => vi.fn())
const route = reactive({ query: { did: 'did:test:archive', contract_version: '2' } })
vi.mock('vue-router', () => ({ useRoute: () => route, RouterLink: { template: '<a><slot /></a>' } }))
vi.mock('@/router/router', () => ({ ROUTES: { ARCHIVE: { DASHBOARD: 'archive' } } }))
vi.mock('@/services/archive-service', () => ({ archiveService: { detail } }))
function snapshot(): Contract {
  const doc = useDcsDraftStore().templateDocument
  return {
    did: 'did:test:archive',
    contract_version: 2,
    state: 'SIGNED',
    name: 'Frozen snapshot',
    created_by: 'creator',
    created_at: '',
    updated_at: '',
    contract_data: { ...doc, '@type': 'dcs:Contract' },
  }
}
function render() {
  return mount(ArchiveDetailView, { global: { stubs: { TemplatePreview: true } } })
}
describe('exact immutable snapshot lifecycle', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    detail.mockReset()
    route.query = { did: 'did:test:archive', contract_version: '2' }
  })
  it.each(['array', '@list'])(
    'renders the real preview with %s layout without changing the snapshot',
    async (format) => {
      const archived = snapshot()
      const layout = [
        {
          '@id': 'root',
          '@type': 'dcs:LayoutNode',
          'dcs:isRoot': true,
          'dcs:children': { '@list': [{ '@id': 'second' }, { '@id': 'first' }] },
        },
      ]
      Object.assign(archived.contract_data!['dcs:documentStructure'], {
        'dcs:layout': format === 'array' ? layout : { '@list': layout },
        'dcs:blocks': {
          '@list': [
            { '@id': 'first', '@type': 'dcs:TextBlock', 'dcs:text': 'First stored block' },
            { '@id': 'second', '@type': 'dcs:TextBlock', 'dcs:text': 'Second stored block' },
          ],
        },
      })
      const original = JSON.stringify(archived.contract_data)
      detail.mockResolvedValueOnce(archived)
      const wrapper = mount(ArchiveDetailView)
      await flushPromises()
      expect(wrapper.find('[role="alert"]').exists()).toBe(false)
      expect(wrapper.get('section[aria-label="Archived contract content"]').text()).toBe(
        'Second stored blockFirst stored block',
      )
      expect(wrapper.text()).toContain('did:test:archive')
      expect(wrapper.text()).toContain('Version 2')
      expect(JSON.parse(wrapper.get('[data-testid="machine-readable-payload"]').text())).toEqual(JSON.parse(original))
      expect(JSON.stringify(archived.contract_data)).toBe(original)
      wrapper.unmount()
    },
  )
  it('places full technical metadata after the document in a closed disclosure', async () => {
    detail.mockResolvedValueOnce({
      ...snapshot(),
      description: 'Stored description',
      start_date: '2026-10-01',
      exp_date: '2027-10-01',
    })
    const wrapper = render()
    await flushPromises()
    const disclosure = wrapper.get('details')
    expect(disclosure.attributes('open')).toBeUndefined()
    expect(disclosure.get('summary').text()).toBe('Archive metadata')
    expect(disclosure.text()).toContain('Stored description')
    expect(disclosure.text()).toContain('Created bycreator')
    const content = wrapper.get('section[aria-label="Archived contract content"]').element
    expect(content.compareDocumentPosition(disclosure.element) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    const identity = wrapper.get('dl')
    expect(identity.text()).toContain('did:test:archive')
    expect(identity.text()).toContain('SIGNED')
    expect(identity.text()).toContain('2026-10-01')
    expect(identity.text()).toContain('2027-10-01')
    expect(identity.text()).not.toContain('Created by')
    wrapper.unmount()
  })
  it('removes the previous snapshot while another version loads and ignores stale replies', async () => {
    detail.mockResolvedValueOnce(snapshot())
    const wrapper = render()
    await flushPromises()
    expect(wrapper.text()).toContain('Frozen snapshot')
    let resolve!: (value: Contract) => void
    detail
      .mockReturnValueOnce(
        new Promise<Contract>((res) => {
          resolve = res
        }),
      )
      .mockRejectedValueOnce(new Error('Deleted'))
    route.query.contract_version = '3'
    await flushPromises()
    expect(wrapper.text()).not.toContain('Frozen snapshot')
    route.query.contract_version = '4'
    await flushPromises()
    resolve({ ...snapshot(), contract_version: 3 })
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('Deleted')
    expect(wrapper.find('[data-testid="machine-readable-view"]').exists()).toBe(false)
    wrapper.unmount()
  })
  it.each(['identity', 'missing content'])('rejects %s instead of showing a misleading document', async (problem) => {
    detail.mockResolvedValue(
      problem === 'identity' ? { ...snapshot(), contract_version: 99 } : { ...snapshot(), contract_data: undefined },
    )
    const wrapper = render()
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('incomplete or does not match')
    expect(wrapper.find('[data-testid="machine-readable-view"]').exists()).toBe(false)
  })
})
