<script setup lang="ts">
import { computed } from 'vue'
import { type Contract, ExpirationPolicy } from '@/models/contract/contract'
import { ContractState } from '@/types/contract-state'

const props = defineProps<{
  contract: Contract
}>()

// DCS-FR-CWE-10: expiry triggers the workflow the configured policy names. The
// Renew and Terminate actions accept an EXPIRED contract; archiving happened at
// signing (DCS-FR-CSA-08).
const nextStep: Record<ExpirationPolicy, string> = {
  [ExpirationPolicy.renewal]: 'Its expiration policy is renewal: renew it to continue the agreement.',
  [ExpirationPolicy.termination]: 'Its expiration policy is termination: terminate it to close the agreement formally.',
  [ExpirationPolicy.archiving]: 'Its expiration policy is archiving: the signed contract stays in the archive.',
}

const message = computed(() => {
  const policy = props.contract.exp_policy
  if (props.contract.state !== ContractState.expired || !policy || !(policy in nextStep)) return ''
  return `This contract has expired. ${nextStep[policy]}`
})
</script>

<template>
  <div v-if="message" class="alert text-sm alert-warning" role="status" data-testid="expired-policy-notice">
    {{ message }}
  </div>
</template>
