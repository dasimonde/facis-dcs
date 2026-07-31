<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import DataObjectNode from '@template-repository/components/data-objects/DataObjectNode.vue'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { loadShapeLibraries, type ShapeClass } from '@template-repository/utils/shape-library'
import type { SemanticConditionValue } from '@/models/contract/contract-data'
import type { SemanticConditionValueSetter } from '@contract-workflow-engine/models/contract-content-values-store'

/**
 * Semantic data objects (ADR-23): the document's dcs:contractData graph,
 * authored from the Semantic Hub's registered SHACL libraries. A Template
 * Creator picks a class, clicks it into the document, fills fixed values,
 * nests linked objects, and marks leaves negotiable; the contract flow
 * fills those leaves through the same value store the clause chips use.
 */

const props = defineProps<{
  mode: 'template' | 'contract'
  editable: boolean
  semanticConditionValues?: SemanticConditionValue[]
  setSemanticConditionValue?: SemanticConditionValueSetter
}>()

const store = useDcsDraftStore()

const classes = ref<readonly ShapeClass[]>([])
const selectedClass = ref('')
const loadError = ref('')
const loading = ref(true)

onMounted(async () => {
  try {
    classes.value = await loadShapeLibraries()
  } catch (err) {
    loadError.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
})

// Roots are the objects no other domain object references — nested objects
// render inside their parent.
const rootObjects = computed(() => {
  const referenced = new Set<string>()
  for (const object of store.contractData) {
    for (const [property, value] of Object.entries(object)) {
      if (property.startsWith('@')) continue
      const members = Array.isArray(value) ? value : [value]
      for (const member of members) {
        if (typeof member === 'object' && member !== null && '@id' in member) referenced.add(member['@id'])
      }
    }
  }
  return store.contractData.filter((object) => !referenced.has(object['@id']))
})

function addObject() {
  if (!selectedClass.value) return
  store.addDataObject(selectedClass.value)
}
</script>

<template>
  <div class="space-y-3" data-testid="data-objects-editor">
    <div v-if="loading" class="flex justify-center p-4"><span class="loading loading-spinner" /></div>
    <div v-else-if="loadError" class="alert text-sm alert-error">{{ loadError }}</div>

    <template v-else>
      <p v-if="!classes.length" class="text-sm opacity-70">
        No shape libraries are registered in the Semantic Hub beyond the document envelope — register a SHACL library to
        author typed data objects from it.
      </p>

      <div v-if="mode === 'template' && editable && classes.length" class="flex items-center gap-2">
        <select v-model="selectedClass" class="select-bordered select select-sm" data-testid="data-object-class">
          <option value="">choose a class</option>
          <option v-for="entry in classes" :key="entry.iri" :value="entry.iri">
            {{ entry.label }} ({{ entry.library }})
          </option>
        </select>
        <button
          type="button"
          class="btn btn-sm btn-primary"
          :disabled="!selectedClass"
          data-testid="add-data-object"
          @click="addObject"
        >
          Add object
        </button>
      </div>

      <p v-if="!rootObjects.length && classes.length" class="text-sm opacity-70">
        No data objects yet{{ mode === 'template' ? ' — pick a class and add one' : '' }}.
      </p>

      <DataObjectNode
        v-for="object in rootObjects"
        :key="object['@id']"
        :object="object"
        :classes="classes"
        :mode="props.mode"
        :editable="props.editable"
        :depth="0"
        :semantic-condition-values="semanticConditionValues"
        :set-semantic-condition-value="setSemanticConditionValue"
      />
    </template>
  </div>
</template>
