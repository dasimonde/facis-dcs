export interface ContractEvidence {
  evidence_id: string
  did: string
  contract_version: number
  recorded_by: string
  recorded_at: string
  filename: string
  description: string
  media_type: string
  sha256: string
  content?: string
}
export interface StoreContractEvidence {
  did: string
  updated_at: string
  content: string
  filename: string
  description: string
  media_type?: string
}
