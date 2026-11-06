export interface ContractResponsible {
  creator: string
  approvers: string[]
  reviewers: string[]
  negotiators: string[]
  counterparty?: string
}
