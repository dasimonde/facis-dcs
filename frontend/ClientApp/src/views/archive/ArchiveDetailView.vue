<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import TemplatePreview from '@template-repository/components/builder-editor/preview/TemplatePreview.vue'
import { getSemanticConditionsFromTemplateData } from '@template-repository/store/dcsDraftStore'
import { useContractDataPreprocess } from '@contract-workflow-engine/composables/useContractDataPreprocess'
import ContractMachineReadableView from '@/components/contract/ContractMachineReadableView.vue'
import { ROUTES } from '@/router/router'
import { archiveService } from '@/services/archive-service'
import type { Contract } from '@/models/contract/contract'

const route = useRoute()
const contract = ref<Contract | null>(null)
const error = ref('')
const loading = ref(false)
const { preprocessContractData } = useContractDataPreprocess()
const document = computed(() => preprocessContractData(contract.value?.contract_data))
const conditions = computed(() => getSemanticConditionsFromTemplateData(contract.value?.contract_data))
const retry = ref(0)
watch(
  [() => route.query.did, () => route.query.contract_version, retry],
  async ([did, version], _, onCleanup) => {
    let current = true
    onCleanup(() => {
      current = false
    })
    contract.value = null
    error.value = ''
    loading.value = false
    const number = Number(version)
    if (typeof did !== 'string' || !Number.isInteger(number) || number < 1) {
      error.value = 'An archive DID and exact version are required.'
      return
    }
    loading.value = true
    try {
      const snapshot = await archiveService.detail(did, number)
      if (!current) return
      if (
        snapshot.did !== did ||
        snapshot.contract_version !== number ||
        !preprocessContractData(snapshot.contract_data)
      )
        throw new Error('The archive snapshot is incomplete or does not match the requested version.')
      contract.value = snapshot
    } catch (err) {
      if (current) error.value = err instanceof Error ? err.message : 'The archive snapshot could not be loaded.'
    } finally {
      if (current) loading.value = false
    }
  },
  { immediate: true },
)
</script>
<template>
  <div class="mx-auto max-w-5xl space-y-4 p-4">
    <RouterLink :to="{ name: ROUTES.ARCHIVE.DASHBOARD }" class="link">Back to archive</RouterLink>
    <h1 class="text-2xl font-bold">Archived contract</h1>
    <p v-if="loading" role="status">Loading archive snapshot…</p>
    <div v-else-if="error" role="alert" class="alert flex-wrap alert-error">
      {{ error }}
      <button class="btn btn-sm" @click="retry++">Retry</button>
    </div>
    <template v-else-if="contract && document">
      <h2 class="text-xl font-semibold break-words">
        {{ contract.name || contract.did }} — Version {{ contract.contract_version }}
      </h2>
      <p>This is the immutable archived version.</p>
      <dl class="grid gap-2 break-words sm:grid-cols-2">
        <div>
          <dt class="font-semibold">DID</dt>
          <dd class="break-all">{{ contract.did }}</dd>
        </div>
        <div>
          <dt class="font-semibold">State</dt>
          <dd>{{ contract.state }}</dd>
        </div>
        <div>
          <dt class="font-semibold">Valid from</dt>
          <dd>{{ contract.start_date || '—' }}</dd>
        </div>
        <div>
          <dt class="font-semibold">Valid until</dt>
          <dd>{{ contract.exp_date || '—' }}</dd>
        </div>
      </dl>
      <section class="overflow-x-auto rounded-box border border-base-300 p-4" aria-label="Archived contract content">
        <TemplatePreview
          :layout="document.layout"
          :blocks="document.blocks"
          :semantic-conditions="conditions"
          :semantic-condition-values="document.semanticConditionValues"
          disabled
        />
      </section>
      <details class="rounded-box border border-base-300 p-4">
        <summary class="cursor-pointer font-semibold">Archive metadata</summary>
        <dl class="mt-4 grid gap-2 break-words sm:grid-cols-2">
          <div>
            <dt class="font-semibold">Description</dt>
            <dd>{{ contract.description }}</dd>
          </div>
          <div>
            <dt class="font-semibold">Created by</dt>
            <dd>{{ contract.created_by }}</dd>
          </div>
          <div>
            <dt class="font-semibold">Created at</dt>
            <dd>{{ contract.created_at }}</dd>
          </div>
          <div>
            <dt class="font-semibold">Updated at</dt>
            <dd>{{ contract.updated_at }}</dd>
          </div>
          <div>
            <dt class="font-semibold">Expiration policy</dt>
            <dd>{{ contract.exp_policy || '—' }}</dd>
          </div>
          <div>
            <dt class="font-semibold">Notice period</dt>
            <dd>{{ contract.exp_notice_period ?? '—' }}</dd>
          </div>
          <template v-if="contract.responsible">
            <div>
              <dt class="font-semibold">Creator</dt>
              <dd>{{ contract.responsible.creator }}</dd>
            </div>
            <div>
              <dt class="font-semibold">Reviewers</dt>
              <dd>{{ contract.responsible.reviewers?.join(', ') || '—' }}</dd>
            </div>
            <div>
              <dt class="font-semibold">Approvers</dt>
              <dd>{{ contract.responsible.approvers?.join(', ') || '—' }}</dd>
            </div>
            <div>
              <dt class="font-semibold">Negotiators</dt>
              <dd>{{ contract.responsible.negotiators?.join(', ') || '—' }}</dd>
            </div>
          </template>
        </dl>
      </details>
      <ContractMachineReadableView :contract="contract" />
    </template>
  </div>
</template>
