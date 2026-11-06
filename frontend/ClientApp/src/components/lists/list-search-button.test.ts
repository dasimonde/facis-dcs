import { flushPromises, mount } from '@vue/test-utils'
import { expect, it, vi } from 'vitest'
import ListSearch from './ListSearch.vue'

function searchButton(wrapper: ReturnType<typeof mount>) {
  return wrapper.findAll('button').find((button) => button.text() === 'Search')!
}

it('Search uses typed text even when Filters are empty and change has not fired', async () => {
  const searchFn = vi.fn().mockResolvedValue([{ did: 'did:web:hit' }])
  const wrapper = mount(ListSearch, {
    props: {
      items: [],
      emptyItem: { did: '' },
      filterLabels: { did: 'DID' },
      searchFn,
      extraRequest: {},
    },
  })

  const input = wrapper.get('input')

  ;(input.element as HTMLInputElement).value = 'did:web:hit'
  await searchButton(wrapper).trigger('click')
  await flushPromises()

  expect(searchFn).toHaveBeenCalledWith({ did: 'did:web:hit' })
  const emitted = wrapper.emitted('searchResult')!
  expect(emitted[emitted.length - 1]?.[0]).toEqual([{ did: 'did:web:hit' }])
  wrapper.unmount()
})

it('Search keeps a typed query that was never selected from the autocomplete', async () => {
  const searchFn = vi.fn().mockResolvedValue([{ did: 'did:web:hit' }])
  const wrapper = mount(ListSearch, {
    props: {
      items: [] as { did: string }[],
      emptyItem: { did: '' },
      filterLabels: { did: 'DID' },
      searchFn,
      extraRequest: {},
    },
  })

  const input = wrapper.get('input')
  ;(input.element as HTMLInputElement).value = 'did:web:hit'
  await input.trigger('input')
  expect((input.element as HTMLInputElement).value).toBe('did:web:hit')
  await searchButton(wrapper).trigger('click')
  await flushPromises()

  expect(searchFn).toHaveBeenCalledWith({ did: 'did:web:hit' })
  const emitted = wrapper.emitted('searchResult')!
  expect(emitted[emitted.length - 1]?.[0]).toEqual([{ did: 'did:web:hit' }])
  wrapper.unmount()
})

it('keeps Search enabled and accepts a newer query while an older search is still open', async () => {
  let resolveFirst!: (value: { did: string }[]) => void
  const searchFn = vi
    .fn()
    .mockImplementationOnce(
      () =>
        new Promise<{ did: string }[]>((resolve) => {
          resolveFirst = resolve
        }),
    )
    .mockResolvedValueOnce([{ did: 'did:web:second' }])

  const wrapper = mount(ListSearch, {
    props: {
      items: [],
      emptyItem: { did: '' },
      filterLabels: { did: 'DID' },
      searchFn,
      extraRequest: { party_type: 'SINGLE_PARTY' },
    },
  })

  await searchButton(wrapper).trigger('click')
  expect(searchButton(wrapper).attributes('disabled')).toBeUndefined()

  await wrapper.setProps({ extraRequest: { party_type: 'MULTI_PARTY' } })
  await searchButton(wrapper).trigger('click')
  await flushPromises()

  expect(searchFn).toHaveBeenCalledTimes(2)
  expect(searchFn).toHaveBeenLastCalledWith({ party_type: 'MULTI_PARTY' })
  const afterSecond = wrapper.emitted('searchResult')!
  expect(afterSecond[afterSecond.length - 1]?.[0]).toEqual([{ did: 'did:web:second' }])

  resolveFirst([{ did: 'did:web:stale' }])
  await flushPromises()
  const afterStale = wrapper.emitted('searchResult')!
  expect(afterStale[afterStale.length - 1]?.[0]).toEqual([{ did: 'did:web:second' }])
  wrapper.unmount()
})
