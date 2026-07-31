<script setup lang="ts">
import { useQRCode } from '@vueuse/integrations/useQRCode'
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  clearOid4vpBrowserSession,
  flushPendingLoginChallenge,
  normalizeCallbackRedirect,
  OID4VP_AUTHORIZE_DONE_KEY,
  OID4VP_AUTHORIZE_URL_KEY,
  OID4VP_PRESENTATION_URL_KEY,
  OID4VP_STATE_KEY,
} from '@/hydra-login-guard'
import { isLoginPollError, isLoginStatusResponse, LOGIN_POLL_ERROR } from '@/models/responses/auth-response'
import { ROUTES } from '@/router/router'
import { authenticationService } from '@/services/authentication-service'
import { useErrorStore } from '@/stores/error-store'

const route = useRoute()
const router = useRouter()
const presentationUrl = ref('')
const copyHint = ref('')
const qrCodeDataUrl = useQRCode(computed(() => presentationUrl.value || ''))

const errorStore = useErrorStore()

let pollTimer: ReturnType<typeof setInterval> | undefined
let refreshTimer: ReturnType<typeof setTimeout> | undefined
let pollGeneration = 0
let pollInFlight = false
let redirectStarted = false

/** Refresh the wallet link slightly before backend TTL so QR/copy never point at an expired state. */
const PRESENTATION_REFRESH_BUFFER_SEC = 5

onUnmounted(() => {
  stopPolling()
  stopRefreshTimer()
})

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = undefined
  }
}

function stopRefreshTimer() {
  if (refreshTimer) {
    clearTimeout(refreshTimer)
    refreshTimer = undefined
  }
}

function schedulePresentationRefresh(expiresIn: number) {
  stopRefreshTimer()
  if (redirectStarted || expiresIn <= 0) return
  const delayMs = Math.max(1000, (expiresIn - PRESENTATION_REFRESH_BUFFER_SEC) * 1000)
  refreshTimer = setTimeout(() => {
    void refreshPresentationLink('Presentation link refreshed (session was about to expire).')
  }, delayMs)
}

function persistLoginSession(state: string, presentation: string, authorizeUrl: string, expiresIn: number) {
  sessionStorage.setItem(OID4VP_STATE_KEY, state)
  sessionStorage.setItem(OID4VP_PRESENTATION_URL_KEY, presentation)
  sessionStorage.setItem(OID4VP_AUTHORIZE_URL_KEY, authorizeUrl)
  presentationUrl.value = presentation
  schedulePresentationRefresh(expiresIn)
}

function beginPolling(state: string, generation: number) {
  stopPolling()
  void pollLogin(state, generation).then((result) => {
    if (result === 'done' || redirectStarted) return
    pollTimer = setInterval(() => {
      void pollLogin(state, generation).then((pollResult) => {
        if (pollResult === 'done' || redirectStarted) stopPolling()
      })
    }, 2000)
  })
}

function canNavigateTo(url: string): boolean {
  try {
    const parsed = new URL(url)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:'
  } catch {
    return false
  }
}

function goToHydraAuthorize(authorizeUrl: string, state: string) {
  if (!authorizeUrl || redirectStarted) return
  sessionStorage.setItem(OID4VP_AUTHORIZE_URL_KEY, authorizeUrl)
  sessionStorage.setItem(OID4VP_STATE_KEY, state)
  sessionStorage.setItem(OID4VP_AUTHORIZE_DONE_KEY, state)
  window.location.assign(authorizeUrl)
}

function finishCompleteRedirect(redirectUri: string) {
  if (redirectStarted) return
  const target = normalizeCallbackRedirect(redirectUri)
  if (!canNavigateTo(target)) {
    console.error('OpenID4VP: invalid redirect_uri', target)
    return
  }
  redirectStarted = true
  stopPolling()
  stopRefreshTimer()
  try {
    if (new URL(target).searchParams.has('code')) {
      clearOid4vpBrowserSession()
    }
  } catch {
    /* hydra continue URL */
  }
  window.location.assign(target)
}

async function refreshPresentationLink(reason: string) {
  if (redirectStarted) return
  const state = sessionStorage.getItem(OID4VP_STATE_KEY)?.trim() ?? ''
  if (!state) {
    await restartLoginFull(reason)
    return
  }

  stopPolling()
  stopRefreshTimer()
  pollGeneration++
  const generation = pollGeneration

  const renewed = await authenticationService.loginRenew(state)
  if (generation !== pollGeneration) return
  if (!renewed) {
    await restartLoginFull(reason)
    return
  }

  persistLoginSession(renewed.state, renewed.presentationUrl, renewed.authorizeUrl, renewed.expiresIn)
  sessionStorage.setItem(OID4VP_AUTHORIZE_URL_KEY, renewed.authorizeUrl)

  copyHint.value = reason
  beginPolling(renewed.state, generation)
}

async function restartLogin(reason: string) {
  if (redirectStarted) return
  if (sessionStorage.getItem(OID4VP_STATE_KEY)?.trim()) {
    await refreshPresentationLink(reason)
    return
  }
  await restartLoginFull(reason)
}

async function restartLoginFull(reason: string) {
  if (redirectStarted) return
  stopPolling()
  stopRefreshTimer()
  clearOid4vpBrowserSession()
  presentationUrl.value = ''
  copyHint.value = reason
  await startLogin()
}

async function pollLogin(state: string, generation: number): Promise<'continue' | 'done'> {
  if (pollInFlight) return 'continue'
  pollInFlight = true
  try {
    return await pollLoginOnce(state, generation)
  } finally {
    pollInFlight = false
  }
}

async function pollLoginOnce(state: string, generation: number): Promise<'continue' | 'done'> {
  const status = await authenticationService.loginPollStatus(state)
  if (generation !== pollGeneration) return 'done'

  if (isLoginPollError(status)) {
    const reason =
      status === LOGIN_POLL_ERROR.NOT_FOUND
        ? 'Login session not found — starting a new session…'
        : 'Login status timed out — starting a new session…'
    await restartLogin(reason)
    return 'done'
  }
  if (!isLoginStatusResponse(status)) return 'continue'

  if (status.status === 'complete' && status.redirect_uri) {
    finishCompleteRedirect(status.redirect_uri)
    return 'done'
  }
  if (status.status === 'expired') {
    await refreshPresentationLink('Login session expired — presentation link refreshed.')
    return 'done'
  }
  if (status.status === 'pending' && status.expires_in > 0) {
    schedulePresentationRefresh(status.expires_in)
  }
  if (status.status === 'failed') {
    stopPolling()
    clearOid4vpBrowserSession()
    console.error('OpenID4VP login failed:', status.error_message ?? status.status)
    errorStore.add(status.error_message ?? status.status)
    return 'done'
  }
  return 'continue'
}

function resumeAuthorizeIfNeeded(state: string): boolean {
  if (sessionStorage.getItem(OID4VP_AUTHORIZE_DONE_KEY) === state) {
    return false
  }
  const authorizeUrl = sessionStorage.getItem(OID4VP_AUTHORIZE_URL_KEY)
  if (!authorizeUrl) {
    return false
  }
  goToHydraAuthorize(authorizeUrl, state)
  return true
}

async function startLogin() {
  pollGeneration++
  const generation = pollGeneration

  let state = sessionStorage.getItem(OID4VP_STATE_KEY)
  let presentation = sessionStorage.getItem(OID4VP_PRESENTATION_URL_KEY)
  if (presentation) {
    presentationUrl.value = presentation
  }

  if (state) {
    const existing = await authenticationService.loginPollStatus(state)
    if (generation !== pollGeneration) return
    if (isLoginStatusResponse(existing)) {
      if (existing.status === 'expired') {
        await refreshPresentationLink('Login session expired — presentation link refreshed.')
        return
      }
      if (existing.status === 'complete' && existing.redirect_uri) {
        finishCompleteRedirect(existing.redirect_uri)
        return
      }
      if (existing.status === 'complete') {
        copyHint.value = 'Login complete but missing redirect_uri — check Hydra flow.'
        return
      }
      if (existing.status === 'failed') {
        clearOid4vpBrowserSession()
        state = null
        presentation = null
        presentationUrl.value = ''
      } else if (resumeAuthorizeIfNeeded(state)) {
        return
      }
    } else if (existing === LOGIN_POLL_ERROR.NOT_FOUND) {
      await refreshPresentationLink('Login session not found — presentation link refreshed.')
      return
    } else if (existing === LOGIN_POLL_ERROR.TIMEOUT) {
      clearOid4vpBrowserSession()
      state = null
      presentation = null
      presentationUrl.value = ''
    } else if (resumeAuthorizeIfNeeded(state)) {
      return
    }
  }

  if (!state || !presentation) {
    const initiated = await authenticationService.login()
    if (generation !== pollGeneration) return
    if (!initiated) {
      clearOid4vpBrowserSession()
      copyHint.value = 'Could not start login — check backend and database.'
      return
    }
    persistLoginSession(initiated.state, initiated.presentationUrl, initiated.authorizeUrl, initiated.expiresIn)
    state = initiated.state
    presentation = initiated.presentationUrl
    sessionStorage.removeItem(OID4VP_AUTHORIZE_DONE_KEY)
    await flushPendingLoginChallenge(state)
    goToHydraAuthorize(initiated.authorizeUrl, state)
    return
  }

  presentationUrl.value = presentation
  const existingStatus = await authenticationService.loginPollStatus(state)
  if (generation !== pollGeneration) return
  if (isLoginStatusResponse(existingStatus)) {
    schedulePresentationRefresh(existingStatus.expires_in)
  }

  beginPolling(state, generation)
}

onMounted(async () => {
  await router.isReady()

  const storedPresentation = sessionStorage.getItem(OID4VP_PRESENTATION_URL_KEY)
  if (storedPresentation) {
    presentationUrl.value = storedPresentation
  }

  const authError = route.query.auth_error
  if (typeof authError === 'string' && authError) {
    clearOid4vpBrowserSession()
    copyHint.value =
      typeof route.query.auth_error_description === 'string'
        ? `Login cancelled: ${route.query.auth_error_description}`
        : `Login cancelled (${authError}).`
    await router.replace({ path: route.path, query: {} })
  }

  if (route.query.code && route.query.state) {
    await router.replace({ name: ROUTES.AUTH.SUCCESS, query: route.query })
    return
  }

  await startLogin()
})

async function copyPresentationUrl() {
  if (!presentationUrl.value) return
  await navigator.clipboard.writeText(presentationUrl.value)
  copyHint.value = 'Link copied.'
}
</script>

<template>
  <div class="flex min-h-screen flex-col items-center justify-center gap-6 bg-base-200 p-6" role="main">
    <div v-if="presentationUrl" class="card w-full max-w-md bg-base-100 shadow-md">
      <div class="card-body items-center gap-4 text-center">
        <h1 class="card-title text-lg">Sign in with wallet</h1>
        <p class="text-sm opacity-80">Scan the QR code with your wallet.</p>
        <figure class="rounded-box bg-white p-3">
          <img
            v-if="qrCodeDataUrl"
            :src="qrCodeDataUrl"
            alt="OpenID4VP request URI QR code"
            class="mx-auto h-48 w-48"
          />
        </figure>
        <button type="button" class="btn btn-sm btn-primary" @click="copyPresentationUrl">Copy link</button>
        <p v-if="copyHint" class="text-sm text-warning">{{ copyHint }}</p>
        <p class="text-xs opacity-70">
          Keep this tab open — the QR / link refreshes automatically before it expires (about every 5 minutes). You will
          be redirected after the wallet presents credentials.
        </p>
      </div>
    </div>
    <div v-else class="flex flex-col items-center gap-3">
      <span class="loading loading-lg loading-spinner" />
      <p class="text-sm opacity-70">Starting login…</p>
    </div>
  </div>
</template>
