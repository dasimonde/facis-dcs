import type { ApprovalTaskState } from "@/types/approval-task-state"

export interface ContractTemplateApprovalTask {
    type: 'template'
    did: string
    document_number?: string
    version?: number
    state: ApprovalTaskState
    approver: string
    created_at: string
}
