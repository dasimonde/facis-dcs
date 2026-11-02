import { flushPromises, mount } from '@vue/test-utils'
import { expect, it, vi } from 'vitest'
import TemplateListSearch from './TemplateListSearch.vue'
import type { PartialContractTemplate } from '@/models/contract-template/contract-template'

/**
 * DCS-IR-TR-07, DCS-IR-TR-08: the dashboard search filters the templates by
 * state on the server and shows the creator of each result.
 */

const search = vi.fn()

vi.mock('@/services/contract-template-service', () => ({
  contractTemplateService: {
    search: (request: Record<string, unknown>) => search(request),
  },
}))

it('sends the chosen state and keeps the creator of each result', async () => {
  search.mockResolvedValue([
    {
      did: 'did:web:example.com:template:1',
      version: '2',
      state: 'REJECTED',
      party_type: 'SINGLE_PARTY',
      template_type: 'CONTRACT_TEMPLATE',
      created_at: '2026-07-01T08:00:00Z',
      created_by: 'did:web:example.com:creator',
      updated_at: '2026-07-01T09:00:00Z',
    },
  ])
  const wrapper = mount(TemplateListSearch, { props: { templates: [] } })
  const stateLabel = wrapper.findAll('label').find((label) => label.text() === 'State')!

  await wrapper.get(`#${CSS.escape(stateLabel.attributes('for')!)}`).setValue('REJECTED')
  await flushPromises()

  expect(search).toHaveBeenCalledWith(expect.objectContaining({ state: 'REJECTED' }))
  const emitted = wrapper.emitted('searchResult')!
  const [result] = emitted[emitted.length - 1] as [PartialContractTemplate[]]
  expect(result.map((template) => [template.version, template.created_by])).toEqual([
    [2, 'did:web:example.com:creator'],
  ])
})

it('sends the creation days and the owner', async () => {
  search.mockResolvedValue([])
  const wrapper = mount(TemplateListSearch, { props: { templates: [] } })
  const input = (text: string) => {
    const label = wrapper.findAll('label').find((candidate) => candidate.text() === text)!
    return wrapper.get(`#${CSS.escape(label.attributes('for')!)}`)
  }

  await input('Created from').setValue('2026-03-01')
  await input('Created to').setValue('2026-03-31')
  await flushPromises()

  expect(search).toHaveBeenLastCalledWith(
    expect.objectContaining({ created_from: '2026-03-01', created_to: '2026-03-31' }),
  )
  expect(wrapper.text()).toContain('Owner')
})
