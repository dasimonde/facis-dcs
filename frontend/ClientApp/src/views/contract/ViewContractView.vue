<script setup lang="ts">
import SubmitSelectionDialog from '@/components/SubmitSelectionDialog.vue'
import type { Contract } from '@/models/contract/contract'
import type { SelectedUserRole } from '@/models/user'
import { ROUTES } from '@/router/router'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { useAuthStore } from '@/stores/auth-store'
import { ContractState } from '@/types/contract-state'
import { computed, ref, watch, type Ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

const route = useRoute()
const router = useRouter()

const authStore = useAuthStore()

const contract: Ref<Contract | null> = ref(null)

const isCreator = computed(() => {
  return contract.value?.created_by === authStore.user?.username
})

watch(
  () => !!route.params.did,
  async (value) => {
    if (value) {
      try {
        const id = route.params.did
        if (id && !Array.isArray(id)) {
          contract.value = await contractWorkflowService.retrieveById({ did: id })
        }
      } catch (err: any) {
        console.error('Failed to load contract', err)
      }
    }
  },
  { immediate: true },
)

const submitContract = async (result: SelectedUserRole[]) => {
  if (!contract.value) return
  const reviewers = result.filter((user) => user.role === 'CONTRACT_REVIEWER').map((user) => user.user.username)
  const approver = result.find((user) => user.role === 'CONTRACT_APPROVER')?.user.username!
  const negotiators = result.filter((user) => user.role === 'CONTRACT_NEGOTIATOR').map((user) => user.user.username)
  const response = await contractWorkflowService.submit({
    did: contract.value?.did,
    updated_at: contract.value?.updated_at,
    reviewers,
    approver,
    negotiators,
  })
  if (response.did) {
    router.push({ name: ROUTES.CONTRACTS.LIST })
  }
}
</script>

<template>
  <div class="flex flex-col min-h-full -mx-4 md:-mx-8 -my-4 md:-my-8">
    <div v-if="!!contract" class="max-w-4xl mx-auto px-6 py-12">
      <fieldset class="fieldset p-0 border-none">
        <legend class="fieldset-legend">Global Name</legend>
        <input v-model="contract.name" class="input input-bordered w-full" type="text" required disabled />
      </fieldset>

      <fieldset class="fieldset p-0 border-none">
        <legend class="fieldset-legend">Base Description</legend>
        <textarea v-model="contract.description" class="textarea textarea-bordered w-full h-24" required disabled></textarea>
      </fieldset>
    </div>
    <div class="sticky bottom-0 shrink-0 border-t border-base-300 bg-base-100">
      <div class="max-w-4xl mx-auto px-6 py-3 flex flex-col md:flex-row gap-3">
        <button class="btn btn-ghost md:w-32" @click="$router.back()">Cancel</button>
        <SubmitSelectionDialog
          v-if="isCreator && contract?.state === ContractState.draft"
          dialog-type="contract"
          @submit="submitContract"
          class="btn btn-primary flex-1"
        />
      </div>
    </div>
  </div>
</template>
