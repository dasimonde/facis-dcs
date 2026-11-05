import { mount } from '@vue/test-utils'
import { flushPromises } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import SemanticSearchFilters from './SemanticSearchFilters.vue'

const parse = vi.hoisted(() =>
  vi.fn((content: string) => ({
    ontologyId: 'urn:field',
    label: 'Field',
    parameterName: 'field',
    type: content === 'v1' ? 'boolean' : 'string',
  })),
)
vi.mock('@template-repository/utils/ontology-domain-fields', () => ({
  parseSearchFieldFromSchema: parse,
  ONTOLOGY_VALUE_CONSTRAINTS: [],
  ONTOLOGY_DOMAIN_FIELDS: [
    { valueConstraint: { allowedValuesRef: 'ISO 3166-1 alpha-3', valueOptions: [{ value: 'DEU', label: 'Germany' }] } },
  ],
}))
vi.mock('@/services/semantic-hub-service', () => ({
  getSearchFilters: vi.fn().mockResolvedValue({
    revision: 1,
    filters: [
      { id: 'field', label: 'Field', field_iri: 'urn:field', source: { name: 'test', kind: 'ontology', version: 1 } },
    ],
  }),
  getSchemaVersions: vi.fn().mockResolvedValue([
    { name: 'test', kind: 'ontology', version: 1, content: 'v1' },
    { name: 'test', kind: 'ontology', version: 2, content: 'v2' },
  ]),
}))

describe('SemanticSearchFilters', () => {
  it('uses the configured source version even when a newer version exists', async () => {
    const wrapper = mount(SemanticSearchFilters)
    await flushPromises()
    expect(parse).toHaveBeenCalledWith('v1', 'ontology', 'urn:field')
    expect(wrapper.find('select').exists()).toBe(true)
    await wrapper.find('select').setValue('false')
    const emissions = wrapper.emitted('update:modelValue') ?? []
    expect(emissions[emissions.length - 1]?.[0]).toBe('[{"filter_id":"field","value":false}]')
    wrapper.unmount()
  })

  it('uses allowed values when the options array is empty', async () => {
    parse.mockReturnValueOnce({
      ontologyId: 'urn:field',
      label: 'Field',
      parameterName: 'field',
      type: 'enum',
      valueConstraint: { valueOptions: [], allowedValues: ['DE'] },
    } as never)
    const wrapper = mount(SemanticSearchFilters)
    await flushPromises()
    expect(wrapper.findAll('option').map((option) => option.text())).toContain('DE')
    wrapper.unmount()
  })

  it('resolves referenced choices from the ontology catalog', async () => {
    parse.mockReturnValueOnce({
      ontologyId: 'urn:field',
      label: 'Field',
      parameterName: 'field',
      type: 'string',
      valueConstraint: { allowedValuesRef: 'iso 3166-1 alpha-3' },
    } as never)
    const wrapper = mount(SemanticSearchFilters)
    await flushPromises()
    expect(wrapper.findAll('option').map((option) => option.text())).toContain('Germany')
    wrapper.unmount()
  })

  it('sends numeric catalog values with their declared datatype', async () => {
    parse.mockReturnValueOnce({
      ontologyId: 'urn:field',
      label: 'Field',
      parameterName: 'field',
      type: 'enum',
      datatype: 'xsd:integer',
      valueConstraint: { allowedValues: ['0', '1'] },
    } as never)
    const wrapper = mount(SemanticSearchFilters)
    await flushPromises()
    await wrapper.find('select').setValue('0')
    const emissions = wrapper.emitted('update:modelValue') ?? []
    expect(emissions[emissions.length - 1]?.[0]).toBe('[{"filter_id":"field","value":"0"}]')
    wrapper.unmount()
  })

  it('sends large integers without rounding', async () => {
    parse.mockReturnValueOnce({
      ontologyId: 'urn:field',
      label: 'Field',
      parameterName: 'field',
      type: 'integer',
      datatype: 'xsd:integer',
    } as never)
    const wrapper = mount(SemanticSearchFilters)
    await flushPromises()
    await wrapper.get('input').setValue('9007199254740992')
    const emissions = wrapper.emitted('update:modelValue') ?? []
    expect(emissions[emissions.length - 1]?.[0]).toBe('[{"filter_id":"field","value":"9007199254740992"}]')
    wrapper.unmount()
  })
})
