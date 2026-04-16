import type { NegotiationTaskState } from "@/types/negotiation-task-state"

export interface ContractNegotiationTask {
  type: 'contract'
  did: string
  contract_version?: number
  state: NegotiationTaskState
  negotiator: string
  created_at: string
}
