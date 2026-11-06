export type ContractState = (typeof ContractState)[keyof typeof ContractState]

// Kept in sync with backend/internal/contractworkflowengine/datatype/contractstate/contractstate.go.
// offered/withdrawn/active/revoked are first-class contract-formation/
// post-signing states alongside the core workflow states.
export const ContractState = {
  draft: 'DRAFT',
  offered: 'OFFERED',
  rejected: 'REJECTED',
  withdrawn: 'WITHDRAWN',
  negotiation: 'NEGOTIATION',
  submitted: 'SUBMITTED',
  reviewed: 'REVIEWED',
  approved: 'APPROVED',
  signed: 'SIGNED',
  active: 'ACTIVE',
  revoked: 'REVOKED',
  terminated: 'TERMINATED',
  expired: 'EXPIRED',
} as const

export const contractStates: ContractState[] = Object.values(ContractState)

// A rejected contract is back in draft status (DCS-FR-CWE-15): its creator edits
// and resubmits it like a draft.
export const isEditableContractState = (state?: string): boolean =>
  state === ContractState.draft || state === ContractState.rejected

// A contract out of force (backend contractstate.Erasable): its archive entry
// can be deleted and its keys shredded (DCS-FR-CSA-17). It cannot be terminated.
export const erasableContractStates: string[] = [
  ContractState.terminated,
  ContractState.expired,
  ContractState.revoked,
  ContractState.rejected,
  ContractState.withdrawn,
]
