const GROUP_SEPARATOR = ' '
const GROUPING_WHITESPACE = /[\s\u00a0\u202f]/g

export function formatNumberInput(value: string | number | boolean | null | undefined): string {
  const raw = String(value ?? '').replace(GROUPING_WHITESPACE, '')
  if (!raw) return ''

  const decimalSeparator = raw.includes(',') ? ',' : raw.includes('.') ? '.' : ''
  const parts = decimalSeparator ? raw.split(decimalSeparator) : [raw]
  const rawInteger = parts.shift() ?? ''
  const fractionParts = parts
  const negative = rawInteger.startsWith('-')
  const integerDigits = rawInteger.replace(/[^\d]/g, '')
  const groupedInteger = integerDigits.replace(/\B(?=(\d{3})+(?!\d))/g, GROUP_SEPARATOR)
  const integer = `${negative ? '-' : ''}${groupedInteger}`

  if (!decimalSeparator) return integer
  const fraction = fractionParts.join('').replace(/[^\d]/g, '')
  return `${integer}${decimalSeparator}${fraction}`
}

export function normalizeNumberInput(value: string): string {
  return value.replace(GROUPING_WHITESPACE, '').replace(',', '.')
}
