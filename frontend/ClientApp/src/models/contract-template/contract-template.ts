import type { DcsTemplateData } from '../dcs-jsonld'
import type { ContractTemplateResponsible } from './contract-template-responsible'
import type { ContractTemplateState } from '@/types/contract-template-state'
import type { PartyType } from '@/types/party-type'
import type { TemplateType } from '@/types/template-type'

export const templateComplianceStatuses = ['UNASSESSED', 'COMPLIANT', 'NON_COMPLIANT'] as const
export type TemplateComplianceStatus = (typeof templateComplianceStatuses)[number]

/** DCS-FR-TR-10: the searchable template metadata. */
export interface TemplateSearchableMetadata {
  jurisdiction?: string
  industry?: string
  compliance_status?: TemplateComplianceStatus
}

export const templateLevels = ['FRAME_AGREEMENT', 'AGREEMENT', 'SUB_AGREEMENT', 'ADDENDUM'] as const
export type TemplateLevel = (typeof templateLevels)[number]

/** DCS-FR-TR-02, DCS-FR-TR-23: the hierarchy level and the templates a parent contract must come from. */
export interface TemplateStructure {
  template_level?: TemplateLevel
  required_templates?: string[]
}

const templateLevelRank: Record<TemplateLevel, number> = {
  FRAME_AGREEMENT: 0,
  AGREEMENT: 1,
  SUB_AGREEMENT: 1,
  ADDENDUM: 2,
}

/** A template can require only a template of a higher level (backend templatestructure.CheckDependency). */
export function canRequire(level: TemplateLevel, dependency: TemplateLevel): boolean {
  return templateLevelRank[dependency] < templateLevelRank[level]
}

const requirableStates: ReadonlySet<ContractTemplateState> = new Set(['APPROVED', 'REGISTERED', 'PUBLISHED'])

/** The templates a template of the given level can require: approved contract templates of a higher level. */
export function requirableTemplates(
  templates: ContractTemplate[],
  level: TemplateLevel,
  selfDid: string | null,
): ContractTemplate[] {
  return templates.filter(
    (template) =>
      template.did !== selfDid &&
      template.template_type === 'CONTRACT_TEMPLATE' &&
      requirableStates.has(template.state) &&
      canRequire(level, template.template_level ?? 'AGREEMENT'),
  )
}

/** DCS-IR-TR-04, DCS-IR-TR-05: a review or approval decision with its comments, or a rejection reason. */
export interface TemplateReviewNote {
  kind: 'RETURNED_TO_DRAFT' | 'FORWARDED_TO_APPROVAL' | 'RETURNED_TO_REVIEW' | 'REJECTED' | 'APPROVED'
  text: string[]
  /** Holder DID of the person who decided */
  by?: string
  created_at: string
}

export interface ContractTemplate extends TemplateSearchableMetadata, TemplateStructure {
  did: string
  created_by: string
  created_at: string
  version: number
  party_type: PartyType
  template_type: TemplateType
  state: ContractTemplateState
  name?: string
  description?: string
  template_data?: DcsTemplateData
  updated_at: string
  responsible?: ContractTemplateResponsible
  outdated?: boolean
  latest_did?: string
  review_notes?: TemplateReviewNote[]
}

export type PartialContractTemplate = ContractTemplate

export type ContractTemplateData = DcsTemplateData
