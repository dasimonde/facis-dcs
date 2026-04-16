import type { DocumentBlock, DocumentOutline, MetaData, SemanticCondition } from "@/modules/template-repository/models/contract-templace"
import type { ContractTemplateState } from "@/types/contract-template-state"
import type { TemplateType } from "@/types/template-type"

export interface ContractTemplate {
    did: string
    created_by: string
    created_at: string
    document_number?: string
    version?: number
    template_type: TemplateType
    state: ContractTemplateState
    name?: string
    description?: string
    template_data?: ContractTemplateData
    updated_at: string
}

export type PartialContractTemplate = Omit<ContractTemplate, 'template_data'>

export interface ContractTemplateData {
    documentOutline: DocumentOutline
    semanticConditions: SemanticCondition[]
    documentBlocks: DocumentBlock[]
    customMetaData: MetaData[]
    // Only when the template is a frame contract, it can have sub-templates
    subTemplateSnapshots?: SubTemplateSnapshot[]
}

export interface SubTemplateSnapshot {
    did: string
    document_number?: string
    version?: number
    name?: string
    description?: string
    template_data?: Omit<ContractTemplateData, 'subTemplateSnapshots'>
}