import type {
  ContractTemplateApproveEvent,
  ContractTemplateArchiveEvent,
  ContractTemplateAuditEvent,
  ContractTemplateCreateEvent,
  ContractTemplateCopyEvent,
  ContractTemplatePublishEvent,
  ContractTemplateRegisterEvent,
  ContractTemplateRejectEvent,
  ContractTemplateRetrieveAllEvent,
  ContractTemplateRetrieveByIDEvent,
  ContractTemplateSearchEvent,
  ContractTemplateSubmitEvent,
  ContractTemplateUpdateEvent,
  ContractTemplateVerifyEvent,
} from '@/models/contract-template-event'
import type { ContractTemplateAuditResponseItem } from '@/models/responses/template-response'

export function useContractTemplateEventType() {
  const isCreateEvent = (
    event: ContractTemplateAuditResponseItem,
  ): event is ContractTemplateAuditResponseItem & { event_data: ContractTemplateCreateEvent } => {
    return event.event_type === 'CREATE_CONTRACT_TEMPLATE'
  }

  const isCopyEvent = (
    event: ContractTemplateAuditResponseItem,
  ): event is ContractTemplateAuditResponseItem & { event_data: ContractTemplateCopyEvent } => {
    return event.event_type === 'COPY_CONTRACT_TEMPLATE'
  }

  const isSubmitEvent = (
    event: ContractTemplateAuditResponseItem,
  ): event is ContractTemplateAuditResponseItem & { event_data: ContractTemplateSubmitEvent } => {
    return event.event_type === 'SUBMIT_CONTRACT_TEMPLATE'
  }

  const isApproveEvent = (
    event: ContractTemplateAuditResponseItem,
  ): event is ContractTemplateAuditResponseItem & { event_data: ContractTemplateApproveEvent } => {
    return event.event_type === 'APPROVE_CONTRACT_TEMPLATE'
  }

  const isRejectEvent = (
    event: ContractTemplateAuditResponseItem,
  ): event is ContractTemplateAuditResponseItem & { event_data: ContractTemplateRejectEvent } => {
    return event.event_type === 'REJECT_CONTRACT_TEMPLATE'
  }

  const isVerifyEvent = (
    event: ContractTemplateAuditResponseItem,
  ): event is ContractTemplateAuditResponseItem & { event_data: ContractTemplateVerifyEvent } => {
    return event.event_type === 'VERIFY_CONTRACT_TEMPLATE'
  }

  const isUpdateEvent = (
    event: ContractTemplateAuditResponseItem,
  ): event is ContractTemplateAuditResponseItem & { event_data: ContractTemplateUpdateEvent } => {
    return event.event_type === 'UPDATE_CONTRACT_TEMPLATE'
  }

  const isSearchEvent = (
    event: ContractTemplateAuditResponseItem,
  ): event is ContractTemplateAuditResponseItem & { event_data: ContractTemplateSearchEvent } => {
    return event.event_type === 'SEARCH_CONTRACT_TEMPLATE'
  }

  const isRetrieveAllEvent = (
    event: ContractTemplateAuditResponseItem,
  ): event is ContractTemplateAuditResponseItem & { event_data: ContractTemplateRetrieveAllEvent } => {
    return event.event_type === 'RETRIEVE_ALL_CONTRACT_TEMPLATES'
  }

  const isRetrieveByIDEvent = (
    event: ContractTemplateAuditResponseItem,
  ): event is ContractTemplateAuditResponseItem & { event_data: ContractTemplateRetrieveByIDEvent } => {
    return event.event_type === 'RETRIEVE_CONTRACT_TEMPLATE_BY_ID'
  }

  const isArchiveEvent = (
    event: ContractTemplateAuditResponseItem,
  ): event is ContractTemplateAuditResponseItem & { event_data: ContractTemplateArchiveEvent } => {
    return event.event_type === 'ARCHIVE_CONTRACT_TEMPLATE'
  }

  const isRegisterEvent = (
    event: ContractTemplateAuditResponseItem,
  ): event is ContractTemplateAuditResponseItem & { event_data: ContractTemplateRegisterEvent } => {
    return event.event_type === 'REGISTER_CONTRACT_TEMPLATE'
  }

  const isPublishEvent = (
    event: ContractTemplateAuditResponseItem,
  ): event is ContractTemplateAuditResponseItem & { event_data: ContractTemplatePublishEvent } => {
    return event.event_type === 'PUBLISH_CONTRACT_TEMPLATE'
  }

  const isAuditEvent = (
    event: ContractTemplateAuditResponseItem,
  ): event is ContractTemplateAuditResponseItem & { event_data: ContractTemplateAuditEvent } => {
    return event.event_type === 'AUDIT_CONTRACT_TEMPLATE'
  }

  return {
    isCreateEvent,
    isCopyEvent,
    isSubmitEvent,
    isApproveEvent,
    isRejectEvent,
    isVerifyEvent,
    isUpdateEvent,
    isSearchEvent,
    isRetrieveAllEvent,
    isRetrieveByIDEvent,
    isArchiveEvent,
    isRegisterEvent,
    isPublishEvent,
    isAuditEvent,
  }
}
