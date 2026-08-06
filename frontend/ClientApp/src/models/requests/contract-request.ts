import type { ContractChangeRequest, ExpirationPolicy } from '../contract/contract'
import type { ContractData } from '../contract/contract-data'
import type { ContractActionFlag } from '@/types/contract-action-flag'
import type { ContractState } from '@/types/contract-state'
import type { NegotiationActionFlag } from '@/types/negotiation-action-flag'

export interface ContractCreateRequest {
  template_did: string
  /** The counterparty DCS this contract is offered to and negotiated with (a
   *  `did:web` peer, ADR-13). Omitted for a purely local contract. */
  counterparty?: string
  /** The contractual role the creating organization declares for itself; binds
   *  the origin DID to that role's party node in the contract's ODRL rules. */
  originator_role?: string
  /** Organizations authorized to read the contract, by legal name. Read
   *  authorization only — not the ODRL rule parties. */
  parties?: string[]
}

export interface ContractUpdateRequest {
  did: string
  updated_at: string
  start_date?: string
  exp_date?: string
  exp_notice_period?: number
  exp_policy?: ExpirationPolicy
  name?: string
  description?: string
  /** The data of the contract */
  contract_data?: ContractData
}

export interface ContractSubmitRequest {
  did: string
  updated_at: string
  forward_to?: ContractActionFlag
  comments?: string[]
  /** Optional updated contract data to persist atomically before submit validation */
  contract_data?: ContractData
  reviewers?: string[]
  approvers?: string[]
  negotiators?: string[]
}

export interface ContractRetrieveRequest {
  offset?: number
  limit?: number
}

export interface ContractRetrieveByIdRequest {
  did: string
}

export interface ContractReviewRequest {
  did: string
}

export interface ContractSearchRequest {
  offset?: number
  limit?: number
  did?: string
  contract_version?: number
  state?: ContractState
  name?: string
  description?: string
  filter?: string
}

export interface ContractOfferAcceptRequest {
  did: string
  updated_at: string
  accepted_by: string
}

export interface ContractNegotiationRequest {
  did: string
  updated_at: string
  negotiated_by: string
  change_request: ContractChangeRequest
}

export interface ContractNegotiationDraftSaveRequest {
  did: string
  change_request: ContractChangeRequest
}

export interface ContractNegotiationDraftRetrieveRequest {
  did: string
}

export interface ContractNegotiationRespondRequest {
  id: string
  did: string
  action_flag: NegotiationActionFlag
  rejection_reason?: string
}

export interface ContractApproveRequest {
  did: string
  updated_at: string
}

export interface ContractRejectRequest {
  did: string
  updated_at: string
  /** Reason for rejecting the contract */
  reason: string
}

export interface ContractWithdrawRequest {
  did: string
  updated_at: string
}

export interface ContractRenewRequest {
  did: string
  updated_at: string
  new_start_date?: string
  new_exp_date?: string
  new_exp_policy?: ExpirationPolicy
  new_exp_notice_period?: number
}

export interface ContractTerminateRequest {
  did: string
  updated_at: string
  /** Reason for terminating the contract */
  reason: string
}

export interface ContractAuditRequest {
  did: string
}

export interface ContractHistoryRetrieveRequest {
  did: string
}

export interface ContractDeployRequest {
  did: string
  updated_at: string
}

export interface ContractOfferRequest {
  did: string
  updated_at: string
}

/** Register or change a Contract Target System (ADR-25, UC-09-01). */
export interface ContractTargetWriteRequest {
  name: string
  url: string
  description?: string
  enabled?: boolean
}

/** Point a contract at the target system it deploys to. An empty target_id
 *  clears the designation. */
export interface ContractTargetDesignateRequest {
  did: string
  updated_at: string
  target_id?: string
}

/** Register or change a machine identity: a non-human caller that reaches DCS
 *  over its API (ADR-27). The credential is issued separately so a change of
 *  roles never silently reissues a secret. */
export interface MachineIdentityWriteRequest {
  name: string
  participant_did: string
  roles: string[]
  description?: string
}
