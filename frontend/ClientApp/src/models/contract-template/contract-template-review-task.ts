import type { ReviewTaskState } from '@/types/review-task-state'

export interface ContractTemplateReviewTask {
  type: 'template'
  did: string
  version: number
  state: ReviewTaskState
  reviewer: string
  created_at: string
}
