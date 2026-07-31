import { flattenPolicySet } from '@template-repository/store/dcsDraftStore'
import {
  collectDeclaredRequirements,
  fromDocumentSemanticValues,
} from '@contract-workflow-engine/utils/semantic-condition-values'
import { isDcsDocumentData } from '@/models/dcs-jsonld'
import type { SemanticConditionValue } from '@/models/contract/contract-data'
import type { DcsBlock, DcsContractData, DcsContractField, DcsLayoutNode, OdrlRule } from '@/models/dcs-jsonld'

export interface PreprocessedContractData {
  blocks: DcsBlock[]
  layout: DcsLayoutNode[]
  /** Flat dcs:ContractField declarations (the negotiable terms). */
  contractFields: DcsContractField[]
  /** Typed domain objects binding to fields by @id. */
  contractData: DcsContractData['dcs:contractData']
  /** Flattened from the stored enclosing odrl:Set — dcsDraftStore keeps the flat rule array as its internal source of truth. */
  policies: OdrlRule[]
  semanticConditionValues: SemanticConditionValue[]
  derivedFromTemplate?: DcsContractData['derivedFromTemplate']
}

/**
 * Preprocesses contract data (DcsContractData) for the contract workflow
 * engine. A document is self-contained: its blocks, layout, fields and typed
 * domain objects are already explicit, so this just reads them through.
 */
export function useContractDataPreprocess() {
  function preprocessContractData(cd: unknown): PreprocessedContractData | null {
    if (!isDcsDocumentData(cd)) return null

    const contractData = cd as DcsContractData
    return {
      blocks: contractData['dcs:documentStructure']['dcs:blocks']['@list'],
      layout: contractData['dcs:documentStructure']['dcs:layout']['@list'],
      contractFields: contractData['dcs:contractFields'],
      contractData: contractData['dcs:contractData'],
      policies: flattenPolicySet(contractData['dcs:policies']),
      semanticConditionValues: fromDocumentSemanticValues(collectDeclaredRequirements(contractData)),
      derivedFromTemplate: contractData.derivedFromTemplate,
    }
  }

  return { preprocessContractData }
}
