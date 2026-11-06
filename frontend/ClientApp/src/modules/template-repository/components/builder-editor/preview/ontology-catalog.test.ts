import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it } from 'vitest'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { ONTOLOGY_DOMAIN_FIELDS } from '@template-repository/utils/ontology-domain-fields'
import { resolveValueOptions } from '@template-repository/utils/value-option-catalog'
import {
  applyInlineSemanticValues,
  fromDocumentSemanticValues,
} from '@contract-workflow-engine/utils/semantic-condition-values'
import PreviewParamInput from './PreviewParamInput.vue'

const NS = 'https://w3id.org/facis/dcs/taxonomy/v1#'
const historicalFields = [
  ['field-company-legalName', 'string'],
  ['field-company-registrationNumber', 'string'],
  ['field-company-vatId', 'string'],
  ['field-company-representative-name', 'string'],
  ['field-company-representative-role', 'string'],
  ['field-company-contact-email', 'string'],
  ['field-company-contact-phone', 'string'],
  ['field-company-location-street', 'string'],
  ['field-company-location-postalCode', 'string'],
  ['field-company-location-city', 'string'],
  ['field-company-location-region', 'string'],
  ['field-company-location-country', 'string'],
  ['field-contract-type', 'string'],
  ['field-contract-title', 'string'],
  ['field-contract-jurisdiction', 'string'],
  ['field-contract-governingLaw', 'string'],
  ['field-contract-disputeResolution-venue', 'string'],
  ['field-contract-validity-startDate', 'date'],
  ['field-contract-validity-endDate', 'date'],
  ['field-contract-effectiveDate', 'date'],
  ['field-contract-renewal-noticePeriodDays', 'integer'],
  ['field-contract-termination-noticePeriodDays', 'integer'],
  ['field-contract-payment-amount', 'decimal'],
  ['field-contract-payment-currency', 'string'],
  ['field-contract-payment-dueDate', 'date'],
  ['field-contract-payment-dueDays', 'integer'],
  ['field-contract-liability-capAmount', 'decimal'],
  ['field-contract-liability-currency', 'string'],
  ['field-contract-insurance-minimumCoverage', 'decimal'],
  ['field-contract-confidentiality-durationMonths', 'integer'],
  ['field-contract-dataProtection-role', 'string'],
  ['field-contract-disputeResolution-method', 'string'],
  ['field-contract-auditRights-frequency', 'string'],
  ['field-contract-ipRights-ownership', 'string'],
  ['field-contract-forceMajeure-noticeDays', 'integer'],
  ['field-service-description', 'string'],
  ['field-service-scope', 'string'],
  ['field-service-deliverable-name', 'string'],
  ['field-service-deliverable-acceptanceCriteria', 'string'],
  ['field-service-sla-availability', 'decimal'],
  ['field-service-sla-responseTime', 'integer'],
  ['field-service-sla-resolutionTime', 'integer'],
  ['field-service-sla-supportHours', 'string'],
  ['field-signature-requiredLevel', 'string'],
  ['field-signature-requiredSignerRole', 'string'],
  ['field-signature-deadline', 'date'],
] as const
function field(name: string) {
  const result = ONTOLOGY_DOMAIN_FIELDS.find((candidate) => candidate.ontologyId === NS + name)
  expect(result, name).toBeDefined()
  return result!
}

describe('seeded domain-field catalog', () => {
  it('restores all historical identities and datatypes and retains the newer fields', () => {
    expect(ONTOLOGY_DOMAIN_FIELDS).toHaveLength(48)
    for (const [name, datatype] of historicalFields) {
      expect(field(name).datatype).toBe(`xsd:${datatype}`)
      expect(field(name).label).toBeTruthy()
    }
    for (const name of ['industry', 'regulatoryComplianceStatus']) {
      expect(field(`field-contract-${name}`).type).toBe('string')
      expect(resolveValueOptions(field(`field-contract-${name}`).valueConstraint)).toEqual([])
    }
  })

  it.each([
    ['field-contract-jurisdiction', 23],
    ['field-company-location-country', 23],
    ['field-contract-payment-currency', 6],
    ['field-contract-liability-currency', 6],
    ['field-contract-governingLaw', 7],
    ['field-contract-type', 6],
    ['field-contract-dataProtection-role', 4],
    ['field-contract-disputeResolution-method', 3],
    ['field-contract-auditRights-frequency', 4],
    ['field-contract-ipRights-ownership', 4],
    ['field-signature-requiredLevel', 3],
  ])('provides labelled choices for %s', (name, count) => {
    const definition = field(String(name))
    const options = resolveValueOptions(definition.valueConstraint)
    expect(options.length).toBeGreaterThanOrEqual(Number(count))
    expect(new Set(options.map((option) => option.value)).size).toBe(options.length)
    expect(options.map((option) => option.value).sort()).toEqual([...definition.valueConstraint!.allowedValues!].sort())
    for (const option of options) {
      expect(option.label).toBeTruthy()
      expect(option.label).not.toBe(option.value)
    }
    const wrapper = mount(PreviewParamInput, {
      props: { type: definition.type, valueConstraint: definition.valueConstraint },
    })
    expect(wrapper.find('select').exists()).toBe(true)
    wrapper.unmount()
  })

  it.each(['DEU', 'AUT'])('displays country names and stores %s through a draft round trip', async (code) => {
    setActivePinia(createPinia())
    const definition = field('field-contract-jurisdiction')
    const store = useDcsDraftStore()
    const id = 'urn:uuid:jurisdiction'
    store.addClauseWithMeaning({
      title: 'Jurisdiction',
      content: [{ '@id': id }],
      fields: [
        { id, parameterName: definition.parameterName, domainFieldIri: definition.ontologyId, label: definition.label },
      ],
      assets: [],
      rule: null,
    })
    const wrapper = mount(PreviewParamInput, {
      props: { type: definition.type, label: definition.label, valueConstraint: definition.valueConstraint },
    })
    expect(wrapper.get('option[value="DEU"]').text()).toBe('Germany (DEU)')
    expect(wrapper.get('option[value="AUT"]').text()).toBe('Austria (AUT)')
    await wrapper.get('select').setValue(code)
    const emitted = wrapper.emitted('update:value')!
    const value = emitted[emitted.length - 1]![0] as string
    expect(value).toBe(code)
    store.contractFields = applyInlineSemanticValues(store.contractFields, [
      { blockId: '', conditionId: id, parameterName: definition.parameterName, parameterValue: value },
    ])
    expect(store.contractFields[0]?.['dcs:value']).toEqual({ '@type': 'xsd:string', '@value': code })
    const saved: unknown = JSON.parse(JSON.stringify(store.templateDocument))
    setActivePinia(createPinia())
    const reloaded = useDcsDraftStore()
    reloaded.loadDocument(saved, { did: 'urn:uuid:template', name: 'SLA', description: '' })
    expect(fromDocumentSemanticValues(reloaded.contractFields)[0]?.parameterValue).toBe(code)
    await wrapper.setProps({ value: fromDocumentSemanticValues(reloaded.contractFields)[0]?.parameterValue })
    expect(wrapper.get('select').element.value).toBe(code)
    wrapper.unmount()
  })

  it.each([
    ['field-company-contact-email', 'string', 'text'],
    ['field-contract-validity-startDate', 'date', 'date'],
    ['field-contract-payment-amount', 'decimal', 'text'],
    ['field-contract-payment-dueDays', 'integer', 'text'],
  ])('retains the input datatype of %s', (name, type, inputType) => {
    const definition = field(name)
    expect(definition.type).toBe(type)
    const wrapper = mount(PreviewParamInput, {
      props: { type: definition.type, valueConstraint: definition.valueConstraint },
    })
    expect(wrapper.find('select').exists()).toBe(false)
    expect(wrapper.get('input').attributes('type')).toBe(inputType)
    wrapper.unmount()
  })

  it('restores nonnegative SLA minutes and keeps availability bounded', () => {
    for (const name of ['responseTime', 'resolutionTime']) {
      expect(field(`field-service-sla-${name}`).valueConstraint).toMatchObject({ min: 0, format: 'duration-minutes' })
    }
    expect(field('field-service-sla-availability').valueConstraint).toMatchObject({ min: 0, max: 100 })
  })
})
