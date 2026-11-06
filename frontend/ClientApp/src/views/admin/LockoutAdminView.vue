<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { type LockoutInfo, lockoutService } from '@/services/lockout-service'

/**
 * Active lockouts after repeated invalid credentials (FR-UC-01-4). A lock ends
 * by time; the System Administrator can end it now (NFR-SEC-11).
 */

const lockouts = ref<LockoutInfo[]>([])
const loading = ref(false)
const error = ref('')
const unlocking = ref('')

const load = async () => {
  loading.value = true
  error.value = ''
  try {
    lockouts.value = await lockoutService.list()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Could not load the lockouts'
  } finally {
    loading.value = false
  }
}

const unlock = async (key: string) => {
  unlocking.value = key
  error.value = ''
  try {
    await lockoutService.unlock(key)
    await load()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Could not end the lockout'
  } finally {
    unlocking.value = ''
  }
}

onMounted(load)

function formatTimestamp(value: string): string {
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString()
}
</script>

<template>
  <div data-testid="lockouts" class="mx-auto flex w-full max-w-7xl min-w-0 flex-col gap-6 p-4 sm:p-6">
    <div>
      <h1 class="text-2xl font-semibold">Lockouts</h1>
      <p class="mt-1 opacity-70">
        Client addresses and credential holder keys that are locked out after repeated invalid credentials. A lock ends
        by itself at the time shown. Unlock ends it now.
      </p>
    </div>

    <div v-if="error" data-testid="lockouts-error" class="alert rounded-box alert-error" role="alert">
      {{ error }}
    </div>

    <section class="min-w-0" aria-labelledby="active-lockouts-heading">
      <h2 id="active-lockouts-heading" class="mb-3 text-lg font-semibold">Active lockouts</h2>
      <div v-if="loading" class="flex items-center gap-2 opacity-70" role="status">
        <span class="loading loading-sm loading-spinner" aria-hidden="true"></span>
        Loading…
      </div>
      <div v-else-if="lockouts.length === 0" data-testid="lockouts-empty-state" class="alert rounded-box alert-info">
        No address or holder key is locked out.
      </div>
      <div v-else class="max-w-full overflow-x-auto rounded-box border border-base-300">
        <table class="table min-w-160">
          <thead>
            <tr>
              <th>Locked key</th>
              <th>Locked until</th>
              <th><span class="sr-only">Actions</span></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="lock in lockouts" :key="lock.key" data-testid="lockout-row">
              <td class="font-mono text-sm break-all">{{ lock.key }}</td>
              <td>{{ formatTimestamp(lock.locked_until) }}</td>
              <td class="text-right">
                <button
                  type="button"
                  class="btn btn-sm"
                  :disabled="unlocking !== ''"
                  :aria-label="`Unlock ${lock.key}`"
                  data-testid="lockout-unlock"
                  @click="unlock(lock.key)"
                >
                  Unlock
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </div>
</template>
