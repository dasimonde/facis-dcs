import type { ContractTemplate } from '../contract-template/contract-template'
import type {
  ContractTemplateApproveRequest,
  ContractTemplateArchiveRequest,
  ContractTemplateAuditRequest,
  ContractTemplateCopyRequest,
  ContractTemplateCreateRequest,
  ContractTemplatePublishRequest,
  ContractTemplateRegisterRequest,
  ContractTemplateRejectRequest,
  ContractTemplateRetrieveByIdRequest,
  ContractTemplateRetrieveRequest,
  ContractTemplateSearchRequest,
  ContractTemplateSubmitRequest,
  ContractTemplateUpdateManageRequest,
  ContractTemplateUpdateRequest,
  ContractTemplateVerifyRequest,
} from '../requests/template-request'
import type {
  ContractTemplateApproveResponse,
  ContractTemplateArchiveResponse,
  ContractTemplateAuditResponse,
  ContractTemplateCopyResponse,
  ContractTemplateCreateResponse,
  ContractTemplatePublishResponse,
  ContractTemplateRegisterResponse,
  ContractTemplateRejectResponse,
  ContractTemplateRetrieveResponse,
  ContractTemplateSearchResponse,
  ContractTemplateSubmitResponse,
  ContractTemplateUpdateManageResponse,
  ContractTemplateUpdateResponse,
  ContractTemplateVerifyResponse,
} from '../responses/template-response'

export interface ContractTemplateService {
  create: (request: ContractTemplateCreateRequest) => Promise<ContractTemplateCreateResponse>
  copy: (request: ContractTemplateCopyRequest) => Promise<ContractTemplateCopyResponse>
  submit: (request: ContractTemplateSubmitRequest) => Promise<ContractTemplateSubmitResponse>
  update: (request: ContractTemplateUpdateRequest) => Promise<ContractTemplateUpdateResponse>
  updateManage: (request: ContractTemplateUpdateManageRequest) => Promise<ContractTemplateUpdateManageResponse>
  search: (request: ContractTemplateSearchRequest) => Promise<ContractTemplateSearchResponse>
  retrieve: (request?: ContractTemplateRetrieveRequest) => Promise<ContractTemplateRetrieveResponse>
  retrieveById: (request: ContractTemplateRetrieveByIdRequest) => Promise<ContractTemplate | null>
  approve: (request: ContractTemplateApproveRequest) => Promise<ContractTemplateApproveResponse>
  reject: (request: ContractTemplateRejectRequest) => Promise<ContractTemplateRejectResponse>
  verify: (request: ContractTemplateVerifyRequest) => Promise<ContractTemplateVerifyResponse>
  archive: (request: ContractTemplateArchiveRequest) => Promise<ContractTemplateArchiveResponse>
  register: (request: ContractTemplateRegisterRequest) => Promise<ContractTemplateRegisterResponse>
  audit: (request: ContractTemplateAuditRequest) => Promise<ContractTemplateAuditResponse>
  publish: (request: ContractTemplatePublishRequest) => Promise<ContractTemplatePublishResponse>
  exportPdf: (did: string) => Promise<Blob>
  verifyPdf: (
    did: string,
  ) => Promise<{ match: boolean; jsonld_hash: string; base_pdf_hash: string; stored_base_pdf_hash: string }>
}
