<script setup lang="ts">
import type { Contract } from '@/models/contract/contract'
import { ROUTES } from '@/router/router'
import { useAuthStore } from '@/stores/auth-store'
import { useContractsStore } from '@/stores/contracts-store'
import { ContractState } from '@/types/contract-state'
import { computed } from 'vue'

const props = defineProps<{
  contract: Contract
}>()

const authStore = useAuthStore()
const contractsStore = useContractsStore()

const canEdit = computed(() => {
  return props.contract.created_by === authStore.user?.username && props.contract.state === ContractState.draft
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
</script>

<template>
  <li class="list-row w-full min-w-0">
    <div class="list-col-grow card w-full min-w-0 border-base-content/10 bg-base-100 card-border hover:bg-base-300">
      <div class="card-body min-w-0">
        <h2 class="card-title flex-wrap sm:justify-between">
          <div class="flex gap-8 sm:h-full">
            <div>Name: {{ contract.name }}</div>
          </div>
          <div class="badge badge-secondary">{{ contract.state }}</div>
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
            <RouterLink
              :to="{ name: resolveViewRouteName, params: { did: contract.did } }"
              class="btn btn-sm btn-primary"
            >
              View
            </RouterLink>
            <RouterLink
              :to="
                canEdit
                  ? {
                      name: ROUTES.CONTRACTS.EDIT,
                      params: { did: contract.did },
                    }
                  : '#'
              "
              class="btn gap-2 btn-sm btn-primary"
              :class="{ 'btn-disabled': !canEdit }"
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
