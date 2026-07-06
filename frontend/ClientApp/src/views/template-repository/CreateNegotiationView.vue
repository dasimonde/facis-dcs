<script setup lang="ts">
import NegotiationPartiesTabView from '@/modules/template-catalogue/components/negotiation/NegotiationPartiesTabView.vue'
import type { Participant } from '@/modules/template-catalogue/models/participant'
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'

const router = useRouter()
const activeTab = ref<'parties' | 'contractFilling' | 'preview'>('parties')

const initiator = ref<Participant | null>(null)
const participants = ref<Participant[]>([])
const selectedResponderIndexes = ref<number[]>([])

const selectedResponders = computed(() =>
  selectedResponderIndexes.value
    .map((index) => ({ index, participant: participants.value[index] }))
    .filter((entry): entry is { index: number; participant: Participant } => !!entry.participant),
)

function toggleParticipant(index: number) {
  if (selectedResponderIndexes.value.includes(index)) {
    selectedResponderIndexes.value = selectedResponderIndexes.value.filter((item) => item !== index)
    return
  }
  selectedResponderIndexes.value = [...selectedResponderIndexes.value, index]
}
</script>

<template>
  <div class="-mx-4 -my-4 flex min-h-full flex-col md:-mx-8 md:-my-8">
    <div class="sticky top-0 z-10 shrink-0 border-b border-base-300 bg-base-200">
      <div class="mx-auto max-w-4xl px-6 pt-3">
        <p class="mb-2 text-xs font-black tracking-widest text-base-content/40 uppercase">Create Negotiation</p>
        <!-- tabs -->
        <div role="tablist" class="tabs-lift tabs tabs-lg">
          <a role="tab" class="tab" :class="{ 'tab-active': activeTab === 'parties' }" @click="activeTab = 'parties'">
            Parties
          </a>
          <a
            role="tab"
            class="tab"
            :class="{ 'tab-active': activeTab === 'contractFilling' }"
            @click="activeTab = 'contractFilling'"
          >
            Contract Filling
          </a>
          <a role="tab" class="tab" :class="{ 'tab-active': activeTab === 'preview' }" @click="activeTab = 'preview'">
            Preview
          </a>
        </div>
      </div>
    </div>

    <div class="mt-5 grow">
      <div class="mx-auto max-w-4xl p-6">
        <section v-if="activeTab === 'parties'" class="space-y-4">
          <NegotiationPartiesTabView
            :initiator="initiator"
            :participants="participants"
            :selected-responder-indexes="selectedResponderIndexes"
            :selected-responders="selectedResponders"
            @toggle-participant="toggleParticipant"
          />
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
      <div class="mx-auto flex max-w-4xl flex-col gap-3 px-6 py-3 md:flex-row">
        <button class="btn btn-outline md:w-32" @click="router.back()">Back</button>
        <button class="btn flex-1 btn-primary" disabled>Create</button>
      </div>
    </div>
  </div>
</template>
