import type { ContractTemplate } from '../contract-template'
import type {
  ContractTemplateApproveRequest,
  ContractTemplateArchiveRequest,
  ContractTemplateAuditRequest,
  ContractTemplateCreateRequest,
  ContractTemplateRegisterRequest,
  ContractTemplateRejectRequest,
  ContractTemplateRetrieveByIdRequest,
  ContractTemplateRetrieveRequest,
  ContractTemplateSearchRequest,
  ContractTemplateSubmitRequest,
  ContractTemplateUpdateRequest,
  ContractTemplateVerifyRequest,
} from '../requests/template-request'
import type {
  ContractTemplateApproveResponse,
  ContractTemplateArchiveResponse,
  ContractTemplateAuditResponse,
  ContractTemplateCreateResponse,
  ContractTemplateRegisterResponse,
  ContractTemplateRejectResponse,
  ContractTemplateRetrieveResponse,
  ContractTemplateSearchResponse,
  ContractTemplateSubmitResponse,
  ContractTemplateUpdateResponse,
  ContractTemplateVerifyResponse,
} from '../responses/template-response'

export interface ContractTemplateService {
  create: (request: ContractTemplateCreateRequest) => Promise<ContractTemplateCreateResponse>
  submit: (request: ContractTemplateSubmitRequest) => Promise<ContractTemplateSubmitResponse>
  update: (request: ContractTemplateUpdateRequest) => Promise<ContractTemplateUpdateResponse>
  search: (request: ContractTemplateSearchRequest) => Promise<ContractTemplateSearchResponse>
  retrieve: (request?: ContractTemplateRetrieveRequest) => Promise<ContractTemplateRetrieveResponse>
  retrieveById: (request: ContractTemplateRetrieveByIdRequest) => Promise<ContractTemplate | null>
  approve: (request: ContractTemplateApproveRequest) => Promise<ContractTemplateApproveResponse>
  reject: (request: ContractTemplateRejectRequest) => Promise<ContractTemplateRejectResponse>
  verify: (request: ContractTemplateVerifyRequest) => Promise<ContractTemplateVerifyResponse>
  archive: (request: ContractTemplateArchiveRequest) => Promise<ContractTemplateArchiveResponse>
  register: (request: ContractTemplateRegisterRequest) => Promise<ContractTemplateRegisterResponse>
  audit: (request: ContractTemplateAuditRequest) => Promise<ContractTemplateAuditResponse>
}
