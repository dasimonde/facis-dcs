<script setup lang="ts">
import { computed, ref, useId } from 'vue'
import { setSigningDeadline, setSigningOrder, signatureFieldNames, signingWorkflow } from '@/utils/signing-workflow'
import type { Contract } from '@/models/contract/contract'

const props = defineProps<{
  disabled?: boolean
  nameError?: string
  descriptionError?: string
}>()

const contract = defineModel<Contract>('contract', { required: true })
const inserted = defineModel<ContractDetailData>('inserted', { required: false })

function getTemplateLink(contract: Contract): string {
  return `/ui/templates/view/${contract.template_did}`
}

const expirationPolicies = [
  { name: 'Renewal', value: 'RENEWAL' },
  { name: 'Archiving', value: 'ARCHIVING' },
  { name: 'Termination', value: 'TERMINATION' },
]

interface ContractDetailData {
  name?: string
  description?: string
  start_date?: string
  exp_date?: string
  exp_notice_period?: string
  exp_policy?: string
}

const nameId = useId()
const descriptionId = useId()
const nameErrorId = useId()
const descriptionErrorId = useId()
const startDateId = useId()
const expDateId = useId()
const exp_notice_periodId = useId()
const exp_policyId = useId()

const originalContract = ref(Object.assign({}, contract.value))

const minSelectableDate = computed(() => {
  const d = new Date()
  d.setUTCHours(0, 0, 0, 0)
  d.setUTCDate(d.getUTCDate() + 1)
  return d.toISOString().slice(0, 10)
})

function toDateInputValue(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return d.toISOString().slice(0, 10)
}

function fromDateInputValue(dateOnly: string): string | undefined {
  if (!dateOnly) return undefined
  return `${dateOnly}T00:00:00Z`
}

const startDateInput = computed({
  get: () => toDateInputValue(inserted.value?.start_date ?? contract.value.start_date),
  set: (value: string) => {
    if (inserted.value) return
    contract.value.start_date = fromDateInputValue(value)
  },
})

const expDateInput = computed({
  get: () => toDateInputValue(inserted.value?.exp_date ?? contract.value.exp_date),
  set: (value: string) => {
    if (inserted.value) return
    contract.value.exp_date = fromDateInputValue(value)
  },
})

// DCS-FR-SM-13: the signing order and deadline live in the contract document,
// where the signing workflow reads and enforces them.
const signingDeadlineId = useId()
const signatureFields = computed(() => signatureFieldNames(contract.value.contract_data))
const workflow = computed(() => signingWorkflow(contract.value.contract_data))

function signingPosition(field: string): number | undefined {
  return workflow.value.order.find((entry) => entry.field === field)?.position
}

function onSigningPosition(field: string, event: Event) {
  if (!contract.value.contract_data) return
  const value = (event.target as HTMLInputElement).value
  setSigningOrder(contract.value.contract_data, field, value === '' ? undefined : Number(value))
}

const signingDeadlineInput = computed({
  get: () => workflow.value.deadline?.toISOString().slice(0, 10) ?? '',
  set: (value: string) => {
    if (contract.value.contract_data) setSigningDeadline(contract.value.contract_data, value)
  },
})
</script>

<template>
  <div class="card border border-base-300 bg-base-100 shadow-sm">
    <div class="card-body gap-5">
      <h2 class="card-title justify-between text-sm">
        <div class="flex gap-2">Contract Details</div>
        <div data-testid="contract-state-badge" class="badge badge-sm badge-secondary">{{ contract.state }}</div>
      </h2>

      <fieldset class="fieldset border-none p-0">
        <legend class="fieldset-legend">Version: {{ contract.contract_version }}</legend>
      </fieldset>

      <fieldset class="fieldset border-none p-0">
        <legend class="fieldset-legend">Base Template</legend>
        <a class="badge badge-sm badge-primary" :href="getTemplateLink(contract)">{{ contract.template_did }}</a>
      </fieldset>

      <fieldset class="fieldset border-none p-0">
        <legend class="fieldset-legend">Global Name</legend>
        <label :for="nameId" class="sr-only">Global Name</label>
        <input
          v-if="!inserted?.name"
          :id="nameId"
          v-model="contract.name"
          class="input-bordered input w-full"
          data-testid="contract-global-name"
          :class="{ 'border-2 input-primary': !!inserted && originalContract.name !== contract.name }"
          type="text"
          :disabled="disabled"
          required
          :aria-invalid="!!props.nameError"
          :aria-describedby="props.nameError ? nameErrorId : undefined"
        />
        <input
          v-else
          :id="nameId"
          v-model="inserted.name"
          class="input-bordered input w-full"
          :class="{ 'text-red-400': inserted.name !== contract.name }"
          type="text"
          disabled
        />
        <p v-if="props.nameError && !inserted?.name" :id="nameErrorId" class="text-sm text-error">
          {{ props.nameError }}
        </p>
      </fieldset>

      <fieldset class="fieldset border-none p-0">
        <legend class="fieldset-legend">Base Description</legend>
        <label :for="descriptionId" class="sr-only">Base Description</label>
        <textarea
          v-if="!inserted?.description"
          :id="descriptionId"
          v-model="contract.description"
          class="textarea-bordered textarea h-24 w-full"
          data-testid="contract-base-description"
          :class="{ 'border-2 textarea-primary': originalContract.description !== contract.description }"
          :disabled="disabled"
          required
          :aria-invalid="!!props.descriptionError"
          :aria-describedby="props.descriptionError ? descriptionErrorId : undefined"
        />
        <textarea
          v-else
          :id="descriptionId"
          v-model="inserted.description"
          class="textarea-bordered textarea h-24 w-full"
          :class="{ 'text-red-400': !!inserted && inserted.description !== contract.description }"
          disabled
        />
        <p v-if="props.descriptionError && !inserted?.description" :id="descriptionErrorId" class="text-sm text-error">
          {{ props.descriptionError }}
        </p>
      </fieldset>

      <fieldset class="fieldset border-none p-0">
        <legend class="fieldset-legend">Start date</legend>
        <label :for="startDateId" class="sr-only">Start date</label>
        <input
          v-if="!inserted?.start_date"
          :id="startDateId"
          v-model="startDateInput"
          type="date"
          class="input-bordered input w-full"
          data-testid="contract-start-date"
          :min="minSelectableDate"
          :disabled="disabled"
          :class="{ 'border-2 input-primary': !!inserted && originalContract.start_date !== contract.start_date }"
        />
        <input
          v-else
          :id="startDateId"
          :value="toDateInputValue(inserted.start_date)"
          type="date"
          class="input-bordered input w-full"
          :class="{ 'text-red-400': inserted.start_date !== contract.start_date }"
          disabled
        />
      </fieldset>

      <fieldset class="fieldset border-none p-0">
        <legend class="fieldset-legend">Expiration date</legend>
        <label :for="expDateId" class="sr-only">Expiration date</label>
        <input
          v-if="!inserted?.exp_date"
          :id="expDateId"
          v-model="expDateInput"
          type="date"
          class="input-bordered input w-full"
          data-testid="contract-exp-date"
          :min="minSelectableDate"
          :disabled="disabled"
          :class="{ 'border-2 input-primary': !!inserted && originalContract.exp_date !== contract.exp_date }"
        />
        <input
          v-else
          :id="expDateId"
          :value="toDateInputValue(inserted.exp_date)"
          type="date"
          class="input-bordered input w-full"
          :class="{ 'text-red-400': inserted.exp_date !== contract.exp_date }"
          disabled
        />
      </fieldset>

      <fieldset class="fieldset border-none p-0">
        <legend class="fieldset-legend">Expiration Notice Period (in days)</legend>
        <label :for="exp_notice_periodId" class="sr-only">Expiration Notice Period (in days)</label>
        <input
          v-if="!inserted?.exp_notice_period"
          :id="exp_notice_periodId"
          v-model="contract.exp_notice_period"
          type="number"
          min="0"
          class="input w-full"
          :disabled="disabled"
        />
        <input
          v-else
          :id="exp_notice_periodId"
          v-model="inserted.exp_notice_period"
          type="text"
          class="input-bordered input w-full"
          :class="{ 'text-red-400': inserted.exp_notice_period !== contract.exp_notice_period?.toString() }"
          disabled
        />
      </fieldset>
      <fieldset class="fieldset border-none p-0">
        <legend class="fieldset-legend">Expiration Policy</legend>
        <label :for="exp_policyId" class="sr-only">Expiration Policy</label>
        <select
          v-if="!inserted?.exp_policy"
          :id="exp_policyId"
          v-model="contract.exp_policy"
          class="select w-full"
          :disabled="disabled"
        >
          <option v-for="policy in expirationPolicies" :key="policy.value" :value="policy.value">
            {{ policy.name }}
          </option>
        </select>
        <input
          v-else
          :id="exp_policyId"
          v-model="inserted.exp_policy"
          type="text"
          class="input-bordered input w-full"
          :class="{ 'text-red-400': inserted.exp_policy !== contract.exp_policy }"
          disabled
        />
      </fieldset>

      <template v-if="!inserted && contract.contract_data">
        <fieldset class="fieldset border-none p-0">
          <legend class="fieldset-legend">Signing deadline</legend>
          <label :for="signingDeadlineId" class="sr-only">Signing deadline</label>
          <input
            :id="signingDeadlineId"
            v-model="signingDeadlineInput"
            type="date"
            class="input-bordered input w-full"
            data-testid="contract-signing-deadline"
            :disabled="disabled"
          />
          <p class="label">No signature is accepted after this day (UTC).</p>
        </fieldset>

        <fieldset v-if="signatureFields.length > 0" class="fieldset border-none p-0">
          <legend class="fieldset-legend">Signing order</legend>
          <p class="label">Lower positions sign first. Leave a position empty to sign at any time.</p>
          <label v-for="field in signatureFields" :key="field" class="flex items-center gap-2">
            <input
              :value="signingPosition(field) ?? ''"
              type="number"
              min="1"
              step="1"
              class="input-bordered input w-24"
              :aria-label="`Signing position of ${field}`"
              :data-testid="`contract-signing-order-${field}`"
              :disabled="disabled"
              @change="onSigningPosition(field, $event)"
            />
            <span class="font-mono text-xs break-all">{{ field }}</span>
          </label>
        </fieldset>
      </template>
    </div>
  </div>
</template>
