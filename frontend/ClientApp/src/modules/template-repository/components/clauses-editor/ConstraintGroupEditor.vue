<script setup lang="ts">
import { computed, useId } from 'vue'
import {
  type AtomicDraft,
  CONSTRAINT_COMBINATORS,
  type ConstraintNodeDraft,
  type GroupDraft,
  isGroupDraft,
  newAtomic,
  newGroup,
  type OperandDraftValue,
} from '@template-repository/components/clauses-editor/constraint-draft'
import { ODRL_CONTEXT_OPERANDS, ODRL_OPERATORS } from '@template-repository/utils/odrl-vocabulary'
import { ONTOLOGY_VALUE_CONSTRAINTS } from '@template-repository/utils/ontology-domain-fields'
import { resolveConstraintForLeftOperand } from '@template-repository/utils/value-constraint-catalog'
import {
  formatValueOption,
  groupValueOptions,
  resolveValueOptions,
} from '@template-repository/utils/value-option-catalog'

/**
 * Authors one ODRL constraint group — a combinator over child nodes, each an
 * atomic constraint or a nested group (recursion via the component's own name,
 * an arbitrarily deep constraint tree, ODRL IM §2.6). The rule and every duty
 * embed one root group.
 */

const props = defineProps<{
  /** Fields offered as a constraint's left operand and negotiated boundary. */
  fields: { id: string; label: string }[]
  /** The title on this group's combinator select (targets the top-level one). */
  combineTitle?: string
}>()

const group = defineModel<GroupDraft>({ required: true })

function addConstraint() {
  group.value.children.push(
    newAtomic(ODRL_CONTEXT_OPERANDS[0]?.id ?? 'odrl:spatial', ODRL_OPERATORS[0]?.id ?? 'odrl:eq'),
  )
}
function addGroup() {
  group.value.children.push(newGroup())
}
function removeChild(index: number) {
  group.value.children.splice(index, 1)
}

// Narrows a child node to a group for the recursive editor. The editor mutates
// this same reactive object in place, so a one-way :model-value binding still
// propagates every edit up through the shared draft graph.
function childGroup(child: ConstraintNodeDraft): GroupDraft {
  return child as GroupDraft
}

function isSetOperator(operator: string): boolean {
  return operator === 'odrl:isAnyOf' || operator === 'odrl:isNoneOf' || operator === 'odrl:isAllOf'
}

function valueOptionsFor(child: AtomicDraft) {
  return resolveValueOptions(resolveConstraintForLeftOperand(child.leftOperand))
}

function valueOptionGroupsFor(child: AtomicDraft) {
  return groupValueOptions(valueOptionsFor(child))
}

function isFieldOperand(child: AtomicDraft): boolean {
  return props.fields.some((field) => field.id === child.leftOperand)
}

/** The selection key for an option under a given left operand: a field-bound
 *  option is keyed by its notation (a field holds one distinct value anyway),
 *  a context-operand option by its concept IRI, which stays unique when
 *  notations collide across schemes. */
function optionKey(option: { iri?: string; value: string }, child: AtomicDraft): string {
  if (isFieldOperand(child)) return option.value
  return option.iri ?? option.value
}

function optionOperand(optionValue: string, child: AtomicDraft): OperandDraftValue {
  const option = valueOptionsFor(child).find((item) => item.value === optionValue || item.iri === optionValue)
  // A field-bound operand carries the option's notation (e.g. "DEU"): the
  // policy audit compares it against the field's filled dcs:value, which
  // holds the notation. A context operand (odrl:spatial, odrl:purpose) is
  // deferred to use-time policy evaluation and keeps the concept IRI.
  if (isFieldOperand(child)) return { '@value': option?.value ?? optionValue, '@type': 'xsd:string' }
  if (option?.iri) return { '@id': option.iri }
  return { '@value': optionValue, '@type': 'xsd:string' }
}

function operandKey(value: OperandDraftValue): string {
  return '@id' in value ? value['@id'] : String(value['@value'])
}

function selectedOptionValues(child: AtomicDraft): string[] {
  return child.values.map(operandKey)
}

function fixedValueFor(child: AtomicDraft): string {
  const [first] = child.values
  return first ? operandKey(first) : ''
}

function setSingleOption(child: AtomicDraft, event: Event) {
  const value = (event.target as HTMLSelectElement).value
  child.values = value ? [optionOperand(value, child)] : []
  child.value = ''
}

function toggleOption(child: AtomicDraft, optionValue: string) {
  const selected = new Set(selectedOptionValues(child))
  if (selected.has(optionValue)) selected.delete(optionValue)
  else selected.add(optionValue)
  child.values = valueOptionsFor(child)
    .map((option) => optionKey(option, child))
    .filter((value) => selected.has(value))
    .map((value) => optionOperand(value, child))
  child.value = ''
}

function clearFixedValues(child: AtomicDraft) {
  child.values = []
}

function resetFixedOperand(child: AtomicDraft) {
  child.value = ''
  child.values = []
}

const unitListId = `constraint-units-${useId()}`

// odrl:unit takes an IRI, so the suggestions are the concept IRIs of the
// currency schemes the Semantic Hub declares (the one unit vocabulary it
// carries). Any other unit — one a template declares itself — is typed in.
const unitOptions = computed(() => {
  const options = new Map<string, string>()
  for (const constraint of ONTOLOGY_VALUE_CONSTRAINTS) {
    if (constraint.format !== 'iso-4217') continue
    for (const option of constraint.valueOptions ?? []) {
      if (option.iri) options.set(option.iri, `${option.label ?? option.value} (${option.value})`)
    }
  }
  return [...options].map(([iri, label]) => ({ iri, label }))
})
</script>

<template>
  <div class="space-y-1">
    <div class="flex items-center gap-1">
      <select
        v-if="group.children.length > 1"
        v-model="group.combine"
        class="select-bordered select select-xs"
        :title="combineTitle ?? 'How this group combines'"
      >
        <option v-for="c in CONSTRAINT_COMBINATORS" :key="c.op" :value="c.op">{{ c.label }}</option>
      </select>
      <button type="button" class="btn btn-ghost btn-xs" @click="addConstraint">+ constraint</button>
      <button type="button" class="btn btn-ghost btn-xs" @click="addGroup">+ group</button>
    </div>

    <template v-for="(child, i) in group.children" :key="i">
      <!-- A nested group: recurse into this same editor, indented. -->
      <div v-if="isGroupDraft(child)" class="ml-3 space-y-1 rounded border border-dashed border-base-300 p-1">
        <div class="flex items-center justify-between">
          <span class="label-text text-2xs opacity-60">group</span>
          <button type="button" class="btn btn-ghost btn-xs" @click="removeChild(i)">✕</button>
        </div>
        <ConstraintGroupEditor :model-value="childGroup(child)" :fields="fields" />
      </div>

      <!-- An atomic constraint row. -->
      <div v-else class="flex flex-wrap items-center gap-1">
        <select v-model="child.leftOperand" class="select-bordered select select-xs" @change="resetFixedOperand(child)">
          <optgroup v-if="fields.length" label="Data fields">
            <option v-for="f in fields" :key="f.id" :value="f.id">{{ f.label }}</option>
          </optgroup>
          <optgroup label="Access context">
            <option v-for="o in ODRL_CONTEXT_OPERANDS" :key="o.id" :value="o.id">{{ o.label }}</option>
          </optgroup>
        </select>
        <select v-model="child.operator" class="select-bordered select select-xs">
          <option v-for="op in ODRL_OPERATORS" :key="op.id" :value="op.id">{{ op.label }}</option>
        </select>
        <select v-model="child.rightSource" class="select-bordered select select-xs" title="What the boundary is">
          <option value="">a fixed value</option>
          <optgroup v-if="fields.length" label="Agreed at negotiation">
            <option v-for="f in fields" :key="f.id" :value="f.id">the “{{ f.label }}”</option>
          </optgroup>
        </select>
        <input
          v-if="!child.rightSource && !valueOptionsFor(child).length"
          v-model="child.value"
          type="text"
          placeholder="value"
          class="input-bordered input input-xs w-28"
          @input="clearFixedValues(child)"
        />
        <select
          v-else-if="!child.rightSource && valueOptionsFor(child).length && !isSetOperator(child.operator)"
          :value="fixedValueFor(child)"
          class="select-bordered select min-w-36 select-xs"
          @change="setSingleOption(child, $event)"
        >
          <option value="">choose value</option>
          <option
            v-for="option in valueOptionsFor(child)"
            :key="optionKey(option, child)"
            :value="optionKey(option, child)"
            :selected="fixedValueFor(child) === optionKey(option, child)"
          >
            {{ formatValueOption(optionKey(option, child), valueOptionsFor(child)) }}
          </option>
        </select>
        <details v-else-if="!child.rightSource" data-testid="constraint-value-multiselect" class="dropdown max-w-full">
          <summary class="btn min-w-36 btn-outline btn-xs">
            {{ child.values.length ? `${child.values.length} selected` : 'choose values' }}
          </summary>
          <div
            class="dropdown-content z-10 mt-1 max-h-64 w-64 max-w-[calc(100vw-2rem)] overflow-auto rounded-box border border-base-content/10 bg-base-100 p-2 shadow"
          >
            <fieldset v-for="catalog in valueOptionGroupsFor(child)" :key="catalog.iri || 'values'">
              <legend v-if="catalog.label" class="px-2 py-1 text-xs font-semibold opacity-70">
                {{ catalog.label }}
              </legend>
              <label
                v-for="option in catalog.options"
                :key="optionKey(option, child)"
                class="flex min-h-8 items-center gap-2 rounded px-2 hover:bg-base-200"
              >
                <input
                  type="checkbox"
                  class="checkbox checkbox-sm checkbox-primary"
                  :value="optionKey(option, child)"
                  :checked="selectedOptionValues(child).includes(optionKey(option, child))"
                  @change="toggleOption(child, optionKey(option, child))"
                />
                <span class="text-sm">
                  {{ formatValueOption(optionKey(option, child), valueOptionsFor(child)) }}
                </span>
              </label>
            </fieldset>
          </div>
        </details>
        <select
          v-model="child.unitSource"
          data-testid="constraint-unit-source"
          class="select-bordered select select-xs"
          title="What the unit is"
        >
          <option value="">in a fixed unit</option>
          <optgroup v-if="fields.length" label="Agreed at negotiation">
            <option v-for="f in fields" :key="f.id" :value="f.id">in the “{{ f.label }}”</option>
          </optgroup>
        </select>
        <input
          v-if="!child.unitSource"
          v-model="child.unit"
          data-testid="constraint-unit"
          type="text"
          :list="unitListId"
          placeholder="unit IRI"
          class="input-bordered input input-xs w-28"
          title="Unit the boundary is measured in (an IRI) — optional"
        />
        <button type="button" class="btn btn-ghost btn-xs" @click="removeChild(i)">✕</button>
      </div>
    </template>

    <datalist :id="unitListId">
      <option v-for="unit in unitOptions" :key="unit.iri" :value="unit.iri" :label="unit.label" />
    </datalist>
  </div>
</template>
