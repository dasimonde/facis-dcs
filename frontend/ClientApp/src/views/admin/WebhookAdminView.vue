<script setup lang="ts">
import axios from 'axios'
import { onMounted, ref, useTemplateRef } from 'vue'
import ConfirmationModal from '@/components/ConfirmationModal.vue'
import {
  type CreatedWebhookSubscription,
  type WebhookEvent,
  webhookService,
  type WebhookSubscription,
} from '@/services/webhook-service'

/**
 * Webhook subscriptions of the organization (DCS-FR-TR-22, DCS-NFR-SQ-08,
 * ADR-29): a third-party system such as ORCE gets the DCS events it subscribes
 * to, signed with the secret of its subscription. The secret is in the response
 * that creates the subscription and in no other.
 */

const subscriptions = ref<WebhookSubscription[]>([])
const events = ref<WebhookEvent[]>([])
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const pendingRemovals = ref(new Set<string>())
const created = ref<CreatedWebhookSubscription | null>(null)
const copied = ref(false)
const form = ref({ event: '', callbackURL: '' })
const confirmationModal = useTemplateRef<InstanceType<typeof ConfirmationModal>>('confirmation-modal')

// The webhook API answers {"error": "..."}; show that rather than the status.
const messageOf = (err: unknown, fallback: string) => {
  if (axios.isAxiosError(err) && typeof err.response?.data?.error === 'string') return err.response.data.error
  return err instanceof Error ? err.message : fallback
}

const load = async () => {
  loading.value = true
  error.value = ''
  try {
    const [subs, known] = await Promise.all([webhookService.list(), webhookService.listEvents()])
    subscriptions.value = subs
    events.value = known
  } catch (err) {
    error.value = messageOf(err, 'Could not load the webhook subscriptions')
  } finally {
    loading.value = false
  }
}

onMounted(load)

const subscribe = async () => {
  if (saving.value) return
  saving.value = true
  error.value = ''
  try {
    created.value = await webhookService.create(form.value.event, form.value.callbackURL.trim())
    copied.value = false
    form.value = { event: '', callbackURL: '' }
    subscriptions.value = await webhookService.list()
  } catch (err) {
    error.value = messageOf(err, 'Could not create the subscription')
  } finally {
    saving.value = false
  }
}

const copySecret = async () => {
  if (!created.value) return
  await navigator.clipboard.writeText(created.value.secret)
  copied.value = true
}

const remove = async (sub: WebhookSubscription) => {
  if (pendingRemovals.value.has(sub.id)) return
  const result = await confirmationModal.value?.reveal({
    message: `Delete the subscription to “${sub.event}” at ${sub.callback_url}? The receiver gets no more events from it.`,
  })
  if (!result || result.isCanceled) return
  pendingRemovals.value = new Set(pendingRemovals.value).add(sub.id)
  error.value = ''
  try {
    await webhookService.remove(sub.id)
    subscriptions.value = subscriptions.value.filter((entry) => entry.id !== sub.id)
  } catch (err) {
    error.value = messageOf(err, 'Could not delete the subscription')
  } finally {
    const next = new Set(pendingRemovals.value)
    next.delete(sub.id)
    pendingRemovals.value = next
  }
}
</script>

<template>
  <div data-testid="webhook-admin" class="mx-auto flex w-full max-w-7xl min-w-0 flex-col gap-6 p-4 sm:p-6">
    <div>
      <h1 class="text-2xl font-semibold">Webhooks</h1>
      <p class="mt-1 opacity-70">
        Systems of your organization that receive DCS events. Each delivery is signed with the secret of its
        subscription.
      </p>
    </div>

    <div v-if="error" data-testid="webhook-admin-error" class="alert rounded-box alert-error" role="alert">
      {{ error }}
    </div>

    <div v-if="created" data-testid="webhook-secret" class="alert flex-col items-start alert-warning" role="status">
      <span data-testid="webhook-secret-once">
        Signing secret of the new subscription to “{{ created.event }}”. It is shown once and cannot be retrieved. If it
        is lost, delete the subscription and create a new one.
      </span>
      <div class="flex w-full min-w-0 flex-col gap-2 sm:flex-row">
        <input
          :value="created.secret"
          data-testid="webhook-secret-value"
          readonly
          aria-label="Signing secret"
          class="input-bordered input w-full min-w-0 font-mono text-sm"
        />
        <button type="button" class="btn shrink-0" data-testid="webhook-secret-copy" @click="copySecret">
          {{ copied ? 'Copied' : 'Copy' }}
        </button>
        <button
          type="button"
          class="btn shrink-0 btn-primary"
          data-testid="webhook-secret-done"
          @click="created = null"
        >
          I have copied it
        </button>
      </div>
    </div>

    <section class="card bg-base-200" aria-labelledby="webhook-new-heading">
      <form class="card-body grid min-w-0 gap-3 md:grid-cols-2" @submit.prevent="subscribe">
        <h2 id="webhook-new-heading" class="card-title md:col-span-2">New subscription</h2>
        <label class="flex min-w-0 flex-col gap-2">
          <span class="label-text">Event</span>
          <select v-model="form.event" data-testid="webhook-event" required class="select-bordered select w-full">
            <option value="" disabled>Select an event</option>
            <option v-for="event in events" :key="event.name" :value="event.name">
              {{ event.name }} ({{ event.description }})
            </option>
          </select>
        </label>
        <label class="flex min-w-0 flex-col gap-2">
          <span class="label-text">Callback URL</span>
          <input
            v-model="form.callbackURL"
            data-testid="webhook-url"
            required
            type="url"
            class="input-bordered input w-full min-w-0"
          />
        </label>
        <div class="md:col-span-2">
          <button
            type="submit"
            data-testid="webhook-subscribe"
            :disabled="saving"
            :aria-busy="saving"
            class="btn btn-primary"
          >
            <span v-if="saving" class="loading loading-sm loading-spinner" aria-hidden="true"></span>
            <span>{{ saving ? 'Saving…' : 'Subscribe' }}</span>
          </button>
        </div>
      </form>
    </section>

    <section class="min-w-0" aria-labelledby="webhook-list-heading">
      <h2 id="webhook-list-heading" class="mb-3 text-lg font-semibold">Subscriptions</h2>
      <div v-if="loading" class="flex items-center gap-2 opacity-70" role="status">
        <span class="loading loading-sm loading-spinner" aria-hidden="true"></span>
        Loading…
      </div>
      <div
        v-else-if="subscriptions.length === 0"
        data-testid="webhook-empty-state"
        class="alert rounded-box alert-info"
      >
        Your organization has no webhook subscription.
      </div>
      <div v-else class="max-w-full overflow-x-auto rounded-box border border-base-300">
        <table class="table">
          <thead>
            <tr>
              <th>Event</th>
              <th>Callback URL</th>
              <th>Created</th>
              <th><span class="sr-only">Actions</span></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="sub in subscriptions" :key="sub.id" data-testid="webhook-row">
              <td data-testid="webhook-row-event">{{ sub.event }}</td>
              <td class="font-mono text-xs">{{ sub.callback_url }}</td>
              <td class="text-xs">{{ new Date(sub.created_at).toLocaleString() }}</td>
              <td>
                <button
                  class="btn btn-outline btn-xs btn-error"
                  data-testid="webhook-delete"
                  :disabled="pendingRemovals.has(sub.id)"
                  @click="remove(sub)"
                >
                  {{ pendingRemovals.has(sub.id) ? 'Deleting…' : 'Delete' }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <ConfirmationModal ref="confirmation-modal" />
  </div>
</template>
