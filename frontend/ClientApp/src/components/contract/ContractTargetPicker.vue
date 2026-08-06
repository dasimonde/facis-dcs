<script setup lang="ts">
import { computed, onMounted, ref, useId, watch } from 'vue'
import { useContractPermissions } from '@contract-workflow-engine/composables/useContractPermissions'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { ContractState } from '@/types/contract-state'
import type { Contract } from '@/models/contract/contract'
import type { ContractTarget } from '@/models/responses/contract-response'

/**
 * Where this contract deploys to (ADR-25). Signing completion deploys the
 * contract automatically (DCS-FR-SM-12) with nobody present to choose a
 * destination, so the choice belongs to the contract and has to be made before
 * then — a contract that reaches signing without one does not deploy.
 */

const props = defineProps<{ contract: Contract }>()
const emit = defineEmits<{ designated: [] }>()

const { isManager } = useContractPermissions()

const targets = ref<ContractTarget[]>([])
const selected = ref(props.contract.target_id ?? '')
const saving = ref(false)
const error = ref('')

const selectedId = useId()
const selectedLabelId = useId()

// Terminal states aside, the designation stays editable while the contract is
// not yet in force: a target can be corrected right up to signing, and after
// activation a re-dispatch elsewhere is the manual Redeploy control's job.
const editable = computed(
  () =>
    isManager.value &&
    props.contract.state !== ContractState.terminated &&
    props.contract.state !== ContractState.expired &&
    props.contract.state !== ContractState.revoked,
)

const load = async () => {
  try {
    targets.value = (await contractWorkflowService.listTargets()).filter(
      // A disabled target cannot receive deployments, so offering it would
      // produce a contract that deploys nowhere and only says so at signing.
      // One already designated stays listed so the destination reads correctly.
      (target) => target.enabled || target.id === props.contract.target_id,
    )
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Could not load target systems'
  }
}

// The registry read is scoped to the manager roles (design listContractTargets).
// Every other role the contract routes admit reads the designation off the
// contract itself, so fetching for them only paints a permission error.
onMounted(() => {
  if (editable.value) void load()
})
watch(
  () => props.contract.target_id,
  (id) => (selected.value = id ?? ''),
)

const designate = async () => {
  saving.value = true
  error.value = ''
  try {
    await contractWorkflowService.designateTarget({
      did: props.contract.did,
      updated_at: props.contract.updated_at,
      target_id: selected.value || undefined,
    })
    emit('designated')
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Could not set the target system'
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div data-testid="contract-target-picker" class="flex flex-col gap-2">
    <h2 :id="selectedLabelId" class="text-lg font-semibold">Deployment target</h2>
    <p v-if="!contract.target_id" data-testid="contract-target-unset" class="text-sm opacity-70">
      No target system is set. The contract will not deploy when signing completes until one is chosen.
    </p>
    <p v-else data-testid="contract-target-name" class="text-sm opacity-70">
      Deploys to
      <span class="font-medium">{{ contract.target_name ?? contract.target_id }}</span>
    </p>

    <div v-if="editable" class="flex flex-wrap items-center gap-2">
      <select
        :id="selectedId"
        v-model="selected"
        data-testid="contract-target-select"
        :aria-labelledby="selectedLabelId"
        class="select-bordered select select-sm"
      >
        <option value="">None</option>
        <option v-for="target in targets" :key="target.id" :value="target.id">{{ target.name }}</option>
      </select>
      <button
        data-testid="contract-target-save"
        class="btn btn-sm btn-primary"
        :disabled="saving || selected === (contract.target_id ?? '')"
        @click="designate"
      >
        {{ saving ? 'Saving…' : 'Set target' }}
      </button>
    </div>

    <div v-if="error" data-testid="contract-target-error" class="alert rounded-box alert-error">{{ error }}</div>
  </div>
</template>
