export type ExtrinsicLifecycle = (typeof ExtrinsicLifecycle)[keyof typeof ExtrinsicLifecycle]

// The peer-facing lifecycle the backend infers per contract (ADR-13), kept in
// sync with backend/internal/contractworkflowengine/datatype/contractstate/extrinsic.go.
// Unlike the intrinsic ContractState it is not stored and not a local workflow
// fact: "executed" means every signature field the contract declares is
// evidenced, which the intrinsic SIGNED state — written on the FIRST signature
// — does not say. Off-ramp states (rejected, withdrawn, revoked, terminated,
// expired) are passed through lowercased and are not listed here.
export const ExtrinsicLifecycle = {
  proposed: 'proposed',
  agreed: 'agreed',
  executed: 'executed',
} as const
