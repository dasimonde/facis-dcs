import { toProperCase } from './string'

const contractEventLabels: Record<string, string> = {
  CREATE_CONTRACT: 'Created contract',
  UPDATE_CONTRACT: 'Updated contract',
  NEGOTIATE_CONTRACT: 'Proposed contract changes',
  ACCEPT_RESPOND_CONTRACT: 'Accepted negotiation response',
  REJECT_RESPOND_CONTRACT: 'Rejected negotiation response',
  INCREASE_CONTRACT_VERSION: 'Increased contract version',
  APPROVE_CONTRACT: 'Approved contract',
  REJECT_CONTRACT: 'Rejected contract',
  VERIFY_CONTRACT: 'Verified contract',
  REVIEW_CONTRACT: 'Reviewed contract',
  AUDIT_CONTRACT: 'Audited contract',
  TERMINATE_CONTRACT: 'Terminated contract',
  RECORD_EVIDENCE: 'Recorded evidence',
  CONTRACT_EXPIRED: 'Contract expired',
  RETRIEVE_ALL_CONTRACTS: 'Retrieved contracts',
  RETRIEVE_CONTRACT_BY_ID: 'Retrieved contract',
  RETRIEVE_CONTRACT_HISTORY_BY_DID: 'Retrieved contract history',
  SEARCH_CONTRACT: 'Searched contracts',
  STORE_ARCHIVED_CONTRACT: 'Stored contract in archive',
  ARCHIVE_ENTRY_AUDIT_SUMMARY: 'Archive entry audit summary',
  ARCHIVE_INTEGRITY_AUDIT_CHECK: 'Archive integrity audit check',
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

  return 'Submitted contract'
}

function stringField(value: unknown, key: string): string | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return undefined
  }
  const raw = (value as Record<string, unknown>)[key]
  return typeof raw === 'string' && raw.trim() ? raw : undefined
}
