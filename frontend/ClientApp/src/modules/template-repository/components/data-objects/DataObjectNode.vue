<script setup lang="ts">
import { computed } from 'vue'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { type ShapeClass, type ShapeProperty } from '@template-repository/utils/shape-library'
import { type DcsContractDataObject, localNameOf } from '@/models/dcs-jsonld'
import type { SemanticConditionValue } from '@/models/contract/contract-data'
import type { SemanticConditionValueSetter } from '@contract-workflow-engine/models/contract-content-values-store'

/**
 * One typed domain object of the contract-data graph, rendered from its hub
 * shape (ADR-23). Template mode authors the graph: fixed literals, nested
 * objects, and per-leaf negotiability (a negotiable leaf declares a
 * dcs:ContractField and binds the property to it). Contract mode presents
 * the agreed graph and takes fills on the negotiable leaves — the same
 * value store the clause chips write, keyed by field @id.
 */

const props = defineProps<{
  object: DcsContractDataObject
  classes: readonly ShapeClass[]
  mode: 'template' | 'contract'
  editable: boolean
  depth: number
  semanticConditionValues?: SemanticConditionValue[]
  setSemanticConditionValue?: SemanticConditionValueSetter
}>()

const store = useDcsDraftStore()

const shape = computed(() => props.classes.find((entry) => entry.iri === props.object['@type']))

type LeafBinding =
  | { kind: 'empty' }
  | { kind: 'fixed' }
  | { kind: 'negotiable'; fieldId: string; label: string; required: boolean }
  | { kind: 'nested'; childId: string }

function binding(property: ShapeProperty): LeafBinding {
  const value = props.object[property.path]
  if (value === undefined || value === null) return { kind: 'empty' }
  if (typeof value === 'object' && !Array.isArray(value) && '@id' in value) {
    const target = value['@id']
    const field = store.contractFields.find((entry) => entry['@id'] === target)
    if (field) {
      return { kind: 'negotiable', fieldId: target, label: field['dcs:label'], required: field['dcs:required'] }
    }
    // An IRI-valued leaf's {"@id"} names an external resource, not a child.
    if (property.iri) return { kind: 'fixed' }
    return { kind: 'nested', childId: target }
  }
  return { kind: 'fixed' }
}

function childObject(childId: string): DcsContractDataObject | undefined {
  return store.contractData.find((entry) => entry['@id'] === childId)
}

function literalText(property: ShapeProperty): string {
  const value = props.object[property.path]
  if (value === undefined || value === null) return ''
  if (typeof value === 'object' && !Array.isArray(value) && '@value' in value) return String(value['@value'])
  if (typeof value !== 'object') return String(value)
  return ''
}

function onLiteralInput(property: ShapeProperty, event: Event) {
  const value = (event.target as HTMLInputElement | HTMLSelectElement).value
  store.setDataObjectLiteral(
    props.object['@id'],
    property.path,
    value,
    property.datatypeIri ?? property.datatype ?? 'xsd:string',
  )
}

function iriText(property: ShapeProperty): string {
  const value = props.object[property.path]
  if (typeof value === 'object' && value !== null && !Array.isArray(value) && '@id' in value) return value['@id']
  return ''
}

function onIriInput(property: ShapeProperty, event: Event) {
  store.setDataObjectIri(props.object['@id'], property.path, (event.target as HTMLInputElement).value.trim())
}

// A leaf can go negotiable when a declared dcs:ContractField can express its
// datatype — an external library's non-XSD datatype IRI cannot ride a field
// declaration, and an IRI-valued fill would materialize as a string literal.
function negotiableSupported(property: ShapeProperty): boolean {
  return !property.datatypeIri && !property.iri && property.datatype !== undefined
}

function toggleNegotiable(property: ShapeProperty, event: Event) {
  const negotiable = (event.target as HTMLInputElement).checked
  if (negotiable) {
    store.makeDataLeafNegotiable(
      props.object['@id'],
      property.path,
      property.label,
      property.datatype ?? 'xsd:string',
      property.required,
      property.options,
    )
  } else {
    store.makeDataLeafFixed(props.object['@id'], property.path)
  }
}

function addNested(property: ShapeProperty) {
  if (!property.classRef) return
  store.addNestedDataObject(props.object['@id'], property.path, property.classRef)
}

function removeNested(property: ShapeProperty, childId: string) {
  store.makeDataLeafFixed(props.object['@id'], property.path)
  store.removeDataObject(childId)
}

function nestedClassLabel(property: ShapeProperty): string {
  const target = props.classes.find((entry) => entry.iri === property.classRef)
  return target?.label ?? localNameOf(property.classRef ?? '')
}

// Contract-mode fill plumbing: a negotiable leaf's fill is keyed exactly
// like the clause chips key it — blockId '', conditionId = field @id,
// parameterName = field label — so the load snapshot and the store agree.
function fillValue(fieldId: string): string {
  const match = props.semanticConditionValues?.find((entry) => entry.conditionId === fieldId)
  return match?.parameterValue === undefined ? '' : String(match.parameterValue)
}

function onFillInput(leaf: { fieldId: string; label: string }, event: Event) {
  const value = (event.target as HTMLInputElement | HTMLSelectElement).value
  props.setSemanticConditionValue?.('', leaf.fieldId, leaf.label, value)
}

const typeLabel = computed(() => shape.value?.label ?? localNameOf(props.object['@type']))

// The first filled literal distinguishes same-class instances ("Legal
// Person · Musterfirma GmbH") in a graph with several of them.
const instanceHint = computed(() => {
  for (const property of shape.value?.properties ?? []) {
    const value = props.object[property.path]
    if (typeof value === 'string' && value) return value
    if (typeof value === 'object' && value !== null && !Array.isArray(value) && '@value' in value) {
      return String(value['@value'])
    }
  }
  return ''
})
</script>

<template>
  <div
    class="rounded border border-base-300 p-3"
    :class="depth > 0 ? 'ml-4 border-dashed' : ''"
    :data-testid="`data-object-${localNameOf(object['@type'])}`"
  >
    <div class="mb-2 flex items-center justify-between">
      <span class="font-semibold">
        {{ typeLabel }}
        <span v-if="instanceHint" class="font-normal opacity-60">· {{ instanceHint }}</span>
      </span>
      <button
        v-if="mode === 'template' && editable && depth === 0"
        type="button"
        class="btn btn-ghost btn-xs"
        :data-testid="`remove-data-object-${localNameOf(object['@type'])}`"
        @click="store.removeDataObject(object['@id'])"
      >
        ✕
      </button>
    </div>

    <p v-if="!shape" class="text-sm opacity-70">
      No registered shape describes {{ object['@type'] }}. The object is carried verbatim.
    </p>

    <div v-else class="space-y-2">
      <div v-for="property in shape.properties" :key="property.path" class="flex flex-wrap items-center gap-2">
        <span class="w-44 text-sm" :title="property.path">
          {{ property.label }}
          <span v-if="property.required" class="text-error">*</span>
        </span>

        <!-- IRI-valued leaf (sh:nodeKind sh:IRI): names an external resource -->
        <template v-if="property.iri">
          <input
            v-if="mode === 'template'"
            :value="iriText(property)"
            type="url"
            placeholder="https://…"
            class="input-bordered input input-sm w-72"
            :data-testid="`iri-${localNameOf(property.path)}`"
            :disabled="!editable"
            @input="onIriInput(property, $event)"
          />
          <span v-else class="text-sm break-all" :data-testid="`iri-${localNameOf(property.path)}`">
            {{ iriText(property) || '—' }}
          </span>
        </template>

        <!-- Literal-valued leaf -->
        <template v-else-if="property.datatype">
          <template v-if="binding(property).kind === 'negotiable'">
            <template v-if="mode === 'contract'">
              <select
                v-if="property.options"
                :value="fillValue((binding(property) as { fieldId: string }).fieldId)"
                class="select-bordered select w-48 select-sm"
                :data-testid="`fill-${localNameOf(property.path)}`"
                :disabled="!editable"
                @change="onFillInput(binding(property) as { fieldId: string; label: string }, $event)"
              >
                <option value=""></option>
                <option v-for="option in property.options" :key="option" :value="option">{{ option }}</option>
              </select>
              <input
                v-else
                :value="fillValue((binding(property) as { fieldId: string }).fieldId)"
                :type="property.datatype === 'xsd:decimal' || property.datatype === 'xsd:integer' ? 'number' : 'text'"
                class="input-bordered input input-sm w-48"
                :data-testid="`fill-${localNameOf(property.path)}`"
                :disabled="!editable"
                @input="onFillInput(binding(property) as { fieldId: string; label: string }, $event)"
              />
              <span class="badge badge-outline badge-sm">negotiated</span>
            </template>
            <span v-else class="badge badge-sm badge-primary" :data-testid="`negotiable-${localNameOf(property.path)}`">
              negotiated at contract time
            </span>
          </template>
          <template v-else-if="mode === 'template'">
            <select
              v-if="property.options"
              :value="literalText(property)"
              class="select-bordered select w-48 select-sm"
              :data-testid="`literal-${localNameOf(property.path)}`"
              :disabled="!editable"
              @change="onLiteralInput(property, $event)"
            >
              <option value=""></option>
              <option v-for="option in property.options" :key="option" :value="option">{{ option }}</option>
            </select>
            <input
              v-else
              :value="literalText(property)"
              :type="property.datatype === 'xsd:decimal' || property.datatype === 'xsd:integer' ? 'number' : 'text'"
              class="input-bordered input input-sm w-48"
              :data-testid="`literal-${localNameOf(property.path)}`"
              :disabled="!editable"
              @input="onLiteralInput(property, $event)"
            />
          </template>
          <span v-else class="text-sm" :data-testid="`literal-${localNameOf(property.path)}`">
            {{ literalText(property) || '—' }}
          </span>

          <label
            v-if="mode === 'template' && editable && negotiableSupported(property)"
            class="label cursor-pointer gap-1 text-xs"
            :title="'Negotiable: agreed during contract negotiation instead of fixed here'"
          >
            <input
              type="checkbox"
              class="checkbox checkbox-xs"
              :checked="binding(property).kind === 'negotiable'"
              :data-testid="`toggle-negotiable-${localNameOf(property.path)}`"
              @change="toggleNegotiable(property, $event)"
            />
            negotiable
          </label>
        </template>

        <!-- Object-valued leaf -->
        <template v-else-if="property.classRef">
          <button
            v-if="binding(property).kind !== 'nested' && mode === 'template' && editable"
            type="button"
            class="btn btn-outline btn-xs"
            :data-testid="`add-nested-${localNameOf(property.path)}`"
            @click="addNested(property)"
          >
            Add {{ nestedClassLabel(property) }}
          </button>
          <span v-else-if="binding(property).kind !== 'nested'" class="text-sm opacity-60">—</span>
        </template>
      </div>

      <!-- Nested objects render as children below their parent's rows. -->
      <template v-for="property in shape.properties" :key="`nested-${property.path}`">
        <template v-if="property.classRef && binding(property).kind === 'nested'">
          <div class="flex items-start gap-1">
            <div class="grow">
              <DataObjectNode
                v-if="childObject((binding(property) as { childId: string }).childId)"
                :object="childObject((binding(property) as { childId: string }).childId)!"
                :classes="classes"
                :mode="mode"
                :editable="editable"
                :depth="depth + 1"
                :semantic-condition-values="semanticConditionValues"
                :set-semantic-condition-value="setSemanticConditionValue"
              />
            </div>
            <button
              v-if="mode === 'template' && editable"
              type="button"
              class="btn btn-ghost btn-xs"
              :data-testid="`remove-nested-${localNameOf(property.path)}`"
              @click="removeNested(property, (binding(property) as { childId: string }).childId)"
            >
              ✕
            </button>
          </div>
        </template>
      </template>
    </div>
  </div>
</template>
