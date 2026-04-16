import type { ApprovalTaskState } from "@/types/approval-task-state";

export interface ContractApprovalTask {
  type: 'contract'
  did: string;
  contract_version?: string;
  state: ApprovalTaskState;
  approver: string;
  created_at: string;
}
