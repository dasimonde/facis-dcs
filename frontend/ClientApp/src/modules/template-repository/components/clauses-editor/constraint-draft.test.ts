import { describe, expect, it } from 'vitest'
import { typed } from '@template-repository/components/clauses-editor/constraint-draft'

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
