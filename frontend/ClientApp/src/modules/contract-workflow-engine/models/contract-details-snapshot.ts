import { formatSigningOrderForDisplay, signingDeadlineDateString } from '@/utils/signing-workflow'

export interface ContractDetailsSnapshot {
  name?: string
  description?: string
  contract_version?: number
  start_date?: string
  exp_date?: string
  exp_policy?: string
  exp_notice_period?: number
  signing_deadline?: string
  signing_order?: string
}

/** Adds signing deadline/order from contract_data onto a details snapshot. */
export function withSigningDetailsFromContractData(
  snapshot: ContractDetailsSnapshot | null | undefined,
  contractData: unknown,
): ContractDetailsSnapshot | null {
  if (snapshot == null && contractData == null) return null
  const deadline = signingDeadlineDateString(contractData)
  const order = formatSigningOrderForDisplay(contractData)
  return {
    ...(snapshot ?? {}),
    signing_deadline: deadline || undefined,
    signing_order: order || undefined,
  }
}
