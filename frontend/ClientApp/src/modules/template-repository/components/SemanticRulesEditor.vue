<template>
  <div class="space-y-6">
    <!-- Section 1: New rule -->
    <section v-if="uiStore.isTemplateEditable" class="rounded-lg border border-base-300 bg-base-100 p-4 shadow-sm">
      <SemanticRuleForm :existing-conditions="conditions" @add-rule="handleAddRule" />
    </section>

    <!-- Section 2: Existing rules -->
    <section class="rounded-lg border border-base-300 bg-base-100 p-4 shadow-sm">
      <h3 class="mb-4 text-sm font-semibold text-base-content/80">Existing rules</h3>
      <div class="space-y-2">
        <template v-for="rule in conditionItems" :key="rule.condition.conditionId">
          <div
            v-if="editingConditionId === rule.condition.conditionId"
            class="rounded-lg border border-base-300 bg-base-100 p-4 shadow-sm"
          >
            <SemanticRuleForm
              mode="edit"
              :existing-conditions="conditions"
              :initial-condition="rule.condition"
              @update-rule="handleUpdateRule"
              @cancel="stopEdit"
            />
          </div>
          <SemanticRuleItem
            v-else
            :condition="rule.condition"
            :used-in-clause-count="rule.usedInClauseCount"
            :is-editable="uiStore.isTemplateEditable"
            @edit-rule="startEdit"
            @delete-rule="deleteRule"
          />
        </template>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useTemplateDraftStore } from '@template-repository/store/templateDraftStore'
import { type SemanticCondition, isClauseBlock } from '@template-repository/models/contract-templace'
import type { SubTemplateReference } from '@template-repository/models/template-draft-store'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'
import SemanticRuleForm from '@template-repository/components/semantic-rules-editor/SemanticRuleForm.vue'
import SemanticRuleItem from '@template-repository/components/semantic-rules-editor/SemanticRuleItem.vue'

const store = useTemplateDraftStore()
const uiStore = useTemplateEditorUiStore()
const { semanticConditions: mainSemanticConditions, documentBlocks, subTemplateSnapshots } = storeToRefs(store)
const editingConditionId = ref<string | null>(null)

type NewConditionPayload = Omit<SemanticCondition, 'conditionId'>
interface SemanticItem {
  condition: SemanticCondition
  usedInClauseCount: number
  subTemplateRef?: SubTemplateReference
}

const allBlocks = computed(() => {
  const subTemplateBlocks = subTemplateSnapshots.value.flatMap(
    (subTemplate) => subTemplate.template_data?.documentBlocks ?? [],
  )
  return [...documentBlocks.value, ...subTemplateBlocks]
})

/** Number of clause blocks that reference each conditionId. */
const clauseCountByConditionId = computed(() => {
  const counts: Record<string, number> = {}
  for (const block of allBlocks.value) {
    if (!isClauseBlock(block)) continue
    for (const id of block.conditionIds) {
      counts[id] = (counts[id] ?? 0) + 1
    }
  }
  return counts
})

const conditionItems = computed<SemanticItem[]>(() => {
  const mainConditions: SemanticItem[] = mainSemanticConditions.value.map((condition) => ({
    condition,
    usedInClauseCount: clauseCountByConditionId.value[condition.conditionId] ?? 0,
  }))

  // It's empty when the template type is FRAME_CONTRACT
  const subTemplateConditions: SemanticItem[] = subTemplateSnapshots.value.flatMap((template) => {
    const conditions = template.template_data?.semanticConditions ?? []
    return conditions.map((condition) => ({
      condition,
      usedInClauseCount: clauseCountByConditionId.value[condition.conditionId] ?? 0,
      subTemplateRef: {
        did: template.did,
        version: template.version,
        document_number: template.document_number,
      },
    }))
  })

  return [...mainConditions, ...subTemplateConditions]
})

const conditions = computed<SemanticCondition[]>(() => conditionItems.value.map((item) => item.condition))
const editingCondition = computed<SemanticItem | null>(() => {
  if (!editingConditionId.value) return null
  return conditionItems.value.find((item) => item.condition.conditionId === editingConditionId.value) ?? null
})

function handleAddRule(payload: NewConditionPayload) {
  store.addSemanticCondition(payload)
}

function handleUpdateRule(payload: { conditionId: string; data: NewConditionPayload }) {
  const subTemplateRef = editingCondition.value?.subTemplateRef
  store.updateSemanticCondition(payload.conditionId, payload.data, subTemplateRef)
  editingConditionId.value = null
}

function deleteRule(conditionId: string) {
  const condition = conditionItems.value.find((item) => item.condition.conditionId === conditionId)
  if (!condition) return
  if (editingConditionId.value === conditionId) {
    editingConditionId.value = null
  }
  store.deleteSemanticCondition(conditionId, condition.subTemplateRef)
}

function startEdit(conditionId: string) {
  editingConditionId.value = conditionId
}

function stopEdit() {
  editingConditionId.value = null
}
</script>
