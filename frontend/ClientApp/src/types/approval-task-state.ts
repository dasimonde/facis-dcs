export type ApprovalTaskState =
  (typeof ApprovalTaskState)[keyof typeof ApprovalTaskState]

export const ApprovalTaskState = {
  open: 'OPEN',
  rejected: 'REJECTED',
  approved: 'APPROVED',
} as const

export const approvalTaskStates: ApprovalTaskState[] = Object.values(ApprovalTaskState)
