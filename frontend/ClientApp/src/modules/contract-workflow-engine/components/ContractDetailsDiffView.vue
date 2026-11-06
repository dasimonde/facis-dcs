<script setup lang="ts">
import { computed } from 'vue'
import {
  buildWordDiffSegments,
  type TextDiffSegment,
  type TextDiffSegmentType,
} from '@contract-workflow-engine/composables/useContractBlockDiff'
import type { ContractDetailsSnapshot } from '@contract-workflow-engine/models/contract-details-snapshot'

const props = withDefaults(
  defineProps<{
    left?: ContractDetailsSnapshot | null
    right?: ContractDetailsSnapshot | null
    leftPaneTitle?: string
    rightPaneTitle?: string
    leftTestId?: string
    rightTestId?: string
    highlightDiff?: boolean
  }>(),
  {
    leftPaneTitle: 'Left',
    rightPaneTitle: 'Right',
    leftTestId: 'contract-details-diff-left',
    rightTestId: 'contract-details-diff-right',
    highlightDiff: true,
  },
)

type DetailKey = keyof ContractDetailsSnapshot

const rows: { key: DetailKey; label: string }[] = [
  { key: 'name', label: 'Title' },
  { key: 'contract_version', label: 'Version' },
  { key: 'description', label: 'Description' },
  { key: 'start_date', label: 'Start date' },
  { key: 'exp_date', label: 'Expiration date' },
  { key: 'exp_notice_period', label: 'Notice period' },
  { key: 'exp_policy', label: 'Expiry policy' },
  { key: 'signing_deadline', label: 'Signing deadline' },
  { key: 'signing_order', label: 'Signing order' },
]

function displayValue(snapshot: ContractDetailsSnapshot | null | undefined, key: DetailKey): string {
  const value = snapshot?.[key]
  if (value == null || value === '') return ''
  if (key === 'start_date' || key === 'exp_date') {
    const date = new Date(String(value))
    return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString()
  }
  return String(value)
}

function valuesDiffer(key: DetailKey): boolean {
  if (!props.highlightDiff) return false
  const left = props.left?.[key]
  const right = props.right?.[key]
  return String(left ?? '') !== String(right ?? '')
}

function segmentClass(type: TextDiffSegmentType): string {
  if (type === 'added') return 'bg-green-200/80 text-green-900 rounded-sm'
  if (type === 'removed') return 'bg-red-200/80 text-red-900 rounded-sm'
  return ''
}

function segmentsOrPlain(text: string, segments: TextDiffSegment[] | undefined): TextDiffSegment[] {
  if (segments && segments.length > 0) return segments
  return text ? [{ type: 'equal', text }] : [{ type: 'equal', text: '—' }]
}

const fieldRows = computed(() =>
  rows.map((row) => {
    const left = displayValue(props.left, row.key)
    const right = displayValue(props.right, row.key)
    const changed = valuesDiffer(row.key)
    const wordDiff = changed ? buildWordDiffSegments(left || '—', right || '—') : {}
    return {
      ...row,
      left,
      right,
      changed,
      leftSegments: segmentsOrPlain(left, wordDiff.leftSegments),
      rightSegments: segmentsOrPlain(right, wordDiff.rightSegments),
    }
  }),
)
</script>

<template>
  <div data-testid="contract-details-diff" class="grid grid-cols-1 gap-4 lg:grid-cols-2">
    <article :data-testid="leftTestId" class="card border border-base-300 bg-base-100 shadow-sm">
      <div class="card-body min-w-0 p-4">
        <h3 class="mb-2 text-sm font-semibold text-base-content/70">{{ leftPaneTitle }}</h3>
        <dl
          class="grid min-w-0 grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1 text-base leading-6 text-base-content/90"
        >
          <template v-for="row in fieldRows" :key="`L-${row.key}`">
            <dt class="text-base-content/70">{{ row.label }}</dt>
            <dd class="m-0 break-words whitespace-pre-wrap">
              <span
                v-for="(segment, index) in row.leftSegments"
                :key="`L-${row.key}-${index}`"
                :class="segmentClass(segment.type)"
              >
                {{ segment.text }}
              </span>
            </dd>
          </template>
        </dl>
      </div>
    </article>
    <article :data-testid="rightTestId" class="card border border-base-300 bg-base-100 shadow-sm">
      <div class="card-body min-w-0 p-4">
        <h3 class="mb-2 text-sm font-semibold text-base-content/70">{{ rightPaneTitle }}</h3>
        <dl
          class="grid min-w-0 grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1 text-base leading-6 text-base-content/90"
        >
          <template v-for="row in fieldRows" :key="`R-${row.key}`">
            <dt class="text-base-content/70">{{ row.label }}</dt>
            <dd class="m-0 break-words whitespace-pre-wrap">
              <span
                v-for="(segment, index) in row.rightSegments"
                :key="`R-${row.key}-${index}`"
                :class="segmentClass(segment.type)"
              >
                {{ segment.text }}
              </span>
            </dd>
          </template>
        </dl>
      </div>
    </article>
  </div>
</template>
