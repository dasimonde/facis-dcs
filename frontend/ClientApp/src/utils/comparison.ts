export function compareValues<T>(valueA: T, valueB: T, sortBy: keyof T, order: number) {
  const compValueA = toComparableValue(valueA[sortBy])
  const compValueB = toComparableValue(valueB[sortBy])
  if (!compValueA && !compValueB) return 0
  if (!compValueA) return order
  if (!compValueB) return -order

  const result =
    typeof compValueA === 'number' && typeof compValueB === 'number'
      ? Math.sign(compValueB - compValueA)
      : String(compValueA).localeCompare(String(compValueB))
  return order * result
}

export function toComparableValue(value: unknown) {
  if (typeof value === 'number') return value
  if (value instanceof Date) return value.getTime()
  if (typeof value === 'string') {
    if (/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$/.test(value)) {
      const dateTime = new Date(value).getTime()
      if (!Number.isNaN(dateTime)) return dateTime
    }
    return value
  }
  return undefined
}
