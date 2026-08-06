import type { ContractChangeRequest } from './contract'
import type { ContractNegotiationDecision } from './contract-negotiation-decision'

// One accepted change request beat this one to a field when the round folded,
// so the merged contract version does not carry what this request proposed for
// it. Present only once the round has folded and only on the losing request —
// its decisions still read ACCEPTED either way.
export interface ContractNegotiationSupersession {
  superseded_by: string
  fields: string[]
}

export interface ContractNegotiation {
  id: string
  change_request: ContractChangeRequest
  created_by: string
  created_at: string
  contract_version: number
  negotiation_decisions: ContractNegotiationDecision[]
  superseded?: ContractNegotiationSupersession[]
}
