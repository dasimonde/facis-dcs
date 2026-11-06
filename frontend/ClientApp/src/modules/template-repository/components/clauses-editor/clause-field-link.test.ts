import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import { nextTick } from 'vue'
import ClauseTextEditor from './ClauseTextEditor.vue'
import type { SemanticCondition } from '@template-repository/models/contract-template'

const conditions: SemanticCondition[] = [
  {
    conditionId: 'c1',
    conditionName: 'Service',
    schemaVersion: 'v1',
    parameters: [
      {
        parameterName: 'description',
        fieldId: 'urn:uuid:field-service-description',
        type: 'string',
        fieldIri: 'https://w3id.org/facis/dcs/taxonomy/v1#field-service-description',
        isRequired: true,
        operators: [],
        value: undefined,
      },
    ],
  },
]

/** DCS-FR-TR-04: clause prose links to its field in the rule panel. */
describe('clause text to field link', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('highlights the field of a clicked prose placeholder', async () => {
    const wrapper = mount(ClauseTextEditor, {
      attachTo: document.body,
      props: {
        textId: 'prose',
        semanticConditions: conditions,
        modelValue: ['Service: ', { '@id': 'urn:uuid:field-service-description' }],
      },
    })
    await nextTick()
    await nextTick()
    expect(wrapper.find('[data-highlighted]').exists()).toBe(false)

    const chip = (wrapper.element as HTMLElement).querySelector('[data-condition-id="c1"]')
    expect(chip).not.toBeNull()
    chip!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()

    const highlighted = wrapper.findAll('[data-highlighted]')
    expect(highlighted).toHaveLength(1)
    expect(highlighted[0]!.text()).toContain('required')
    wrapper.unmount()
  })
})
