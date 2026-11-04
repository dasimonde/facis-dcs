import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { contractTemplateService } from '@/services/contract-template-service'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import ListSearch from './ListSearch.vue'

vi.mock('@/api/http', () => ({ default: { get: vi.fn().mockRejectedValue(new Error('Search unavailable')) } }))

describe('search failures', () => {
  it.each([contractTemplateService.search, contractWorkflowService.search])(
    'shows a service failure',
    async (search) => {
      const wrapper = mount(ListSearch, {
        props: {
          items: [],
          emptyItem: { did: '' },
          filterLabels: { did: 'DID' },
          searchFn: search,
          filters: {},
        },
      })
      await wrapper.setProps({ filters: { semantic_filters: '[{"filter_id":"field","value":"x"}]' } })
      await flushPromises()
      expect(wrapper.get('[role="alert"]').text()).toContain('Search unavailable')
      expect(wrapper.emitted('searchResult')).toBeUndefined()
      wrapper.unmount()
    },
  )
})
