import type {
  ContractAcceptNegotiationEvent,
  ContractApproveEvent,
  ContractAuditEvent,
  ContractCreateEvent,
  ContractIncreaseContractVersionEvent,
  ContractNegotiationEvent,
  ContractRecordEvidenceEvent,
  ContractRejectEvent,
  ContractRejectNegotiationEvent,
  ContractRetrieveAllEvent,
  ContractRetrieveByIDEvent,
  ContractRetrieveHistoryByDIDEvent,
  ContractReviewEvent,
  ContractSubmitEvent,
  ContractTerminateEvent,
  ContractUpdateEvent,
  ContractVerifyEvent,
} from '@/models/contract/contract-event'
import type { ContractAuditResponseItem } from '@/models/responses/contract-response'

export function useContractEventType() {
  const isCreateEvent = (
    event: ContractAuditResponseItem,
  ): event is ContractAuditResponseItem & { event_data: ContractCreateEvent } => {
    return event.event_type === 'CREATE_CONTRACT'
  }

  const isUpdateEvent = (
    event: ContractAuditResponseItem,
  ): event is ContractAuditResponseItem & { event_data: ContractUpdateEvent } => {
    return event.event_type === 'UPDATE_CONTRACT'
  }

  const isSubmitEvent = (
    event: ContractAuditResponseItem,
  ): event is ContractAuditResponseItem & { event_data: ContractSubmitEvent } => {
    return event.event_type === 'SUBMIT_CONTRACT'
  }

  const isRetrieveByIDEvent = (
    event: ContractAuditResponseItem,
  ): event is ContractAuditResponseItem & { event_data: ContractRetrieveByIDEvent } => {
    return event.event_type === 'RETRIEVE_CONTRACT_BY_ID'
  }

  const isRetrieveHistoryByDIDEvent = (
    event: ContractAuditResponseItem,
  ): event is ContractAuditResponseItem & { event_data: ContractRetrieveHistoryByDIDEvent } => {
    return event.event_type === 'RETRIEVE_CONTRACT_HISTORY_BY_DID'
  }

  const isRetrieveAllEvent = (
    event: ContractAuditResponseItem,
  ): event is ContractAuditResponseItem & { event_data: ContractRetrieveAllEvent } => {
    return event.event_type === 'RETRIEVE_ALL_CONTRACTS'
  }

  const isVerifyEvent = (
    event: ContractAuditResponseItem,
  ): event is ContractAuditResponseItem & { event_data: ContractVerifyEvent } => {
    return event.event_type === 'VERIFY_CONTRACT'
  }

  const isNegotiationEvent = (
    event: ContractAuditResponseItem,
  ): event is ContractAuditResponseItem & { event_data: ContractNegotiationEvent } => {
    return event.event_type === 'NEGOTIATE_CONTRACT'
  }

  const isAcceptNegotiationEvent = (
    event: ContractAuditResponseItem,
  ): event is ContractAuditResponseItem & { event_data: ContractAcceptNegotiationEvent } => {
    return event.event_type === 'ACCEPT_RESPOND_CONTRACT'
  }

  const isRejectNegotiationEvent = (
    event: ContractAuditResponseItem,
  ): event is ContractAuditResponseItem & { event_data: ContractRejectNegotiationEvent } => {
    return event.event_type === 'REJECT_RESPOND_CONTRACT'
  }

  const isApproveEvent = (
    event: ContractAuditResponseItem,
  ): event is ContractAuditResponseItem & { event_data: ContractApproveEvent } => {
    return event.event_type === 'APPROVE_CONTRACT'
  }

  const isRejectEvent = (
    event: ContractAuditResponseItem,
  ): event is ContractAuditResponseItem & { event_data: ContractRejectEvent } => {
    return event.event_type === 'REJECT_CONTRACT'
  }

  const isTerminateEvent = (
    event: ContractAuditResponseItem,
  ): event is ContractAuditResponseItem & { event_data: ContractTerminateEvent } => {
    return event.event_type === 'TERMINATE_CONTRACT'
  }

  const isRecordEvidenceEvent = (
    event: ContractAuditResponseItem,
  ): event is ContractAuditResponseItem & { event_data: ContractRecordEvidenceEvent } => {
    return event.event_type === 'RECORD_EVIDENCE'
  }

  const isAuditEvent = (
    event: ContractAuditResponseItem,
  ): event is ContractAuditResponseItem & { event_data: ContractAuditEvent } => {
    return event.event_type === 'AUDIT_CONTRACT'
  }

  const isReviewEvent = (
    event: ContractAuditResponseItem,
  ): event is ContractAuditResponseItem & { event_data: ContractReviewEvent } => {
    return event.event_type === 'REVIEW_CONTRACT'
  }

  const isIncreaseContractVersionEvent = (
    event: ContractAuditResponseItem,
  ): event is ContractAuditResponseItem & { event_data: ContractIncreaseContractVersionEvent } => {
    return event.event_type === 'INCREASE_CONTRACT_VERSION'
  }

  return {
    isCreateEvent,
    isUpdateEvent,
    isSubmitEvent,
    isRetrieveByIDEvent,
    isRetrieveHistoryByDIDEvent,
    isRetrieveAllEvent,
    isVerifyEvent,
    isNegotiationEvent,
    isAcceptNegotiationEvent,
    isRejectNegotiationEvent,
    isApproveEvent,
    isRejectEvent,
    isTerminateEvent,
    isRecordEvidenceEvent,
    isAuditEvent,
    isReviewEvent,
    isIncreaseContractVersionEvent,
  }
}
