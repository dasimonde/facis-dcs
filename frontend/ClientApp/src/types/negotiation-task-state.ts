export type NegotiationTaskState =
  (typeof NegotiationTaskState)[keyof typeof NegotiationTaskState]

export const NegotiationTaskState = {
  open: 'OPEN',
  accepted: 'ACCEPTED',
} as const

export const negotiationTaskStates: NegotiationTaskState[] = Object.values(NegotiationTaskState)
