<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import DiffView from '@contract-workflow-engine/components/DiffView.vue'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { ContractState, type ContractState as ContractStateType } from '@/types/contract-state'
import type { ContractData } from '@/models/contract/contract-data'
import type { ContractHistoryItem } from '@/models/responses/contract-response'
import type { ContractDetailsSnapshot } from '@contract-workflow-engine/models/contract-details-snapshot'

const DRAFT_ID = 'draft'
const FORMAL_ID = 'formal'

const props = defineProps<{
  contractDid: string
  contractState: ContractStateType
  currentContractData?: ContractData
  currentDetails?: ContractDetailsSnapshot | null
  /** Overrides the default "Current draft" / "Current contract" label (e.g. "Accepted changes"). */
  currentOptionLabel?: string
  /** Stored formal contract before accepted proposals are merged by Submit. */
  formalContractData?: ContractData
  formalDetails?: ContractDetailsSnapshot | null
  formalOptionLabel?: string
  /** Offers to restore the left history version as a new version (DCS-FR-CWE-08). */
  restorable?: boolean
}>()

const emit = defineEmits<{
  restore: [item: ContractHistoryItem]
}>()

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
  kind: 'draft' | 'formal' | 'history'
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

const showCurrent = computed(() => !!props.currentContractData)
const showFormal = computed(() => !!props.formalContractData || !!props.formalDetails)

const defaultCurrentOptionLabel = computed(() =>
  props.contractState === ContractState.draft ||
  props.contractState === ContractState.rejected ||
  props.contractState === ContractState.negotiation
    ? 'Current draft'
    : 'Current contract',
)

const currentOptionLabel = computed(() => props.currentOptionLabel ?? defaultCurrentOptionLabel.value)

const formalOptionLabel = computed(() => props.formalOptionLabel ?? 'Formal contract')

const compareOptions = computed<CompareOption[]>(() => {
  const opts: CompareOption[] = []

  if (showFormal.value) {
    opts.push({
      id: FORMAL_ID,
      kind: 'formal',
      label: formalOptionLabel.value,
    })
  }

  if (showCurrent.value) {
    opts.push({
      id: DRAFT_ID,
      kind: 'draft',
      label: currentOptionLabel.value,
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

// Several saves can snapshot one contract version, so the entry id names the option.
function historyOptionId(item: ContractHistoryItem): string {
  return `h:${item.id}`
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
  const label = when ? `${when} (version ${item.contract_version})` : `(version ${item.contract_version})`
  if (!item.modified_by) return label
  const modifiedAt = formatDateTime(item.modified_at)
  return modifiedAt
    ? `${label}, replaced by ${item.modified_by} at ${modifiedAt}`
    : `${label}, replaced by ${item.modified_by}`
}

function resolveData(id: string): ContractData | undefined {
  if (id === DRAFT_ID) return props.currentContractData
  if (id === FORMAL_ID) return props.formalContractData
  const row = sortedHistory.value.find((item) => historyOptionId(item) === id)
  return row?.contract_data
}

function snapshotFromHistory(item: ContractHistoryItem): ContractDetailsSnapshot {
  return {
    name: item.name,
    description: item.description,
    contract_version: item.contract_version,
    start_date: item.start_date,
    exp_date: item.exp_date,
    exp_policy: item.exp_policy,
    exp_notice_period: item.exp_notice_period,
  }
}

function resolveDetails(id: string): ContractDetailsSnapshot | null {
  if (id === DRAFT_ID) return props.currentDetails ?? null
  if (id === FORMAL_ID) return props.formalDetails ?? null
  const row = sortedHistory.value.find((item) => historyOptionId(item) === id)
  return row ? snapshotFromHistory(row) : null
}

const leftHistoryItem = computed(() => sortedHistory.value.find((item) => historyOptionId(item) === leftPick.value))

const leftContractData = computed((): ContractData | undefined => resolveData(leftPick.value))
const rightContractData = computed((): ContractData | undefined => resolveData(rightPick.value))
const leftDetails = computed(() => resolveDetails(leftPick.value))
const rightDetails = computed(() => resolveDetails(rightPick.value))
const leftOptionLabel = computed(() => compareOptions.value.find((o) => o.id === leftPick.value)?.label ?? 'Left')
const rightOptionLabel = computed(() => compareOptions.value.find((o) => o.id === rightPick.value)?.label ?? 'Right')

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
    const formal = opts.find((o) => o.id === FORMAL_ID)
    const draft = opts.find((o) => o.id === DRAFT_ID)
    const firstHistory = opts.find((o) => o.kind === 'history')
    // Prefer Formal → Accepted changes when accepted proposals are pending Submit.
    if (formal && draft) {
      leftPick.value = formal.id
      rightPick.value = draft.id
    } else if (draft && firstHistory) {
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

watch([historyItems, showCurrent, showFormal], () => normalizePicksAfterHistoryChange(), { immediate: true })

watch([leftPick, rightPick], () => ensureDistinctPicks())
</script>

<template>
  <div class="space-y-4">
    <!-- tool bar -->
    <div class="rounded-lg border border-base-300 bg-base-100 p-4 shadow-sm">
      <p class="mb-1 text-sm font-medium text-base-content">Comparing changes</p>
      <p class="mb-3 text-xs text-base-content/70">Choose two versions to see what's changed.</p>
      <div class="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
        <label class="form-control w-full md:flex-1">
          <span class="label-text text-xs text-base-content/70">Left</span>
          <select
            id="diff-left-pick"
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
            id="diff-right-pick"
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

      <div v-if="restorable && leftHistoryItem" class="mt-4 border-t border-base-300 pt-4">
        <button
          type="button"
          class="btn btn-sm btn-secondary"
          data-testid="restore-history-version"
          @click="emit('restore', leftHistoryItem)"
        >
          Restore left version
        </button>
      </div>

      <p v-if="loading" class="mt-3 text-sm text-base-content/70">Loading history…</p>
      <p v-else-if="loadError" class="mt-3 text-sm text-error">{{ loadError }}</p>
      <p v-else-if="compareOptions.length < 2" class="mt-3 text-sm text-base-content/70">
        Add at least one saved history entry to compare versions.
      </p>
    </div>

    <DiffView
      :left-contract-data="leftContractData"
      :right-contract-data="rightContractData"
      :left-details="leftDetails"
      :right-details="rightDetails"
      :show-line-numbers="showLineNumbers"
      :highlight-diff="highlightDiff"
      :left-pane-title="leftOptionLabel"
      :right-pane-title="rightOptionLabel"
    />
  </div>
</template>
