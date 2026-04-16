export type ReviewTaskState = (typeof ReviewTaskState)[keyof typeof ReviewTaskState]
export type ContractReviewTaskState = Exclude<ReviewTaskState, 'VERIFIED'>

export const ReviewTaskState = {
  open: 'OPEN',
  rejected: 'REJECTED',
  verified: 'VERIFIED',
  approved: 'APPROVED',
} as const

export const reviewTaskStates: ReviewTaskState[] = Object.values(ReviewTaskState)
