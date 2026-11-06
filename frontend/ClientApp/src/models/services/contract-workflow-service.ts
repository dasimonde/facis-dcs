import type { Contract } from '@/models/contract/contract'
import type { ContractEvidence, StoreContractEvidence } from '@/models/contract/contract-evidence'
import type {
  ContractApproveRequest,
  ContractAuditRequest,
  ContractCreateRequest,
  ContractDeployRequest,
  ContractHistoryRetrieveRequest,
  ContractNegotiationDraftRetrieveRequest,
  ContractNegotiationDraftSaveRequest,
  ContractNegotiationRequest,
  ContractNegotiationRespondRequest,
  ContractOfferAcceptRequest,
  ContractOfferRequest,
  ContractRejectRequest,
  ContractRenewRequest,
  ContractRetrieveByIdRequest,
  ContractRetrieveRequest,
  ContractReviewRequest,
  ContractSearchRequest,
  ContractSubmitRequest,
  ContractTargetDesignateRequest,
  ContractTargetWriteRequest,
  ContractTerminateRequest,
  ContractUpdateRequest,
  ContractWithdrawRequest,
  MachineIdentityWriteRequest,
} from '@/models/requests/contract-request'
import type { ContractPdfVerifyResult } from '@/models/responses/contract-response'
import type {
  ApprovedContractTemplateRetrieveResponse,
  ContractApproveResponse,
  ContractAuditResponse,
  ContractCreateResponse,
  ContractDeployResponse,
  ContractHistoryResponse,
  ContractLifecycleEvent,
  ContractNegotiationDraftResponse,
  ContractNegotiationRespondResponse,
  ContractNegotiationResponse,
  ContractOfferAcceptResponse,
  ContractOfferResponse,
  ContractRejectResponse,
  ContractRenewResponse,
  ContractRetrieveResponse,
  ContractReviewFinding,
  ContractReviewResponse,
  ContractSearchResponse,
  ContractSubmitResponse,
  ContractTarget,
  ContractTerminateResponse,
  ContractUpdateResponse,
  ContractWithdrawResponse,
  MachineCredential,
  MachineIdentity,
  MachineIdentityCreateResponse,
} from '@/models/responses/contract-response'

export interface ContractWorkflowService {
  storeEvidence: (request: StoreContractEvidence) => Promise<{ did: string; evidence_id: string }>
  lifecycle: (did: string) => Promise<ContractLifecycleEvent[]>
  reviewFindings: (did: string) => Promise<ContractReviewFinding[]>
  listEvidence: (did: string) => Promise<ContractEvidence[]>
  retrieveEvidence: (did: string, evidenceId: string) => Promise<ContractEvidence>

  create: (request: ContractCreateRequest) => Promise<ContractCreateResponse>
  update: (request: ContractUpdateRequest) => Promise<ContractUpdateResponse>
  offer: (request: ContractOfferRequest) => Promise<ContractOfferResponse>
  submit: (request: ContractSubmitRequest) => Promise<ContractSubmitResponse>
  negotiate: (request: ContractNegotiationRequest) => Promise<ContractNegotiationResponse>
  acceptOffer: (request: ContractOfferAcceptRequest) => Promise<ContractOfferAcceptResponse>
  saveNegotiationDraft: (request: ContractNegotiationDraftSaveRequest) => Promise<ContractNegotiationDraftResponse>
  retrieveNegotiationDraft: (
    request: ContractNegotiationDraftRetrieveRequest,
  ) => Promise<ContractNegotiationDraftResponse>
  deleteNegotiationDraft: (
    request: ContractNegotiationDraftRetrieveRequest,
  ) => Promise<ContractNegotiationDraftResponse>
  respond: (request: ContractNegotiationRespondRequest) => Promise<ContractNegotiationRespondResponse>
  review: (request: ContractReviewRequest) => Promise<ContractReviewResponse>
  retrieveApprovedTemplates: () => Promise<ApprovedContractTemplateRetrieveResponse>
  retrieve: (request?: ContractRetrieveRequest) => Promise<ContractRetrieveResponse>
  retrieveById: (request: ContractRetrieveByIdRequest) => Promise<Contract | null>
  search: (request: ContractSearchRequest) => Promise<ContractSearchResponse>
  approve: (request: ContractApproveRequest) => Promise<ContractApproveResponse>
  reject: (request: ContractRejectRequest) => Promise<ContractRejectResponse>
  withdraw: (request: ContractWithdrawRequest) => Promise<ContractWithdrawResponse>
  renew: (request: ContractRenewRequest) => Promise<ContractRenewResponse>
  terminate: (request: ContractTerminateRequest) => Promise<ContractTerminateResponse>
  publishCatalogue: (did: string) => Promise<{ did: string; asset_hash: string }>
  deploy: (request: ContractDeployRequest) => Promise<ContractDeployResponse>
  // Contract target systems (ADR-25)
  listTargets: () => Promise<ContractTarget[]>
  createTarget: (request: ContractTargetWriteRequest) => Promise<ContractTarget>
  updateTarget: (request: ContractTargetWriteRequest & { id: string }) => Promise<ContractTarget>
  deleteTarget: (id: string) => Promise<unknown>
  designateTarget: (request: ContractTargetDesignateRequest) => Promise<unknown>
  /** Issues the credential a target authenticates its callbacks with. The
   *  secret is in the response and nowhere else (ADR-27). */
  rotateTargetSecret: (id: string) => Promise<MachineCredential>
  listMachineIdentities: () => Promise<MachineIdentity[]>
  createMachineIdentity: (request: MachineIdentityWriteRequest) => Promise<MachineIdentityCreateResponse>
  updateMachineIdentity: (
    request: MachineIdentityWriteRequest & { id: string; enabled: boolean },
  ) => Promise<MachineIdentity>
  deleteMachineIdentity: (id: string) => Promise<unknown>
  rotateMachineIdentitySecret: (id: string) => Promise<MachineCredential>
  audit: (request: ContractAuditRequest) => Promise<ContractAuditResponse>
  retrieveHistoryByDid: (request: ContractHistoryRetrieveRequest) => Promise<ContractHistoryResponse>
  exportPdf: (did: string) => Promise<Blob>
  exportBundle: (did: string) => Promise<Blob>
  verifyPdf: (did: string) => Promise<ContractPdfVerifyResult>
  verifyPdfCopy: (did: string, copy: Blob) => Promise<ContractPdfVerifyResult>
}
