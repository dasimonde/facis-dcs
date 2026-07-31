import { shallowMount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  type AtomicDraft,
  composeConstraintTree,
  type ConstraintNodeDraft,
  type GroupDraft,
  parseConstraintTree,
} from '@template-repository/components/clauses-editor/constraint-draft'
import OdrlRuleBuilder from '@template-repository/components/clauses-editor/OdrlRuleBuilder.vue'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import type { OdrlConstraint, OdrlLogicalConstraint, OdrlRule } from '@/models/dcs-jsonld'

/**
 * Drives the contract builder's own code with the SLA hosting inputs and
 * asserts what it emits.
 */

const TEMPLATE_IRI = 'did:web:example:template:sla-hosting'
const EUR = 'http://publications.europa.eu/resource/authority/currency/EUR'
const NODE_UNIT = 'https://w3id.org/facis/sla/hosting/v1#unit-node'
const REGION_EMEA = 'https://w3id.org/facis/dcs/taxonomy/v1#service-region-EMEA'

function atomic(leftOperand: string, operator: string, extra: Partial<AtomicDraft> = {}): AtomicDraft {
  return { kind: 'atomic', leftOperand, operator, rightSource: '', value: '', values: [], unit: '', ...extra }
}

function group(combine: GroupDraft['combine'], children: ConstraintNodeDraft[]): GroupDraft {
  return { kind: 'group', combine, children }
}

/** SLA R3: at most N nodes, in an agreed OR listed region, until the end date. */
function workloadConstraintRoot(): GroupDraft {
  return group('and', [
    atomic('odrl:count', 'odrl:lteq', { rightSource: `${TEMPLATE_IRI}#field-provisioned-nodes`, unit: NODE_UNIT }),
    group('or', [
      atomic('odrl:spatial', 'odrl:eq', { rightSource: `${TEMPLATE_IRI}#field-service-region` }),
      atomic('odrl:spatial', 'odrl:isAnyOf', { values: [{ '@id': REGION_EMEA }] }),
    ]),
    atomic('odrl:dateTime', 'odrl:lteq', { rightSource: `${TEMPLATE_IRI}#field-contract-end-date` }),
  ])
}

/** SLA R2: the monthly fee, denominated in EUR. */
function paymentConstraintRoot(): GroupDraft {
  return group('and', [
    atomic('odrl:payAmount', 'odrl:eq', { rightSource: `${TEMPLATE_IRI}#field-monthly-fee`, unit: EUR }),
    atomic('odrl:elapsedTime', 'odrl:lteq', { value: 'P14D' }),
  ])
}

function isLogical(node: OdrlConstraint | OdrlLogicalConstraint): node is OdrlLogicalConstraint {
  return node['@type'] === 'odrl:LogicalConstraint'
}

/** The first member of a list the assertion expects to be there. */
function first<T>(items: readonly T[] | undefined, what: string): T {
  const [head] = items ?? []
  if (head === undefined) throw new Error(`expected ${what} to be emitted`)
  return head
}

describe('composeConstraintTree — SLA constraints', () => {
  it('denominates a payment in EUR and a node count in its unit (odrl:unit)', () => {
    const nodes = composeConstraintTree(paymentConstraintRoot())
    expect(nodes).toBeDefined()
    const [payment, elapsed] = nodes as OdrlConstraint[]
    expect(payment).toEqual({
      '@type': 'odrl:Constraint',
      'odrl:leftOperand': { '@id': 'odrl:payAmount' },
      'odrl:operator': { '@id': 'odrl:eq' },
      'odrl:rightOperand': { '@id': `${TEMPLATE_IRI}#field-monthly-fee` },
      'odrl:unit': { '@id': EUR },
    })
    // A fixed literal boundary carries no unit unless one is authored.
    expect(elapsed?.['odrl:rightOperand']).toEqual({ '@value': 'P14D', '@type': 'xsd:duration' })
    expect(elapsed?.['odrl:unit']).toBeUndefined()

    const count = (composeConstraintTree(workloadConstraintRoot()) as OdrlConstraint[])[0]
    expect(count?.['odrl:unit']).toEqual({ '@id': NODE_UNIT })
  })

  it('emits a depth-3 tree: and( count, or( spatial eq, spatial isAnyOf ), dateTime )', () => {
    const nodes = composeConstraintTree(workloadConstraintRoot())
    expect(nodes).toHaveLength(3)
    const [count, logical, dateTime] = nodes as [OdrlConstraint, OdrlLogicalConstraint, OdrlConstraint]

    // Level 1 is a bare conjunction array, not a wrapping LogicalConstraint.
    expect(count['@type']).toBe('odrl:Constraint')
    expect(dateTime['odrl:rightOperand']).toEqual({ '@id': `${TEMPLATE_IRI}#field-contract-end-date` })

    // Level 2: the OR group, its members ordered in an explicit @list.
    expect(isLogical(logical)).toBe(true)
    const members = logical['odrl:or']?.['@list'] as OdrlConstraint[]
    expect(members).toHaveLength(2)
    // Level 3: a field reference on one branch, a set operand on the other.
    expect(members[0]?.['odrl:rightOperand']).toEqual({ '@id': `${TEMPLATE_IRI}#field-service-region` })
    expect(members[1]?.['odrl:operator']).toEqual({ '@id': 'odrl:isAnyOf' })
    expect(members[1]?.['odrl:rightOperand']).toEqual([{ '@id': REGION_EMEA }])
  })

  it('round-trips: compose → parse → compose is stable', () => {
    const first = composeConstraintTree(workloadConstraintRoot())
    expect(first).toBeDefined()
    const reparsed = parseConstraintTree(first!)
    const second = composeConstraintTree(reparsed)
    expect(second).toEqual(first)
    // And once more, so a loss that only shows on the second pass is caught.
    expect(composeConstraintTree(parseConstraintTree(second!))).toEqual(first)
  })
})

/** Mounts the rule builder, drives its draft, and returns the rule it emits. */
async function emitRule(seed: (draft: RuleBuilderDraft) => void): Promise<OdrlRule> {
  const wrapper = shallowMount(OdrlRuleBuilder, {
    props: {
      modelValue: null,
      fields: [{ id: `${TEMPLATE_IRI}#field-provisioned-nodes`, label: 'Provisioned nodes' }],
      assets: [{ id: `${TEMPLATE_IRI}#asset-hosting-environment`, label: 'Hosting environment' }],
      parties: [
        { id: `${TEMPLATE_IRI}#party-provider`, label: 'Provider' },
        { id: `${TEMPLATE_IRI}#party-customer`, label: 'Customer' },
      ],
      proseId: `${TEMPLATE_IRI}#block-clause-environment`,
      contractTargetId: TEMPLATE_IRI,
    },
  })
  const draft = (wrapper.vm as unknown as { draft: RuleBuilderDraft }).draft
  seed(draft)
  await wrapper.vm.$nextTick()
  const emitted = wrapper.emitted('update:modelValue')
  expect(emitted).toBeTruthy()
  const last = emitted![emitted!.length - 1]?.[0] as OdrlRule | null
  expect(last).not.toBeNull()
  return last!
}

interface RuleBuilderDraft {
  type: string
  actions: string[]
  assigneeId: string
  assignerId: string
  targetId: string
  root: GroupDraft
  duties: { action: string; root: GroupDraft; consequences: { action: string; root: GroupDraft }[] }[]
}

/** The SLA's workload-execution permission, as the rule builder assembles it. */
function seedWorkloadPermission(draft: RuleBuilderDraft): void {
  draft.type = 'odrl:Permission'
  draft.actions = ['odrl:execute']
  draft.assignerId = `${TEMPLATE_IRI}#party-provider`
  draft.assigneeId = `${TEMPLATE_IRI}#party-customer`
  draft.targetId = `${TEMPLATE_IRI}#asset-hosting-environment`
  draft.root = workloadConstraintRoot()
  draft.duties = [
    {
      action: 'dcs:provideCompliantValue',
      root: group('and', [atomic('odrl:percentage', 'odrl:gteq', { value: '99.5' })]),
      consequences: [
        {
          action: 'odrl:compensate',
          root: group('and', [
            atomic('odrl:percentage', 'odrl:eq', { rightSource: `${TEMPLATE_IRI}#field-service-credit-rate` }),
          ]),
        },
      ],
    },
  ]
}

describe('OdrlRuleBuilder — the SLA permission', () => {
  it('emits a permission whose duty carries a consequence', async () => {
    const rule = await emitRule(seedWorkloadPermission)

    expect(rule['@type']).toBe('odrl:Permission')
    expect(rule['odrl:action']).toEqual({ '@id': 'odrl:execute' })
    expect(rule['odrl:target']).toEqual({ '@id': `${TEMPLATE_IRI}#asset-hosting-environment` })
    expect(rule['dcs:prose']).toEqual({ '@id': `${TEMPLATE_IRI}#block-clause-environment` })
    expect(rule['odrl:constraint']).toHaveLength(3)

    const duty = first(rule['odrl:duty'], 'a duty')
    expect(duty['@type']).toBe('odrl:Duty')
    expect(duty['odrl:action']).toEqual({ '@id': 'dcs:provideCompliantValue' })
    const consequence = first(duty['odrl:consequence'], 'a consequence duty')
    expect(consequence['odrl:action']).toEqual({ '@id': 'odrl:compensate' })
    expect(first(consequence['odrl:constraint'], 'a consequence constraint')).toMatchObject({
      'odrl:rightOperand': { '@id': `${TEMPLATE_IRI}#field-service-credit-rate` },
    })

    // buildDuty emits no dcs:prose — the store binds it when the rule is
    // attached to its clause (bindRuleProse/bindDutyProse). A duty read
    // straight off the component is therefore not yet hub-conformant.
    expect(duty['dcs:prose']).toBeUndefined()
    expect(consequence['dcs:prose']).toBeUndefined()
  })
})

describe('the assembled SLA template document', () => {
  let counter = 0

  beforeEach(() => {
    setActivePinia(createPinia())
    counter = 0
    vi.spyOn(crypto, 'randomUUID').mockImplementation(() => {
      counter += 1
      return `00000000-0000-4000-8000-${String(counter).padStart(12, '0')}`
    })
  })

  afterEach(() => vi.restoreAllMocks())

  it('binds duty prose on assembly and writes the builder-emitted fixture', async () => {
    const permission = await emitRule(seedWorkloadPermission)
    const payment = await emitRule((draft) => {
      draft.type = 'odrl:Duty'
      draft.actions = ['odrl:compensate']
      draft.assignerId = `${TEMPLATE_IRI}#party-provider`
      draft.assigneeId = `${TEMPLATE_IRI}#party-customer`
      draft.targetId = TEMPLATE_IRI
      draft.root = paymentConstraintRoot()
    })

    const store = useDcsDraftStore()
    store.$patch({ did: TEMPLATE_IRI, name: 'SLA Hosting', description: 'Managed Kubernetes hosting SLA' })

    store.addClauseWithMeaning({
      title: 'Hosting environment',
      content: ['The provider operates the managed Kubernetes environment agreed below.'],
      fields: [
        {
          id: `${TEMPLATE_IRI}#field-provisioned-nodes`,
          parameterName: 'provisionedNodes',
          domainFieldIri: 'https://w3id.org/facis/sla/hosting/v1#provisionedNodes',
          label: 'Provisioned nodes',
        },
        {
          id: `${TEMPLATE_IRI}#field-service-region`,
          parameterName: 'serviceRegion',
          domainFieldIri: 'https://w3id.org/facis/sla/hosting/v1#serviceRegion',
          label: 'Service region',
        },
      ],
      assets: [
        {
          id: `${TEMPLATE_IRI}#asset-hosting-environment`,
          classIri: 'https://w3id.org/facis/sla/hosting/v1#ManagedKubernetesEnvironment',
          properties: [
            {
              fieldId: `${TEMPLATE_IRI}#field-provisioned-nodes`,
              path: 'https://w3id.org/facis/sla/hosting/v1#provisionedNodes',
            },
            {
              fieldId: `${TEMPLATE_IRI}#field-service-region`,
              path: 'https://w3id.org/facis/sla/hosting/v1#serviceRegion',
            },
          ],
        },
      ],
      rule: permission,
    })

    store.addClauseWithMeaning({
      title: 'Charges',
      content: ['The customer pays the monthly fee for the billing period.'],
      fields: [
        {
          id: `${TEMPLATE_IRI}#field-monthly-fee`,
          parameterName: 'monthlyFee',
          domainFieldIri: 'https://w3id.org/facis/sla/hosting/v1#field-service-monthlyFee',
          label: 'Payment Amount',
        },
      ],
      rule: payment,
    })

    const document = store.templateDocument
    const policies = document['dcs:policies']

    expect(policies['@type']).toBe('odrl:Offer')
    expect(policies['odrl:profile']['@id']).toBeTruthy()
    const assembled = first(policies['odrl:permission'], 'the assembled permission')
    const assembledDuty = first(assembled['odrl:duty'], 'the assembled duty')
    // The store binds prose down the whole duty chain; the component does not.
    expect(assembledDuty['dcs:prose']).toEqual(assembled['dcs:prose'])
    expect(assembledDuty['odrl:consequence']?.[0]?.['dcs:prose']).toEqual(assembled['dcs:prose'])
  })
})
