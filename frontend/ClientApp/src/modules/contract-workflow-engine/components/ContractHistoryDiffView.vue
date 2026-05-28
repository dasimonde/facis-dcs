<template>
  <div class="space-y-4">
    <!-- tool bar -->
    <div class="rounded-lg border border-base-300 bg-base-100 p-4 shadow-sm">
      <p class="mb-1 text-sm font-medium text-base-content">Comparing changes</p>
      <p class="mb-3 text-xs text-base-content/50">Choose two versions to see what's changed.</p>
      <div class="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
        <label class="form-control w-full md:flex-1">
          <span class="label-text text-xs text-base-content/70">Left</span>
          <select
            v-model="leftPick"
            class="select-bordered select w-full select-sm"
            :disabled="loading || compareOptions.length < 2"
          >
            <option v-for="opt in compareOptions" :key="`L-${opt.id}`" :value="opt.id" :disabled="opt.id === rightPick">
              {{ opt.label }}
            </option>
          </select>
        </label>
        <label class="form-control w-full md:flex-1">
          <span class="label-text text-xs text-base-content/70">Right</span>
          <select
            v-model="rightPick"
            class="select-bordered select w-full select-sm"
            :disabled="loading || compareOptions.length < 2"
          >
            <option v-for="opt in compareOptions" :key="`R-${opt.id}`" :value="opt.id" :disabled="opt.id === leftPick">
              {{ opt.label }}
            </option>
          </select>
        </label>
      </div>

      <div class="mt-4 flex flex-wrap items-center gap-6 border-t border-base-300 pt-4">
        <label
          for="contract-diff-line-numbers"
          class="flex cursor-pointer items-center gap-3 text-sm text-base-content/80"
        >
          <span class="select-none">Line numbers</span>
          <input id="contract-diff-line-numbers" v-model="showLineNumbers" type="checkbox" class="checkbox mt-1" />
        </label>
        <label
          for="contract-diff-highlight"
          class="flex cursor-pointer items-center gap-3 text-sm text-base-content/80"
        >
          <span class="select-none">Highlight changes</span>
          <input id="contract-diff-highlight" v-model="highlightDiff" type="checkbox" class="checkbox mt-1" />
        </label>
      </div>

      <p v-if="loading" class="mt-3 text-sm text-base-content/60">Loading history…</p>
      <p v-else-if="loadError" class="mt-3 text-sm text-error">{{ loadError }}</p>
      <p v-else-if="compareOptions.length < 2" class="mt-3 text-sm text-base-content/60">
        Add at least one saved history entry to compare versions.
      </p>
    </div>

    <DiffView
      :left-contract-data="leftContractData"
      :right-contract-data="rightContractData"
      :show-line-numbers="showLineNumbers"
      :highlight-diff="highlightDiff"
    />
  </div>
</template>

<script setup lang="ts">
import type { ContractData } from '@/models/contract-data'
import type { ContractHistoryItem } from '@/models/responses/contract-response'
import { useContractDataPreprocess } from '@/modules/contract-workflow-engine/composables/useContractDataPreprocess'
import DiffView from '@/modules/contract-workflow-engine/components/DiffView.vue'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { ContractState, type ContractState as ContractStateType } from '@/types/contract-state'
import { computed, ref, watch } from 'vue'

const DRAFT_ID = 'draft'

const props = defineProps<{
  contractDid: string
  contractState: ContractStateType
  currentContractData?: ContractData
}>()

const { preprocessContractData } = useContractDataPreprocess()

const loading = ref(false)
const loadError = ref('')
const historyItems = ref<ContractHistoryItem[]>([])
const leftPick = ref(DRAFT_ID)
const rightPick = ref(DRAFT_ID)
const showLineNumbers = ref(true)
const highlightDiff = ref(true)

interface CompareOption {
  id: string
  label: string
  kind: 'draft' | 'history'
  historyItem?: ContractHistoryItem
}

const sortedHistory = computed(() => {
  const list = [...historyItems.value]
  list.sort((a, b) => {
    const ta = Date.parse(a.updated_at ?? '') || 0
    const tb = Date.parse(b.updated_at ?? '') || 0
    if (tb !== ta) return tb - ta
    const va = a.contract_version
    const vb = b.contract_version
    return vb - va
  })
  return list
})

const showCurrentDraft = computed(
  () =>
    props.contractState === ContractState.draft ||
    props.contractState === ContractState.rejected ||
    props.contractState === ContractState.negotiation,
)

const compareOptions = computed<CompareOption[]>(() => {
  const opts: CompareOption[] = []

  if (showCurrentDraft.value) {
    opts.push({
      id: DRAFT_ID,
      kind: 'draft',
      label: 'Current draft',
    })
  }

  for (const row of sortedHistory.value) {
    opts.push({
      id: historyOptionId(row),
      kind: 'history',
      label: formatHistoryOptionLabel(row),
      historyItem: row,
    })
  }
  return opts
})

function historyOptionId(item: ContractHistoryItem): string {
  return `h:${item.did}:${item.contract_version}:${item.updated_at ?? ''}`
}

function formatDateTime(iso?: string): string {
  if (!iso) return ''

  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ''

  const pad = (value: number) => String(value).padStart(2, '0')

  return (
    [date.getFullYear(), pad(date.getMonth() + 1), pad(date.getDate())].join('-') +
    ' ' +
    [pad(date.getHours()), pad(date.getMinutes()), pad(date.getSeconds())].join(':')
  )
}

function formatHistoryOptionLabel(item: ContractHistoryItem): string {
  const when = formatDateTime(item.updated_at)
  return when ? `${when} (version ${item.contract_version})` : `(version ${item.contract_version})`
}

function resolveData(id: string): ContractData | undefined {
  if (id === DRAFT_ID) return props.currentContractData
  const row = sortedHistory.value.find((item) => historyOptionId(item) === id)
  const raw = row?.contract_data
  if (!raw) return undefined
  return preprocessContractData(raw)
}

const leftContractData = computed((): ContractData | undefined => resolveData(leftPick.value))
const rightContractData = computed((): ContractData | undefined => resolveData(rightPick.value))

function firstPickExcluding(exclude: string): string {
  const first = compareOptions.value.find((o) => o.id !== exclude)
  return first?.id ?? compareOptions.value[0]?.id ?? DRAFT_ID
}

function ensureDistinctPicks() {
  const opts = compareOptions.value
  if (opts.length < 2) return
  if (leftPick.value === rightPick.value) {
    rightPick.value = firstPickExcluding(leftPick.value)
  }
  if (leftPick.value === rightPick.value) {
    leftPick.value = firstPickExcluding(rightPick.value)
  }
}

watch(
  () => props.contractDid,
  async (did) => {
    loadError.value = ''
    historyItems.value = []
    leftPick.value = DRAFT_ID
    rightPick.value = DRAFT_ID
    if (!did) return
    loading.value = true
    try {
      historyItems.value = await contractWorkflowService.retrieveHistoryByDid({ did })
    } catch {
      loadError.value = 'Could not load contract history.'
      historyItems.value = []
    } finally {
      loading.value = false
    }
  },
  { immediate: true },
)

function normalizePicksAfterHistoryChange() {
  const opts = compareOptions.value
  const optionIds = new Set(opts.map((o) => o.id))

  if (opts.length < 2) {
    leftPick.value = opts[0]?.id ?? DRAFT_ID
    rightPick.value = opts[0]?.id ?? DRAFT_ID
    return
  }

  const picksValid =
    optionIds.has(leftPick.value) && optionIds.has(rightPick.value) && leftPick.value !== rightPick.value

  if (!picksValid) {
    const draft = opts.find((o) => o.id === DRAFT_ID)
    const firstHistory = opts.find((o) => o.kind === 'history')
    if (draft && firstHistory) {
      leftPick.value = firstHistory.id
      rightPick.value = DRAFT_ID
    } else {
      const first = opts[0]
      const second = opts[1]
      if (first && second) {
        leftPick.value = first.id
        rightPick.value = second.id
      }
    }
  }
  ensureDistinctPicks()
}

watch([historyItems, showCurrentDraft], () => normalizePicksAfterHistoryChange(), { immediate: true })

watch([leftPick, rightPick], () => ensureDistinctPicks())
</script>
