export type ContractState = (typeof ContractState)[keyof typeof ContractState]

export const ContractState = {
  draft: 'DRAFT',
  rejected: 'REJECTED',
  negotiation: 'NEGOTIATION',
  submitted: 'SUBMITTED',
  reviewed: 'REVIEWED',
  approved: 'APPROVED',
  terminated: 'TERMINATED',
  expired: 'EXPIRED',
} as const

export const contractStates: ContractState[] = Object.values(ContractState)
