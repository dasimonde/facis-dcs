import http from '@/api/http'
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
import type {
  ApprovedContractTemplateRetrieveResponse,
  ContractApproveResponse,
  ContractAuditResponse,
  ContractCreateResponse,
  ContractDeployResponse,
  ContractHistoryResponse,
  ContractNegotiationDraftResponse,
  ContractNegotiationRespondResponse,
  ContractNegotiationResponse,
  ContractOfferAcceptResponse,
  ContractOfferResponse,
  ContractRejectResponse,
  ContractRenewResponse,
  ContractRetrieveByIdResponse,
  ContractRetrieveResponse,
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
import type { ContractWorkflowService } from '@/models/services/contract-workflow-service'

export const contractWorkflowService: ContractWorkflowService = {
  async create(request: ContractCreateRequest) {
    return http.post<ContractCreateResponse>('/contract/create', request).then((res) => res.data)
  },

  async update(request: ContractUpdateRequest) {
    return http.put<ContractUpdateResponse>('/contract/update', request).then((res) => res.data)
  },

  async offer(request: ContractOfferRequest) {
    return http.post<ContractOfferResponse>('/contract/offer', request).then((res) => res.data)
  },

  async submit(request: ContractSubmitRequest) {
    return http.post<ContractSubmitResponse>('/contract/submit', request).then((res) => res.data)
  },

  async negotiate(request: ContractNegotiationRequest) {
    return http.post<ContractNegotiationResponse>('/contract/negotiate', request).then((res) => res.data)
  },

  // Accept an inbound offer unchanged: mints this instance's negotiation task
  // for the offer's round and takes the contract OFFERED -> NEGOTIATION. Not
  // respond(), which decides one already-proposed change request.
  async acceptOffer(request: ContractOfferAcceptRequest) {
    return http.post<ContractOfferAcceptResponse>('/contract/accept-offer', request).then((res) => res.data)
  },

  async saveNegotiationDraft(request: ContractNegotiationDraftSaveRequest) {
    return http.put<ContractNegotiationDraftResponse>('/contract/negotiation_draft', request).then((res) => res.data)
  },

  async retrieveNegotiationDraft(request: ContractNegotiationDraftRetrieveRequest) {
    return http
      .get<ContractNegotiationDraftResponse>(`/contract/negotiation_draft/${request.did}`)
      .then((res) => res.data)
  },

  async deleteNegotiationDraft(request: ContractNegotiationDraftRetrieveRequest) {
    return http
      .delete<ContractNegotiationDraftResponse>(`/contract/negotiation_draft/${request.did}`)
      .then((res) => res.data)
  },

  async respond(request: ContractNegotiationRespondRequest) {
    return http.post<ContractNegotiationRespondResponse>('/contract/respond', request).then((res) => res.data)
  },

  async review(request: ContractReviewRequest) {
    return http.get<ContractReviewResponse>('/contract/review', { params: request }).then((res) => res.data)
  },

  async retrieve(request?: ContractRetrieveRequest) {
    return http
      .get<ContractRetrieveResponse>('/contract/retrieve', { params: request })
      .then((res) => res.data)
      .catch((err: unknown) => {
        console.error('Retrieve Error:', err)
        return {
          contracts: [],
          review_tasks: [],
          approval_tasks: [],
          negotiation_tasks: [],
        }
      })
  },

  async retrieveApprovedTemplates() {
    return http
      .get<ApprovedContractTemplateRetrieveResponse>('/contract/templates')
      .then((res) => res.data)
      .catch((err: unknown) => {
        console.error('Retrieve Error:', err)
        return []
      })
  },

  async retrieveById(request: ContractRetrieveByIdRequest) {
    return http
      .get<ContractRetrieveByIdResponse>(`/contract/retrieve/${request.did}`)
      .then((res) => ({ ...res.data }))
      .catch((err: unknown) => {
        console.error('Retrieve ID Error:', err)
        return null
      })
  },

  async search(request: ContractSearchRequest) {
    return http
      .get<ContractSearchResponse>('/contract/search', { params: request })
      .then((res) => res.data)
      .catch((err: unknown) => {
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

  async withdraw(request: ContractWithdrawRequest) {
    return http.post<ContractWithdrawResponse>('/contract/withdraw', request).then((res) => res.data)
  },

  async renew(request: ContractRenewRequest) {
    return http.post<ContractRenewResponse>('/contract/renew', request).then((res) => res.data)
  },

  async terminate(request: ContractTerminateRequest) {
    return http.post<ContractTerminateResponse>('/contract/terminate', request).then((res) => res.data)
  },

  async deploy(request: ContractDeployRequest) {
    return http.post<ContractDeployResponse>('/contract/deploy', request).then((res) => res.data)
  },

  // ---- Contract target systems (ADR-25) ------------------------------------

  async listTargets() {
    return http.get<ContractTarget[]>('/contract/targets').then((res) => res.data)
  },

  async createTarget(request: ContractTargetWriteRequest) {
    return http.post<ContractTarget>('/contract/targets', request).then((res) => res.data)
  },

  async updateTarget(request: ContractTargetWriteRequest & { id: string }) {
    return http.put<ContractTarget>('/contract/targets', request).then((res) => res.data)
  },

  async deleteTarget(id: string) {
    return http.delete('/contract/targets', { data: { id } }).then((res) => res.data)
  },

  /** Issue a new callback credential for a target. The secret comes back once
   *  and the previous one stops working immediately (ADR-27). */
  async rotateTargetSecret(id: string) {
    return http
      .post<MachineCredential>(`/contract/targets/${encodeURIComponent(id)}/credential`)
      .then((res) => res.data)
  },

  // ---- Machine identities (ADR-27) -----------------------------------------

  async listMachineIdentities() {
    return http.get<{ identities: MachineIdentity[] }>('/machine-identities').then((res) => res.data.identities)
  },

  /** Registers the identity and issues its first credential. The secret is in
   *  this response and in no other. */
  async createMachineIdentity(request: MachineIdentityWriteRequest) {
    return http.post<MachineIdentityCreateResponse>('/machine-identities', request).then((res) => res.data)
  },

  async updateMachineIdentity(request: MachineIdentityWriteRequest & { id: string; enabled: boolean }) {
    return http
      .put<MachineIdentity>(`/machine-identities/${encodeURIComponent(request.id)}`, request)
      .then((res) => res.data)
  },

  async deleteMachineIdentity(id: string) {
    return http.delete(`/machine-identities/${encodeURIComponent(id)}`).then((res) => res.data)
  },

  async rotateMachineIdentitySecret(id: string) {
    return http
      .post<MachineCredential>(`/machine-identities/${encodeURIComponent(id)}/credential`)
      .then((res) => res.data)
  },

  async designateTarget(request: ContractTargetDesignateRequest) {
    return http.post('/contract/target/designate', request).then((res) => res.data)
  },

  async audit(request: ContractAuditRequest) {
    return http.post<ContractAuditResponse>('/contract/audit', request).then((res) => res.data)
  },

  async retrieveHistoryByDid(request: ContractHistoryRetrieveRequest) {
    return http
      .get<ContractHistoryResponse>(`/contract/history/${request.did}`)
      .then((res) => res.data ?? [])
      .catch((err: unknown) => {
        console.error('Retrieve Error:', err)
        return []
      })
  },

  async exportPdf(did: string): Promise<Blob> {
    return http
      .get<Blob>(`/pdf/export/contract/${encodeURIComponent(did)}`, { responseType: 'blob' })
      .then((res) => res.data)
  },

  async exportBundle(did: string): Promise<Blob> {
    return http
      .get<Blob>(`/contract/export/${encodeURIComponent(did)}`, { responseType: 'blob' })
      .then((res) => res.data)
  },

  async verifyPdf(did: string): Promise<{
    match: boolean
    jsonld_hash: string
    base_pdf_hash: string
    stored_base_pdf_hash: string
    c2pa_manifest_found?: boolean
    c2pa_signature_status?: string
    vc_proof_status?: string
    status_list_uri?: string
    lifecycle_status?: string
    status_list_status?: string
  }> {
    return http.get(`/pdf/verify/contract/${encodeURIComponent(did)}`).then((res) => res.data)
  },
}
