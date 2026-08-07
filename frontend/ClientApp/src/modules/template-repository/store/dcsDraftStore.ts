import { defineStore } from 'pinia'
import { TemplateType } from '@template-repository/models/contract-template'
import {
  CONTRACT_PARTY_ROLE_OPTIONS,
  ONTOLOGY_ASSETS,
  ONTOLOGY_DOMAIN_FIELDS,
} from '@template-repository/utils/ontology-domain-fields'
import { DCS_ODRL_PROFILE_IRI, DEFAULT_FIELD_CONSTRAINT_ACTION } from '@template-repository/utils/sla-ontology-catalog'
import { applyInlineSemanticValues } from '@contract-workflow-engine/utils/semantic-condition-values'
import {
  type DcsBlock,
  type DcsContentSegment,
  type DcsContractData,
  type DcsContractDataObject,
  type DcsContractField,
  type DcsDocumentData,
  type DcsDocumentStructure,
  type DcsLayoutNode,
  type DcsTemplateData,
  fieldFillScalar,
  isAtomicConstraint,
  isDcsClause,
  isDcsDocumentData,
  isDcsTemplateData,
  type JsonLdReference,
  type JsonLdTypedValue,
  type OdrlConstraint,
  type OdrlConstraintNode,
  type OdrlDuty,
  type OdrlRule,
  type OdrlSet,
  typedFieldFill,
  type XsdDatatype,
} from '@/models/dcs-jsonld'
import { declaredPartyRoles } from '@/utils/participant-selection'
import type { SemanticConditionValue } from '@/models/contract/contract-data'
import type { ContractTemplate } from '@/models/contract-template/contract-template'
import type { ContractTemplateResponsible } from '@/models/contract-template/contract-template-responsible'
import type {
  ContractTemplateCreateRequest,
  ContractTemplateUpdateManageRequest,
  ContractTemplateUpdateRequest,
} from '@/models/requests/template-request'
import type { DcsOperator } from '@/models/semantic/facis-dcs-semantic'
import type { ContractTemplateState } from '@/types/contract-template-state'
import type {
  DomainFieldDefinition,
  MetaData,
  SemanticCondition,
  SemanticConditionParameter,
  SemanticParameterOperator,
  TemplateTypeValue,
} from '@template-repository/models/contract-template'
import type {
  AddBlockOptions,
  AddBlockPayload,
  TemplateDraftState,
} from '@template-repository/models/template-draft-store'

const storeId = 'dcsDraft'

export interface LoadDocumentMeta {
  did: string
  name: string
  description: string
  templateType?: TemplateTypeValue
  state?: ContractTemplateState | null
  version?: number | null
  updated_at?: string | null
  created_by?: string
  responsible?: ContractTemplateResponsible | null
}

export const useDcsDraftStore = defineStore(storeId, {
  state: (): TemplateDraftState => getInitialState(),
  getters: {
    hasTemplateId(): boolean {
      return !!this.did
    },
    blockIdsInOutline(): Set<string> {
      return collectBlockIdsInLayout(this.layout)
    },
    /** Enriched semantic conditions derived from stored JSON-LD contractData + policies. */
    semanticConditions(): SemanticCondition[] {
      return contractFieldsToSemanticConditions(this.contractFields, this.policies)
    },
    /** Parties a clause rule can bind (assigner/assignee/target), by label. */
    partyAnchors(): { id: string; label: string }[] {
      const documentId = this.documentIri ?? this.did ?? undefined
      return CONTRACT_PARTY_ROLE_OPTIONS.map((role) => ({
        id: objectIri('party', role.value, documentId),
        label: role.label,
      }))
    },
    /** The contract/asset IRI an ODRL rule targets. */
    contractTargetIri(): string {
      return targetReference(this.documentIri ?? this.did ?? undefined)['@id']
    },
    /** Assembles the canonical JSON-LD document from store state — no conversion needed. */
    templateDocument(): DcsTemplateData {
      return assembleCanonicalDocument({
        documentType: 'dcs:ContractTemplate',
        documentId: this.documentIri ?? this.did ?? undefined,
        name: this.name,
        description: this.description,
        templateType: this.templateType,
        blocks: this.blocks,
        layout: this.layout,
        contractFields: this.contractFields,
        contractData: this.contractData,
        policies: this.policies,
        customMetaData: this.customMetaData,
      }) as DcsTemplateData
    },
    templateCreateRequestData(): ContractTemplateCreateRequest {
      return {
        name: this.name,
        description: this.description,
        template_type: this.templateType,
        template_data: this.templateDocument,
      }
    },
    templateUpdateRequestData(): ContractTemplateUpdateRequest | null {
      if (!this.did || !this.updated_at) return null
      return {
        did: this.did,
        updated_at: this.updated_at,
        name: this.name,
        description: this.description,
        template_data: this.templateDocument,
      }
    },
    templateUpdateManageRequestData(): ContractTemplateUpdateManageRequest | null {
      if (!this.did || !this.updated_at) return null
      return {
        did: this.did,
        updated_at: this.updated_at,
        template_type: this.templateType,
        name: this.name,
        description: this.description,
        template_data: this.templateDocument,
      }
    },
  },
  actions: {
    /** Loads the canonical JSON-LD envelope plus DB-level metadata into store state. */
    loadDocument(rawDoc: unknown, meta: LoadDocumentMeta): void {
      const templateType = meta.templateType ?? TemplateType.component

      if (isDcsTemplateData(rawDoc)) {
        const metadata = rawDoc['dcs:metadata']
        const jsonLdType = metadata['dcs:templateType']
        const derivedTemplateType: TemplateTypeValue =
          jsonLdType === 'dcs:ContractTemplate' ? TemplateType.contractTemplate : TemplateType.component
        const structure = rawDoc['dcs:documentStructure']

        this.reset({
          did: meta.did,
          documentIri: rawDoc['@id'] ?? null,
          name: meta.name ? meta.name : (metadata['dcs:title'] ?? ''),
          description: meta.description ? meta.description : (metadata['dcs:description'] ?? ''),
          templateType: templateType !== TemplateType.component ? templateType : derivedTemplateType,
          state: meta.state ?? undefined,
          version: meta.version ?? null,
          updated_at: meta.updated_at ?? null,
          created_by: meta.created_by ?? '',
          responsible: meta.responsible ?? null,
          blocks: extractBlockList(structure['dcs:blocks']),
          layout: extractLayoutList(structure['dcs:layout']).length
            ? extractLayoutList(structure['dcs:layout'])
            : getInitialLayout(),
          contractFields: rawDoc['dcs:contractFields'],
          contractData: rawDoc['dcs:contractData'],
          policies: flattenPolicySet(rawDoc['dcs:policies']),
          customMetaData: (metadata['dcs:customMetaData'] as MetaData[]) ?? [],
        })
        return
      }

      // Unknown / empty format — start fresh
      this.reset({
        did: meta.did,
        name: meta.name,
        description: meta.description,
        templateType,
        state: meta.state ?? undefined,
        version: meta.version ?? null,
        updated_at: meta.updated_at ?? null,
        created_by: meta.created_by ?? '',
        responsible: meta.responsible ?? null,
      })
    },
    addBlock(parentBlockId: string, insertIndex: number, payload: AddBlockPayload, options?: AddBlockOptions): string {
      return addBlock(this.layout, this.blocks, parentBlockId, insertIndex, payload, options, this.did ?? undefined)
    },
    /**
     * Flatten-on-compose: inlines an approved component's blocks, top-level
     * placeholders and ODRL policies directly into this document with fresh
     * unique @ids, splicing the component's top-level blocks into the target
     * parent at insertIndex. The result stays a self-contained document — no
     * reference block, no sub-template snapshot.
     */
    inlineComponent(component: ContractTemplate, parentBlockId: string, insertIndex: number): void {
      const templateData = component.template_data
      if (!isDcsTemplateData(templateData)) {
        throw new Error(`component ${component.did} has no document structure to inline`)
      }
      const inlined = inlineComponentDocument(templateData, this.did ?? undefined)
      this.blocks.push(...inlined.blocks)
      this.layout.push(...inlined.layoutNodes)
      this.contractFields.push(...inlined.contractFields)
      this.contractData.push(...inlined.contractData)
      this.policies.push(...inlined.policies)

      const parent = this.layout.find((n) => n['@id'] === parentBlockId)
      if (!parent) throw new Error(`inlineComponent: parent not found: ${parentBlockId}`)
      const children = parent['dcs:children']['@list'].map((r) => r['@id'])
      children.splice(insertIndex, 0, ...inlined.rootChildIds)
      parent['dcs:children'] = { '@list': children.map((id) => ({ '@id': id })) }
    },
    deleteBlock(blockId: string): void {
      const before = this.blocks.slice()
      deleteBlock(this.layout, this.blocks, blockId)
      this.withdrawRemovedBlockDeclarations(before)
    },
    /** What removed prose takes with it: the rules it backed and the field
     *  declarations nothing binds any more. A rule outliving its `dcs:prose`
     *  is refused by the hub shapes, and an unbound required declaration
     *  blocks the offer over a placeholder the document no longer shows —
     *  the same closedness removeDataObject keeps for a leaf's field. */
    withdrawRemovedBlockDeclarations(before: DcsBlock[]): void {
      const remaining = new Set(this.blocks.map((b) => b['@id']))
      const removed = before.filter((b) => !remaining.has(b['@id']))
      if (removed.length === 0) return

      this.policies = this.policies.filter((policy) => {
        const prose = policy['dcs:prose']?.['@id']
        return !prose || remaining.has(prose)
      })

      const declared = new Set(this.contractFields.map((field) => field['@id']))
      const withdrawn = new Set<string>()
      collectFieldRefs(removed, declared, withdrawn)
      if (withdrawn.size === 0) return

      const stillBound = new Set<string>()
      collectFieldRefs(this.blocks, declared, stillBound)
      collectFieldRefs(this.policies, declared, stillBound)
      collectFieldRefs(this.contractData, declared, stillBound)
      this.contractFields = this.contractFields.filter(
        (field) => !withdrawn.has(field['@id']) || stillBound.has(field['@id']),
      )
    },
    updateBlock(
      blockId: string,
      payload: {
        title?: string
        text?: string
        content?: DcsContentSegment[]
      },
    ): void {
      const block = this.blocks.find((b) => b['@id'] === blockId)
      if (!block) return
      if (isDcsClause(block)) {
        if (payload.title !== undefined) block['dcs:title'] = payload.title || undefined
        if (payload.content !== undefined) block['dcs:content'] = { '@list': payload.content }
      } else if (block['@type'] === 'dcs:TextBlock') {
        if (payload.text !== undefined) block['dcs:text'] = payload.text
      } else if (block['@type'] === 'dcs:Section') {
        if (payload.title !== undefined) block['dcs:title'] = payload.title || undefined
      }
    },
    moveBlock(blockId: string, parentBlockId: string, insertIndex: number): void {
      moveBlock(this.layout, blockId, parentBlockId, insertIndex)
    },
    updateFieldPolicies(
      fieldId: string,
      conditionId: string,
      parameterName: string,
      parameterType: SemanticConditionParameter['type'],
      operators: SemanticParameterOperator[],
    ): void {
      const documentId = this.documentIri ?? this.did ?? undefined
      const partyRoles = declaredPartyRoles({ 'dcs:policies': this.policies })
      if (partyRoles.length !== 2) return
      this.policies = this.policies.filter((p) => !ruleLeftOperands(p).includes(fieldId))
      operators.forEach((operator, index) => {
        if (!isStandardOdrlOperator(operator.operate)) return
        const rightOperand = odrlRightOperand(operator, parameterType)
        this.policies.push({
          '@id': policyIri(conditionId, parameterName, index, documentId),
          '@type': 'odrl:Duty',
          'odrl:action': { '@id': DEFAULT_FIELD_CONSTRAINT_ACTION },
          'odrl:assigner': partyReference(partyRoles[0]!, documentId),
          'odrl:assignee': partyReference(partyRoles[1]!, documentId),
          'odrl:target': targetReference(documentId),
          'dcs:prose': proseBlockForField(this.blocks, fieldId),
          'odrl:constraint': [
            {
              '@type': 'odrl:Constraint',
              'odrl:leftOperand': { '@id': fieldId },
              'odrl:operator': { '@id': operator.operate },
              ...(rightOperand !== undefined ? { 'odrl:rightOperand': rightOperand } : {}),
            },
          ],
        } satisfies OdrlRule)
      })
    },
    /**
     * Clicks a typed domain object into dcs:contractData from a hub shape
     * class (ADR-23 graph authoring). Property keys and @type use absolute
     * IRIs — external vocabularies carry no prefix in the document context.
     * Returns the new object's @id.
     */
    addDataObject(classIri: string): string {
      const documentId = this.documentIri ?? this.did ?? undefined
      const id = objectIri('object', crypto.randomUUID(), documentId)
      this.contractData.push({ '@id': id, '@type': classIri })
      return id
    },
    /** Adds a nested typed object and links it from the parent's property. */
    addNestedDataObject(parentId: string, path: string, classIri: string): string {
      const parent = this.contractData.find((object) => object['@id'] === parentId)
      if (!parent) return ''
      const id = this.addDataObject(classIri)
      parent[path] = { '@id': id }
      return id
    },
    /** Sets a fixed literal on a domain-object property, serialized as a
     *  typed {@value} literal of the property's datatype (a compact xsd
     *  term, or an external library's exact datatype IRI). xsd:string emits
     *  the bare form — RDF 1.1's simple literal — so strict term-comparing
     *  validators match it against a library's plain sh:in members. */
    setDataObjectLiteral(objectId: string, path: string, value: string, datatype: string): void {
      const object = this.contractData.find((entry) => entry['@id'] === objectId)
      if (!object) return
      if (value === '') delete object[path]
      else object[path] = datatype === 'xsd:string' ? value : typedFieldFill(value, datatype)
    },
    /** Sets a fixed IRI value on a domain-object property (an sh:nodeKind
     *  sh:IRI leaf naming an external resource). */
    setDataObjectIri(objectId: string, path: string, iri: string): void {
      const object = this.contractData.find((entry) => entry['@id'] === objectId)
      if (!object) return
      if (iri === '') delete object[path]
      else object[path] = { '@id': iri }
    },
    /**
     * Makes a domain-object leaf negotiable: declares a dcs:ContractField
     * (the negotiation/fill/closedness machinery picks it up from there)
     * and binds the property to it by @id. Returns the field id.
     */
    makeDataLeafNegotiable(
      objectId: string,
      path: string,
      label: string,
      datatype: XsdDatatype,
      required: boolean,
      allowedValues?: readonly string[],
    ): string {
      const object = this.contractData.find((entry) => entry['@id'] === objectId)
      if (!object) return ''
      const documentId = this.documentIri ?? this.did ?? undefined
      const fieldId = objectIri('field', crypto.randomUUID(), documentId)
      this.contractFields.push({
        '@id': fieldId,
        '@type': 'dcs:ContractField',
        'dcs:label': label,
        'dcs:datatype': datatype,
        'dcs:required': required,
        ...(allowedValues?.length ? { 'dcs:valueConstraint': { allowedValues } } : {}),
      })
      object[path] = { '@id': fieldId }
      return fieldId
    },
    /** Reverts a negotiable leaf to fixed authoring: unbinds the property
     *  and withdraws the field declaration. */
    makeDataLeafFixed(objectId: string, path: string): void {
      const object = this.contractData.find((entry) => entry['@id'] === objectId)
      if (!object) return
      const ref = object[path]
      delete object[path]
      const fieldId = typeof ref === 'object' && ref !== null && !Array.isArray(ref) && '@id' in ref ? ref['@id'] : ''
      if (fieldId && this.contractFields.some((field) => field['@id'] === fieldId)) {
        this.contractFields = this.contractFields.filter((field) => field['@id'] !== fieldId)
      }
    },
    /** Removes a domain object, its nested objects, the fields its leaves
     *  declared, and every reference to any of them — the graph stays
     *  closed (validateContractDataGraph rejects dangling references). */
    removeDataObject(objectId: string): void {
      const removedObjects = new Set<string>()
      const collect = (id: string) => {
        if (removedObjects.has(id)) return
        removedObjects.add(id)
        const object = this.contractData.find((entry) => entry['@id'] === id)
        if (!object) return
        for (const [property, value] of Object.entries(object)) {
          if (property.startsWith('@')) continue
          const members = Array.isArray(value) ? value : [value]
          for (const member of members) {
            if (typeof member === 'object' && member !== null && '@id' in member) {
              const target = member['@id']
              if (this.contractData.some((entry) => entry['@id'] === target)) collect(target)
            }
          }
        }
      }
      collect(objectId)
      const removedFields = new Set<string>()
      for (const id of removedObjects) {
        const object = this.contractData.find((entry) => entry['@id'] === id)
        if (!object) continue
        for (const [property, value] of Object.entries(object)) {
          if (property.startsWith('@')) continue
          const members = Array.isArray(value) ? value : [value]
          for (const member of members) {
            if (typeof member === 'object' && member !== null && '@id' in member) {
              const target = member['@id']
              if (this.contractFields.some((field) => field['@id'] === target)) removedFields.add(target)
            }
          }
        }
      }
      this.contractData = this.contractData.filter((entry) => !removedObjects.has(entry['@id']))
      this.contractFields = this.contractFields.filter((field) => !removedFields.has(field['@id']))
    },
    addSemanticCondition(payload: Omit<SemanticCondition, 'conditionId'>): void {
      const conditionId = crypto.randomUUID()
      const documentId = this.documentIri ?? this.did ?? undefined
      const fields = payload.parameters.map((p) => semanticParamToContractField(conditionId, p, documentId))
      this.contractFields.push(...fields)
      this.contractData.push(semanticConditionToContractData({ ...payload, conditionId }, fields, documentId))
      this.policies.push(
        ...semanticConditionToPolicies(
          { ...payload, conditionId },
          this.contractFields,
          this.blocks,
          documentId,
          declaredPartyRoles({ 'dcs:policies': this.policies }),
        ),
      )
    },
    updateSemanticCondition(conditionId: string, payload: Omit<SemanticCondition, 'conditionId'>): void {
      const documentId = this.documentIri ?? this.did ?? undefined
      const oldFieldIds = new Set(
        this.contractFields.filter((field) => field['@id'].includes(conditionId)).map((field) => field['@id']),
      )
      if (oldFieldIds.size === 0) return
      const fields = payload.parameters.map((p) => semanticParamToContractField(conditionId, p, documentId))
      this.contractFields = [...this.contractFields.filter((field) => !oldFieldIds.has(field['@id'])), ...fields]
      this.contractData = [
        ...this.contractData.filter((object) => object['@id'] !== contractDataObjectIri(conditionId, documentId)),
        semanticConditionToContractData({ ...payload, conditionId }, fields, documentId),
      ]
      this.policies = this.policies.filter((p) => !ruleLeftOperands(p).some((op) => oldFieldIds.has(op)))
      this.policies.push(
        ...semanticConditionToPolicies(
          { ...payload, conditionId },
          this.contractFields,
          this.blocks,
          documentId,
          declaredPartyRoles({ 'dcs:policies': this.policies }),
        ),
      )
    },
    deleteSemanticCondition(conditionId: string): void {
      const fieldIds = new Set(
        this.contractFields.filter((field) => field['@id'].includes(conditionId)).map((field) => field['@id']),
      )
      if (fieldIds.size === 0) return

      // Remove field references from clause blocks.
      for (const block of this.blocks) {
        if (block['@type'] !== 'dcs:Clause') continue
        const clause = block
        const content = clause['dcs:content']
        if (typeof content === 'string') continue
        clause['dcs:content'] = {
          '@list': content['@list'].filter((seg) => typeof seg === 'string' || !fieldIds.has(seg['@id'])),
        }
      }

      this.contractFields = this.contractFields.filter((field) => !fieldIds.has(field['@id']))
      this.contractData = this.contractData.filter(
        (object) => object['@id'] !== contractDataObjectIri(conditionId, this.documentIri ?? this.did ?? undefined),
      )
      this.policies = this.policies.filter((p) => !ruleLeftOperands(p).some((op) => fieldIds.has(op)))
    },
    /** Adds a clause as prose + its machine-readable ODRL rule (linked by
     *  dcs:prose), declaring the hub fields the rule constrains as requirement
     *  fields — one clause, both readings, exactly as the SRS split editor. */
    addClauseWithMeaning(payload: {
      title: string
      content: DcsContentSegment[]
      fields: { id: string; parameterName: string; domainFieldIri: string; label?: string }[]
      /** Declared assets: each becomes a typed dcs:contractData object whose
       *  properties reference the declared fields — the ODRL target names
       *  the object, never a pseudo-field (ADR-23). */
      assets?: { id: string; classIri: string; properties: { fieldId: string; path: string }[] }[]
      rule: OdrlRule | null
    }): void {
      const blockId = this.addClause({ title: payload.title, content: payload.content })
      // A field brought in with a declared asset is described by that class's
      // property shape, not by a flat ontology domain field.
      const assetClassByFieldId = new Map<string, string>()
      for (const asset of payload.assets ?? []) {
        for (const property of asset.properties) assetClassByFieldId.set(property.fieldId, asset.classIri)
      }
      for (const f of payload.fields) {
        this.contractFields.push(
          contractFieldFromDomainField(f.id, f.parameterName, f.domainFieldIri, f.label, assetClassByFieldId.get(f.id)),
        )
      }
      for (const asset of payload.assets ?? []) {
        this.contractData.push({
          '@id': asset.id,
          '@type': asset.classIri,
          ...Object.fromEntries(asset.properties.map((p) => [p.path, { '@id': p.fieldId }])),
        })
      }
      if (payload.rule) {
        this.policies.push(bindRuleProse(payload.rule, blockId))
      }
    },
    addClause(payload: { title?: string; content: DcsContentSegment[] }): string {
      const blockId = crypto.randomUUID()
      const id = blockIri(blockId, this.did ?? undefined)
      const block: import('@/models/dcs-jsonld').DcsClause = {
        '@type': 'dcs:Clause',
        '@id': id,
        'dcs:content': { '@list': payload.content },
        ...(payload.title ? { 'dcs:title': payload.title } : {}),
      }
      this.blocks.push(block)
      return id
    },
    deleteClause(blockId: string): void {
      const before = this.blocks.slice()
      removeClauseFromLayout(this.layout, blockId)
      this.blocks = this.blocks.filter((b) => b['@id'] !== blockId)
      // A machine-readable rule must never outlive the prose it is backed by,
      // and neither may a field declaration outlive the placeholder that bound
      // it.
      this.withdrawRemovedBlockDeclarations(before)
    },
    updateClause(blockId: string, payload: { title?: string; content?: DcsContentSegment[] }): void {
      this.updateBlock(blockId, payload)
    },
    /** Replaces a clause's prose and its bound ODRL meaning. New fields/assets
     *  are appended; existing ids keep their store rows so shared document
     *  references stay intact. Policies previously bound to this clause's
     *  prose are withdrawn and replaced by the edited rule. */
    updateClauseWithMeaning(
      blockId: string,
      payload: {
        title: string
        content: DcsContentSegment[]
        fields: { id: string; parameterName: string; domainFieldIri: string; label?: string }[]
        assets?: { id: string; classIri: string; properties: { fieldId: string; path: string }[] }[]
        rule: OdrlRule | null
      },
    ): void {
      this.updateBlock(blockId, { title: payload.title, content: payload.content })
      this.policies = this.policies.filter((policy) => policy['dcs:prose']?.['@id'] !== blockId)

      const assetClassByFieldId = new Map<string, string>()
      for (const asset of payload.assets ?? []) {
        for (const property of asset.properties) assetClassByFieldId.set(property.fieldId, asset.classIri)
      }
      for (const field of payload.fields) {
        if (this.contractFields.some((existing) => existing['@id'] === field.id)) continue
        this.contractFields.push(
          contractFieldFromDomainField(
            field.id,
            field.parameterName,
            field.domainFieldIri,
            field.label,
            assetClassByFieldId.get(field.id),
          ),
        )
      }
      for (const asset of payload.assets ?? []) {
        const next = {
          '@id': asset.id,
          '@type': asset.classIri,
          ...Object.fromEntries(asset.properties.map((p) => [p.path, { '@id': p.fieldId }])),
        }
        const index = this.contractData.findIndex((object) => object['@id'] === asset.id)
        if (index >= 0) this.contractData[index] = next
        else this.contractData.push(next)
      }
      if (payload.rule) {
        this.policies.push(bindRuleProse(payload.rule, blockId))
      }
    },
    addMetaData(payload: MetaData): boolean {
      const name = payload.name.trim()
      const value = payload.value
      if (!name) return false
      const lower = name.toLowerCase()
      const hasDuplicate = this.customMetaData.some((m) => m.name.trim().toLowerCase() === lower)
      if (hasDuplicate) return false
      this.customMetaData.push({ name, value })
      return true
    },
    deleteMetaData(index: number): void {
      if (index < 0 || index >= this.customMetaData.length) return
      this.customMetaData.splice(index, 1)
    },
    updateMetaData(index: number, payload: MetaData): boolean {
      if (index < 0 || index >= this.customMetaData.length) return false
      const name = payload.name.trim()
      const value = payload.value
      if (!name) return false
      const lower = name.toLowerCase()
      const hasDuplicate = this.customMetaData.some((m, idx) => {
        if (idx === index) return false
        return m.name.trim().toLowerCase() === lower
      })
      if (hasDuplicate) return false
      this.customMetaData[index] = { name, value }
      return true
    },
    updateTemplateType(templateType: TemplateTypeValue): void {
      if (this.did !== null && this.did !== undefined) {
        throw new Error('Cannot change template type after template is created')
      }
      this.templateType = templateType
    },
    updateName(name: string): void {
      this.name = name
    },
    updateDescription(description: string): void {
      this.description = description
    },
    reset(overrides?: Partial<TemplateDraftState>) {
      Object.assign(this, getInitialState())
      if (overrides) Object.assign(this, overrides)
    },
  },
})

// ---- JSON-LD IRI helpers ----

const UUID_URN_PREFIX = 'urn:uuid:'

function objectIri(kind: string, id: string, documentId?: string): string {
  const fragment = `${kind}-${encodeURIComponent(id)}`
  return documentId ? `${documentId}#${fragment}` : `${UUID_URN_PREFIX}${fragment}`
}

function blockIri(id: string, documentId?: string): string {
  return objectIri('block', id, documentId)
}

function fieldIri(conditionId: string, parameterName: string, documentId?: string): string {
  return objectIri('field', `${conditionId}-${parameterName}`, documentId)
}

function policyIri(conditionId: string, parameterName: string, index: number, documentId?: string): string {
  return objectIri('policy', `${conditionId}-${parameterName}-${index}`, documentId)
}

function policySetIri(documentId?: string): string {
  return documentId ? `${documentId}#policy-set` : `${UUID_URN_PREFIX}policy-set`
}

// ---- ODRL rule parties/target (DCS ODRL profile: assigner/assignee/target required) ----
//
// Template = open parties (ODRL-Offer character): the two sides of a rule
// aren't bound to real party DIDs yet, so a role-derived open reference is
// used. Contract instance = bound parties (ODRL-Agreement character): once
// bound to a real contract, the same role-derived reference still resolves
// consistently against that contract's own DID, which is what the profile
// requires (presence of odrl:assigner/odrl:assignee/odrl:target); resolving
// to the real counterpart legal-entity DID is left to the semantic mapper
// that already publishes bound envelopes for peer exchange.

function partyReference(role: string, documentId?: string): JsonLdReference {
  return { '@id': objectIri('party', role, documentId) }
}

function targetReference(documentId?: string): JsonLdReference {
  return { '@id': documentId ?? `${UUID_URN_PREFIX}pending-target` }
}

/** Assembles the single enclosing odrl:Offer from the flat internal rule array; the first signature seals it into an odrl:Agreement server-side. */
function assemblePolicySet(policies: readonly OdrlRule[], documentId?: string): OdrlSet {
  const set: OdrlSet = {
    '@id': policySetIri(documentId),
    '@type': 'odrl:Offer',
    'odrl:profile': { '@id': DCS_ODRL_PROFILE_IRI },
  }
  const duties = policies.filter((p) => p['@type'] === 'odrl:Duty')
  const permissions = policies.filter((p) => p['@type'] === 'odrl:Permission')
  const prohibitions = policies.filter((p) => p['@type'] === 'odrl:Prohibition')
  if (duties.length) set['odrl:obligation'] = duties
  if (permissions.length) set['odrl:permission'] = permissions
  if (prohibitions.length) set['odrl:prohibition'] = prohibitions
  return set
}

/** Flattens the enclosing ODRL policy (or the empty "no policies yet" array) into the flat internal rule array. */
export function flattenPolicySet(policies: OdrlSet | OdrlRule[] | undefined): OdrlRule[] {
  if (!policies) return []
  if (Array.isArray(policies)) return policies
  return [
    ...(policies['odrl:obligation'] ?? []),
    ...(policies['odrl:permission'] ?? []),
    ...(policies['odrl:prohibition'] ?? []),
  ]
}

// ---- Document assembly (trivial — blocks/layout already in JSON-LD) ----

interface CanonicalDocumentInput {
  documentType: DcsDocumentData['@type']
  documentId?: string
  name?: string
  description?: string
  templateType?: TemplateTypeValue
  blocks: DcsBlock[]
  layout: DcsLayoutNode[]
  contractFields: DcsContractField[]
  contractData: DcsContractDataObject[]
  policies: OdrlRule[]
  customMetaData?: MetaData[]
  semanticConditionValues?: SemanticConditionValue[]
  parentContractDid?: string
  derivedFromTemplate?: DcsContractData['derivedFromTemplate']
}

function assembleCanonicalDocument(input: CanonicalDocumentInput): DcsDocumentData {
  const isContract = input.documentType === 'dcs:Contract'
  const submittedValues = input.semanticConditionValues ?? []
  // A contract carries submitted values on the field declarations; domain
  // objects and clause prose continue to reference those fields only by @id.
  const contractFields = isContract
    ? applyInlineSemanticValues(input.contractFields, submittedValues)
    : input.contractFields
  const canonicalBlocks = canonicalizeBlocks(input.blocks)
  const canonicalLayout = canonicalizeLayout(input.layout)
  const commonMetadata = {
    ...(input.documentId ? { '@id': `${input.documentId}#metadata` } : {}),
    ...(input.name ? { 'dcs:title': input.name } : {}),
    ...(input.description ? { 'dcs:description': input.description } : {}),
    ...(input.customMetaData?.length ? { 'dcs:customMetaData': input.customMetaData } : {}),
  }
  const metadata =
    input.documentType === 'dcs:ContractTemplate'
      ? {
          '@type': 'dcs:TemplateMetadata' as const,
          ...commonMetadata,
          'dcs:templateType':
            input.templateType === TemplateType.contractTemplate ? 'dcs:ContractTemplate' : 'dcs:Component',
        }
      : { '@type': 'dcs:ContractMetadata' as const, ...commonMetadata }

  return {
    '@type': input.documentType,
    ...(input.documentId ? { '@id': input.documentId } : {}),
    'dcs:metadata': metadata,
    'dcs:documentStructure': {
      '@type': 'dcs:DocumentStructure',
      ...(input.documentId ? { '@id': `${input.documentId}#document-structure` } : {}),
      'dcs:blocks': { '@list': canonicalBlocks },
      'dcs:layout': { '@list': canonicalLayout },
    },
    'dcs:contractFields': contractFields,
    'dcs:contractData': input.contractData,
    'dcs:policies': assemblePolicySet(input.policies, input.documentId),
    ...(isContract
      ? {
          ...(input.parentContractDid ? { 'dcs:parentContract': { '@id': input.parentContractDid } } : {}),
          ...(input.derivedFromTemplate ? { derivedFromTemplate: input.derivedFromTemplate } : {}),
        }
      : {}),
  }
}

function canonicalizeBlocks(blocks: DcsBlock[]): DcsBlock[] {
  return [...blocks]
}

function canonicalizeLayout(layout: DcsLayoutNode[]): DcsLayoutNode[] {
  return layout.map((node) => ({
    ...node,
    '@type': 'dcs:LayoutNode',
    'dcs:children': { '@list': [...node['dcs:children']['@list']] },
  }))
}

// ---- buildContractDocument (public API for contract workflow) ----

export interface ContractDocumentInput {
  documentId: string
  /**
   * The document as the server handed it out. The editor rebuilds the contract
   * from its own state, so anything it does not model is absent from what it
   * rebuilds — the server-owned fields are carried over from here.
   */
  storedDocument?: DcsContractData
  name?: string
  description?: string
  blocks: DcsBlock[]
  layout: DcsLayoutNode[]
  contractFields: DcsContractField[]
  contractData: DcsContractDataObject[]
  policies: OdrlRule[]
  semanticConditionValues: SemanticConditionValue[]
  parentContractDid?: string
  derivedFromTemplate?: DcsContractData['derivedFromTemplate']
}

/**
 * Fields of a contract document the server owns and the client only carries.
 * The editor cannot reproduce them, so rebuilding the document from editor
 * state drops them — and posting that back discards data the server put there:
 * the party nodes a signature is attributed to (with the signatory and Power of
 * Attorney already recorded on them), the signature fields naming those
 * parties, and the shapes graph the document is pinned to.
 */
export function buildContractDocument(input: ContractDocumentInput): DcsContractData {
  const { storedDocument, ...editorState } = input
  const document = assembleCanonicalDocument({
    ...editorState,
    documentType: 'dcs:Contract',
  }) as DcsContractData
  if (!storedDocument) return document
  if (storedDocument['dcs:parties'] !== undefined) document['dcs:parties'] = storedDocument['dcs:parties']
  if (storedDocument['dcs:signatureFields'] !== undefined) {
    document['dcs:signatureFields'] = storedDocument['dcs:signatureFields']
  }
  if (storedDocument['sh:shapesGraph'] !== undefined) document['sh:shapesGraph'] = storedDocument['sh:shapesGraph']
  return document
}

export function getSemanticConditionsFromTemplateData(td: DcsDocumentData | undefined): SemanticCondition[] {
  if (!isDcsDocumentData(td)) return []
  return contractFieldsToSemanticConditions(td['dcs:contractFields'], flattenPolicySet(td['dcs:policies']))
}

// ---- Flatten-on-compose ----

interface InlinedComponent {
  /** Non-root layout nodes of the component, id-remapped. */
  layoutNodes: DcsLayoutNode[]
  /** The remapped @ids of the component's root-level blocks, in order. */
  rootChildIds: string[]
  blocks: DcsBlock[]
  contractFields: DcsContractField[]
  contractData: DcsContractDataObject[]
  policies: OdrlRule[]
}

/**
 * Deep-clones a component document and rewrites every component-owned @id
 * (blocks, layout nodes, contract fields/data, policies) to fresh unique ids,
 * keeping all in-document references (@id links in layout children, clause field
 * refs, ODRL leftOperand/rightOperand/prose) consistent. Two inlines of the
 * same component never collide.
 */
function inlineComponentDocument(component: DcsTemplateData, documentId?: string): InlinedComponent {
  const structure = component['dcs:documentStructure']
  const blocks = deepClone(structure['dcs:blocks']['@list'])
  const layout = deepClone(extractLayoutList(structure['dcs:layout']))
  const contractFields = deepClone(component['dcs:contractFields'] ?? [])
  const contractData = deepClone(component['dcs:contractData'] ?? [])
  const policies = deepClone(flattenPolicySet(component['dcs:policies']))

  const idMap = new Map<string, string>()
  const remap = (id: string): string => {
    let fresh = idMap.get(id)
    if (!fresh) {
      fresh = blockIri(crypto.randomUUID(), documentId)
      idMap.set(id, fresh)
    }
    return fresh
  }

  for (const block of blocks) remap(block['@id'])
  for (const node of layout) remap(node['@id'])
  for (const field of contractFields) remap(field['@id'])
  for (const object of contractData) if (object['@id']) remap(object['@id'])
  for (const rule of policies) if (rule['@id']) remap(rule['@id'])

  const rewrittenBlocks = blocks.map((block) => ({ ...block, '@id': remap(block['@id']) }))
  const rewrittenContractFields = contractFields.map((field) => ({
    ...field,
    '@id': remap(field['@id']),
  }))
  const rewrittenContractData = contractData.map((object) => rewriteContractDataObject(object, idMap, remap))
  rewriteContentRefs(rewrittenBlocks, idMap)

  const root = layout.find((node) => node['dcs:isRoot'])
  const rootChildIds = root ? root['dcs:children']['@list'].map((ref) => remap(ref['@id'])) : []
  const layoutNodes: DcsLayoutNode[] = layout
    .filter((node) => !node['dcs:isRoot'])
    .map((node) => ({
      '@id': remap(node['@id']),
      '@type': 'dcs:LayoutNode',
      'dcs:children': { '@list': node['dcs:children']['@list'].map((ref) => ({ '@id': remap(ref['@id']) })) },
    }))

  const rewrittenPolicies = policies.map((rule) => remapRuleIds(rule, idMap))

  return {
    layoutNodes,
    rootChildIds,
    blocks: rewrittenBlocks,
    contractFields: rewrittenContractFields,
    contractData: rewrittenContractData,
    policies: rewrittenPolicies,
  }
}

/** Rewrites contract-field references inside clause content to their fresh ids. */
function rewriteContentRefs(blocks: DcsBlock[], idMap: Map<string, string>): void {
  for (const block of blocks) {
    if (!isDcsClause(block)) continue
    const content = block['dcs:content']
    if (typeof content === 'string') continue
    block['dcs:content'] = {
      '@list': content['@list'].map((segment) =>
        typeof segment === 'string' ? segment : { '@id': idMap.get(segment['@id']) ?? segment['@id'] },
      ),
    }
  }
}

function isDataReference(value: unknown): value is JsonLdReference {
  return typeof value === 'object' && value !== null && !Array.isArray(value) && '@id' in value
}

function rewriteContractDataObject(
  object: DcsContractDataObject,
  idMap: Map<string, string>,
  remap: (id: string) => string,
): DcsContractDataObject {
  const rewritten: DcsContractDataObject = {
    ...object,
    '@id': remap(object['@id']),
  }
  // Only @id references are rewritten; literals and typed {@value} literals
  // pass through untouched — the graph admits all three value kinds.
  for (const [property, value] of Object.entries(object)) {
    if (property.startsWith('@') || value === undefined) continue
    if (Array.isArray(value)) {
      rewritten[property] = value.map((member) =>
        isDataReference(member) ? { '@id': idMap.get(member['@id']) ?? member['@id'] } : member,
      )
    } else if (isDataReference(value)) {
      rewritten[property] = { '@id': idMap.get(value['@id']) ?? value['@id'] }
    }
  }
  return rewritten
}

/** Binds a rule and every duty nested in it — the duties themselves and their
 *  consequences, each an odrl:Duty — to the clause block the rule was authored
 *  in. The nested nodes are fragments of the same clause, so they are backed by
 *  the same prose; a duty carrying none is refused by the hub shapes at submit. */
function bindRuleProse(rule: OdrlRule, blockId: string): OdrlRule {
  const bound: OdrlRule = { ...rule, 'dcs:prose': { '@id': blockId } }
  if (bound['odrl:duty']) bound['odrl:duty'] = bound['odrl:duty'].map((duty) => bindDutyProse(duty, blockId))
  return bound
}

function bindDutyProse(duty: OdrlDuty, blockId: string): OdrlDuty {
  const bound: OdrlDuty = { ...duty, 'dcs:prose': { '@id': blockId } }
  if (bound['odrl:consequence']) {
    bound['odrl:consequence'] = bound['odrl:consequence'].map((consequence) => bindDutyProse(consequence, blockId))
  }
  return bound
}

/** Rewrites a rule's own @id plus every component-owned @id it references (prose, constraint operands, nested duties). */
function remapRuleIds(rule: OdrlRule, idMap: Map<string, string>): OdrlRule {
  const next: OdrlRule = { ...rule }
  if (next['@id']) next['@id'] = idMap.get(next['@id']) ?? next['@id']
  if (next['dcs:prose']) next['dcs:prose'] = { '@id': idMap.get(next['dcs:prose']['@id']) ?? next['dcs:prose']['@id'] }
  if (next['odrl:constraint']) {
    next['odrl:constraint'] = next['odrl:constraint'].map((node) => remapConstraintIds(node, idMap))
  }
  if (next['odrl:duty']) {
    next['odrl:duty'] = next['odrl:duty'].map((duty) => remapDutyIds(duty, idMap))
  }
  return next
}

function remapDutyIds(duty: OdrlDuty, idMap: Map<string, string>): OdrlDuty {
  const next = { ...duty }
  if (next['@id']) next['@id'] = idMap.get(next['@id']) ?? next['@id']
  if (next['dcs:prose']) next['dcs:prose'] = { '@id': idMap.get(next['dcs:prose']['@id']) ?? next['dcs:prose']['@id'] }
  if (next['odrl:constraint']) {
    next['odrl:constraint'] = next['odrl:constraint'].map((node) => remapConstraintIds(node, idMap))
  }
  if (next['odrl:consequence']) {
    next['odrl:consequence'] = next['odrl:consequence'].map((consequence) => remapDutyIds(consequence, idMap))
  }
  return next
}

function remapConstraintIds(node: OdrlConstraintNode, idMap: Map<string, string>): OdrlConstraintNode {
  const mapRef = (ref: JsonLdReference): JsonLdReference => ({ '@id': idMap.get(ref['@id']) ?? ref['@id'] })
  if (isAtomicConstraint(node)) {
    const next: OdrlConstraint = { ...node, 'odrl:leftOperand': mapRef(node['odrl:leftOperand']) }
    const right = node['odrl:rightOperand']
    if (right && !Array.isArray(right) && typeof right === 'object' && '@id' in right) {
      next['odrl:rightOperand'] = mapRef(right)
    }
    // A unit may name a contract field too (a negotiated unit); one naming a
    // fixed concept is not in the map and passes through.
    const unit = node['odrl:unit']
    if (unit) next['odrl:unit'] = mapRef(unit)
    return next
  }
  const next = { ...node }
  for (const op of ['odrl:and', 'odrl:or', 'odrl:xone', 'odrl:andSequence'] as const) {
    const group = node[op]
    if (group) next[op] = { '@list': group['@list'].map((child) => remapConstraintIds(child, idMap)) }
  }
  return next
}

// ---- Layout helpers ----

function extractBlockList(raw: DcsDocumentStructure['dcs:blocks'] | DcsBlock[]): DcsBlock[] {
  return Array.isArray(raw) ? raw : raw['@list']
}

function extractLayoutList(raw: DcsDocumentStructure['dcs:layout'] | DcsLayoutNode[]): DcsLayoutNode[] {
  return Array.isArray(raw) ? raw : raw['@list']
}

function getInitialLayout(): DcsLayoutNode[] {
  return [
    {
      '@id': `${UUID_URN_PREFIX}block-${crypto.randomUUID()}`,
      '@type': 'dcs:LayoutNode',
      'dcs:isRoot': true,
      'dcs:children': { '@list': [] },
    },
  ]
}

function layoutNodeChildren(node: DcsLayoutNode): string[] {
  return node['dcs:children']['@list'].map((ref) => ref['@id'])
}

function collectBlockIdsInLayout(layout: DcsLayoutNode[]): Set<string> {
  const ids = new Set<string>()
  const nodeById = new Map(layout.map((n) => [n['@id'], n]))
  function visit(id: string) {
    if (ids.has(id)) return
    ids.add(id)
    const node = nodeById.get(id)
    if (node) layoutNodeChildren(node).forEach(visit)
  }
  const root = layout.find((n) => n['dcs:isRoot'])
  if (root) visit(root['@id'])
  return ids
}

// ---- Block mutation helpers ----

function addBlock(
  layout: DcsLayoutNode[],
  blocks: DcsBlock[],
  parentBlockId: string,
  insertIndex: number,
  payload: AddBlockPayload,
  options?: AddBlockOptions,
  documentId?: string,
): string {
  const addToOutline = options?.addToOutline !== false

  if (payload.clauseBlockId) {
    const clauseId = payload.clauseBlockId
    const exists = blocks.find((b) => b['@id'] === clauseId)
    if (exists?.['@type'] !== 'dcs:Clause') {
      throw new Error(`addBlock: clause block not found: ${clauseId}`)
    }
    const inLayout = collectBlockIdsInLayout(layout)
    if (inLayout.has(clauseId)) return clauseId
    const parent = layout.find((n) => n['@id'] === parentBlockId)
    if (!parent) throw new Error(`addBlock: parent not found: ${parentBlockId}`)
    const children = layoutNodeChildren(parent)
    children.splice(insertIndex, 0, clauseId)
    parent['dcs:children'] = { '@list': children.map((id) => ({ '@id': id })) }
    return clauseId
  }

  const uuid = crypto.randomUUID()
  const id = blockIri(uuid, documentId)
  const block = createBlock(id, payload)

  if (payload.blockType === 'dcs:Clause' && !addToOutline) {
    blocks.push(block)
    return id
  }

  const parent = layout.find((n) => n['@id'] === parentBlockId)
  if (!parent) throw new Error(`addBlock: parent not found: ${parentBlockId}`)
  const children = layoutNodeChildren(parent)
  children.splice(insertIndex, 0, id)
  parent['dcs:children'] = { '@list': children.map((ref) => ({ '@id': ref })) }

  if (payload.blockType === 'dcs:Section') {
    layout.push({ '@id': id, '@type': 'dcs:LayoutNode', 'dcs:children': { '@list': [] } })
  }
  blocks.push(block)
  return id
}

function createBlock(id: string, payload: AddBlockPayload): DcsBlock {
  switch (payload.blockType) {
    case 'dcs:Section':
      return {
        '@type': 'dcs:Section',
        '@id': id,
        ...(payload.title ? { 'dcs:title': payload.title } : {}),
      }
    case 'dcs:TextBlock':
      return { '@type': 'dcs:TextBlock', '@id': id, 'dcs:text': payload.text ?? '' }
    case 'dcs:Clause':
      return {
        '@type': 'dcs:Clause',
        '@id': id,
        'dcs:content': { '@list': payload.content ?? [] },
        ...(payload.title ? { 'dcs:title': payload.title } : {}),
      }
    default:
      throw new Error('Unknown blockType')
  }
}

function moveBlock(layout: DcsLayoutNode[], blockId: string, parentBlockId: string, insertIndex: number): void {
  const oldParent = layout.find((n) => layoutNodeChildren(n).includes(blockId))
  const newParent = layout.find((n) => n['@id'] === parentBlockId)
  if (!oldParent || !newParent) return

  if (oldParent['@id'] === newParent['@id']) {
    const siblings = layoutNodeChildren(oldParent).filter((id) => id !== blockId)
    const idx = Math.min(insertIndex, siblings.length)
    siblings.splice(idx, 0, blockId)
    oldParent['dcs:children'] = { '@list': siblings.map((id) => ({ '@id': id })) }
    return
  }
  const oldChildren = layoutNodeChildren(oldParent).filter((id) => id !== blockId)
  oldParent['dcs:children'] = { '@list': oldChildren.map((id) => ({ '@id': id })) }
  const newChildren = [...layoutNodeChildren(newParent)]
  const idx = Math.min(insertIndex, newChildren.length)
  newChildren.splice(idx, 0, blockId)
  newParent['dcs:children'] = { '@list': newChildren.map((id) => ({ '@id': id })) }
}

function deleteBlock(layout: DcsLayoutNode[], blocks: DcsBlock[], blockId: string): void {
  const block = blocks.find((b) => b['@id'] === blockId)
  const parent = layout.find((n) => layoutNodeChildren(n).includes(blockId))
  if (!parent) return

  if (block?.['@type'] === 'dcs:Clause') {
    const newChildren = layoutNodeChildren(parent).filter((id) => id !== blockId)
    parent['dcs:children'] = { '@list': newChildren.map((id) => ({ '@id': id })) }
    // A clause may be placed more than once; the block survives as long as any
    // layout node still holds it. Once the last placement is gone the block
    // must go too: assembleCanonicalDocument serializes dcs:blocks whole, so a
    // clause kept without a placement stays in the document while nothing
    // renders it — and its prose keeps binding required fields that no user
    // can reach any more (backend validation.ValidateContractClosed reads
    // exactly that block list, so the offer is refused over an invisible
    // placeholder).
    if (!layout.some((n) => layoutNodeChildren(n).includes(blockId))) {
      const kept = blocks.filter((b) => b['@id'] !== blockId)
      blocks.length = 0
      blocks.push(...kept)
    }
    return
  }

  const nodeById = new Map(layout.map((n) => [n['@id'], n]))
  const toRemove = collectDescendantIds(blockId, nodeById)
  const newChildren = layoutNodeChildren(parent).filter((id) => id !== blockId)
  parent['dcs:children'] = { '@list': newChildren.map((id) => ({ '@id': id })) }
  const layoutToKeep = layout.filter((n) => (n['dcs:isRoot'] ?? false) || !toRemove.has(n['@id']))
  layout.length = 0
  layout.push(...layoutToKeep)
  const blocksToKeep = blocks.filter((b) => !toRemove.has(b['@id']))
  blocks.length = 0
  blocks.push(...blocksToKeep)
}

/** Collects which of `declared` the value still references, at any depth.
 *  A field is bound by a prose segment, an ODRL operand or a data leaf — all
 *  of them a `{"@id": …}` reference, so one walk answers for every kind. */
function collectFieldRefs(value: unknown, declared: Set<string>, found: Set<string>): void {
  if (Array.isArray(value)) {
    for (const entry of value) collectFieldRefs(entry, declared, found)
    return
  }
  if (typeof value !== 'object' || value === null) return
  for (const [key, member] of Object.entries(value)) {
    if (key === '@id' && typeof member === 'string' && declared.has(member)) found.add(member)
    else collectFieldRefs(member, declared, found)
  }
}

function removeClauseFromLayout(layout: DcsLayoutNode[], blockId: string): void {
  for (const node of layout) {
    const children = layoutNodeChildren(node)
    if (children.includes(blockId)) {
      node['dcs:children'] = { '@list': children.filter((id) => id !== blockId).map((id) => ({ '@id': id })) }
    }
  }
}

function collectDescendantIds(blockId: string, nodeById: Map<string, DcsLayoutNode>): Set<string> {
  const set = new Set<string>([blockId])
  const node = nodeById.get(blockId)
  const childIds = node ? layoutNodeChildren(node) : []
  childIds.forEach((id) => collectDescendantIds(id, nodeById).forEach((x) => set.add(x)))
  return set
}

// ---- Initial state ----

const defaultState: Readonly<Omit<TemplateDraftState, 'blocks' | 'layout'>> = {
  did: null,
  documentIri: null,
  name: '',
  description: '',
  templateDataVersion: 1,
  contractFields: [],
  contractData: [],
  policies: [],
  customMetaData: [],
  templateType: TemplateType.component,
  state: undefined,
  version: null,
  updated_at: null,
  created_by: '',
  responsible: null,
  workflow: 'template',
}

function getInitialState(): TemplateDraftState {
  return {
    ...(defaultState as TemplateDraftState),
    blocks: [],
    layout: getInitialLayout(),
    contractFields: [],
    contractData: [],
    policies: [],
    customMetaData: [],
  }
}

function deepClone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

// ---- Semantic condition helpers (contractFields ↔ SemanticCondition[]) ----

/** xsd datatype ↔ the UI parameter type. */
const PARAM_TYPE_TO_XSD: Record<SemanticConditionParameter['type'], import('@/models/dcs-jsonld').XsdDatatype> = {
  string: 'xsd:string',
  enum: 'xsd:string',
  decimal: 'xsd:decimal',
  integer: 'xsd:integer',
  boolean: 'xsd:boolean',
  date: 'xsd:date',
}

function xsdToParamType(
  datatype: import('@/models/dcs-jsonld').XsdDatatype,
  hasOptions: boolean,
): SemanticConditionParameter['type'] {
  switch (datatype) {
    case 'xsd:decimal':
      return 'decimal'
    case 'xsd:integer':
      return 'integer'
    case 'xsd:boolean':
      return 'boolean'
    case 'xsd:date':
    case 'xsd:dateTime':
      return 'date'
    // A duration is typed as free text (an ISO-8601 token, "P14D"), but the
    // field keeps its declared xsd:duration — the parameter type only picks
    // the input widget, and PARAM_TYPE_TO_XSD is never applied to a field
    // that already declares its own datatype.
    case 'xsd:duration':
    case 'xsd:string':
      return hasOptions ? 'enum' : 'string'
  }
}

/** Builds a self-contained ContractField declaration from an authoring parameter. */
function semanticParamToContractField(
  conditionId: string,
  parameter: SemanticConditionParameter,
  documentId?: string,
): DcsContractField {
  const domainField = ONTOLOGY_DOMAIN_FIELDS.find((f) => f.ontologyId === parameter.fieldIri)
  const value = parameter.value
  const hasValue = value !== undefined && value !== null && value !== ''
  const constraint = parameter.valueConstraint ?? domainField?.valueConstraint
  return {
    '@id': fieldIri(conditionId, parameter.parameterName, documentId),
    '@type': 'dcs:ContractField',
    'dcs:label': parameter.uiMetadata?.label ?? parameter.parameterName,
    'dcs:datatype': PARAM_TYPE_TO_XSD[parameter.type],
    ...(parameter.fieldIri ? { 'dcs:shape': { '@id': domainField?.ontologyId ?? parameter.fieldIri } } : {}),
    'dcs:required': parameter.isRequired,
    ...(constraint ? { 'dcs:valueConstraint': cloneValueConstraint(constraint) } : {}),
    ...(hasValue
      ? { 'dcs:value': typedFieldFill(value as string | number | boolean, PARAM_TYPE_TO_XSD[parameter.type]) }
      : {}),
  }
}

/** The hub definition a picked field derives from: a flat ontology domain
 *  field, or — when the field came in with a declared asset — the property
 *  shape the asset's class declares for that path, so the shape's datatype
 *  (sh:datatype) and value constraints (sh:in, sh:minInclusive/sh:maxInclusive,
 *  sh:pattern) reach the emitted field. */
function hubFieldDefinition(domainFieldIri: string, classIri?: string): DomainFieldDefinition | undefined {
  const owner = classIri ? ONTOLOGY_ASSETS.find((asset) => asset.id === classIri) : undefined
  return (
    owner?.properties.find((property) => property.ontologyId === domainFieldIri) ??
    ONTOLOGY_DOMAIN_FIELDS.find((f) => f.ontologyId === domainFieldIri) ??
    ONTOLOGY_ASSETS.flatMap((asset) => asset.properties).find((property) => property.ontologyId === domainFieldIri)
  )
}

/** Builds a ContractField for a clause-editor field binding. */
function contractFieldFromDomainField(
  id: string,
  parameterName: string,
  domainFieldIri: string,
  label?: string,
  classIri?: string,
): DcsContractField {
  const domainField = hubFieldDefinition(domainFieldIri, classIri)
  return {
    '@id': id,
    '@type': 'dcs:ContractField',
    'dcs:label': label ?? domainField?.label ?? parameterName,
    // The hub's declared datatype wins: the parameter type is the input
    // widget, whose vocabulary has no duration and no instant, so deriving
    // the datatype from it silently rewrites an imported xsd:duration field
    // to xsd:string and every boundary over it then compares bytes.
    'dcs:datatype': domainField?.datatype ?? PARAM_TYPE_TO_XSD[domainField?.type ?? 'string'],
    'dcs:shape': { '@id': domainFieldIri },
    'dcs:required': true,
    ...(domainField?.valueConstraint
      ? { 'dcs:valueConstraint': cloneValueConstraint(domainField.valueConstraint) }
      : {}),
  }
}

function contractDataObjectIri(conditionId: string, documentId?: string): string {
  return objectIri('contract-data', conditionId, documentId)
}

function semanticConditionToContractData(
  condition: SemanticCondition,
  fields: readonly DcsContractField[],
  documentId?: string,
): DcsContractDataObject {
  const localType = condition.conditionName
    .replace(/[^A-Za-z0-9]+/g, ' ')
    .trim()
    .split(/\s+/)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join('')
  const object: DcsContractDataObject = {
    '@id': contractDataObjectIri(condition.conditionId, documentId),
    '@type': `dcs:${localType || 'ContractData'}${localType.endsWith('Clause') ? '' : 'Clause'}`,
  }
  condition.parameters.forEach((parameter, index) => {
    const property = `dcs:${parameter.parameterName.replace(/[^A-Za-z0-9_]/g, '') || `field${index + 1}`}`
    const field = fields[index]
    if (field) object[property] = { '@id': field['@id'] }
  })
  return object
}

function proseBlockForField(blocks: readonly DcsBlock[], fieldId: string): JsonLdReference {
  for (const block of blocks) {
    if (!isDcsClause(block)) continue
    const content = block['dcs:content']
    const segments = typeof content === 'string' ? [] : content['@list']
    for (const segment of segments) {
      if (typeof segment !== 'string' && segment['@id'] === fieldId) {
        return { '@id': block['@id'] }
      }
    }
  }
  throw new Error(
    `No clause text binds field ${fieldId}: every machine-readable rule must be backed by human-readable prose (place the field reference in a clause first).`,
  )
}

function semanticConditionToPolicies(
  condition: SemanticCondition,
  _contractFields: DcsContractField[],
  blocks: readonly DcsBlock[],
  documentId?: string,
  partyRoles: readonly string[] = [],
): OdrlRule[] {
  const role = condition.entityRole
  if (!role || partyRoles.length !== 2 || !partyRoles.includes(role)) return []
  const counterpart = partyRoles.find((candidate) => candidate !== role)
  if (!counterpart) return []
  return condition.parameters.flatMap((parameter) =>
    parameter.operators.flatMap((operator, index) => {
      if (!isStandardOdrlOperator(operator.operate)) return []
      const fieldId = parameter.fieldId ?? fieldIri(condition.conditionId, parameter.parameterName, documentId)
      const rightOperand = odrlRightOperand(operator, parameter.type)
      return [
        {
          '@id': policyIri(condition.conditionId, parameter.parameterName, index, documentId),
          '@type': 'odrl:Duty',
          'odrl:action': { '@id': DEFAULT_FIELD_CONSTRAINT_ACTION },
          'odrl:assigner': partyReference(role, documentId),
          'odrl:assignee': partyReference(counterpart, documentId),
          'odrl:target': targetReference(documentId),
          'dcs:prose': proseBlockForField(blocks, fieldId),
          'odrl:constraint': [
            {
              '@type': 'odrl:Constraint',
              'odrl:leftOperand': { '@id': fieldId },
              'odrl:operator': { '@id': operator.operate },
              ...(rightOperand !== undefined ? { 'odrl:rightOperand': rightOperand } : {}),
            },
          ],
        } satisfies OdrlRule,
      ]
    }),
  )
}

/** Flattens a constraint list to its atomic leaves, descending logical constraints.
 * ODRL/JSON-LD lets `odrl:constraint` be a single node or a list, so a bare
 * constraint object is normalized to a one-element list before descent. */
function atomicConstraintLeaves(nodes: readonly OdrlConstraintNode[] | OdrlConstraintNode): OdrlConstraint[] {
  const list = Array.isArray(nodes) ? nodes : [nodes]
  const leaves: OdrlConstraint[] = []
  for (const node of list) {
    if (isAtomicConstraint(node)) {
      leaves.push(node)
      continue
    }
    for (const op of ['odrl:and', 'odrl:or', 'odrl:xone', 'odrl:andSequence'] as const) {
      const list = node[op]
      if (list) leaves.push(...atomicConstraintLeaves(list['@list']))
    }
  }
  return leaves
}

/** The left-operand IRIs a rule's constraints reference (across logical trees). */
function ruleLeftOperands(rule: OdrlRule): string[] {
  return atomicConstraintLeaves(rule['odrl:constraint'] ?? []).map(
    (constraint) => constraint['odrl:leftOperand']['@id'],
  )
}

function contractFieldsToSemanticConditions(
  fields: readonly DcsContractField[],
  policies: readonly OdrlRule[],
): SemanticCondition[] {
  const operatorsByField = new Map<string, SemanticParameterOperator[]>()
  for (const policy of policies) {
    for (const constraint of atomicConstraintLeaves(policy['odrl:constraint'] ?? [])) {
      const operate = constraint['odrl:operator']['@id'] as DcsOperator
      if (!isStandardOdrlOperator(operate)) continue
      const rightOperand = constraint['odrl:rightOperand']
      // A right operand may be a bare literal (95), a typed value ({@value}), a
      // field reference ({@id} — a negotiated boundary whose bound is whatever
      // that field ends up holding), or a list of those. Only an OBJECT can be
      // probed with `in`; guarding it keeps a primitive operand from throwing
      // and blanking the whole clause render.
      const operands = rightOperand === undefined ? [] : Array.isArray(rightOperand) ? rightOperand : [rightOperand]
      const isReference = (operand: (typeof operands)[number]): operand is JsonLdReference =>
        typeof operand === 'object' && operand !== null && '@id' in operand
      const targets = operands.filter((operand) => !isReference(operand)).map(jsonLdValue)
      const targetRefs = operands.filter(isReference).map((operand) => operand['@id'])
      const fieldId = constraint['odrl:leftOperand']['@id']
      operatorsByField.set(fieldId, [...(operatorsByField.get(fieldId) ?? []), { operate, targets, targetRefs }])
    }
  }

  // Each field is surfaced as a single-parameter condition whose conditionId
  // is the field @id; its input type comes straight from
  // dcs:datatype and its constraint from the inline dcs:valueConstraint.
  return fields.map((field) => {
    const shapeIri = field['dcs:shape']?.['@id']
    const ontologyField = ONTOLOGY_DOMAIN_FIELDS.find((candidate) => candidate.ontologyId === shapeIri)
    const constraint = field['dcs:valueConstraint'] ?? ontologyField?.valueConstraint
    const hasOptions = !!constraint?.valueOptions?.length || !!constraint?.allowedValues?.length
    const label = field['dcs:label']
    return {
      conditionId: field['@id'],
      conditionName: label,
      schemaVersion: 'v1' as const,
      parameters: [
        {
          parameterName: label,
          fieldId: field['@id'],
          type: xsdToParamType(field['dcs:datatype'], hasOptions),
          fieldIri: shapeIri ?? field['@id'],
          valueConstraint: constraint ? cloneValueConstraint(constraint) : undefined,
          uiMetadata: { label },
          isRequired: field['dcs:required'],
          operators: operatorsByField.get(field['@id']) ?? [],
          value: fieldFillScalar(field['dcs:value'], field['dcs:datatype']),
        },
      ],
    }
  })
}

function odrlRightOperand(
  operator: SemanticParameterOperator,
  parameterType: SemanticConditionParameter['type'],
): JsonLdTypedValue | JsonLdTypedValue[] | undefined {
  if (!operator.targets.length) return undefined
  const operands = operator.targets.map((target) => typedJsonLdValue(target, parameterType))
  // A set operand is an unordered JSON-LD set (bare array): the policy audit
  // expands and matches each member individually, while an @list would reach
  // it as a single nested list value.
  if (isSetOperator(operator.operate)) return operands
  return operands[0]
}

function typedJsonLdValue(value: unknown, parameterType: SemanticConditionParameter['type']): JsonLdTypedValue {
  return {
    '@value': jsonLdLexicalValue(value, parameterType),
    '@type': xsdTypeForParameter(parameterType),
  }
}

function jsonLdLexicalValue(value: unknown, parameterType: SemanticConditionParameter['type']): string {
  if (parameterType === 'boolean') return value === true || value === 'true' ? 'true' : 'false'
  if (value === null || value === undefined) return ''
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'bigint') return value.toString()
  return JSON.stringify(value) ?? ''
}

function xsdTypeForParameter(parameterType: SemanticConditionParameter['type']): JsonLdTypedValue['@type'] {
  switch (parameterType) {
    case 'decimal':
      return 'xsd:decimal'
    case 'integer':
      return 'xsd:integer'
    case 'boolean':
      return 'xsd:boolean'
    case 'date':
      return 'xsd:date'
    case 'string':
    case 'enum':
      return 'xsd:string'
  }
}

function isStandardOdrlOperator(operator: string): operator is DcsOperator {
  return [
    'odrl:eq',
    'odrl:neq',
    'odrl:gt',
    'odrl:gteq',
    'odrl:lt',
    'odrl:lteq',
    'odrl:isAnyOf',
    'odrl:isNoneOf',
    'odrl:hasPart',
  ].includes(operator)
}

function isSetOperator(operator: string): boolean {
  return operator === 'odrl:isAnyOf' || operator === 'odrl:isNoneOf'
}

function jsonLdValue(value: JsonLdTypedValue | JsonLdReference): unknown {
  if ('@id' in value) return value['@id']
  switch (value['@type']) {
    case 'xsd:decimal':
    case 'xsd:integer':
      return Number(value['@value'])
    case 'xsd:boolean':
      return value['@value'] === 'true'
    // Every other datatype keeps its lexical form. The switch used to list
    // the cases it knew and fall off the end for the rest, which turned an
    // xsd:duration boundary into `undefined` — the bound vanished and the
    // constraint reported "Expected <= undefined" against every value.
    // compareXsdValues orders the lexical form inside its own value space.
    default:
      return value['@value']
  }
}

function cloneValueConstraint(
  constraint: SemanticConditionParameter['valueConstraint'],
): SemanticConditionParameter['valueConstraint'] {
  if (!constraint) return undefined
  return {
    ...constraint,
    allowedValues: constraint.allowedValues ? [...constraint.allowedValues] : undefined,
    valueOptions: constraint.valueOptions?.map((option) => ({ ...option })),
    odrlLeftOperands: constraint.odrlLeftOperands ? [...constraint.odrlLeftOperands] : undefined,
  }
}
