import http from '@/api/http'

export interface SignatureContract {
  did: string
  contract_version?: number
  state: string
  name?: string
  description?: string
  created_at: string
  updated_at: string
  // The full JSON-LD contract document (clauses, terms, policies). Present on
  // the by-DID retrieval; absent from the list feed.
  contract_data?: unknown
}

export interface SignatureContractDetail {
  contract: SignatureContract
  // Absent for an APPROVED-unsigned contract with no signature yet (ADR-12).
  signature_envelope?: SignatureEnvelope
  key_version?: number
}

export interface SignatureEnvelope {
  contract_did: string
  signer_did: string
  credential_type: string
  status: string
  signed_at?: string
  revoked_at?: string
  ipfs_cid?: string
}

export interface SignatureVerifyResult {
  did: string
  match: boolean
  jsonld_hash?: string
  base_pdf_hash?: string
  sig_count: number
  findings?: string[]
}

export interface ProvenanceEntry {
  label: string
  lifecycle?: Record<string, string>
}

export interface ProvenanceChainResult {
  did: string
  chain: ProvenanceEntry[]
}

export interface SignatureValidateResult {
  did: string
  findings?: string[]
  dss?: DSSReport
}

export interface SignatureComplianceResult {
  did: string
  findings?: string[]
}

// EU DSS (ETSI EN 319 102-1) validation report surfaced for the compliance
// viewer: the external AdES validator's view of trust anchors, crypto
// integrity, signature level, and timestamp (DCS-FR-SM-18/-26).
export interface DSSReport {
  indication: string
  sub_indication?: string
  signed_by?: string
  signature_format?: string
  signing_time?: string
}

// One applied signature's compliance metadata (DCS-FR-SM-26): signer identity,
// credential class/level, status, timestamps, and the cryptographic integrity
// proof bound into the embedded ContractSigningSummaryCredential.
export interface SignatureViewItem {
  signer_did: string
  field_name?: string
  credential_type: string
  status: string
  signed_at?: string
  revoked_at?: string
  format: string
  jades?: string
  ceremony_id?: string
  content_hash?: string
  pdf_hash?: string
  kb_sd_hash?: string
  validation_report_hash?: string
}

export interface SignatureViewResult {
  did: string
  contract_state: string
  signatures: SignatureViewItem[]
  integrity_findings: string[]
  dss?: DSSReport
}

export interface SignatureAuditEntry {
  id: number
  component: string
  event_type: string
  event_data: unknown
  did?: string
  created_at: string
  res_log_pred_cid?: string
}

export type CeremonyStatus = 'pending' | 'verified' | 'expired' | 'failed'

export interface CeremonyStartResult {
  ceremony_id: string
  wallet_uri: string
  expires_at: string
  status: CeremonyStatus
}

export interface CeremonyStatusResult {
  ceremony_id: string
  contract_did: string
  field_name?: string
  status: CeremonyStatus
  signer_did?: string
  expires_at?: string
}

export const signatureManagementService = {
  async retrieveContracts(): Promise<SignatureContract[]> {
    return http
      .get<{ contracts: SignatureContract[]; signing_tasks: unknown[] }>('/signature/retrieve')
      .then((res) => res.data.contracts ?? [])
  },

  // The Secure Contract Viewer's per-contract read (GET /signature/retrieve/{did}):
  // the full contract document to render for signing, plus its latest signature
  // envelope. Tolerates an APPROVED-unsigned contract that has no envelope yet.
  async retrieveById(did: string): Promise<SignatureContractDetail> {
    return http.get<SignatureContractDetail>(`/signature/retrieve/${encodeURIComponent(did)}`).then((res) => res.data)
  },

  async startCeremony(contractDid: string, fieldName: string): Promise<CeremonyStartResult> {
    return http
      .post<CeremonyStartResult>('/signature/request', { contract_did: contractDid, field_name: fieldName })
      .then((res) => res.data)
  },

  async getCeremonyStatus(ceremonyId: string): Promise<CeremonyStatusResult> {
    return http.get<CeremonyStatusResult>(`/signature/request/${ceremonyId}`).then((res) => res.data)
  },

  // The DCS holds no signing key (ADR-12). Signing is two steps: prepare the
  // to-be-signed PDF (PoA + summary embedded, signature field placed), which the
  // signatory signs externally (their wallet/QTSP, or a desktop PAdES signer),
  // then submit the signed PDF for validation and recording.
  async prepareSignature(did: string, signerDid: string, credentialType: string): Promise<Blob> {
    const res = await http.post<{ document: string }>('/signature/prepare', {
      did,
      signer_did: signerDid,
      credential_type: credentialType,
    })
    const bytes = Uint8Array.from(atob(res.data.document), (c) => c.charCodeAt(0))
    return new Blob([bytes], { type: 'application/pdf' })
  },

  async submitSignature(
    did: string,
    signerDid: string,
    credentialType: string,
    signedPdf: Blob,
  ): Promise<SignatureEnvelope | undefined> {
    const buffer = await signedPdf.arrayBuffer()
    let binary = ''
    new Uint8Array(buffer).forEach((b) => {
      binary += String.fromCharCode(b)
    })
    const res = await http.post<{ did: string; signature_envelope?: SignatureEnvelope }>('/signature/submit', {
      did,
      signer_did: signerDid,
      credential_type: credentialType,
      signed_pdf: btoa(binary),
      jades_signature: '',
    })
    return res.data.signature_envelope
  },

  // The C2PA provenance chain embedded in the signed/exported contract PDF:
  // one entry per manifest (oldest first) with its dcs.lifecycle assertion.
  async getProvenanceChain(did: string): Promise<ProvenanceChainResult> {
    return http.get<ProvenanceChainResult>(`/signature/provenance/${encodeURIComponent(did)}`).then((res) => res.data)
  },

  async verifySignature(did: string): Promise<SignatureVerifyResult> {
    return http.post<SignatureVerifyResult>('/signature/verify', { did }).then((res) => res.data)
  },

  async validateSignature(did: string): Promise<SignatureValidateResult> {
    return http.post<SignatureValidateResult>('/signature/validate', { did }).then((res) => res.data)
  },

  async complianceCheck(did: string): Promise<SignatureComplianceResult> {
    return http.post<SignatureComplianceResult>('/signature/compliance', { did }).then((res) => res.data)
  },

  // The Signature Compliance Viewer's read feed (DCS-FR-SM-26): per-signature
  // signer identity, credential chain, integrity proof, plus the contract's
  // integrity findings and the EU DSS validation report.
  async getSignatureView(did: string): Promise<SignatureViewResult> {
    return http.get<SignatureViewResult>('/signature/view', { params: { did } }).then((res) => res.data)
  },

  async revokeSignature(did: string, signerDid: string): Promise<void> {
    await http.post('/signature/revoke', { did, signer_did: signerDid })
  },

  async getAudit(did: string): Promise<SignatureAuditEntry[]> {
    return http.get<SignatureAuditEntry[]>('/signature/audit', { params: { did } }).then((res) => res.data ?? [])
  },
}
