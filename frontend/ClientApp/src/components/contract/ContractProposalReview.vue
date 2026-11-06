<script setup lang="ts">
import { computed, ref } from 'vue'
import NegotiationList from '@/components/lists/contract/negotiation/NegotiationList.vue'
import { useAuthStore } from '@/stores/auth-store'
import { activeNegotiations } from '@/utils/contract-negotiations'
import type { Contract } from '@/models/contract/contract'

const props = defineProps<{
  contract: Contract
  localInstanceDid: string
}>()

const emit = defineEmits<{
  responded: []
  refresh: []
}>()

const isRefreshing = ref(false)

const authStore = useAuthStore()
const participantId = computed(() => authStore.user?.issuer ?? '')

const hasActiveNegotiations = computed(() => activeNegotiations(props.contract).length > 0)

const handleResponded = () => {
  emit('responded')
}

const refreshProposals = () => {
  if (isRefreshing.value) return
  isRefreshing.value = true
  emit('refresh')
  // Parent reloads asynchronously; clear the spinner even if the payload is unchanged.
  window.setTimeout(() => {
    isRefreshing.value = false
  }, 800)
}

defineExpose({
  hasActiveNegotiations,
})
</script>

<template>
  <div class="space-y-4" data-testid="contract-proposal-review">
    <section data-testid="active-negotiations" class="card border border-base-300 bg-base-100 shadow-sm">
      <div class="card-body gap-3">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <h2 class="card-title text-base">Active proposals</h2>
          <button
            type="button"
            class="btn btn-outline btn-sm btn-primary"
            data-testid="refresh-proposals"
            :disabled="isRefreshing"
            @click="refreshProposals"
          >
            <span v-if="isRefreshing" class="loading loading-sm loading-spinner"></span>
            Refresh
          </button>
        </div>
        <p class="text-sm text-base-content/70">
          Change requests for this contract version. Accept or reject when asked.
        </p>
        <NegotiationList
          v-if="hasActiveNegotiations && props.localInstanceDid"
          :contract="contract"
          :local-instance-did="props.localInstanceDid"
          :participant-id="participantId"
          @responded="handleResponded"
        />
        <p v-else-if="hasActiveNegotiations" class="text-sm text-base-content/60">Loading party identity…</p>
        <p v-else class="text-sm text-base-content/60">No active proposals for this contract.</p>
      </div>
    </section>
  </div>
</template>
