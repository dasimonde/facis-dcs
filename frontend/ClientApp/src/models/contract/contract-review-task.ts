import type { ContractReviewTaskState } from "@/types/review-task-state";

export interface ContractReviewTask {
  type: 'contract'
  did: string;
  contract_version?: string;
  state: ContractReviewTaskState;
  reviewer: string;
  created_at: string;
}
