<script setup lang="ts">
import { computed } from 'vue'
import { useContractPermissions } from '@contract-workflow-engine/composables/useContractPermissions'
import { ROUTES } from '@/router/router'
import { requestContractSync } from '@/services/dcs-to-dcs-service'
import { useContractsStore } from '@/stores/contracts-store'
import { ContractState } from '@/types/contract-state'
import type { Contract } from '@/models/contract/contract'

const props = defineProps<{
  contract: Contract
}>()

const { isCreator } = useContractPermissions()

const contractsStore = useContractsStore()

const canEdit = computed(() => {
  return (
    (props.contract.state === ContractState.draft || props.contract.state === ContractState.rejected) && isCreator.value
  )
})

const hasNegotiationTask = computed(() => contractsStore.hasNegotiationTask(props.contract))
const hasReviewTask = computed(() => contractsStore.hasReviewTask(props.contract))
const hasApprovalTask = computed(() => contractsStore.hasApprovalTask(props.contract))

const resolveViewRouteName = computed(() => {
  if (props.contract.state === ContractState.negotiation && hasNegotiationTask.value) {
    return ROUTES.CONTRACTS.NEGOTIATE
  }
  if (props.contract.state === ContractState.submitted && hasReviewTask.value) {
    return ROUTES.CONTRACTS.REVIEW
  }
  if (props.contract.state === ContractState.reviewed && hasApprovalTask.value) {
    return ROUTES.CONTRACTS.APPROVE
  }
  return ROUTES.CONTRACTS.VIEW
})

interface TimeUntil {
  days: number
  hours: number
  minutes: number
  totalDays: number // for badge logic
}

function timeUntil(date: string | Date | undefined): TimeUntil {
  if (!date) return { days: 0, hours: 0, minutes: 0, totalDays: 0 }

  const diffMs = new Date(date).getTime() - new Date().getTime()

  if (diffMs <= 0) return { days: 0, hours: 0, minutes: 0, totalDays: 0 }

  const days = Math.floor(diffMs / (1000 * 60 * 60 * 24))
  const hours = Math.floor((diffMs % (1000 * 60 * 60 * 24)) / (1000 * 60 * 60))
  const minutes = Math.floor((diffMs % (1000 * 60 * 60)) / (1000 * 60))
  const totalDays = Math.ceil(diffMs / (1000 * 60 * 60 * 24))

  return { days, hours, minutes, totalDays: totalDays }
}

function expirationBadgeClass(timeUtil: TimeUntil, noticePeriod?: number): string {
  if (!noticePeriod) {
    return 'flex'
  }

  if (timeUtil.days > noticePeriod) {
    return 'flex'
  } else if (timeUtil.days > 0) {
    return 'flex badge badge-warning'
  } else {
    return 'flex badge badge-error'
  }
}

function expirationMessage(timeUtil: TimeUntil): string {
  if (timeUtil.days > 0) {
    return `Contract expires in ${timeUtil.days} days`
  } else if (timeUtil.hours > 0) {
    return `Contract expires in ${timeUtil.hours} hours`
  } else {
    return `Contract expires in ${timeUtil.minutes} minutes`
  }
}

function getTemplateLink(contract: Contract): string {
  return `/ui/templates/view/${contract.latest_template_did}`
}

function isTemplateVersionWarningVisible(contract: Contract) {
  if (contract.state === ContractState.terminated || contract.state === ContractState.signed) {
    return false
  }
  if (contract?.latest_template_did === null || contract?.latest_template_did === undefined) {
    return false
  }
  return (
    contract.template_is_deprecated == false &&
    contract?.latest_template_did !== null &&
    contract.template_did !== contract.latest_template_did
  )
}

function isTemplateVersionErrorVisible(contract: Contract) {
  if (contract.state === ContractState.terminated || contract.state === ContractState.signed) {
    return false
  }
  return contract.template_is_deprecated
}

async function onRequestSync(contract: Contract) {
  try {
    const resp = await requestContractSync(contract.did)
    console.log(resp)
  } catch (err) {
    console.error(err)
  }
}
</script>

<template>
  <li class="list-row w-full min-w-0">
    <div class="list-col-grow card w-full min-w-0 border-base-content/10 bg-base-100 card-border hover:bg-base-300">
      <div class="card-body min-w-0">
        <div v-if="isTemplateVersionErrorVisible(contract)" class="-mt-9 flex w-full justify-center">
          <a
            class="badge justify-self-center badge-md badge-error max-sm:h-fit max-sm:link"
            :href="getTemplateLink(contract)"
          >
            This contract uses a deprecated template
          </a>
        </div>

        <div v-if="isTemplateVersionWarningVisible(contract)" class="-mt-9 flex w-full justify-center">
          <a
            class="badge justify-self-center badge-md badge-warning max-sm:h-fit max-sm:link"
            :href="getTemplateLink(contract)"
          >
            A newer template version is available
          </a>
        </div>

        <h2 class="card-title justify-between">
          <div class="flex min-w-0 flex-1 items-center gap-2">
            <div class="truncate">Name: {{ contract.name }}</div>
          </div>
          <div class="ml-10 badge shrink-0 badge-secondary">{{ contract.state }}</div>
        </h2>
        <div class="flex justify-start">
          <div v-if="contract.contract_version">Version: {{ contract.contract_version }}</div>
        </div>
        <div class="flex min-w-0 justify-between">
          <div>Creation date: {{ new Date(contract.created_at).toLocaleString() }}</div>
          <div v-if="contract.description" class="hidden min-w-0 flex-1 truncate px-10 sm:block">
            {{ contract.description }}
          </div>
          <div class="card-actions justify-end">
            <button class="btn btn-ghost btn-sm" title="Sync" @click="onRequestSync(contract)">
              <svg
                xmlns="http://www.w3.org/2000/svg"
                class="h-4 w-4"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                stroke-width="2"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
                />
              </svg>
            </button>
            <RouterLink
              :to="{ name: resolveViewRouteName, params: { did: contract.did } }"
              class="btn btn-sm btn-primary"
            >
              View
            </RouterLink>
            <RouterLink
              v-if="canEdit"
              :to="{
                name: ROUTES.CONTRACTS.EDIT,
                params: { did: contract.did },
              }"
              class="btn gap-2 btn-sm btn-primary"
            >
              Edit
            </RouterLink>
          </div>
        </div>
        <div v-if="contract?.start_date" class="flex justify-between">
          <div>Start date: {{ new Date(contract?.start_date ?? '').toLocaleString() }}</div>
        </div>
        <div v-if="contract?.exp_date" class="flex justify-between">
          <div>Expiration date: {{ new Date(contract?.exp_date ?? '').toLocaleString() }}</div>
          <div
            v-if="timeUntil(contract?.exp_date).totalDays > 0"
            :class="expirationBadgeClass(timeUntil(contract?.exp_date), contract?.exp_notice_period)"
          >
            {{ expirationMessage(timeUntil(contract?.exp_date)) }}
          </div>
        </div>
      </div>
    </div>
  </li>
</template>
