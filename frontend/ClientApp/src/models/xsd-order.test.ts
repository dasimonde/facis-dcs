import { describe, expect, it } from 'vitest'
import { compareXsdValues, parseXsdDuration, XSD_DURATION } from '@/models/xsd-order'

describe('parseXsdDuration', () => {
  it.each([
    ['P14D', { months: 0, seconds: 14 * 86400 }],
    ['P5D', { months: 0, seconds: 5 * 86400 }],
    ['PT6H', { months: 0, seconds: 21600 }],
    ['PT24H', { months: 0, seconds: 86400 }],
    ['P1D', { months: 0, seconds: 86400 }],
    ['P1W', { months: 0, seconds: 7 * 86400 }],
    ['P1Y6M', { months: 18, seconds: 0 }],
    ['PT1M30S', { months: 0, seconds: 90 }],
    ['-P5D', { months: 0, seconds: -5 * 86400 }],
  ])('reads %s as a month/second magnitude', (lexical, expected) => {
    expect(parseXsdDuration(lexical)).toEqual(expected)
  })

  it.each(['P', 'PT', '14D', 'P14', 'six hours', ''])('rejects %s', (lexical) => {
    expect(parseXsdDuration(lexical)).toBeNull()
    expect(XSD_DURATION.test(lexical)).toBe(false)
  })
})

describe('compareXsdValues', () => {
  // The defect this guards: "PT6H" > "PT24H" byte-wise, so a lexical
  // comparison reads a six-hour window as breaching a 24-hour bound.
  it('orders PT6H before PT24H, which byte order does not', () => {
    expect('PT6H' < 'PT24H').toBe(false)
    expect(compareXsdValues('PT6H', 'PT24H')).toBeLessThan(0)
  })

  it.each([
    ['P5D', 'P14D', -1],
    ['P30D', 'P14D', 1],
    ['P14D', 'P14D', 0],
    ['PT24H', 'P1D', 0],
    ['P1W', 'P14D', -1],
    ['P1M', 'P1Y', -1],
  ])('orders %s against %s', (left, right, expected) => {
    expect(Math.sign(compareXsdValues(left, right)!)).toBe(expected)
  })

  // A month is 28 to 31 days, so XSD leaves this pair unordered. Answering
  // anyway is the same class of bug as comparing bytes.
  it('reports a month against a day count as incomparable', () => {
    expect(compareXsdValues('P1M', 'P30D')).toBeNull()
  })

  it('orders numbers and numeric strings numerically', () => {
    expect(compareXsdValues(9500, 10000)).toBeLessThan(0)
    expect(compareXsdValues('9500', 10000)).toBeLessThan(0)
    expect(compareXsdValues(99.5, 99.5)).toBe(0)
  })

  it('orders instants as instants', () => {
    expect(compareXsdValues('2025-05-09T10:00:00Z', '2025-05-10T23:59:59Z')).toBeLessThan(0)
    expect(compareXsdValues('2025-05-11', '2025-05-10')).toBeGreaterThan(0)
  })

  it('shares no ordering across value spaces', () => {
    expect(compareXsdValues('P5D', 1)).toBeNull()
    expect(compareXsdValues(5, '2025-05-10')).toBeNull()
    expect(compareXsdValues('a label', 'another')).toBeNull()
  })
})
