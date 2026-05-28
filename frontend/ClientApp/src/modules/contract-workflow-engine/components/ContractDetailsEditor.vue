<template>
  <div class="card border border-base-300 bg-base-100 shadow-sm">
    <div class="card-body gap-5">
      <fieldset class="fieldset border-none p-0">
        <legend class="fieldset-legend">Version: {{ contract.contract_version }}</legend>
      </fieldset>
      <fieldset class="fieldset border-none p-0">
        <legend class="fieldset-legend">Global Name</legend>
        <input
          v-if="!inserted?.name"
          v-model="contract.name"
          class="input-bordered input w-full"
          :class="{ 'border-2 input-primary': !!inserted && originalContract.name !== contract.name }"
          type="text"
          :disabled="disabled"
          required
        />
        <input
          v-else
          v-model="inserted.name"
          class="input-bordered input w-full"
          :class="{ 'text-red-400': inserted.name !== contract.name }"
          type="text"
          disabled
        />
      </fieldset>
      <fieldset class="fieldset border-none p-0">
        <legend class="fieldset-legend">Base Description</legend>
        <textarea
          v-if="!inserted?.description"
          v-model="contract.description"
          class="textarea-bordered textarea h-24 w-full"
          :class="{ 'border-2 textarea-primary': originalContract.description !== contract.description }"
          :disabled="disabled"
          required
        />
        <textarea
          v-else
          v-model="inserted.description"
          class="textarea-bordered textarea h-24 w-full"
          :class="{ 'text-red-400': !!inserted && inserted.description !== contract.description }"
          disabled
        />
      </fieldset>
      <fieldset class="fieldset border-none p-0">
        <legend class="fieldset-legend">Expiration Notice Period (in days)</legend>
        <input
          v-if="!inserted?.exp_notice_period"
          v-model="contract.exp_notice_period"
          type="number"
          min="0"
          class="input w-full"
          :disabled="disabled"
        />
        <input
          v-else
          v-model="inserted.exp_notice_period"
          type="text"
          class="input-bordered input w-full"
          :class="{ 'text-red-400': inserted.exp_notice_period !== contract.exp_notice_period?.toString() }"
          disabled
        />
      </fieldset>
      <fieldset class="fieldset border-none p-0">
        <legend class="fieldset-legend">Expiration Policy</legend>
        <select v-if="!inserted?.exp_policy" v-model="contract.exp_policy" class="select w-full" :disabled="disabled">
          <option v-for="policy in expirationPolicies" :key="policy.value" :value="policy.value">
            {{ policy.name }}
          </option>
        </select>
        <input
          v-else
          v-model="inserted.exp_policy"
          type="text"
          class="input-bordered input w-full"
          :class="{ 'text-red-400': inserted.exp_policy !== contract.exp_policy }"
          disabled
        />
      </fieldset>
      <fieldset v-if="showResponsiblities" class="fieldset border-none p-0">
        <div class="collapse-arrow collapse [&>input~.collapse-title::after]:scale-75">
          <input type="checkbox" name="responsibles" />
          <legend class="collapse-title fieldset-legend pl-0 font-semibold">Responsible Persons</legend>
          <div class="collapse-content grid">
            <ul class="list col-start-1 row-start-1">
              <li class="p-4 pb-2 text-xs tracking-wide opacity-60">Creator</li>
              <li class="list-row py-0">{{ contract.responsible_persons?.creator }}</li>
            </ul>
            <ul class="list col-start-2 row-start-1">
              <li class="p-4 pb-2 text-xs tracking-wide opacity-60">Approvers:</li>
              <li
                v-for="(approver, i) in contract.responsible_persons?.approvers"
                :key="i + approver"
                class="list-row py-0"
              >
                {{ approver }}
              </li>
            </ul>
            <ul class="list col-start-1 row-start-2">
              <li class="p-4 pb-2 text-xs tracking-wide opacity-60">Negotiators:</li>
              <li
                v-for="(negotiator, i) in contract.responsible_persons?.negotiators"
                :key="i + negotiator"
                class="list-row py-0"
              >
                {{ negotiator }}
              </li>
            </ul>
            <ul class="list col-start-2 row-start-2">
              <li class="p-4 pb-2 text-xs tracking-wide opacity-60">Reviewers</li>
              <li
                v-for="(reviewer, i) in contract.responsible_persons?.reviewers"
                :key="i + reviewer"
                class="list-row py-0"
              >
                {{ reviewer }}
              </li>
            </ul>
          </div>
        </div>
      </fieldset>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Contract } from '@/models/contract/contract'
import { ContractState } from '@/types/contract-state'
import { computed, ref } from 'vue'

defineProps<{
  disabled?: boolean
}>()

const contract = defineModel<Contract>('contract', { required: true })
const inserted = defineModel<ContractDetailData>('inserted', { required: false })

const expirationPolicies = [
  { name: 'Renewal', value: 'RENEWAL' },
  { name: 'Archiving', value: 'ARCHIVING' },
  { name: 'Termination', value: 'TERMINATION' },
]

interface ContractDetailData {
  name?: string
  description?: string
  exp_notice_period?: string
  exp_policy?: string
}

const originalContract = ref(Object.assign({}, contract.value))

const showResponsiblities = computed(
  () => !([ContractState.draft, ContractState.terminated] as ContractState[]).includes(contract.value.state),
)
</script>
