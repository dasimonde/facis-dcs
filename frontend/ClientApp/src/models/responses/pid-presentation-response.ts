export interface PidPresentationResponse {
  presentation_url: string
  state: string
  expires_in: number
}

export interface PidPresentationStatusResponse {
  state: string
  status: 'pending' | 'complete' | 'failed' | 'expired'
  expires_in: number
  error_message?: string
}

export type PidPresentationPollError = 'timeout' | 'not_found'

export const PID_POLL_ERROR = {
  TIMEOUT: 'timeout',
  NOT_FOUND: 'not_found',
} as const satisfies Record<string, PidPresentationPollError>

export type PidPresentationPollStatus = PidPresentationStatusResponse | PidPresentationPollError | null

export function isPidPresentationStatusResponse(
  value: PidPresentationPollStatus,
): value is PidPresentationStatusResponse {
  return typeof value === 'object' && value !== null && 'status' in value
}

export function isPidPresentationPollError(value: PidPresentationPollStatus): value is PidPresentationPollError {
  return value === PID_POLL_ERROR.TIMEOUT || value === PID_POLL_ERROR.NOT_FOUND
}
