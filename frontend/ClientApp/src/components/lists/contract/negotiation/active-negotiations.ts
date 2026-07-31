import type { ContractNegotiation } from '@/models/contract/contract-negotiation'

export function isActiveNegotiation(negotiation: ContractNegotiation, currentVersion: number): boolean {
  return (
    negotiation.contract_version === currentVersion ||
    negotiation.negotiation_decisions.some((decision) => decision.decision == null)
  )
}

export function activeNegotiations(
  negotiations: ContractNegotiation[] | undefined,
  currentVersion: number,
): ContractNegotiation[] {
  return negotiations?.filter((negotiation) => isActiveNegotiation(negotiation, currentVersion)) ?? []
}
