<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import type {
  SemanticConditionParameter,
  SemanticOperateType,
  SemanticParameterOperator,
} from '@/modules/template-repository/models/contract-template'
import {
  formatValueOption,
  isTokenValueConstraint,
  resolveValueOptions,
} from '@template-repository/utils/value-option-catalog'
import { formatNumberInput, normalizeNumberInput } from '@template-repository/utils/number-format'

const props = defineProps<{
  parameter: SemanticConditionParameter
  operators: SemanticParameterOperator[]
}>()

const emit = defineEmits<{
  'update:operators': [operators: SemanticParameterOperator[]]
  'validity-change': [isValid: boolean]
}>()

const draftOperator = ref<SemanticOperateType | ''>('')
const draftTarget = ref('')
const draftSetTargets = ref<string[]>([])
const draftTokenTargets = ref('')
const valueOptionSearch = ref('')
let isSyncingFromProps = false

const valueConstraint = computed(() => props.parameter.valueConstraint)
const isNumericParameter = computed(() => props.parameter.type === 'decimal' || props.parameter.type === 'integer')
const valueOptions = computed(() => resolveValueOptions(valueConstraint.value))
const usesSetConstraintEditor = computed(() => {
  const constraint = valueConstraint.value
  const supportsSetConstraints = props.parameter.type === 'string' || props.parameter.type === 'enum'
  return supportsSetConstraints && !!constraint && (valueOptions.value.length > 0 || isTokenValueConstraint(constraint))
})
const activeSetTargets = computed(() => setConstraintTargets())
const operatorOptions = computed(() =>
  usesSetConstraintEditor.value ? setOperatorOptions() : operatorOptionsForType(props.parameter.type),
)
const filteredValueOptions = computed(() => {
  const query = valueOptionSearch.value.trim().toLowerCase()
  if (!query) return valueOptions.value
  return valueOptions.value.filter(
    (option) => option.value.toLowerCase().includes(query) || option.label.toLowerCase().includes(query),
  )
})
const operatorError = computed(() => validateDraftOperator())

watch(
  () => props.operators,
  (operators) => syncDraftFromOperators(operators),
  { immediate: true, deep: true },
)

watch(
  () => props.parameter.semanticPath,
  () => syncDraftFromOperators(props.operators),
)

watch(draftOperator, (operator) => {
  if (isSyncingFromProps) return
  if (!operator) draftTarget.value = ''
  if (isSetOperator(operator)) {
    draftTarget.value = ''
    emitOperators()
    return
  }
  clearSetConstraintTargets()
  emitOperators()
})

watch(
  [draftTarget, draftSetTargets, draftTokenTargets],
  () => {
    if (!isSyncingFromProps) emitOperators()
  },
  { deep: true },
)

watch(operatorError, (error) => emit('validity-change', !error), { immediate: true })

watch(usesSetConstraintEditor, (usesSet) => {
  if (isSyncingFromProps) return
  if (usesSet) {
    if (!isSetOperator(draftOperator.value)) draftOperator.value = ''
    draftTarget.value = ''
    return
  }
  clearSetConstraintTargets()
})

function syncDraftFromOperators(operators: readonly SemanticParameterOperator[]) {
  isSyncingFromProps = true
  const operator = operators[0]
  draftOperator.value = operator?.operate ?? ''
  const targets = operator?.targets ?? []
  if (isSetOperator(draftOperator.value)) {
    draftTarget.value = ''
    draftSetTargets.value = targets.map((target) => formatOperatorTarget(target))
    draftTokenTargets.value = draftSetTargets.value.join(', ')
  } else {
    const target = formatOperatorTarget(targets[0])
    draftTarget.value = isNumericParameter.value ? formatNumberInput(target) : target
    clearSetConstraintTargets()
  }
  valueOptionSearch.value = ''
  void nextTick(() => {
    isSyncingFromProps = false
  })
}

function formatOperatorTarget(target: unknown): string {
  if (target === undefined || target === null) return ''
  if (typeof target === 'string') return target
  if (typeof target === 'number' || typeof target === 'boolean' || typeof target === 'bigint') return String(target)
  return JSON.stringify(target) ?? ''
}

function emitOperators() {
  const operators = buildDraftOperators()
  emit('validity-change', !operatorError.value)
  if (!sameOperators(operators, props.operators)) emit('update:operators', operators)
}

function sameOperators(left: readonly SemanticParameterOperator[], right: readonly SemanticParameterOperator[]) {
  return JSON.stringify(left) === JSON.stringify(right)
}

function buildDraftOperators(): SemanticParameterOperator[] {
  if (usesSetConstraintEditor.value) {
    if (!isSetOperator(draftOperator.value)) return []
    const targets = setConstraintTargets()
    return targets.length ? [{ operate: draftOperator.value, targets }] : []
  }
  if (!draftOperator.value) return []
  return [{ operate: draftOperator.value, targets: [parseDraftTarget()] }]
}

function setConstraintTargets(): string[] {
  if (!isSetOperator(draftOperator.value)) return []
  if (valueOptions.value.length) return [...draftSetTargets.value]
  return draftTokenTargets.value
    .split(',')
    .map((value) => value.trim())
    .filter(Boolean)
}

function parseDraftTarget(): unknown {
  const raw = draftTarget.value.trim()
  switch (props.parameter.type) {
    case 'decimal':
    case 'integer':
      return Number(normalizeDecimalInput(raw))
    case 'boolean':
      return raw === 'true'
    default:
      return raw
  }
}

function validateDraftOperator(): string {
  if (usesSetConstraintEditor.value) {
    const targets = setConstraintTargets()
    if (!targets.length) return ''
    const invalid = targets.find((target) => !targetMatchesConstraint(target))
    return invalid ? `"${invalid}" does not match the field format.` : ''
  }
  if (!draftOperator.value) return ''
  const raw = draftTarget.value.trim()
  if (raw === '') return 'Target is required.'
  if (props.parameter.type === 'decimal' || props.parameter.type === 'integer') {
    const number = Number(normalizeDecimalInput(raw))
    if (!Number.isFinite(number)) return 'Target must be numeric.'
    if (props.parameter.type === 'integer' && !Number.isInteger(number)) return 'Target must be an integer.'
    const constraint = valueConstraint.value
    if (constraint?.min !== undefined && number < constraint.min) return `Minimum is ${constraint.min}.`
    if (constraint?.max !== undefined && number > constraint.max) return `Maximum is ${constraint.max}.`
  }
  if (props.parameter.type === 'boolean' && raw !== 'true' && raw !== 'false') return 'Use true or false.'
  return ''
}

function targetMatchesConstraint(target: string): boolean {
  const constraint = valueConstraint.value
  if (!constraint) return true
  if (valueOptions.value.length) return valueOptions.value.some((option) => option.value === target)
  if (constraint.pattern) return new RegExp(constraint.pattern).test(target)
  if (constraint.format === 'iso-3166-1-alpha-3') return /^[A-Z]{3}$/.test(target)
  if (constraint.format === 'iso-4217') return /^[A-Z]{3}$/.test(target)
  return true
}

function normalizeDecimalInput(value: string): string {
  return normalizeNumberInput(value)
}

function formatNumericTarget(event: Event) {
  if (!isNumericParameter.value) return
  draftTarget.value = formatNumberInput((event.target as HTMLInputElement | null)?.value ?? '')
}

function operatorOptionsForType(type: SemanticConditionParameter['type']) {
  const equality = [
    { value: 'odrl:eq' as SemanticOperateType, label: 'Must equal' },
    { value: 'odrl:neq' as SemanticOperateType, label: 'Must not equal' },
  ]
  if (type === 'decimal' || type === 'integer' || type === 'date') {
    return [
      { value: 'odrl:gt' as SemanticOperateType, label: 'Must be greater than' },
      { value: 'odrl:gteq' as SemanticOperateType, label: 'Must be at least' },
      { value: 'odrl:lt' as SemanticOperateType, label: 'Must be less than' },
      { value: 'odrl:lteq' as SemanticOperateType, label: 'Must be at most' },
      ...equality,
    ]
  }
  if (type === 'string' || type === 'enum') {
    return [
      ...equality,
      { value: 'odrl:hasPart' as SemanticOperateType, label: 'Must contain' },
      { value: 'dcs:matchesRegex' as SemanticOperateType, label: 'Must match pattern' },
    ]
  }
  return equality
}

function setOperatorOptions() {
  return [
    { value: 'odrl:isAnyOf' as SemanticOperateType, label: 'Allow only' },
    { value: 'odrl:isNoneOf' as SemanticOperateType, label: 'Exclude' },
  ]
}

function isSetOperator(
  operator: SemanticOperateType | '',
): operator is Extract<SemanticOperateType, 'odrl:isAnyOf' | 'odrl:isNoneOf'> {
  return operator === 'odrl:isAnyOf' || operator === 'odrl:isNoneOf'
}

function formatSelectedValue(value: string): string {
  return formatValueOption(value, valueOptions.value)
}

function clearSetConstraintTargets() {
  draftSetTargets.value = []
  draftTokenTargets.value = ''
  valueOptionSearch.value = ''
}
</script>

<template>
  <div class="contents">
    <div class="flex flex-col gap-1 md:col-span-3">
      <label class="label min-h-0 py-0">
        <span class="label-text text-xs text-base-content/60">Obligation</span>
      </label>
      <select v-model="draftOperator" class="select-bordered select h-9 w-full select-sm">
        <option value="">None</option>
        <option v-for="option in operatorOptions" :key="option.value" :value="option.value">
          {{ option.label }}
        </option>
      </select>
    </div>

    <div v-if="!usesSetConstraintEditor" class="flex flex-col gap-1 md:col-span-2">
      <label class="label min-h-0 py-0">
        <span class="label-text text-xs text-base-content/60">Value</span>
      </label>
      <select
        v-if="parameter.type === 'boolean'"
        v-model="draftTarget"
        class="select-bordered select h-9 w-full select-sm"
        :disabled="!draftOperator"
      >
        <option value="">Select</option>
        <option value="true">true</option>
        <option value="false">false</option>
      </select>
      <input
        v-else
        v-model="draftTarget"
        :type="parameter.type === 'date' ? 'date' : 'text'"
        :inputmode="isNumericParameter ? 'decimal' : undefined"
        class="input-bordered input input-sm h-9 w-full"
        :disabled="!draftOperator"
        placeholder=""
        @input="formatNumericTarget"
      />
      <p v-if="operatorError" class="text-xs text-error">{{ operatorError }}</p>
    </div>

    <div
      v-if="usesSetConstraintEditor && isSetOperator(draftOperator)"
      class="flex flex-col gap-1 md:col-span-5 md:col-start-5"
    >
      <label class="label min-h-0 py-0">
        <span class="label-text text-xs text-base-content/60">Values</span>
        <span v-if="!activeSetTargets.length" class="label-text-alt text-xs text-warning">
          Select at least one value
        </span>
      </label>
      <input
        v-if="valueOptions.length"
        v-model="valueOptionSearch"
        type="search"
        class="input-bordered input input-sm h-9 w-full"
        placeholder="Search values"
      />
      <div v-if="valueOptions.length" class="max-h-36 overflow-y-auto rounded border border-base-300 bg-base-100 p-2">
        <label
          v-for="option in filteredValueOptions"
          :key="option.value"
          class="flex cursor-pointer items-center gap-2 rounded px-2 py-1 text-sm hover:bg-base-200"
        >
          <input
            v-model="draftSetTargets"
            type="checkbox"
            class="checkbox checkbox-xs checkbox-primary"
            :value="option.value"
          />
          <span>{{ option.label }} ({{ option.value }})</span>
        </label>
      </div>
      <input
        v-else
        v-model="draftTokenTargets"
        type="text"
        class="input-bordered input input-sm h-9 w-full"
        placeholder=""
      />
      <div v-if="draftSetTargets.length" class="flex flex-wrap gap-1">
        <span v-for="value in draftSetTargets" :key="value" class="badge badge-outline badge-sm">
          {{ formatSelectedValue(value) }}
        </span>
      </div>
      <p v-if="operatorError" class="text-xs text-error">{{ operatorError }}</p>
    </div>
  </div>
</template>
