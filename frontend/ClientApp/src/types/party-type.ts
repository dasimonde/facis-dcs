export const partyTypes = [
  { value: 'SINGLE_PARTY', label: 'Single-party' },
  { value: 'MULTI_PARTY', label: 'Multi-party' },
] as const

export type PartyType = (typeof partyTypes)[number]['value']
