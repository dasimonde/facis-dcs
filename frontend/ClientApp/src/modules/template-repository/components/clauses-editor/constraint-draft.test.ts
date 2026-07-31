import { describe, expect, it } from 'vitest'
import {
  type AtomicDraft,
  composeConstraintTree,
  type GroupDraft,
  parseConstraintTree,
  typed,
} from '@template-repository/components/clauses-editor/constraint-draft'
import type { OdrlConstraint } from '@/models/dcs-jsonld'

/**
 * A typed-in boundary literal must carry the XSD datatype a target system
 * needs to evaluate it — a duration is not a label, a node count is not a
 * decimal amount.
 */
describe('typed', () => {
  it.each([
    ['P14D', 'xsd:duration'],
    ['P5D', 'xsd:duration'],
    ['PT4H', 'xsd:duration'],
    ['P1Y6M', 'xsd:duration'],
    ['2028-12-31', 'xsd:date'],
    ['2028-12-31T23:59', 'xsd:dateTime'],
    ['60', 'xsd:integer'],
    ['-3', 'xsd:integer'],
    ['99.5', 'xsd:decimal'],
    ['EMEA', 'xsd:string'],
    ['Platinum', 'xsd:string'],
    ['P', 'xsd:string'],
  ])('types %s as %s', (value, datatype) => {
    expect(typed(value)).toEqual({ '@value': value, '@type': datatype })
  })
})

const TEMPLATE = 'did:web:example:template:payment'
const AMOUNT = `${TEMPLATE}#field-payment-amount`
const CURRENCY = `${TEMPLATE}#field-payment-currency`
const EUR_CONCEPT = 'https://w3id.org/facis/dcs/taxonomy/v1#currency-EUR'
const FIELDS = [AMOUNT, CURRENCY]

function atomic(extra: Partial<AtomicDraft> = {}): AtomicDraft {
  return {
    kind: 'atomic',
    leftOperand: 'odrl:payAmount',
    operator: 'odrl:lteq',
    rightSource: '',
    value: '',
    values: [],
    unitSource: '',
    unit: '',
    ...extra,
  }
}

function root(...children: AtomicDraft[]): GroupDraft {
  return { kind: 'group', combine: 'and', children }
}

function onlyAtomic(group: GroupDraft): AtomicDraft {
  const [child] = group.children
  if (child?.kind !== 'atomic') throw new Error('expected a single atomic constraint')
  return child
}

/**
 * A boundary and the unit it is measured in are both negotiable: a payment
 * amount agreed in a currency the parties also agree. Both serialize as
 * {"@id": …}, so only the declared field list tells a negotiated reference
 * from a fixed concept IRI — the editor must not confuse the two in either
 * direction, or a reload silently rewrites the contract.
 */
describe('a negotiated odrl:unit', () => {
  it('emits the field IRI as the constraint unit', () => {
    const [constraint] = composeConstraintTree(
      root(atomic({ rightSource: AMOUNT, unitSource: CURRENCY })),
    ) as OdrlConstraint[]
    expect(constraint).toEqual({
      '@type': 'odrl:Constraint',
      'odrl:leftOperand': { '@id': 'odrl:payAmount' },
      'odrl:operator': { '@id': 'odrl:lteq' },
      'odrl:rightOperand': { '@id': AMOUNT },
      'odrl:unit': { '@id': CURRENCY },
    })
  })

  it('reads a unit naming a declared field back as negotiated', () => {
    const nodes = composeConstraintTree(root(atomic({ rightSource: AMOUNT, unitSource: CURRENCY })))!
    const child = onlyAtomic(parseConstraintTree(nodes, FIELDS))
    expect(child.unitSource).toBe(CURRENCY)
    expect(child.unit).toBe('')
  })

  it('reads a unit naming a concept back as fixed', () => {
    const nodes = composeConstraintTree(root(atomic({ value: '5000', unit: EUR_CONCEPT })))!
    const child = onlyAtomic(parseConstraintTree(nodes, FIELDS))
    expect(child.unit).toBe(EUR_CONCEPT)
    expect(child.unitSource).toBe('')
  })

  it('round-trips both unit kinds through save and reload', () => {
    const drafted = root(
      atomic({ rightSource: AMOUNT, unitSource: CURRENCY }),
      atomic({ leftOperand: 'odrl:count', value: '12', unit: EUR_CONCEPT }),
    )
    const first = composeConstraintTree(drafted)!
    const second = composeConstraintTree(parseConstraintTree(first, FIELDS))
    expect(second).toEqual(first)
    // Once more, so a loss that only shows on the second pass is caught.
    expect(composeConstraintTree(parseConstraintTree(second!, FIELDS))).toEqual(first)
  })

  it('keeps a fixed concept boundary out of the negotiated right-operand slot', () => {
    // odrl:spatial eq <a region concept> is a fixed boundary, not a field the
    // parties negotiate — reading it as one would offer the author a field
    // picker with an IRI no field carries.
    const region = 'https://w3id.org/facis/dcs/taxonomy/v1#service-region-EMEA'
    const nodes = composeConstraintTree(
      root(atomic({ leftOperand: 'odrl:spatial', operator: 'odrl:eq', values: [{ '@id': region }] })),
    )!
    const child = onlyAtomic(parseConstraintTree(nodes, FIELDS))
    expect(child.rightSource).toBe('')
    expect(child.values).toEqual([{ '@id': region }])
  })
})
