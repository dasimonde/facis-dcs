<script setup lang="ts">
import { computed, ref, useId, watch } from 'vue'

/**
 * Picks an ODRL term that is an IRI reference — an action, a party
 * (assigner/assignee), a target. Offers the curated vocabulary the DCS knows
 * plus a "Custom IRI…" escape, because ODRL places no closed vocabulary on
 * these: any IRI (a profile action, a party DID, an asset) is valid. Reserved
 * for reference terms — operators and left operands stay curated because the
 * enforcement engine evaluates them.
 */

export interface IriOption {
  value: string
  label: string
}
export interface IriGroup {
  label: string
  options: IriOption[]
}

const props = withDefaults(
  defineProps<{
    modelValue: string
    options?: IriOption[]
    groups?: IriGroup[]
    placeholder?: string
  }>(),
  { options: () => [], groups: () => [], placeholder: 'https://…  (custom IRI)' },
)
const emit = defineEmits<{ 'update:modelValue': [string] }>()

const CUSTOM = '__custom__'

const knownValues = computed(() => {
  const values = new Set(props.options.map((o) => o.value))
  for (const group of props.groups) for (const option of group.options) values.add(option.value)
  return values
})

const custom = ref(!!props.modelValue && !knownValues.value.has(props.modelValue))
const selectValue = computed(() => (custom.value ? CUSTOM : props.modelValue))

const selectValueId = useId()

function onSelect(event: Event) {
  const value = (event.target as HTMLSelectElement).value
  if (value === CUSTOM) {
    custom.value = true
    return
  }
  custom.value = false
  emit('update:modelValue', value)
}

watch(
  () => props.modelValue,
  (value) => {
    if (value && !knownValues.value.has(value)) custom.value = true
  },
)
</script>

<template>
  <div class="flex flex-col gap-1">
    <select :id="selectValueId" :value="selectValue" class="select-bordered select select-sm" @change="onSelect">
      <option v-for="o in options" :key="o.value" :value="o.value">{{ o.label }}</option>
      <optgroup v-for="g in groups" :key="g.label" :label="g.label">
        <option v-for="o in g.options" :key="o.value" :value="o.value">{{ o.label }}</option>
      </optgroup>
      <option :value="CUSTOM">✎ Custom IRI…</option>
    </select>
    <input
      v-if="custom"
      :id="`${selectValueId}-custom`"
      :value="modelValue"
      type="text"
      :placeholder="placeholder"
      class="input-bordered input input-xs"
      @input="emit('update:modelValue', ($event.target as HTMLInputElement).value)"
    />
  </div>
</template>
