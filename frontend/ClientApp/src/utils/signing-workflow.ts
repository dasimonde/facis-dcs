/**
 * The signing workflow a contract document defines (DCS-FR-SM-13): the
 * position of each signature field in the signing order (dcs:signingOrder,
 * lowest signs first) and the signing deadline (dcs:signingDeadline). The
 * backend enforces both at prepare and submit; this reads them for display.
 */
export interface SigningWorkflow {
  order: { field: string; position: number }[]
  deadline?: Date
}

function literal(value: unknown): unknown {
  return value && typeof value === 'object' && '@value' in value ? value['@value'] : value
}

export function signingWorkflow(contractData: unknown): SigningWorkflow {
  const doc = (contractData ?? {}) as Record<string, unknown>
  const fields = (doc['dcs:signatureFields'] ?? []) as Record<string, unknown>[]
  const order = fields
    .map((node) => ({
      field: typeof node['dcs:signatoryName'] === 'string' ? node['dcs:signatoryName'].trim() : '',
      position: Number(literal(node['dcs:signingOrder'])),
    }))
    .filter((entry) => entry.field !== '' && Number.isInteger(entry.position) && entry.position >= 1)
    .sort((a, b) => a.position - b.position)

  const raw = literal(doc['dcs:signingDeadline'])
  let deadline: Date | undefined
  if (typeof raw === 'string' && raw.trim() !== '') {
    // A date without a time includes the whole day in UTC, as on the backend.
    const text = /^\d{4}-\d{2}-\d{2}$/.test(raw.trim()) ? `${raw.trim()}T23:59:59.999Z` : raw.trim()
    const parsed = new Date(text)
    if (!Number.isNaN(parsed.getTime())) deadline = parsed
  }
  return { order, deadline }
}

/** The signatory names of the contract's signature fields, in document order. */
export function signatureFieldNames(contractData: unknown): string[] {
  const fields = ((contractData ?? {}) as Record<string, unknown>)['dcs:signatureFields']
  if (!Array.isArray(fields)) return []
  return fields
    .map((node) => (node as Record<string, unknown>)['dcs:signatoryName'])
    .filter((name): name is string => typeof name === 'string' && name.trim() !== '')
}

/**
 * Sets the signing position of the signature field named field in the
 * contract document. A position that is not a positive integer removes the
 * field from the signing order.
 */
export function setSigningOrder(contractData: object, field: string, position: number | undefined): void {
  const fields = (contractData as Record<string, unknown>)['dcs:signatureFields']
  if (!Array.isArray(fields)) return
  const node = fields.find((item) => (item as Record<string, unknown>)['dcs:signatoryName'] === field) as
    | Record<string, unknown>
    | undefined
  if (!node) return
  if (position !== undefined && Number.isInteger(position) && position >= 1) node['dcs:signingOrder'] = position
  else delete node['dcs:signingOrder']
}

/** Sets the signing deadline (an xsd:date, YYYY-MM-DD) of the contract document; an empty date removes it. */
export function setSigningDeadline(contractData: object, date: string): void {
  const doc = contractData as Record<string, unknown>
  if (/^\d{4}-\d{2}-\d{2}$/.test(date)) doc['dcs:signingDeadline'] = { '@value': date, '@type': 'xsd:date' }
  else delete doc['dcs:signingDeadline']
}
