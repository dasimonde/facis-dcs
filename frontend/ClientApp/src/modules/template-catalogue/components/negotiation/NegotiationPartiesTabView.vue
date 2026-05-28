<template>
  <section class="space-y-4">
    <section class="rounded-xl border border-base-300 p-3">
      <h4 class="mb-3 text-sm font-semibold">Initiator</h4>
      <div v-if="initiator" class="space-y-1">
        <div class="text-sm">
          <span class="font-semibold">Legal Name:</span>
          {{ initiator.legal_name || '-' }}
        </div>
        <div class="text-sm">
          <span class="font-semibold">Registration Number:</span>
          {{ initiator.registration_number || '-' }}
        </div>
      </div>
      <div v-else class="text-sm text-base-content/60">No initiator found.</div>
    </section>

    <div>
      <div class="mb-2 text-xs font-semibold text-base-content/70">Invited ({{ selectedResponderIndexes.length }})</div>
      <div v-if="selectedResponders.length === 0" class="text-sm text-base-content/60">No responders selected.</div>
      <div v-else class="flex flex-wrap gap-2">
        <div
          v-for="entry in selectedResponders"
          :key="entry.participant.registration_number || entry.participant.legal_name || `tag-${entry.index}`"
          class="badge gap-1 badge-outline pr-1 badge-primary"
        >
          <span>{{ entry.participant.legal_name || 'Unnamed participant' }}</span>
          <button
            type="button"
            class="btn btn-circle btn-ghost btn-xs"
            aria-label="Remove invited participant"
            @click="$emit('toggle-participant', entry.index)"
          >
            ✕
          </button>
        </div>
      </div>
    </div>

    <section class="rounded-xl border border-base-300 p-3">
      <h4 class="mb-3 text-sm font-semibold">Responders</h4>
      <div v-if="participants.length === 0" class="text-sm text-base-content/60">No responders found.</div>
      <ul v-else class="max-h-[52vh] space-y-2 overflow-auto pr-1">
        <li
          v-for="(participant, index) in participants"
          :key="participant.registration_number || participant.legal_name || `participant-${index}`"
          class="rounded-lg border border-base-300 p-2"
        >
          <label class="flex cursor-pointer items-start gap-3">
            <input
              :checked="selectedResponderIndexes.includes(index)"
              type="checkbox"
              class="checkbox mt-1"
              @change="$emit('toggle-participant', index)"
            />
            <div class="min-w-0">
              <div class="truncate font-medium">
                {{ participant.legal_name || 'Unnamed participant' }}
              </div>
              <div class="text-xs break-all text-base-content/70">
                {{ participant.registration_number || '-' }}
              </div>
              <div class="text-xs break-all text-base-content/70">
                Legal Entity Identifier: {{ participant.lei_code || '-' }}
              </div>
              <div class="text-xs break-all text-base-content/70">
                Headquarter Address: {{ participant.headquarter_address?.country || '-' }},
                {{ participant.headquarter_address?.locality || '-' }}
              </div>
              <div class="text-xs break-all text-base-content/70">
                Terms and Conditions: {{ participant.terms_and_conditions || '-' }}
              </div>
            </div>
          </label>
        </li>
      </ul>
    </section>
  </section>
</template>

<script setup lang="ts">
import type { Participant } from '@/modules/template-catalogue/models/participant'

defineProps<{
  initiator: Participant | null
  participants: Participant[]
  selectedResponderIndexes: number[]
  selectedResponders: { index: number; participant: Participant }[]
}>()

defineEmits<(e: 'toggle-participant', index: number) => void>()
</script>
