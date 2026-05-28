<template>
  <div
    class="group flex items-start gap-3 rounded-lg border border-base-300 bg-base-200/30 p-3 transition-all hover:shadow-sm"
  >
    <div class="min-w-0 flex-1">
      <div class="text-sm font-semibold text-base-content">
        {{ condition.conditionName }}
        <span class="ml-1 font-normal text-base-content/60">
          (used in {{ usedInClauseCount }} clause{{ usedInClauseCount === 1 ? '' : 's' }})
        </span>
      </div>
      <div class="mt-2 flex flex-wrap gap-2">
        <div v-for="(p, i) in condition.parameters" :key="i" class="badge gap-1 badge-ghost badge-sm">
          <span>{{ p.parameterName }}</span>
          <span class="opacity-70">({{ p.type }}, {{ p.isRequired ? 'required' : 'optional' }})</span>
        </div>
      </div>
    </div>
    <div
      v-if="isEditable"
      class="flex shrink-0 items-center gap-1 opacity-0 transition-opacity group-hover:opacity-100"
    >
      <button
        type="button"
        class="btn btn-ghost btn-xs"
        aria-label="Edit rule"
        @click="$emit('edit-rule', condition.conditionId)"
      >
        <IconEdit class="h-4 w-4" />
      </button>
      <button
        type="button"
        class="btn text-error btn-ghost btn-xs"
        aria-label="Delete rule"
        @click="$emit('delete-rule', condition.conditionId)"
      >
        <IconRemove class="h-4 w-4" />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { SemanticCondition } from '@template-repository/models/contract-templace'
import IconEdit from '@/core/components/icons/IconEdit.vue'
import IconRemove from '@/core/components/icons/IconRemove.vue'

defineProps<{
  condition: SemanticCondition
  usedInClauseCount: number
  isEditable: boolean
}>()

defineEmits<{
  'edit-rule': [conditionId: string]
  'delete-rule': [conditionId: string]
}>()
</script>
