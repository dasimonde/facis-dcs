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
const saving = ref(false)
const saveStatus = ref<'idle' | 'success' | 'error'>('idle')
const saveMessage = ref('')
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
  saveStatus.value = 'idle'
  saveMessage.value = ''
  config.value.filters.push({
    id: crypto.randomUUID(),
    label: option.field.label,
    field_iri: option.field.ontologyId,
    source: option.source,
  })
}

function removeFilter(index: number) {
  saveStatus.value = 'idle'
  saveMessage.value = ''
  config.value?.filters.splice(index, 1)
}

async function load() {
  try {
    config.value = await getSearchFilters()
  } catch (error) {
    errors.add(error instanceof Error ? error.message : 'Could not load search filters')
  }
}

async function save() {
  if (!config.value || saving.value) return
  saving.value = true
  saveStatus.value = 'idle'
  saveMessage.value = ''
  try {
    config.value = await saveSearchFilters(config.value)
    saveStatus.value = 'success'
    saveMessage.value = 'Search filters saved.'
    errors.add('Search filters saved.', 'info')
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Could not save search filters'
    saveStatus.value = 'error'
    saveMessage.value = message
    errors.add(message)
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="space-y-4 rounded-lg border border-base-300 bg-base-100 p-4">
    <div class="space-y-1">
      <h2 class="font-semibold">Search filters</h2>
      <p class="text-sm text-base-content/70">Choose the ontology fields available as filters in local searches.</p>
    </div>

    <ul v-if="config" class="space-y-3" aria-label="Configured search filters">
      <li
        v-for="(filter, index) in config.filters"
        :key="filter.id"
        class="grid gap-3 rounded-md border border-base-300 bg-base-200/40 p-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)_auto] sm:items-end"
      >
        <div class="flex min-w-0 flex-col gap-1.5">
          <label :for="`search-filter-label-${filter.id}`" class="text-xs font-medium text-base-content/70">
            Label
          </label>
          <input
            :id="`search-filter-label-${filter.id}`"
            v-model.trim="filter.label"
            class="input-bordered input w-full"
            required
          />
        </div>

        <div class="flex min-w-0 flex-col gap-1.5">
          <label :for="`search-filter-field-${filter.id}`" class="text-xs font-medium text-base-content/70">
            Ontology field
          </label>
          <select
            :id="`search-filter-field-${filter.id}`"
            class="select-bordered select w-full"
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
        </div>

        <button
          type="button"
          class="btn w-full btn-outline btn-sm sm:w-auto"
          :aria-label="`Remove ${filter.label || 'filter'}`"
          @click="removeFilter(index)"
        >
          Remove
        </button>
      </li>
    </ul>

    <div class="flex flex-wrap items-center gap-3">
      <button type="button" class="btn btn-outline btn-sm" :disabled="!config || !options.length" @click="addFilter">
        Add filter
      </button>
      <button type="button" class="btn btn-sm btn-primary" :disabled="!config || saving" @click="save">
        {{ saving ? 'Saving…' : 'Save filters' }}
      </button>
      <p
        v-if="saveMessage"
        role="status"
        class="text-sm"
        :class="
          saveStatus === 'success' ? 'text-success' : saveStatus === 'error' ? 'text-error' : 'text-base-content/70'
        "
      >
        {{ saveMessage }}
      </p>
    </div>
  </section>
</template>
