import { defineStore } from 'pinia'
import { useAuthStore } from '@/stores/auth-store'
import { contractAuditRoles } from '@/types/contract-audit-roles'
import type { ContractState as ContractStateType } from '@/types/contract-state'
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
    { id: 'proposals', label: 'Proposals' },
    { id: 'audit', label: 'Audit History' },
    { id: 'history', label: 'Lifecycle History' },
    { id: 'structure', label: 'Structure' },
  ],
}

export const useContractEditorUiStore = defineStore(storeId, {
  state: (): ContractEditorUiState => getInitialState(),
  actions: {
    setActiveTab(tab: ContractEditorTabId) {
      this.activeTab = tab
    },
    // `rendered` is the set of tabs the calling view actually has a pane for;
    // a tab outside it would select an empty page body.
    availableTabs(_contractState: ContractStateType, rendered: ContractEditorTabId[]) {
      const isAuditingAuthorized =
        contractAuditRoles.some((role) => useAuthStore().user?.roles?.includes(role)) ?? false

      const forState: ContractEditorTabId[] = ['details', 'content', 'diff', 'proposals', 'history', 'structure']

      return this.tabs.filter(
        (tab) =>
          rendered.includes(tab.id) && (forState.includes(tab.id) || (isAuditingAuthorized && tab.id === 'audit')),
      )
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
