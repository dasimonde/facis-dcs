<script setup lang="ts">
import { computed, ref } from 'vue'
import Pagination from '@/components/Pagination.vue'
import { auditingService } from '@/services/auditing-service'
import { useAuthStore } from '@/stores/auth-store'
import { claimReportedHttpError } from '@/utils/report-action-error'
import type { AccessAttempt } from '@/models/responses/auditing-response'

const authStore = useAuthStore()
const allowed = computed(() => authStore.user?.roles.includes('AUDITOR'))
const justification = ref('')
const loadedJustification = ref('')
const loading = ref(false)
const loaded = ref(false)
const error = ref('')
const attempts = ref<AccessAttempt[]>([])
const cursors = ref<(string | undefined)[]>([undefined])
const page = ref(0)
const nextCursor = ref<string>()

async function load(targetPage = 0, fresh = false) {
  const purpose = fresh ? justification.value.trim() : loadedJustification.value
  if (!allowed.value || loading.value || !purpose) return
  const cursor = fresh ? undefined : targetPage > page.value ? nextCursor.value : cursors.value[targetPage]
  if (targetPage < 0 || (!fresh && targetPage > page.value && !cursor)) return
  loading.value = true
  error.value = ''
  attempts.value = []
  try {
    const result = await auditingService.accessAttempts({ justification: purpose, cursor })
    if (!allowed.value) return
    attempts.value = result.attempts
    nextCursor.value = result.next_cursor
    cursors.value = fresh ? [undefined] : [...cursors.value.slice(0, targetPage), cursor]
    page.value = targetPage
    loadedJustification.value = purpose
    loaded.value = true
  } catch (cause) {
    claimReportedHttpError(cause)
    error.value = 'Access attempts could not be loaded. Please try again.'
    loaded.value = false
    nextCursor.value = undefined
  } finally {
    loading.value = false
  }
}
function formatTimestamp(value: string): string {
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString(undefined, { timeZone: 'UTC' })
}

function shortActor(actor: string): string {
  return actor.length > 24 ? `${actor.slice(0, 12)}…${actor.slice(-8)}` : actor
}
</script>

<template>
  <details v-if="allowed" class="min-w-0 rounded-box border border-base-content/10 bg-base-100 p-4">
    <summary class="cursor-pointer font-semibold">Access attempts</summary>
    <div class="mt-3 min-w-0 space-y-3">
      <p class="text-sm">
        SUCCESS means access allowed; DENIED means permission refused; FAILED means authentication failed.
      </p>
      <form @submit.prevent="load(0, true)">
        <fieldset :disabled="loading" class="flex flex-col items-stretch gap-3 sm:flex-row sm:items-end">
          <div class="min-w-0 flex-1">
            <label for="access-attempt-purpose" class="mb-1 block text-sm font-medium">Access log justification</label>
            <input id="access-attempt-purpose" v-model="justification" required class="input w-full" />
          </div>
          <button type="submit" class="btn w-full btn-primary sm:w-auto" :disabled="!justification.trim()">
            Load access attempts
          </button>
        </fieldset>
      </form>
      <p v-if="error" role="alert" class="text-error">{{ error }}</p>
      <p role="status" aria-live="polite">
        <template v-if="loading">Loading access attempts…</template>
        <template v-else-if="!error && !loaded">Enter a justification to load access attempts.</template>
        <template v-else-if="!error && !attempts.length">No access attempts found.</template>
        <template v-else-if="!error">Access attempts loaded: page {{ page + 1 }}.</template>
      </p>
      <div
        v-if="attempts.length"
        class="max-h-96 max-w-full overflow-auto"
        tabindex="0"
        role="region"
        aria-label="Access attempts table"
      >
        <table class="table table-sm">
          <caption class="sr-only">Access attempts and authorization outcomes</caption>
          <thead class="[&>tr>th]:sticky [&>tr>th]:top-0 [&>tr>th]:z-10 [&>tr>th]:bg-base-100">
            <tr>
              <th scope="col">Outcome</th>
              <th scope="col">Timestamp (UTC)</th>
              <th scope="col">Actor</th>
              <th scope="col">Roles</th>
              <th scope="col">Service</th>
              <th scope="col">Action</th>
              <th scope="col">Scope</th>
              <th scope="col">DID</th>
              <th scope="col">Purpose</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="attempt in attempts"
              :key="attempt.id"
              class="[&>td]:max-w-64 [&>td]:align-top [&>td]:break-words [&>td]:whitespace-normal"
            >
              <td>
                <span
                  class="badge"
                  :class="
                    attempt.outcome === 'DENIED' || attempt.outcome === 'FAILED' ? 'badge-error' : 'badge-success'
                  "
                >
                  {{ attempt.outcome }}
                </span>
              </td>
              <td>
                <time :datetime="attempt.attempted_at" :title="attempt.attempted_at">
                  {{ formatTimestamp(attempt.attempted_at) }}
                </time>
              </td>
              <td>
                <details v-if="attempt.attempt_by && attempt.attempt_by.length > 24">
                  <summary class="cursor-pointer font-mono text-xs" :title="attempt.attempt_by">
                    <span class="sr-only">Full actor identifier:</span>
                    {{ shortActor(attempt.attempt_by) }}
                  </summary>
                  <div class="mt-2 font-mono text-xs break-all">{{ attempt.attempt_by }}</div>
                </details>
                <span v-else>{{ attempt.attempt_by || '—' }}</span>
              </td>
              <td>{{ attempt.roles.join(', ') || '—' }}</td>
              <td>{{ attempt.service }}</td>
              <td>{{ attempt.method }}</td>
              <td>{{ attempt.scope || '—' }}</td>
              <td>{{ attempt.did || '—' }}</td>
              <td>{{ attempt.justification || '—' }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <fieldset v-if="loaded" :disabled="loading" aria-label="Access attempts pages">
        <Pagination
          :current-page="page + 1"
          :has-next-page="!!nextCursor"
          @previous-page="load(page - 1)"
          @next-page="load(page + 1)"
        />
      </fieldset>
    </div>
  </details>
</template>
