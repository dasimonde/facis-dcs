<template>
  <div class="flex flex-col min-h-full -mx-4 md:-mx-8 -my-4 md:-my-8">
    <div class="sticky top-0 z-10 shrink-0 bg-base-200 border-b border-base-300">
      <div class="max-w-4xl mx-auto px-6 pt-3">
        <p class="text-xs font-black uppercase tracking-widest text-base-content/40 mb-2">
          Create Negotiation
        </p>
        <!-- tabs -->
        <div role="tablist" class="tabs tabs-lift tabs-lg">
          <a role="tab" class="tab" :class="{ 'tab-active': activeTab === 'parties' }" @click="activeTab = 'parties'">
            Parties
          </a>
          <a role="tab" class="tab" :class="{ 'tab-active': activeTab === 'contractFilling' }"
            @click="activeTab = 'contractFilling'"> Contract Filling </a>
          <a role="tab" class="tab" :class="{ 'tab-active': activeTab === 'preview' }" @click="activeTab = 'preview'">
            Preview
          </a>
        </div>
      </div>
    </div>

    <div class="grow mt-5">
      <div class="max-w-4xl mx-auto p-6">
        <section v-if="activeTab === 'parties'" class="space-y-4">
          <div v-if="initiatorLoading || loading" class="py-4">Loading...</div>
          <div v-else-if="initiatorError || error" class="alert flex items-start justify-between gap-4">
            <div class="text-sm">Unable to load parties information right now.</div>
            <button type="button" class="btn btn-sm rounded-box self-start" @click="retryLoadParties">Retry</button>
          </div>
          <NegotiationPartiesTabView v-else :initiator="initiator" :participants="participants"
            :selected-responder-indexes="selectedResponderIndexes" :selected-responders="selectedResponders"
            @toggle-participant="toggleParticipant" />
        </section>

        <section v-else-if="activeTab === 'contractFilling'" class="rounded-xl border border-base-300 p-4">
          <div class="text-sm text-base-content/70">TBD</div>
        </section>

        <section v-else class="rounded-xl border border-base-300 p-4">
          <div class="text-sm text-base-content/70">TBD</div>
        </section>
      </div>
    </div>

    <div class="sticky bottom-0 shrink-0 border-t border-base-300 bg-base-100">
      <div class="max-w-4xl mx-auto px-6 py-3 flex flex-col md:flex-row gap-3">
        <button class="btn btn-ghost md:w-32" @click="router.back()">Back</button>
        <button class="btn btn-primary flex-1" disabled>Create</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import NegotiationPartiesTabView from '@/modules/template-catalogue/components/negotiation/NegotiationPartiesTabView.vue'
import type { Participant } from '@/modules/template-catalogue/models/participant'
import { templateCatalogueIntegrationService } from '@/services/template-catalogue-integration-service'
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'

const router = useRouter()
const activeTab = ref<'parties' | 'contractFilling' | 'preview'>('parties')

const initiatorLoading = ref(false)
const initiatorError = ref<string | null>(null)
const initiator = ref<Participant | null>(null)

const loading = ref(false)
const error = ref<string | null>(null)
const participants = ref<Participant[]>([])
const selectedResponderIndexes = ref<number[]>([])

const selectedResponders = computed(() =>
  selectedResponderIndexes.value
    .map((index) => ({ index, participant: participants.value[index] }))
    .filter((entry): entry is { index: number; participant: Participant } => !!entry.participant),
)

function retryLoadParties() {
  loadInitiator()
  loadParticipants()
}

function toggleParticipant(index: number) {
  if (selectedResponderIndexes.value.includes(index)) {
    selectedResponderIndexes.value = selectedResponderIndexes.value.filter((item) => item !== index)
    return
  }
  selectedResponderIndexes.value = [...selectedResponderIndexes.value, index]
}

async function loadParticipants() {
  loading.value = true
  error.value = null
  try {
    participants.value = await templateCatalogueIntegrationService.get_other_participants()
  } catch (e: any) {
    error.value = e?.message || 'Error loading participants'
    participants.value = []
  } finally {
    loading.value = false
  }
}

async function loadInitiator() {
  initiatorLoading.value = true
  initiatorError.value = null
  try {
    initiator.value = await templateCatalogueIntegrationService.get_current_participant_summary()
  } catch (e: any) {
    initiatorError.value = e?.message || 'Error loading initiator'
    initiator.value = null
  } finally {
    initiatorLoading.value = false
  }
}

loadInitiator()
loadParticipants()
</script>
