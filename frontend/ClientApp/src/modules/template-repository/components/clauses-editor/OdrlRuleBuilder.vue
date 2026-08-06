<script setup lang="ts">
import { computed, nextTick, reactive, ref, toRaw, watch } from 'vue'
import {
  composeConstraintTree,
  type GroupDraft,
  newGroup,
  parseConstraintTree,
} from '@template-repository/components/clauses-editor/constraint-draft'
import ConstraintGroupEditor from '@template-repository/components/clauses-editor/ConstraintGroupEditor.vue'
import IriPicker from '@template-repository/components/clauses-editor/IriPicker.vue'
import { ODRL_ACTIONS, ODRL_RULE_TYPES } from '@template-repository/utils/odrl-vocabulary'
import { type OdrlDuty, type OdrlRule } from '@/models/dcs-jsonld'

/**
 * The machine-readable meaning of a clause: an ODRL rule. Reference terms —
 * action, assigner, assignee, target — are picked from the DCS vocabulary or
 * given as a custom IRI (ODRL sets no closed vocabulary on them). A rule is one
 * permission/prohibition/obligation — who (assignee) may/must-not/must do what
 * (action) toward whom (target), bounded by a constraint tree (ODRL IM §2.6).
 * A Permission may carry duties, each an obligation with its own constraint
 * tree and optional consequence. The SRS Appendix C access policy is a
 * permission to use an asset bounded by spatial and dateTime constraints.
 */

export interface Anchor {
  /** The IRI the document references — generated elsewhere, hidden here. */
  id: string
  label: string
}

const props = withDefaults(
  defineProps<{
    modelValue: OdrlRule | null
    /** Data fields declared for this clause (document constraint left operands). */
    fields: Anchor[]
    /** Assets declared for this clause (targetable objects — an ODRL rule's target). */
    assets?: Anchor[]
    /** Parties the rule can bind (assigner/assignee/target). */
    parties: Anchor[]
    /** The prose block this rule is backed by (dcs:prose). */
    proseId: string
    /** The contract/asset IRI the rule targets by default. */
    contractTargetId: string
  }>(),
  { assets: () => [] },
)

const emit = defineEmits<{ 'update:modelValue': [OdrlRule | null] }>()

const actionOptions = ODRL_ACTIONS.map((a) => ({ value: a.id, label: a.label }))
const partyOptions = computed(() => props.parties.map((p) => ({ value: p.id, label: p.label })))
const targetOptions = computed(() => [{ value: props.contractTargetId, label: 'the contract' }])
const targetGroups = computed(() => {
  const groups: { label: string; options: { value: string; label: string }[] }[] = []
  if (props.assets.length) {
    groups.push({ label: 'Asset', options: props.assets.map((a) => ({ value: a.id, label: a.label })) })
  }
  if (props.fields.length) {
    groups.push({ label: 'Data field', options: props.fields.map((f) => ({ value: f.id, label: f.label })) })
  }
  groups.push({ label: 'Parties', options: props.parties.map((p) => ({ value: p.id, label: p.label })) })
  return groups
})

/** A Duty the assignee must fulfil to exercise a Permission (ODRL IM §2.5): its
 *  own action, a constraint tree, and consequence duties triggered when unmet. */
interface DutyDraft {
  action: string
  root: GroupDraft
  consequences: { action: string; root: GroupDraft }[]
}

interface RuleDraft {
  type: string
  actions: string[]
  assigneeId: string
  assignerId: string
  targetId: string
  root: GroupDraft
  duties: DutyDraft[]
}

/** The @ids a constraint may reference as a negotiated boundary or unit. */
function declaredFieldIds(): string[] {
  return props.fields.map((field) => field.id)
}

function seed(rule: OdrlRule | null): RuleDraft {
  return {
    type: rule?.['@type'] ?? ODRL_RULE_TYPES[0]?.type ?? 'odrl:Permission',
    actions: readActions(rule),
    assigneeId: rule?.['odrl:assignee']?.['@id'] ?? '',
    assignerId: rule?.['odrl:assigner']?.['@id'] ?? '',
    targetId: rule?.['odrl:target']?.['@id'] ?? props.contractTargetId,
    root: parseConstraintTree(rule?.['odrl:constraint'] ?? [], declaredFieldIds()),
    duties: readDuties(rule),
  }
}

const draft = reactive<RuleDraft>(seed(props.modelValue))

// Not read inside the emitting `rule` computed as props.modelValue would be:
// that would make each emit retrigger the computed (a reactive feedback loop).
const ruleId = ref(props.modelValue?.['@id'] ?? `urn:uuid:${crypto.randomUUID()}`)

const complete = computed(() => draft.actions.some((a) => !!a) && !!draft.assignerId && !!draft.assigneeId)

function readActions(rule: OdrlRule | null): string[] {
  const action = rule?.['odrl:action']
  if (!action) return [ODRL_ACTIONS[0]?.id ?? 'odrl:use']
  const list = Array.isArray(action) ? action : [action]
  const ids = list.map((a) => a['@id']).filter(Boolean)
  return ids.length ? ids : [ODRL_ACTIONS[0]?.id ?? 'odrl:use']
}

function firstActionId(action: OdrlDuty['odrl:action']): string {
  const first = Array.isArray(action) ? action[0] : action
  return first?.['@id'] ?? ODRL_ACTIONS[0]?.id ?? 'odrl:use'
}

function addAction() {
  draft.actions.push(ODRL_ACTIONS[0]?.id ?? 'odrl:use')
}
function removeAction(index: number) {
  draft.actions.splice(index, 1)
}

// A duty is a fragment: its own action, a constraint tree, and consequence
// duties (each again an action + constraint tree, ODRL IM §2.5).
function readDuties(rule: OdrlRule | null): DutyDraft[] {
  return (rule?.['odrl:duty'] ?? []).map((duty) => ({
    action: firstActionId(duty['odrl:action']),
    root: parseConstraintTree(duty['odrl:constraint'] ?? [], declaredFieldIds()),
    consequences: (duty['odrl:consequence'] ?? []).map((consequence) => ({
      action: firstActionId(consequence['odrl:action']),
      root: parseConstraintTree(consequence['odrl:constraint'] ?? [], declaredFieldIds()),
    })),
  }))
}

function addDuty() {
  draft.duties.push({ action: ODRL_ACTIONS[0]?.id ?? 'odrl:use', root: newGroup(), consequences: [] })
}
function removeDuty(index: number) {
  draft.duties.splice(index, 1)
}
function addConsequence(dutyIndex: number) {
  draft.duties[dutyIndex]?.consequences.push({ action: ODRL_ACTIONS[0]?.id ?? 'odrl:use', root: newGroup() })
}
function removeConsequence(dutyIndex: number, index: number) {
  draft.duties[dutyIndex]?.consequences.splice(index, 1)
}

function buildDuty(action: string, root: GroupDraft, consequences: DutyDraft['consequences'] = []): OdrlDuty {
  const duty: OdrlDuty = { '@type': 'odrl:Duty', 'odrl:action': { '@id': action } }
  const constraints = composeConstraintTree(root)
  if (constraints) duty['odrl:constraint'] = constraints
  const built = consequences.filter((c) => c.action).map((c) => buildDuty(c.action, c.root))
  if (built.length) duty['odrl:consequence'] = built
  return duty
}

const rule = computed<OdrlRule | null>(() => {
  if (!complete.value) return null
  const actions = draft.actions.filter(Boolean)
  const built: OdrlRule = {
    '@id': ruleId.value,
    '@type': draft.type as OdrlRule['@type'],
    'odrl:action': actions.length === 1 ? { '@id': actions[0] ?? '' } : actions.map((a) => ({ '@id': a })),
    'odrl:assigner': { '@id': draft.assignerId },
    'odrl:assignee': { '@id': draft.assigneeId },
    'odrl:target': { '@id': draft.targetId || props.contractTargetId },
    'dcs:prose': { '@id': props.proseId },
  }
  const ruleConstraints = composeConstraintTree(draft.root)
  if (ruleConstraints) built['odrl:constraint'] = ruleConstraints
  // Duties attach only to a Permission (ODRL IM §2.5): the obligations the
  // assignee must fulfil to exercise it.
  if (draft.type === 'odrl:Permission') {
    const duties = draft.duties.filter((d) => d.action).map((d) => buildDuty(d.action, d.root, d.consequences))
    if (duties.length) built['odrl:duty'] = duties
  }
  return built
})

// The builder outlives the clause it authors: the host clears the bound rule
// when a clause is saved, and a draft kept from the previous clause would bind
// fields the next clause never declares. `reseeding` holds until the flush that
// the reseed schedules is over, so the pristine draft is not emitted straight
// back as a new rule.
let lastEmitted: OdrlRule | null = props.modelValue
let reseeding = false

watch(rule, (value) => {
  if (reseeding) return
  lastEmitted = value
  emit('update:modelValue', value)
})

// Only a modelValue this builder did not itself emit is an external one worth
// reseeding from: an in-progress rule that momentarily reads incomplete emits
// null too, and must keep the draft the user is still editing. A host holding
// the rule in a ref hands it back wrapped in a reactive proxy, so the rule this
// builder emitted is recognised by its raw target, not by the reference.
watch(
  () => props.modelValue,
  (value) => {
    if (toRaw(value) === toRaw(lastEmitted)) return
    lastEmitted = value
    ruleId.value = value?.['@id'] ?? `urn:uuid:${crypto.randomUUID()}`
    Object.assign(draft, seed(value))
    reseeding = true
    void nextTick(() => {
      reseeding = false
    })
  },
)
</script>

<template>
  <div class="space-y-3 text-xs">
    <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
      <label class="form-control">
        <span class="label-text text-xs">Rule</span>
        <select id="draft-type-rule" v-model="draft.type" class="select-bordered select select-sm">
          <option v-for="rt in ODRL_RULE_TYPES" :key="rt.type" :value="rt.type">{{ rt.label }}</option>
        </select>
      </label>
      <label class="form-control">
        <span class="label-text text-xs">Action(s)</span>
        <div class="flex flex-col gap-1">
          <div v-for="(_, i) in draft.actions" :key="i" class="flex items-center gap-1">
            <IriPicker
              :model-value="draft.actions[i] ?? ''"
              :options="actionOptions"
              @update:model-value="draft.actions[i] = $event"
            />
            <button v-if="draft.actions.length > 1" type="button" class="btn btn-ghost btn-xs" @click="removeAction(i)">
              ✕
            </button>
          </div>
          <button type="button" class="btn w-fit btn-ghost btn-xs" @click="addAction">+ action</button>
        </div>
      </label>
      <label class="form-control">
        <span class="label-text text-xs">Granted by (assigner)</span>
        <IriPicker v-model="draft.assignerId" :options="partyOptions" :allow-custom="false" />
      </label>
      <label class="form-control">
        <span class="label-text text-xs">Applies to (assignee)</span>
        <IriPicker v-model="draft.assigneeId" :options="partyOptions" :allow-custom="false" />
      </label>
      <label class="form-control">
        <span class="label-text text-xs">Toward (target)</span>
        <IriPicker v-model="draft.targetId" :options="targetOptions" :groups="targetGroups" placeholder="asset IRI" />
      </label>
    </div>

    <div class="space-y-2">
      <span class="label-text text-xs font-semibold">Constraints</span>
      <ConstraintGroupEditor v-model="draft.root" :fields="fields" combine-title="How the constraints combine" />
    </div>

    <!-- Duties: obligations the assignee must fulfil to exercise a Permission
         (ODRL IM §2.5) — each its own action, constraint tree, and consequence. -->
    <div v-if="draft.type === 'odrl:Permission'" class="space-y-2">
      <div class="flex items-center justify-between">
        <span class="label-text text-xs font-semibold">Duties (must fulfil to exercise)</span>
        <button type="button" class="btn btn-ghost btn-xs" @click="addDuty">+ duty</button>
      </div>
      <div
        v-for="(duty, di) in draft.duties"
        :key="di"
        data-testid="odrl-duty"
        class="space-y-1 rounded border border-base-300 p-2"
      >
        <div class="flex items-center gap-1">
          <span class="label-text text-xs">must</span>
          <IriPicker :model-value="duty.action" :options="actionOptions" @update:model-value="duty.action = $event" />
          <button type="button" class="btn ml-auto btn-ghost btn-xs" @click="removeDuty(di)">remove duty</button>
        </div>
        <ConstraintGroupEditor
          v-model="duty.root"
          :fields="fields"
          combine-title="How the duty's constraints combine"
        />

        <!-- Consequence: a duty triggered when this duty is not fulfilled. -->
        <div
          v-for="(consequence, ci) in duty.consequences"
          :key="ci"
          data-testid="odrl-consequence"
          class="ml-3 space-y-1 rounded border border-dashed border-warning/40 p-1"
        >
          <div class="flex items-center gap-1">
            <span class="label-text text-2xs opacity-70">if unmet, must</span>
            <IriPicker
              :model-value="consequence.action"
              :options="actionOptions"
              @update:model-value="consequence.action = $event"
            />
            <button type="button" class="btn ml-auto btn-ghost btn-xs" @click="removeConsequence(di, ci)">✕</button>
          </div>
          <ConstraintGroupEditor
            v-model="consequence.root"
            :fields="fields"
            combine-title="How the consequence's constraints combine"
          />
        </div>
        <button type="button" class="btn btn-ghost btn-xs" @click="addConsequence(di)">+ consequence</button>
      </div>
    </div>
  </div>
</template>
