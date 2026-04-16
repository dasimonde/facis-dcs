import http from '@/api/http'
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
  ContractRetrieveByIdResponse,
  ContractRetrieveResponse,
  ContractReviewResponse,
  ContractSearchResponse,
  ContractStoreResponse,
  ContractSubmitResponse,
  ContractTerminateResponse,
  ContractUpdateResponse,
} from '@/models/responses/contract-response'
import type { ContractWorkflowService } from '@/models/services/contract-workflow-service'

export const contractWorkflowService: ContractWorkflowService = {
  async create(request: ContractCreateRequest) {
    return http.post<ContractCreateResponse>('/contract/create', request).then((res) => res.data)
  },

  async update(request: ContractUpdateRequest) {
    return http.put<ContractUpdateResponse>('/contract/update', request).then((res) => res.data)
  },

  async submit(request: ContractSubmitRequest) {
    return http.post<ContractSubmitResponse>('/contract/submit', request).then((res) => res.data)
  },

  async negotiate(request: ContractNegotiationRequest) {
    return http.post<ContractNegotiationResponse>('/contract/negotiate', request).then((res) => res.data)
  },

  async respond(request: ContractNegotiationRespondRequest) {
    return http.post<ContractNegotiationRespondResponse>('/contract/respond', request).then((res) => res.data)
  },

  async review(request: ContractReviewRequest) {
    return http.get<ContractReviewResponse>('/contract/review', { params: request }).then((res) => res.data)
  },

  async retrieve(_request?: ContractRetrieveRequest) {
    return http
      .get<ContractRetrieveResponse>('/contract/retrieve')
      .then((res) => res.data)
      .catch((err) => {
        console.error('Retrieve Error:', err)
        return {
          contracts: [],
          review_tasks: [],
          approval_tasks: [],
          negotiation_tasks: [],
        } as ContractRetrieveResponse
      })
  },

  async retrieveById(request: ContractRetrieveByIdRequest) {
    return http
      .get<ContractRetrieveByIdResponse>(`/contract/retrieve/${request.did}`)
      .then((res) => ({ ...res.data }))
      .catch((err) => {
        console.error('Retrieve ID Error:', err)
        return null
      })
  },

  async search(request: ContractSearchRequest) {
    return http
      .get<ContractSearchResponse>('/contract/search', { params: request })
      .then((res) => res.data)
      .catch((err) => {
        console.error('Search Error:', err)
        return []
      })
  },

  async approve(request: ContractApproveRequest) {
    return http.post<ContractApproveResponse>('/contract/approve', request).then((res) => res.data)
  },

  async reject(request: ContractRejectRequest) {
    return http.post<ContractRejectResponse>('/contract/reject', request).then((res) => res.data)
  },

  async store(request: ContractStoreRequest) {
    return http.post<ContractStoreResponse>('/contract/store', request).then((res) => res.data)
  },

  async terminate(request: ContractTerminateRequest) {
    return http.post<ContractTerminateResponse>('/contract/terminate', request).then((res) => res.data)
  },

  async audit(request: ContractAuditRequest) {
    return http.post<ContractAuditResponse>('/contract/audit', request).then((res) => res.data)
  },
}
