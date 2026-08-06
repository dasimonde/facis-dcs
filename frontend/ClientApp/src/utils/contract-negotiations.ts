import type { Contract } from '@/models/contract/contract'
import type { ContractNegotiation } from '@/models/contract/contract-negotiation'

/**
 * The negotiations a contract still has to resolve: those targeting its current
 * version, plus any that still carry an undecided decision. Keying on the
 * version alone drops a round whose own counter-proposal already bumped the
 * contract version, which leaves its open decision blocking Submit with nothing
 * on screen to act on.
 */
export function activeNegotiations(contract: Contract | null | undefined): ContractNegotiation[] {
  if (!contract) return []
  return (
    contract.negotiations?.filter(
      (negotiation) =>
        negotiation.contract_version === contract.contract_version ||
        negotiation.negotiation_decisions.some((decision) => !decision.decision),
    ) ?? []
  )
}
