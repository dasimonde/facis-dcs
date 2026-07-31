<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { computed, onMounted, ref, useId, watch } from 'vue'
import ClauseSegmentsPreview from '@template-repository/components/clauses-editor/ClauseSegmentsPreview.vue'
import {
  getPlaceholderLabelFromConditions,
  parseSegmentsFromContent,
  type Segment,
} from '@template-repository/composables/useClauseTextChips'
import { TemplateType } from '@template-repository/models/contract-template'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'
import { contractTemplateService } from '@/services/contract-template-service'
import { useContractTemplatesStore } from '@/stores/contract-templates-store'
import { TemplateState } from '@/types/contract-template-state'
import BlockPaletteItem from './document-block/BlockPaletteItem.vue'
import type { ContractTemplate } from '@/models/contract-template/contract-template'
import type { DcsClause } from '@/models/dcs-jsonld'
import type { NewBlockType } from '@template-repository/models/template-draft-store'

const draftStore = useDcsDraftStore()
const uiStore = useTemplateEditorUiStore()
const templatesStore = useContractTemplatesStore()
const { addBlockModalContext, pendingPlacementClauseBlockId } = storeToRefs(uiStore)
const { blocks, semanticConditions } = storeToRefs(draftStore)
const { contractTemplates: allTemplates } = storeToRefs(templatesStore)

const isContractWorkflow = computed(() => uiStore.workflow === 'contract')
const paletteBlockTypes: { blockType: NewBlockType; label: string }[] = [
  { blockType: 'dcs:Section', label: 'Section' },
  { blockType: 'dcs:TextBlock', label: 'Text' },
  { blockType: 'dcs:Clause', label: 'Clause' },
]

const isContractTemplate = computed(() => draftStore.templateType === TemplateType.contractTemplate)
const showComponentPicker = computed(() => !isContractWorkflow.value && isContractTemplate.value)

onMounted(() => {
  if (showComponentPicker.value) void templatesStore.loadTemplates()
})

const componentSearch = ref('')
const componentTemplates = computed((): ContractTemplate[] => {
  const query = componentSearch.value.trim().toLowerCase()
  const selectableStates = new Set<string>([TemplateState.approved, TemplateState.published])
  return allTemplates.value.filter(
    (t) =>
      selectableStates.has(t.state) &&
      t.template_type === TemplateType.component &&
      (query === '' || (t.name ?? '').toLowerCase().includes(query) || t.did.toLowerCase().includes(query)),
  )
})

const unusedClauses = computed((): DcsClause[] => {
  const inOutline = draftStore.blockIdsInOutline
  const clauses = blocks.value.filter((b): b is DcsClause => b['@type'] === 'dcs:Clause')
  const unused = clauses.filter((c) => !inOutline.has(c['@id']))
  return [...unused].sort((a, b) => (a['dcs:title'] ?? '').localeCompare(b['dcs:title'] ?? ''))
})

const pendingPlacementClause = computed(() =>
  unusedClauses.value.find((clause) => clause['@id'] === pendingPlacementClauseBlockId.value),
)
const clauseSearch = ref('')
const clauseSearchId = useId()
const filteredUnusedClauses = computed((): DcsClause[] => {
  const query = clauseSearch.value.trim().toLowerCase()
  const clauses = unusedClauses.value.filter((clause) => clause['@id'] !== pendingPlacementClauseBlockId.value)
  if (!query) return clauses
  return clauses.filter((clause) => {
    const contentText = clauseContentText(clause)
    return [clause['dcs:title'] ?? '', contentText].some((v) => v.toLowerCase().includes(query))
  })
})

function clauseContentText(clause: DcsClause): string {
  const content = clause['dcs:content']
  if (typeof content === 'string') return content
  return content['@list'].map((seg) => (typeof seg === 'string' ? seg : '')).join(' ')
}

watch(addBlockModalContext, () => {
  clauseSearch.value = ''
  componentSearch.value = ''
})

function getSegments(clause: DcsClause): Segment[] {
  const content = clause['dcs:content']
  if (typeof content === 'string') return []
  return parseSegmentsFromContent(content['@list'], semanticConditions.value)
}

function getPlaceholderLabel(seg: Segment): string {
  return getPlaceholderLabelFromConditions(seg, semanticConditions.value)
}

function handleCancel() {
  uiStore.closeAddBlockModal()
}

function handleAddBlock(blockType: NewBlockType) {
  const ctx = addBlockModalContext.value
  if (ctx === null) return
  draftStore.addBlock(ctx.parentBlockId, ctx.insertIndex, { blockType })
  uiStore.closeAddBlockModal()
}

// Flatten-on-compose: fetch the component's full template_data and inline its
// blocks, placeholders and policies into this document at the insertion point.
async function handleAddComponent(template: ContractTemplate) {
  const ctx = addBlockModalContext.value
  if (ctx === null) return
  const full = await contractTemplateService.retrieveById({
    did: template.did,
  })
  if (!full) return
  draftStore.inlineComponent(full, ctx.parentBlockId, ctx.insertIndex)
  uiStore.closeAddBlockModal()
}

function handleAddClause(clauseBlockId: string) {
  const ctx = addBlockModalContext.value
  if (ctx === null) return
  draftStore.addBlock(ctx.parentBlockId, ctx.insertIndex, {
    blockType: 'dcs:Clause',
    clauseBlockId,
  })
  uiStore.closeAddBlockModal()
}
</script>

<template>
  <Teleport to="body">
    <div
      v-if="addBlockModalContext !== null"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      role="dialog"
      aria-modal="true"
      aria-labelledby="add-block-title"
      @click.self="handleCancel"
    >
      <div
        class="mx-4 flex max-h-[85vh] w-full max-w-2xl flex-col gap-4 overflow-y-auto rounded-2xl bg-base-100 p-6 shadow-xl"
        @click.stop
      >
        <h2 id="add-block-title" class="text-lg font-bold">Add block</h2>
        <div>
          <p class="mb-2 text-sm text-base-content/70">Common:</p>
          <div class="flex flex-col gap-2">
            <BlockPaletteItem
              v-for="item in paletteBlockTypes"
              :key="item.blockType"
              :label="item.label"
              @select="handleAddBlock(item.blockType)"
            />
          </div>
        </div>

        <div v-if="showComponentPicker" class="border-t border-base-300 pt-4">
          <div class="mb-2 flex flex-col gap-2">
            <p class="text-sm text-base-content/70">Components (inlined on add):</p>
            <input
              v-model="componentSearch"
              type="search"
              class="input-bordered input input-sm w-full"
              placeholder="Search components"
              autocomplete="off"
            />
          </div>
          <div class="flex max-h-64 flex-col gap-2 overflow-y-auto">
            <button
              v-for="t in componentTemplates"
              :key="`${t.did}-${t.version}`"
              type="button"
              class="flex min-h-11 cursor-pointer flex-col justify-center rounded-lg border border-base-300 bg-base-100 px-3 py-2 text-left transition-colors select-none hover:bg-base-200"
              @click="handleAddComponent(t)"
            >
              <span class="text-sm font-medium text-base-content">{{ t.name }}</span>
              <p class="mt-0.5 line-clamp-2 text-xs leading-relaxed text-base-content/70">{{ t.description }}</p>
            </button>
            <p v-if="!componentTemplates.length" class="py-2 text-sm text-base-content/50">
              {{ componentSearch ? 'No matching components.' : 'No approved components available.' }}
            </p>
          </div>
        </div>

        <div class="border-t border-base-300 pt-4">
          <div v-if="pendingPlacementClause" class="mb-4 rounded-lg border border-primary/30 bg-primary/5 p-3">
            <p class="mb-2 text-xs font-semibold text-primary">Selected clause</p>
            <button
              type="button"
              class="flex min-h-11 w-full cursor-pointer flex-col justify-center rounded-lg border border-primary/40 bg-base-100 px-3 py-2 text-left transition-colors select-none hover:bg-base-200"
              @click="handleAddClause(pendingPlacementClause['@id'])"
            >
              <span class="text-sm font-medium text-base-content">
                {{ pendingPlacementClause['dcs:title'] || 'Untitled clause' }}
              </span>
              <p class="mt-0.5 line-clamp-2 text-xs leading-relaxed text-base-content/70">
                <ClauseSegmentsPreview
                  :segments="getSegments(pendingPlacementClause)"
                  :get-placeholder-label="getPlaceholderLabel"
                />
              </p>
            </button>
          </div>
          <div class="mb-2 flex flex-col gap-2">
            <p class="text-sm text-base-content/70">Defined clauses:</p>
            <input
              :id="clauseSearchId"
              v-model="clauseSearch"
              type="search"
              class="input-bordered input input-sm w-full"
              placeholder="Search clauses"
              autocomplete="off"
            />
          </div>
          <div class="flex max-h-64 flex-col gap-2 overflow-y-auto">
            <button
              v-for="clause in filteredUnusedClauses"
              :key="clause['@id']"
              type="button"
              class="flex min-h-11 cursor-pointer flex-col justify-center rounded-lg border border-base-300 bg-base-100 px-3 py-2 text-left transition-colors select-none hover:bg-base-200"
              @click="handleAddClause(clause['@id'])"
            >
              <span class="text-sm font-medium text-base-content">
                {{ clause['dcs:title'] || 'Untitled clause' }}
              </span>
              <p class="mt-0.5 line-clamp-2 text-xs leading-relaxed text-base-content/70">
                <ClauseSegmentsPreview :segments="getSegments(clause)" :get-placeholder-label="getPlaceholderLabel" />
              </p>
            </button>
            <p v-if="!filteredUnusedClauses.length" class="py-2 text-sm text-base-content/50">
              {{ unusedClauses.length ? 'No matching clauses.' : 'No unplaced clauses from the Clauses tab.' }}
            </p>
          </div>
        </div>

        <div class="flex justify-end pt-2">
          <button type="button" class="btn btn-outline btn-sm" @click="handleCancel">Cancel</button>
        </div>
      </div>
    </div>
  </Teleport>
</template>
