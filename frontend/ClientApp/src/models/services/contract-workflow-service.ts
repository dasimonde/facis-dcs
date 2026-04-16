import type { Contract } from '@/models/contract/contract'
import type {
  ContractApproveRequest,
  ContractAuditRequest,
  ContractCreateRequest,
  ContractNegotiationRequest,
  ContractNegotiationRespondRequest,
  ContractRejectRequest,
  ContractRetrieveByIdRequest,
  ContractRetrieveRequest,
  ContractReviewRequest,
  ContractSearchRequest,
  ContractStoreRequest,
  ContractSubmitRequest,
  ContractTerminateRequest,
  ContractUpdateRequest,
} from '@/models/requests/contract-requests'
import type {
  ContractApproveResponse,
  ContractAuditResponse,
  ContractCreateResponse,
  ContractNegotiationRespondResponse,
  ContractNegotiationResponse,
  ContractRejectResponse,
  ContractRetrieveResponse,
  ContractReviewResponse,
  ContractSearchResponse,
  ContractStoreResponse,
  ContractSubmitResponse,
  ContractTerminateResponse,
  ContractUpdateResponse,
} from '@/models/responses/contract-response'

export interface ContractWorkflowService {
  create: (request: ContractCreateRequest) => Promise<ContractCreateResponse>
  update: (request: ContractUpdateRequest) => Promise<ContractUpdateResponse>
  submit: (request: ContractSubmitRequest) => Promise<ContractSubmitResponse>
  negotiate: (request: ContractNegotiationRequest) => Promise<ContractNegotiationResponse>
  respond: (request: ContractNegotiationRespondRequest) => Promise<ContractNegotiationRespondResponse>
  review: (request: ContractReviewRequest) => Promise<ContractReviewResponse>
  retrieve: (request?: ContractRetrieveRequest) => Promise<ContractRetrieveResponse>
  retrieveById: (request: ContractRetrieveByIdRequest) => Promise<Contract | null>
  search: (request: ContractSearchRequest) => Promise<ContractSearchResponse>
  approve: (request: ContractApproveRequest) => Promise<ContractApproveResponse>
  reject: (request: ContractRejectRequest) => Promise<ContractRejectResponse>
  store: (request: ContractStoreRequest) => Promise<ContractStoreResponse>
  terminate: (request: ContractTerminateRequest) => Promise<ContractTerminateResponse>
  audit: (request: ContractAuditRequest) => Promise<ContractAuditResponse>
}
