import { defineStore } from 'pinia'
import { useAuthStore } from '@/stores/auth-store'
import { ContractState } from '@/types/contract-state'
import type { ContractState as ContractStateType } from '@/types/contract-state'
import type { UserRole } from '@/types/user-role'
import type {
  ContractEditorTabId,
  ContractEditorUiState,
} from '@contract-workflow-engine/models/contract-editor-ui-store'

const storeId = 'contractEditorUi'
const defaultState: Readonly<ContractEditorUiState> = {
  activeTab: 'details',
  tabs: [
    { id: 'details', label: 'Contract Details' },
    { id: 'content', label: 'Contract Content' },
    { id: 'clauses', label: 'Clauses' },
    { id: 'builder', label: 'Builder' },
    { id: 'diff', label: 'Diff View' },
    { id: 'audit', label: 'Audit History' },
    { id: 'structure', label: 'Structure' },
  ],
}

export const useContractEditorUiStore = defineStore(storeId, {
  state: (): ContractEditorUiState => getInitialState(),
  actions: {
    setActiveTab(tab: ContractEditorTabId) {
      this.activeTab = tab
    },
    availableTabs(contractState: ContractStateType) {
      const isAuditingAuthorized =
        (['AUDITOR', 'COMPLIANCE_OFFICER', 'SYSTEM_ADMINISTRATOR'] as UserRole[]).some((role) =>
          useAuthStore().user?.roles?.includes(role),
        ) ?? false

      switch (contractState) {
        case ContractState.negotiation:
          return this.tabs.filter(
            (tab) =>
              ['details', 'content', 'diff', 'structure'].includes(tab.id) ||
              (isAuditingAuthorized && tab.id === 'audit'),
          )
        default:
          return this.tabs.filter(
            (tab) =>
              ['details', 'content', 'structure'].includes(tab.id) || (isAuditingAuthorized && tab.id === 'audit'),
          )
      }
    },
    reset(overrides?: Partial<ContractEditorUiState>) {
      Object.assign(this, getInitialState())
      if (overrides) Object.assign(this, overrides)
    },
  },
})

function getInitialState(): ContractEditorUiState {
  return {
    ...defaultState,
    tabs: [...defaultState.tabs],
  }
}
