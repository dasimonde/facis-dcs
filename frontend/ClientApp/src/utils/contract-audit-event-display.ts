import { auditEventActor } from './audit-event-actor'
import { toProperCase } from './string'

const contractEventLabels: Record<string, string> = {
  CREATE_CONTRACT: 'Contract created',
  UPDATE_CONTRACT: 'Contract updated',
  OFFER_CONTRACT: 'Contract offered',
  WITHDRAW_CONTRACT: 'Contract withdrawn',
  NEGOTIATE_CONTRACT: 'Contract changes proposed',
  ACCEPT_OFFER_CONTRACT: 'Offer accepted',
  ACCEPT_RESPOND_CONTRACT: 'Negotiation response accepted',
  REJECT_RESPOND_CONTRACT: 'Negotiation response rejected',
  NEGOTIATION_CHANGE_SUPERSEDED: 'Accepted change request superseded',
  INCREASE_CONTRACT_VERSION: 'Contract version increased',
  APPROVE_CONTRACT: 'Contract approved',
  REJECT_CONTRACT: 'Contract rejected',
  VERIFY_CONTRACT: 'Contract verified',
  REVIEW_CONTRACT: 'Contract reviewed',
  AUDIT_CONTRACT: 'Contract audited',
  TERMINATE_CONTRACT: 'Contract terminated',
  RENEW_CONTRACT: 'Contract renewed',
  RECORD_EVIDENCE: 'Evidence recorded',
  CONTRACT_EXPIRED: 'Contract expired',
  APPLIED_SIGNATURE: 'Contract signed',
  REVOKE_SIGNATURE: 'Signature revoked',
  RETRIEVE_ALL_CONTRACTS: 'Contracts retrieved',
  RETRIEVE_CONTRACT_BY_ID: 'Contract retrieved',
  RETRIEVE_CONTRACT_HISTORY_BY_DID: 'Contract history retrieved',
  SEARCH_CONTRACT: 'Contracts searched',
  STORE_ARCHIVED_CONTRACT: 'Contract stored in archive',
  ARCHIVE_ENTRY_AUDIT_SUMMARY: 'Archive entry audit summary',
  ARCHIVE_INTEGRITY_AUDIT_CHECK: 'Archive integrity audit check',
  PEER_EXCHANGE_REFUSED: 'Termination notice refused by the counterparty instance',
}

const submitTransitionLabels: Record<string, string> = {
  'DRAFT->NEGOTIATION': 'Submitted for negotiation',
  'REJECTED->NEGOTIATION': 'Resubmitted for negotiation',
  'NEGOTIATION->SUBMITTED': 'Submitted for review',
  'SUBMITTED->REVIEWED': 'Review completed',
  'SUBMITTED->NEGOTIATION': 'Returned to negotiation',
  'REVIEWED->SUBMITTED': 'Returned to review',
}

export function contractAuditEventDisplayText(eventType?: string, eventData?: unknown): string {
  const normalizedEventType = eventType?.trim().toUpperCase()
  if (!normalizedEventType) {
    return 'Audit event'
  }

  if (normalizedEventType === 'SUBMIT_CONTRACT') {
    return submitContractDisplayText(eventData)
  }

  return contractEventLabels[normalizedEventType] ?? toProperCase(normalizedEventType)
}

/** Participant DID attributed on the event, if any. */
export function contractAuditEventActor(eventData?: unknown): string | undefined {
  return auditEventActor(eventData)
}

export function contractAuditEventOccurredAt(eventData?: unknown, fallback?: string): string | undefined {
  return stringField(eventData, 'occurred_at') ?? (fallback?.trim() ? fallback : undefined)
}

export function contractAuditEventTransition(eventData?: unknown): string | undefined {
  const previousState = stringField(eventData, 'previous_state') ?? stringField(eventData, 'old_state')
  const newState = stringField(eventData, 'new_state')
  if (!previousState || !newState) return undefined
  if (previousState.toUpperCase() === newState.toUpperCase()) return undefined
  return `${toProperCase(previousState)} → ${toProperCase(newState)}`
}

function submitContractDisplayText(eventData: unknown): string {
  const previousState = stringField(eventData, 'previous_state') ?? stringField(eventData, 'old_state')
  const newState = stringField(eventData, 'new_state')
  const transitionKey = [previousState, newState].map((state) => state?.trim().toUpperCase()).join('->')
  const transitionLabel = submitTransitionLabels[transitionKey]
  if (transitionLabel) {
    return transitionLabel
  }

  if (previousState && newState) {
    return `State changed from ${toProperCase(previousState)} to ${toProperCase(newState)}`
  }

  return 'Contract submitted'
}

function stringField(value: unknown, key: string): string | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return undefined
  }
  const raw = (value as Record<string, unknown>)[key]
  return typeof raw === 'string' && raw.trim() ? raw : undefined
}
