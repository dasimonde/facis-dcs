<script setup lang="ts">
import { useQRCode } from '@vueuse/integrations/useQRCode'
import { computed, onMounted, onUnmounted, ref } from 'vue'
import {
  isPidPresentationPollError,
  isPidPresentationStatusResponse,
  PID_POLL_ERROR,
} from '@/models/responses/pid-presentation-response'
import {
  clearPidPresentationSession,
  OID4VP_PID_PRESENTATION_URL_KEY,
  OID4VP_PID_STATE_KEY,
} from '@/pid-presentation-session'
import { pidPresentationService } from '@/services/pid-presentation-service'

const emit = defineEmits<{
  success: []
  failed: [message: string]
}>()

const presentationUrl = ref('')
const copyHint = ref('')
const qrCodeDataUrl = useQRCode(computed(() => presentationUrl.value || ''))

let pollTimer: ReturnType<typeof setInterval> | undefined
let refreshTimer: ReturnType<typeof setTimeout> | undefined
let pollGeneration = 0
let pollInFlight = false
let finished = false

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
  if (finished || expiresIn <= 0) return
  const delayMs = Math.max(1000, (expiresIn - PRESENTATION_REFRESH_BUFFER_SEC) * 1000)
  refreshTimer = setTimeout(() => {
    void refreshPresentationLink('Presentation link refreshed (session was about to expire).')
  }, delayMs)
}

function persistSession(state: string, presentation: string, expiresIn: number) {
  sessionStorage.setItem(OID4VP_PID_STATE_KEY, state)
  sessionStorage.setItem(OID4VP_PID_PRESENTATION_URL_KEY, presentation)
  presentationUrl.value = presentation
  schedulePresentationRefresh(expiresIn)
}

function beginPolling(state: string, generation: number) {
  stopPolling()
  void pollStatus(state, generation).then((result) => {
    if (result === 'done' || finished) return
    pollTimer = setInterval(() => {
      void pollStatus(state, generation).then((pollResult) => {
        if (pollResult === 'done' || finished) stopPolling()
      })
    }, 2000)
  })
}

async function refreshPresentationLink(reason: string) {
  if (finished) return
  const state = sessionStorage.getItem(OID4VP_PID_STATE_KEY)?.trim() ?? ''
  if (!state) {
    await restartPresentation(reason)
    return
  }

  stopPolling()
  stopRefreshTimer()
  pollGeneration++
  const generation = pollGeneration

  const renewed = await pidPresentationService.renew(state)
  if (generation !== pollGeneration) return
  if (!renewed) {
    await restartPresentation(reason)
    return
  }

  persistSession(renewed.state, renewed.presentationUrl, renewed.expiresIn)
  copyHint.value = reason
  beginPolling(renewed.state, generation)
}

async function restartPresentation(reason: string) {
  if (finished) return
  stopPolling()
  stopRefreshTimer()
  clearPidPresentationSession()
  presentationUrl.value = ''
  copyHint.value = reason
  await startPresentation()
}

async function pollStatus(state: string, generation: number): Promise<'continue' | 'done'> {
  if (pollInFlight) return 'continue'
  pollInFlight = true
  try {
    return await pollStatusOnce(state, generation)
  } finally {
    pollInFlight = false
  }
}

async function pollStatusOnce(state: string, generation: number): Promise<'continue' | 'done'> {
  const status = await pidPresentationService.pollStatus(state)
  if (generation !== pollGeneration) return 'done'

  if (isPidPresentationPollError(status)) {
    const reason =
      status === PID_POLL_ERROR.NOT_FOUND
        ? 'PID session not found — starting a new session…'
        : 'PID status timed out — starting a new session…'
    await restartPresentation(reason)
    return 'done'
  }
  if (!isPidPresentationStatusResponse(status)) return 'continue'

  if (status.status === 'complete') {
    finished = true
    stopPolling()
    stopRefreshTimer()
    clearPidPresentationSession()
    emit('success')
    return 'done'
  }
  if (status.status === 'expired') {
    await refreshPresentationLink('PID session expired — presentation link refreshed.')
    return 'done'
  }
  if (status.status === 'pending' && status.expires_in > 0) {
    schedulePresentationRefresh(status.expires_in)
  }
  if (status.status === 'failed') {
    finished = true
    stopPolling()
    clearPidPresentationSession()
    const message = status.error_message ?? 'PID presentation failed'
    emit('failed', message)
    return 'done'
  }
  return 'continue'
}

async function startPresentation() {
  pollGeneration++
  const generation = pollGeneration

  let state = sessionStorage.getItem(OID4VP_PID_STATE_KEY)
  let presentation = sessionStorage.getItem(OID4VP_PID_PRESENTATION_URL_KEY)
  if (presentation) {
    presentationUrl.value = presentation
  }

  if (state) {
    const existing = await pidPresentationService.pollStatus(state)
    if (generation !== pollGeneration) return
    if (isPidPresentationStatusResponse(existing)) {
      if (existing.status === 'expired') {
        await refreshPresentationLink('PID session expired — presentation link refreshed.')
        return
      }
      if (existing.status === 'complete') {
        finished = true
        clearPidPresentationSession()
        emit('success')
        return
      }
      if (existing.status === 'failed') {
        clearPidPresentationSession()
        state = null
        presentation = null
        presentationUrl.value = ''
      }
    } else if (existing === PID_POLL_ERROR.NOT_FOUND) {
      await refreshPresentationLink('PID session not found — presentation link refreshed.')
      return
    } else if (existing === PID_POLL_ERROR.TIMEOUT) {
      clearPidPresentationSession()
      state = null
      presentation = null
      presentationUrl.value = ''
    }
  }

  if (!state || !presentation) {
    const initiated = await pidPresentationService.start()
    if (generation !== pollGeneration) return
    if (!initiated) {
      clearPidPresentationSession()
      copyHint.value = 'Could not start PID presentation — check backend and database.'
      return
    }
    persistSession(initiated.state, initiated.presentationUrl, initiated.expiresIn)
    state = initiated.state
    presentation = initiated.presentationUrl
  }

  presentationUrl.value = presentation
  const existingStatus = await pidPresentationService.pollStatus(state)
  if (generation !== pollGeneration) return
  if (isPidPresentationStatusResponse(existingStatus)) {
    schedulePresentationRefresh(existingStatus.expires_in)
  }

  beginPolling(state, generation)
}

onMounted(() => {
  void startPresentation()
})

async function copyPresentationUrl() {
  if (!presentationUrl.value) return
  await navigator.clipboard.writeText(presentationUrl.value)
  copyHint.value = 'Link copied.'
}
</script>

<template>
  <div v-if="presentationUrl" class="card w-full max-w-md bg-base-100 shadow-md">
    <div class="card-body items-center gap-4 text-center">
      <h2 class="card-title text-lg">Present PID credential</h2>
      <p class="text-sm opacity-80">Scan the QR code with your wallet to present your PID.</p>
      <figure class="rounded-box bg-white p-3">
        <img
          v-if="qrCodeDataUrl"
          :src="qrCodeDataUrl"
          alt="OpenID4VP PID presentation QR code"
          class="mx-auto h-48 w-48"
        />
      </figure>
      <button type="button" class="btn btn-sm btn-primary" @click="copyPresentationUrl">Copy link</button>
      <p v-if="copyHint" class="text-sm text-warning">{{ copyHint }}</p>
      <p class="text-xs opacity-70">
        Keep this tab open — the QR / link refreshes automatically before it expires. This is a one-time identity check
        (no login session is created).
      </p>
    </div>
  </div>
  <div v-else class="flex flex-col items-center gap-3">
    <span class="loading loading-lg loading-spinner" />
    <p class="text-sm opacity-70">Starting PID presentation…</p>
  </div>
</template>
