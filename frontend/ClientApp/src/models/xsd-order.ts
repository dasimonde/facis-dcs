import { normalizeNumberInput } from '@template-repository/utils/number-format'

/**
 * Ordering over XSD value spaces. A boundary check ("elapsed time <= P14D")
 * is only meaningful in the value space the two operands share: numbers order
 * as numbers, instants as instants, durations as durations. Reading a
 * duration as anything else answers the wrong question — "PT6H" is neither a
 * number nor an instant, and byte order puts it after "PT24H".
 */

/** xsd:duration lexical space (XSD 1.1 §3.3.6, plus the ISO-8601 week
 *  designator): at least one component, and a T section, when present, is
 *  non-empty. */
export const XSD_DURATION = /^-?P(?=\d|T\d)(\d+Y)?(\d+M)?(\d+W)?(\d+D)?(T(?=\d)(\d+H)?(\d+M)?(\d+(\.\d+)?S)?)?$/

/** A duration's magnitude as XSD defines it: a month count and a second
 *  count, kept apart because a month is not a fixed number of seconds. */
export interface DurationMagnitude {
  months: number
  seconds: number
}

const DAY_SECONDS = 86400
const COMPONENTS = /(\d+(?:\.\d+)?)([YMWDHS])/g

export function parseXsdDuration(value: string): DurationMagnitude | null {
  const lexical = value.trim()
  if (!XSD_DURATION.test(lexical)) return null
  const sign = lexical.startsWith('-') ? -1 : 1
  const [datePart = '', timePart = ''] = lexical.replace(/^-?P/, '').split('T')
  // 'M' means months in the date part and minutes in the time part, so the
  // two halves are scanned with their own unit tables.
  const dateUnits: Record<string, DurationMagnitude> = {
    Y: { months: 12, seconds: 0 },
    M: { months: 1, seconds: 0 },
    W: { months: 0, seconds: 7 * DAY_SECONDS },
    D: { months: 0, seconds: DAY_SECONDS },
  }
  const timeUnits: Record<string, DurationMagnitude> = {
    H: { months: 0, seconds: 3600 },
    M: { months: 0, seconds: 60 },
    S: { months: 0, seconds: 1 },
  }
  let months = 0
  let seconds = 0
  for (const [part, units] of [
    [datePart, dateUnits],
    [timePart, timeUnits],
  ] as const) {
    for (const match of part.matchAll(COMPONENTS)) {
      const scale = units[match[2] ?? '']
      if (!scale) continue
      const magnitude = Number(match[1])
      months += magnitude * scale.months
      seconds += magnitude * scale.seconds
    }
  }
  // `|| 0` collapses the -0 a negated zero component produces.
  return { months: sign * months || 0, seconds: sign * seconds || 0 }
}

/** The shortest and longest wall-clock spans a magnitude can denote: a month
 *  runs 28 to 31 days, so a duration carrying months covers a range rather
 *  than a point. */
function durationBounds(magnitude: DurationMagnitude): [number, number] {
  const shortest = magnitude.months * 28 * DAY_SECONDS
  const longest = magnitude.months * 31 * DAY_SECONDS
  return [Math.min(shortest, longest) + magnitude.seconds, Math.max(shortest, longest) + magnitude.seconds]
}

/** Orders two durations, or reports them incomparable (null) when their
 *  possible spans overlap without coinciding — "P1M" against "P30D" has no
 *  answer, and inventing one is how a policy silently decides wrong. */
export function compareXsdDurations(left: DurationMagnitude, right: DurationMagnitude): number | null {
  const [leftLow, leftHigh] = durationBounds(left)
  const [rightLow, rightHigh] = durationBounds(right)
  if (leftHigh < rightLow) return -1
  if (leftLow > rightHigh) return 1
  if (leftLow === leftHigh && rightLow === rightHigh) return 0
  return null
}

function numericValue(value: unknown): number | null {
  if (typeof value === 'number') return Number.isFinite(value) ? value : null
  if (typeof value !== 'string') return null
  const trimmed = value.trim()
  if (trimmed === '') return null
  const parsed = Number(normalizeNumberInput(trimmed))
  return Number.isFinite(parsed) ? parsed : null
}

function durationValue(value: unknown): DurationMagnitude | null {
  return typeof value === 'string' ? parseXsdDuration(value) : null
}

function instantValue(value: unknown): number | null {
  if (typeof value !== 'string') return null
  const parsed = Date.parse(value.trim())
  return Number.isNaN(parsed) ? null : parsed
}

function sign(left: number, right: number): number {
  return left < right ? -1 : left > right ? 1 : 0
}

/**
 * Orders two values within the value space they share, or returns null when
 * they share none (a duration against a number, a date against a label) or
 * when the shared space leaves them incomparable. A null verdict states "this
 * pair carries no ordering", which a caller must not read as "unsatisfied".
 */
export function compareXsdValues(left: unknown, right: unknown): number | null {
  const leftDuration = durationValue(left)
  const rightDuration = durationValue(right)
  if (leftDuration && rightDuration) return compareXsdDurations(leftDuration, rightDuration)
  if (leftDuration || rightDuration) return null

  const leftNumber = numericValue(left)
  const rightNumber = numericValue(right)
  if (leftNumber !== null && rightNumber !== null) return sign(leftNumber, rightNumber)
  if (leftNumber !== null || rightNumber !== null) return null

  const leftInstant = instantValue(left)
  const rightInstant = instantValue(right)
  if (leftInstant !== null && rightInstant !== null) return sign(leftInstant, rightInstant)
  return null
}
