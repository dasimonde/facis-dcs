import authHttp from '@/api/auth-http'
import { getConfig } from '@/config'

function consentChallengeFromURL(url: URL): string {
  return url.searchParams.get('consent_challenge')?.trim() ?? ''
}

export const OID4VP_STATE_KEY = 'dcs_oid4vp_state'
export const OID4VP_AUTHORIZE_URL_KEY = 'dcs_oid4vp_authorize_url'
export const OID4VP_AUTHORIZE_DONE_KEY = 'dcs_oid4vp_authorize_done'
export const OID4VP_PRESENTATION_URL_KEY = 'dcs_oid4vp_presentation_url'
/** Last Hydra login_challenge bound (or pending) — used to re-bind after presentation link refresh. */
export const OID4VP_LOGIN_CHALLENGE_KEY = 'dcs_oid4vp_login_challenge'
const OID4VP_PENDING_CHALLENGE_KEY = 'dcs_pending_login_challenge'
const HYDRA_CHALLENGE_BOUND_KEY = 'dcs_hydra_challenge_bound_state'

export function loginChallengeFromURL(url: URL = new URL(window.location.href)): string {
  return url.searchParams.get('login_challenge')?.trim() ?? ''
}

/** Map Hydra RP callback with code to this origin's /api/auth/callback (Vite proxy). */
export function normalizeCallbackRedirect(redirectUri: string): string {
  try {
    const target = new URL(redirectUri)
    if (!target.searchParams.get('code')) {
      return redirectUri
    }
    const apiBase = getConfig().API_BASE_URL.replace(/\/$/, '')
    return `${window.location.origin}${apiBase}/auth/callback${target.search}`
  } catch {
    return redirectUri
  }
}

export async function bindLoginChallengeOnce(state: string, loginChallenge: string): Promise<boolean> {
  const challenge = loginChallenge.trim()
  if (!challenge || sessionStorage.getItem(HYDRA_CHALLENGE_BOUND_KEY) === state) {
    return sessionStorage.getItem(HYDRA_CHALLENGE_BOUND_KEY) === state
  }
  try {
    await authHttp.post('/auth/login/challenge', { state, login_challenge: challenge })
    sessionStorage.setItem(HYDRA_CHALLENGE_BOUND_KEY, state)
    sessionStorage.setItem(OID4VP_AUTHORIZE_DONE_KEY, state)
    sessionStorage.setItem(OID4VP_LOGIN_CHALLENGE_KEY, challenge)
    sessionStorage.removeItem(OID4VP_PENDING_CHALLENGE_KEY)
    return true
  } catch (err) {
    console.error('Failed to bind Hydra login_challenge:', err)
    return false
  }
}

export async function bindHydraLoginChallengeFromURL(): Promise<void> {
  const url = new URL(window.location.href)
  const consentChallenge = consentChallengeFromURL(url)
  if (consentChallenge) {
    const apiBase = getConfig().API_BASE_URL.replace(/\/$/, '')
    window.location.replace(
      `${window.location.origin}${apiBase}/auth/consent?consent_challenge=${encodeURIComponent(consentChallenge)}`,
    )
    return
  }
  const loginChallenge = loginChallengeFromURL(url)
  if (loginChallenge) {
    const state = sessionStorage.getItem(OID4VP_STATE_KEY)
    if (state) {
      await bindLoginChallengeOnce(state, loginChallenge)
    } else {
      sessionStorage.setItem(OID4VP_PENDING_CHALLENGE_KEY, loginChallenge)
      sessionStorage.setItem(OID4VP_LOGIN_CHALLENGE_KEY, loginChallenge)
    }
  }
  stripHydraChallengeQuery(url)
}

export async function flushPendingLoginChallenge(state: string): Promise<void> {
  const pending = sessionStorage.getItem(OID4VP_PENDING_CHALLENGE_KEY)?.trim() ?? ''
  if (pending) {
    await bindLoginChallengeOnce(state, pending)
  }
}

export function clearOid4vpBrowserSession(): void {
  sessionStorage.removeItem(OID4VP_STATE_KEY)
  sessionStorage.removeItem(OID4VP_PRESENTATION_URL_KEY)
  sessionStorage.removeItem(OID4VP_AUTHORIZE_URL_KEY)
  sessionStorage.removeItem(OID4VP_AUTHORIZE_DONE_KEY)
  sessionStorage.removeItem(OID4VP_PENDING_CHALLENGE_KEY)
  sessionStorage.removeItem(OID4VP_LOGIN_CHALLENGE_KEY)
  sessionStorage.removeItem(HYDRA_CHALLENGE_BOUND_KEY)
}

export function stripHydraChallengeQuery(url: URL = new URL(window.location.href)): void {
  if (!url.searchParams.has('login_challenge') && !url.searchParams.has('consent_challenge')) {
    return
  }
  url.searchParams.delete('login_challenge')
  url.searchParams.delete('consent_challenge')
  window.history.replaceState({}, '', url.pathname + url.search + url.hash)
}
