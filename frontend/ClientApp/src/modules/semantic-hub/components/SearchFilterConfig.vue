<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ONTOLOGY_ASSETS, ONTOLOGY_DOMAIN_FIELDS } from '@template-repository/utils/ontology-domain-fields'
import {
  getSearchFilters,
  saveSearchFilters,
  type SearchFilterConfig,
  type SemanticSchemaListEntry,
} from '@/services/semantic-hub-service'
import { useErrorStore } from '@/stores/error-store'

const props = defineProps<{ entries: SemanticSchemaListEntry[] }>()
const config = ref<SearchFilterConfig | null>(null)
const errors = useErrorStore()
const options = computed(() => {
  const fields = [
    ...ONTOLOGY_DOMAIN_FIELDS.map((field) => ({ field, source: field.source })),
    ...ONTOLOGY_ASSETS.flatMap((asset) => asset.properties.map((field) => ({ field, source: asset.source }))),
  ]
  return fields.flatMap(({ field, source }) => {
    const entry = props.entries.find((item) => item.name === source?.name && item.kind === source.kind)
    return source && entry?.active_version
      ? [
          {
            key: `${source.kind}|${source.name}|${field.ontologyId}`,
            field,
            source: { ...source, version: entry.active_version },
          },
        ]
      : []
  })
})

function selectedKey(filter: SearchFilterConfig['filters'][number]) {
  return `${filter.source.kind}|${filter.source.name}|${filter.field_iri}`
}

function setField(index: number, key: string) {
  const option = options.value.find((item) => item.key === key)
  const filter = config.value?.filters[index]
  if (!option || !filter) return
  filter.field_iri = option.field.ontologyId
  filter.source = option.source
}

function addFilter() {
  const option = options.value[0]
  if (!config.value || !option) return
  config.value.filters.push({
    id: crypto.randomUUID(),
    label: option.field.label,
    field_iri: option.field.ontologyId,
    source: option.source,
  })
}

async function load() {
  try {
    config.value = await getSearchFilters()
  } catch (error) {
    errors.add(error instanceof Error ? error.message : 'Could not load search filters')
  }
}

async function save() {
  if (!config.value) return
  try {
    config.value = await saveSearchFilters(config.value)
  } catch (error) {
    errors.add(error instanceof Error ? error.message : 'Could not save search filters')
  }
}

onMounted(load)
</script>

<template>
  <section class="space-y-3 rounded-lg border border-base-300 bg-base-100 p-4">
    <h2 class="font-semibold">Search filters</h2>
    <p class="text-sm text-base-content/70">Choose the ontology fields available as filters in local searches.</p>
    <div v-for="(filter, index) in config?.filters" :key="filter.id" class="flex flex-wrap items-end gap-2">
      <label class="form-control grow">
        <span class="label">Label</span>
        <input v-model.trim="filter.label" class="input-bordered input" required />
      </label>
      <label class="form-control grow">
        <span class="label">Ontology field</span>
        <select
          class="select-bordered select"
          :value="selectedKey(filter)"
          @change="setField(index, ($event.target as HTMLSelectElement).value)"
        >
          <option v-if="!options.some((option) => option.key === selectedKey(filter))" :value="selectedKey(filter)">
            {{ filter.field_iri }}
          </option>
          <option v-for="option in options" :key="option.key" :value="option.key">
            {{ option.field.label }} — {{ option.source.name }}
          </option>
        </select>
      </label>
      <button
        type="button"
        class="btn btn-ghost"
        :aria-label="`Remove ${filter.label}`"
        @click="config?.filters.splice(index, 1)"
      >
        Remove
      </button>
    </div>
    <div class="flex gap-2">
      <button type="button" class="btn btn-outline btn-sm" :disabled="!config || !options.length" @click="addFilter">
        Add filter
      </button>
      <button type="button" class="btn btn-sm btn-primary" :disabled="!config" @click="save">Save filters</button>
    </div>
  </section>
</template>
