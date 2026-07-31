export const OID4VP_PID_STATE_KEY = 'oid4vp_pid_state'
export const OID4VP_PID_PRESENTATION_URL_KEY = 'oid4vp_pid_presentation_url'

export function clearPidPresentationSession() {
  sessionStorage.removeItem(OID4VP_PID_STATE_KEY)
  sessionStorage.removeItem(OID4VP_PID_PRESENTATION_URL_KEY)
}
