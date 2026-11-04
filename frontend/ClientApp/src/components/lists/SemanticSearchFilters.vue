<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { parseSearchFieldFromSchema } from '@template-repository/utils/ontology-domain-fields'
import { resolveValueOptions } from '@template-repository/utils/value-option-catalog'
import { getSchemaVersions, getSearchFilters, type SearchFilter } from '@/services/semantic-hub-service'
import type { DomainFieldDefinition } from '@template-repository/models/contract-template'

const model = defineModel<string>({ default: '' })
const filters = ref<SearchFilter[]>([])
const values = ref<Record<string, string>>({})
const error = ref('')
const fields = ref<Record<string, DomainFieldDefinition>>({})

function field(filter: SearchFilter) {
  return fields.value[filter.id]
}

function options(filter: SearchFilter) {
  return resolveValueOptions(field(filter)?.valueConstraint)
}

watch(
  values,
  () => {
    if (!filters.value.length) return
    error.value = ''
    const selected = filters.value.flatMap((filter) => {
      const value = values.value[filter.id]
      if (value === undefined || value === '') return []
      const type = field(filter)?.type
      const datatype = field(filter)?.datatype
      return [
        {
          filter_id: filter.id,
          value: type === 'boolean' || datatype === 'xsd:boolean' ? value === 'true' : value,
        },
      ]
    })
    model.value = selected.length ? JSON.stringify(selected) : ''
  },
  { deep: true },
)

watch(
  model,
  (value) => {
    if (!value) {
      if (Object.keys(values.value).length) values.value = {}
      return
    }
    try {
      const selected = JSON.parse(value) as { filter_id: string; value: unknown }[]
      const next = Object.fromEntries(selected.map((item) => [item.filter_id, String(item.value)]))
      if (JSON.stringify(next) !== JSON.stringify(values.value)) values.value = next
    } catch {
      error.value = 'Invalid saved search filters'
    }
  },
  { immediate: true },
)

onMounted(async () => {
  try {
    const configured = (await getSearchFilters()).filters
    const schemas = new Map<string, ReturnType<typeof getSchemaVersions>>()
    for (const filter of configured) {
      const key = `${filter.source.kind}|${filter.source.name}`
      if (!schemas.has(key)) schemas.set(key, getSchemaVersions(filter.source.name, filter.source.kind))
    }
    for (const filter of configured) {
      const versions = await schemas.get(`${filter.source.kind}|${filter.source.name}`)!
      const schema = versions.find((item) => item.version === filter.source.version)
      if (!schema) throw new Error(`Source version unavailable for ${filter.label}`)
      const definition = parseSearchFieldFromSchema(schema.content, schema.kind, filter.field_iri)
      if (!definition) throw new Error(`Ontology field unavailable for ${filter.label}`)
      fields.value[filter.id] = definition
    }
    filters.value = configured
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Could not load search filters'
  }
})
</script>

<template>
  <div class="flex flex-wrap items-end gap-2">
    <label v-for="filter in filters" :key="filter.id" class="form-control">
      <span class="label">{{ filter.label }}</span>
      <select v-if="field(filter)?.type === 'boolean'" v-model="values[filter.id]" class="select-bordered select">
        <option value="">Any</option>
        <option value="true">Yes</option>
        <option value="false">No</option>
      </select>
      <select v-else-if="options(filter).length" v-model="values[filter.id]" class="select-bordered select">
        <option value="">Any</option>
        <option v-for="option in options(filter)" :key="option.value" :value="option.value">{{ option.label }}</option>
      </select>
      <input
        v-else
        :value="values[filter.id]"
        class="input-bordered input"
        :step="field(filter)?.type === 'decimal' ? 'any' : undefined"
        :type="
          field(filter)?.type === 'date'
            ? 'date'
            : field(filter)?.type === 'decimal' || field(filter)?.type === 'integer'
              ? 'number'
              : 'text'
        "
        @input="values[filter.id] = ($event.target as HTMLInputElement).value"
      />
    </label>
    <p v-if="error" role="alert" class="text-error">{{ error }}</p>
  </div>
</template>
