import type { ContractState } from '@/types/contract-state'
import type { ContractNegotiation } from './contract-negotiation'

export interface Contract {
  did: string
  contract_version?: number
  state: ContractState
  name?: string
  description?: string
  created_by: string
  created_at: string
  updated_at: string
  expiration_date?: string
  contract_data?: unknown
  negotiations?: ContractNegotiation[]
}
