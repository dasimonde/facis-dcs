import type { Contract } from '@/models/contract/contract'
import type { ContractNegotiation } from '@/models/contract/contract-negotiation'

/**
 * Proposals shown in the Proposals list: current formal version, any older row
 * that still has an open decision, and any rejected proposal (Reject + CLOSED
 * must not hide history after a version bump or A/B decision sync gap).
 */
export function activeNegotiations(contract: Contract | null | undefined): ContractNegotiation[] {
  if (!contract) return []
  return (
    contract.negotiations?.filter((negotiation) => {
      if (negotiation.contract_version === contract.contract_version) return true
      return negotiation.negotiation_decisions.some(
        (decision) => !decision.decision || decision.decision === 'REJECTED',
      )
    }) ?? []
  )
}

/**
 * Proposals that still need a local Accept/Reject. Author rows are never
 * actionable (FR-CWE-07); only an undecided decision for this instance on a
 * proposal someone else authored counts. Used to auto-open the Proposals tab
 * without trapping Submit after every proposal in the round is already handled.
 */
export function locallyActionableNegotiations(
  contract: Contract | null | undefined,
  localInstanceDid: string,
  participantId: string,
): ContractNegotiation[] {
  if (!contract || !localInstanceDid) return []
  return activeNegotiations(contract).filter((negotiation) => {
    if (participantId && negotiation.created_by === participantId) return false
    return negotiation.negotiation_decisions.some(
      (decision) => decision.negotiator === localInstanceDid && !decision.decision,
    )
  })
}

/** States past local settle where a late peer proposal should surface as a warning. */
const postNegotiationStates = new Set(['SUBMITTED', 'REVIEWED', 'APPROVED'])

/**
 * True when this instance already left negotiation (Submit/Review/Approve) but
 * still owes Accept/Reject on a **peer-authored** proposal for the **current**
 * formal version — typically shipped after local Submit.
 *
 * Does not use locallyActionableNegotiations: that helper also surfaces older
 * rounds (open author "Not required" nulls) and treats `participantId` as the
 * human issuer, so Review/Approve (which only know the instance DID) false-positive.
 */
export function hasPendingPeerProposalAfterSettle(
  contract: Contract | null | undefined,
  localInstanceDid: string,
): boolean {
  if (!contract || !localInstanceDid || !postNegotiationStates.has(contract.state)) return false
  return (contract.negotiations ?? []).some((negotiation) => {
    if (negotiation.contract_version !== contract.contract_version) return false
    if (negotiation.created_by === localInstanceDid) return false
    return negotiation.negotiation_decisions.some(
      (decision) => decision.negotiator === localInstanceDid && !decision.decision,
    )
  })
}

/**
 * True when any proposal for this round still needs handling before Submit:
 * we owe Accept/Reject, or our proposal still waits on the peer (missing or
 * undecided counterparty row). Author "Not required" nulls do not count.
 */
export function hasUnresolvedProposals(
  contract: Contract | null | undefined,
  localInstanceDid: string,
  participantId: string,
): boolean {
  if (!contract || !localInstanceDid) return false
  return activeNegotiations(contract).some((negotiation) => {
    const isOwn = !!participantId && negotiation.created_by === participantId
    const decisions = negotiation.negotiation_decisions
    if (isOwn) {
      const peerDecisions = decisions.filter((decision) => decision.negotiator !== localInstanceDid)
      if (peerDecisions.length === 0) return true
      return peerDecisions.some((decision) => !decision.decision)
    }
    return decisions.some((decision) => decision.negotiator === localInstanceDid && !decision.decision)
  })
}

/**
 * Same gate Submit's merge uses after federated Accept: at least one ACCEPTED
 * and no REJECTED. Author instance rows stay null (FR-CWE-07) and must not keep
 * a peer-accepted proposal out of the merge.
 */
export function isFullyAcceptedNegotiation(negotiation: ContractNegotiation): boolean {
  return isAcceptedForEditorBaseline(negotiation)
}

/**
 * Editor/diff preview and merge gate: at least one ACCEPTED and no REJECTED.
 * Author instance rows stay null (FR-CWE-07) and still count as foldable once
 * the counterparty has accepted.
 */
export function isAcceptedForEditorBaseline(negotiation: ContractNegotiation): boolean {
  const decisions = negotiation.negotiation_decisions
  if (decisions.some((decision) => decision.decision === 'REJECTED')) return false
  return decisions.some((decision) => decision.decision === 'ACCEPTED')
}

/**
 * Accepted change requests for the current formal version, oldest first then by
 * id — the same total order MergeChangeRequests applies (last-accepted-wins).
 */
export function acceptedNegotiations(contract: Contract | null | undefined): ContractNegotiation[] {
  if (!contract) return []
  return (contract.negotiations ?? [])
    .filter(
      (negotiation) =>
        negotiation.contract_version === contract.contract_version && isAcceptedForEditorBaseline(negotiation),
    )
    .slice()
    .sort((a, b) => {
      const byTime = new Date(a.created_at).getTime() - new Date(b.created_at).getTime()
      if (byTime !== 0) return byTime
      return a.id.localeCompare(b.id)
    })
}

function foldChangeRequests(contract: Contract, negotiations: ContractNegotiation[]): Contract {
  if (negotiations.length === 0) return contract

  let result: Contract = { ...contract }
  for (const negotiation of negotiations) {
    const change = negotiation.change_request
    result = {
      ...result,
      name: change.name ?? result.name,
      description: change.description ?? result.description,
      start_date: change.start_date ?? result.start_date,
      exp_date: change.exp_date ?? result.exp_date,
      exp_notice_period: change.exp_notice_period ?? result.exp_notice_period,
      exp_policy: change.exp_policy ?? result.exp_policy,
      contract_data: change.contract_data ?? result.contract_data,
    }
  }
  return result
}

function negotiationMergeOrder(a: ContractNegotiation, b: ContractNegotiation): number {
  const byTime = new Date(a.created_at).getTime() - new Date(b.created_at).getTime()
  if (byTime !== 0) return byTime
  return a.id.localeCompare(b.id)
}

/**
 * Formal contract with every accepted proposal of this version folded in.
 * Metadata is last-accepted-wins per field; contract_data is whole-document
 * replace (same as the backend merge). Used so Details/Content show what
 * Submit will produce, and so a further Change Proposal starts from that
 * baseline rather than the still-unmerged formal row.
 */
export function contractWithAcceptedProposals(contract: Contract): Contract {
  return foldChangeRequests(contract, acceptedNegotiations(contract))
}

/**
 * Baseline for showing one proposal: formal + every accepted proposal that
 * precedes it in merge order. Excludes the selected row itself so an already
 * accepted proposal still diffs against what it changed from.
 */
export function proposalComparisonBaseline(contract: Contract, selected: ContractNegotiation): Contract {
  const priorAccepted = acceptedNegotiations(contract).filter(
    (negotiation) => negotiationMergeOrder(negotiation, selected) < 0,
  )
  return foldChangeRequests(contract, priorAccepted)
}
