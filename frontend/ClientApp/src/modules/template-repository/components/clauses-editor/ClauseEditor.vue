<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { computed, onMounted, ref, useId } from 'vue'
import ClauseTextEditor from '@template-repository/components/clauses-editor/ClauseTextEditor.vue'
import OdrlRuleBuilder from '@template-repository/components/clauses-editor/OdrlRuleBuilder.vue'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import {
  type HubAsset,
  ONTOLOGY_ASSETS,
  ONTOLOGY_DOMAIN_FIELDS,
  refreshOntologyDomainFields,
} from '@template-repository/utils/ontology-domain-fields'
import { type DcsContentSegment, localNameOf, type OdrlRule } from '@/models/dcs-jsonld'
import type { DomainFieldDefinition, SemanticCondition } from '@template-repository/models/contract-template'

/**
 * One clause, the SRS split editor: human prose with placeholders on the left,
 * its machine-readable ODRL meaning on the right. Both sides reference objects
 * picked from the Semantic Hub — data fields (flat domain vocabulary) and
 * assets (a shape's target class, e.g. an imported Gaia-X ServiceOffering,
 * whose properties become fields). A clause's meaning IS an ODRL rule; an asset
 * is what that rule targets.
 */

const store = useDcsDraftStore()
const { partyAnchors, contractTargetIri, contractFields } = storeToRefs(store)

// A schema registered in the hub after app startup becomes pickable here on
// the next mount; a failed refresh keeps the startup vocabulary.
onMounted(() => {
  refreshOntologyDomainFields().catch(() => undefined)
})

interface ClauseField {
  id: string
  field: DomainFieldDefinition
  /** Set when this field is a property of a declared asset. */
  assetLocalId?: string
}
interface ClauseAsset {
  id: string
  asset: HubAsset
  /** Instance name distinguishing same-class declarations ("Legal Person 2"),
   *  user-renamable — it names the instance in the target picker, on field
   *  chips, and on the declared contract fields. */
  name: string
  /** Accent color marking this instance's row, chips, and placeholders. */
  color: string
}

/** Deterministic, well-spread accent per declaration order (golden-angle hue). */
const assetAccent = (index: number) => `hsl(${(index * 137) % 360} 65% 45%)`

const title = ref('')
const content = ref<DcsContentSegment[]>([])
const clauseFields = ref<ClauseField[]>([])
const clauseAssets = ref<ClauseAsset[]>([])
const rule = ref<OdrlRule | null>(null)
const objectToAdd = ref('')

const titleId = useId()
const contentId = useId()
const objectToAddId = useId()

const uuid = () => `urn:uuid:${crypto.randomUUID()}`

// One picker of hub objects: an object may be an asset (a shape's target class,
// carrying properties) or a bare data field (a property). Its role — an ODRL
// target, a constraint operand — is decided by how it is wired into the rule,
// not by which list it came from. Assets are marked ▣ because they carry a
// shape whose properties come along.
const objectGroups = computed(() => {
  const groups = new Map<string, { value: string; label: string }[]>()
  const push = (source: string | undefined, option: { value: string; label: string }) => {
    const key = source ?? 'Semantic Hub'
    const group = groups.get(key)
    if (group) group.push(option)
    else groups.set(key, [option])
  }
  for (const asset of ONTOLOGY_ASSETS)
    push(asset.source?.name, { value: `asset:${asset.id}`, label: `▣ ${asset.label}` })
  for (const field of ONTOLOGY_DOMAIN_FIELDS)
    push(field.source?.name, { value: `field:${field.ontologyId}`, label: field.label })
  return [...groups.entries()].map(([name, entries]) => ({ name, entries }))
})

function addObject() {
  const picked = objectToAdd.value
  objectToAdd.value = ''
  if (picked.startsWith('asset:')) {
    const asset = ONTOLOGY_ASSETS.find((a) => a.id === picked.slice('asset:'.length))
    if (!asset) return
    // Declaring an asset makes it an ODRL target and brings in its shape's properties as fields.
    const assetLocalId = uuid()
    const sameClass = clauseAssets.value.filter((a) => a.asset.id === asset.id).length
    clauseAssets.value.push({
      id: assetLocalId,
      asset,
      name: sameClass ? `${asset.label} ${sameClass + 1}` : asset.label,
      color: assetAccent(clauseAssets.value.length),
    })
    for (const property of asset.properties) {
      clauseFields.value.push({ id: uuid(), field: property, assetLocalId })
    }
  } else if (picked.startsWith('field:')) {
    const field = ONTOLOGY_DOMAIN_FIELDS.find((f) => f.ontologyId === picked.slice('field:'.length))
    if (field) clauseFields.value.push({ id: uuid(), field })
  }
}

function removeField(id: string) {
  clauseFields.value = clauseFields.value.filter((cf) => cf.id !== id)
}
function removeAsset(assetLocalId: string) {
  clauseAssets.value = clauseAssets.value.filter((a) => a.id !== assetLocalId)
  clauseFields.value = clauseFields.value.filter((cf) => cf.assetLocalId !== assetLocalId)
}

/** The instance-qualified display label of a clause field: a declared
 *  asset's properties carry the instance name so two same-class assets stay
 *  tellable apart everywhere the field appears. A property named like its
 *  owner (gx:TaxID's schema:taxID) shows once, not "Tax ID · Tax ID". */
function fieldDisplayLabel(cf: ClauseField): string {
  const owner = ownerAsset(cf)
  if (!owner || owner.name === cf.field.label) return cf.field.label
  return `${owner.name} · ${cf.field.label}`
}

const assetFields = (assetLocalId: string) => clauseFields.value.filter((cf) => cf.assetLocalId === assetLocalId)
const standaloneFields = computed(() => clauseFields.value.filter((cf) => !cf.assetLocalId))

function ownerAsset(cf: ClauseField): ClauseAsset | undefined {
  return cf.assetLocalId ? clauseAssets.value.find((a) => a.id === cf.assetLocalId) : undefined
}

const proseConditions = computed<SemanticCondition[]>(() =>
  clauseFields.value.map((cf) => ({
    conditionId: cf.id,
    conditionName: fieldDisplayLabel(cf),
    accentColor: ownerAsset(cf)?.color,
    schemaVersion: 'v1',
    parameters: [
      {
        parameterName: cf.field.parameterName,
        fieldId: cf.id,
        fieldIri: cf.field.ontologyId,
        type: cf.field.type,
        isRequired: true,
        operators: [],
        value: undefined,
      },
    ],
  })),
)

/**
 * Every field a rule authored here may bind: this clause's own declarations
 * first, in declaration order, then the fields already declared elsewhere on
 * this document. A negotiated boundary is a document-level object — the
 * Service Credits clause caps a credit at the fee negotiated in the Charges
 * clause — so confining a rule's operands to its own paragraph would force
 * those boundaries back into fixed literals. Fields declared on a clause added
 * later are absent until that clause is saved; the list follows the store, so
 * it never goes stale afterwards.
 */
const fieldAnchors = computed(() => {
  const own = clauseFields.value.map((cf) => ({ id: cf.id, label: fieldDisplayLabel(cf) }))
  const ownIds = new Set(own.map((anchor) => anchor.id))
  const elsewhere = contractFields.value
    .filter((field) => !ownIds.has(field['@id']))
    .map((field) => ({ id: field['@id'], label: field['dcs:label'] || localNameOf(field['@id']) }))
  return [...own, ...elsewhere]
})
const assetAnchors = computed(() => clauseAssets.value.map((a) => ({ id: a.id, label: a.name })))

const canSave = computed(() => !!title.value.trim() && content.value.length > 0)

function save() {
  if (!canSave.value) return
  store.addClauseWithMeaning({
    title: title.value.trim(),
    content: content.value,
    fields: clauseFields.value.map((cf) => ({
      id: cf.id,
      parameterName: cf.field.parameterName,
      domainFieldIri: cf.field.ontologyId,
      label: fieldDisplayLabel(cf),
    })),
    // A declared asset is NOT a fillable field — it becomes a typed domain
    // object in dcs:contractData whose properties reference the declared
    // fields (ADR-23), and the ODRL target names that object.
    assets: clauseAssets.value.map((a) => ({
      id: a.id,
      classIri: a.asset.id,
      properties: clauseFields.value
        .filter((cf) => cf.assetLocalId === a.id)
        .map((cf) => ({ fieldId: cf.id, path: cf.field.ontologyId })),
    })),
    rule: rule.value,
  })
  title.value = ''
  content.value = []
  clauseFields.value = []
  clauseAssets.value = []
  rule.value = null
}
</script>

<template>
  <div class="space-y-3" data-testid="split-clause-editor">
    <label :for="titleId" class="sr-only">Clause title</label>
    <input
      :id="titleId"
      v-model="title"
      type="text"
      placeholder="Clause title"
      class="input-bordered input input-sm w-full"
    />

    <div class="space-y-2 rounded bg-base-200/50 p-2">
      <div class="flex flex-wrap items-center gap-2">
        <label :for="objectToAddId" class="text-xs text-base-content/80">Objects (Semantic Hub):</label>
        <select :id="objectToAddId" v-model="objectToAdd" class="select-bordered select select-xs" @change="addObject">
          <option value="">+ add object…</option>
          <optgroup v-for="group in objectGroups" :key="group.name" :label="group.name">
            <option v-for="o in group.entries" :key="o.value" :value="o.value">{{ o.label }}</option>
          </optgroup>
        </select>
        <span class="text-[10px] text-base-content/70">▣ = asset (carries a shape)</span>
      </div>

      <!-- One row per declared asset instance: its accent color marks the
           row, its field chips, and the matching prose placeholders. -->
      <div
        v-for="ca in clauseAssets"
        :key="ca.id"
        class="flex flex-wrap items-center gap-1 border-l-4 pl-2"
        :style="{ borderLeftColor: ca.color }"
        :data-asset-row="ca.id"
      >
        <span class="badge gap-1 badge-sm badge-primary" :title="ca.asset.label">
          ▣
          <input
            id="instance-name"
            v-model="ca.name"
            class="w-28 bg-transparent outline-none placeholder:text-primary-content/50"
            :placeholder="ca.asset.label"
            aria-label="Instance name"
          />
          <button type="button" class="text-primary-content/70" @click="removeAsset(ca.id)">✕</button>
        </span>
        <span
          v-for="cf in assetFields(ca.id)"
          :key="cf.id"
          class="badge gap-1 badge-outline badge-sm"
          :style="{ borderLeft: `3px solid ${ca.color}` }"
        >
          {{ cf.field.label }}
          <button type="button" class="text-error" @click="removeField(cf.id)">✕</button>
        </span>
      </div>
      <div v-if="standaloneFields.length" class="flex flex-wrap items-center gap-1">
        <span v-for="cf in standaloneFields" :key="cf.id" class="badge gap-1 badge-outline badge-sm">
          {{ cf.field.label }}
          <button type="button" class="text-error" @click="removeField(cf.id)">✕</button>
        </span>
      </div>
    </div>

    <div class="grid grid-cols-1 gap-3 lg:grid-cols-2">
      <div class="rounded border border-base-300 p-3">
        <h3 :id="contentId" class="mb-2 text-xs font-semibold text-base-content/70">Human prose</h3>
        <ClauseTextEditor
          :text-id="contentId"
          :model-value="content"
          :semantic-conditions="proseConditions"
          @update:model-value="content = $event"
        />
      </div>
      <div class="rounded border border-base-300 p-3">
        <h3 class="mb-2 text-xs font-semibold text-base-content/70">Machine-readable meaning (ODRL)</h3>
        <OdrlRuleBuilder
          v-model="rule"
          :fields="fieldAnchors"
          :assets="assetAnchors"
          :parties="partyAnchors"
          prose-id=""
          :contract-target-id="contractTargetIri"
        />
      </div>
    </div>

    <div class="flex justify-end">
      <button type="button" class="btn btn-sm btn-primary" :disabled="!canSave" @click="save">Add clause</button>
    </div>
  </div>
</template>
