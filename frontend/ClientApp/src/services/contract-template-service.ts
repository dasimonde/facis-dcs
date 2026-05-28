import http from '@/api/http'
import type { ContractTemplate } from '@/models/contract-template'
import type {
  ContractTemplateApproveRequest,
  ContractTemplateArchiveRequest,
  ContractTemplateAuditRequest,
  ContractTemplateCreateRequest,
  ContractTemplateCopyRequest,
  ContractTemplateRegisterRequest,
  ContractTemplateRejectRequest,
  ContractTemplateRetrieveByIdRequest,
  ContractTemplateSearchRequest,
  ContractTemplateSubmitRequest,
  ContractTemplateUpdateRequest,
  ContractTemplateVerifyRequest,
  ContractTemplatePublishRequest,
} from '@/models/requests/template-request'
import type {
  ContractTemplateApproveResponse,
  ContractTemplateArchiveResponse,
  ContractTemplateAuditResponse,
  ContractTemplateCreateResponse,
  ContractTemplateCopyResponse,
  ContractTemplateRegisterResponse,
  ContractTemplateRejectResponse,
  ContractTemplateRetrieveByIdResponse,
  ContractTemplateRetrieveResponse,
  ContractTemplateSearchResponse,
  ContractTemplateSubmitResponse,
  ContractTemplateUpdateResponse,
  ContractTemplateVerifyResponse,
  ContractTemplatePublishResponse,
} from '@/models/responses/template-response'
import type { ContractTemplateService } from '@/models/services/contract-template-service'

export const contractTemplateService: ContractTemplateService = {
  async create(request: ContractTemplateCreateRequest) {
    return http
      .post<ContractTemplateCreateResponse>('/template/create', request)
      .then((res) => res.data)
      .catch((err: unknown) => {
        console.error('Create Error:', err)
        throw err
      })
  },

  async copy(request: ContractTemplateCopyRequest) {
    return http
      .post<ContractTemplateCopyResponse>('/template/copy', request)
      .then((res) => res.data)
      .catch((err: unknown) => {
        console.error('Copy Error:', err)
        throw err
      })
  },

  async submit(request: ContractTemplateSubmitRequest) {
    return http
      .post<ContractTemplateSubmitResponse>('/template/submit', request)
      .then((res) => res.data)
      .catch((err: unknown) => {
        console.error('Submit Error:', err)
        throw err
      })
  },

  async update(request: ContractTemplateUpdateRequest) {
    return http
      .put<ContractTemplateUpdateResponse>('/template/update', request)
      .then((res) => res.data)
      .catch((err: unknown) => {
        console.error('Update Error:', err)
        throw err
      })
  },

  async search(request: ContractTemplateSearchRequest): Promise<ContractTemplateSearchResponse> {
    return http
      .get<ContractTemplateSearchResponse>('/template/search', { params: request })
      .then((res) => {
        return res.data
      })
      .catch((err: unknown) => {
        console.error('Search Error:', err)
        return []
      })
  },

  async retrieve() {
    return http
      .get<ContractTemplateRetrieveResponse>('/template/retrieve')
      .then((res) => res.data)
      .catch((err: unknown) => {
        console.error('Retrieve Error:', err)
        return { contract_templates: [], approval_tasks: [], review_tasks: [] }
      })
  },

  async retrieveById(request: ContractTemplateRetrieveByIdRequest): Promise<ContractTemplate | null> {
    return http
      .get<ContractTemplateRetrieveByIdResponse>(`/template/retrieve/${request.did}`)
      .then((res) => {
        return { ...res.data }
      })
      .catch((err: unknown) => {
        console.error('Retrieve ID Error:', err)
        return null
      })
  },

  async approve(request: ContractTemplateApproveRequest) {
    return http.post<ContractTemplateApproveResponse>('/template/approve', request).then((res) => res.data)
  },

  async reject(request: ContractTemplateRejectRequest) {
    return http.post<ContractTemplateRejectResponse>('/template/reject', request).then((res) => res.data)
  },

  async verify(request: ContractTemplateVerifyRequest) {
    return http.post<ContractTemplateVerifyResponse>(`/template/verify`, request).then((res) => res.data)
  },

  async archive(request: ContractTemplateArchiveRequest) {
    return http.post<ContractTemplateArchiveResponse>('/template/archive', request).then((res) => res.data)
  },

  async register(request: ContractTemplateRegisterRequest) {
    return http.post<ContractTemplateRegisterResponse>('/template/register', request).then((res) => res.data)
  },

  async audit(request: ContractTemplateAuditRequest) {
    return http
      .get<ContractTemplateAuditResponse>('/template/audit', { params: request })
      .then((res) => res.data)
      .catch((err: unknown) => {
        console.error('Audit Error:', err)
        return []
      })
  },

  async publish(request: ContractTemplatePublishRequest) {
    return http.post<ContractTemplatePublishResponse>('/template/publish', request).then((res) => res.data)
  },
}
