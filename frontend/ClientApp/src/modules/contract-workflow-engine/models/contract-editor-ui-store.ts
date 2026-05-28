export type ContractEditorTabId = 'details' | 'content' | 'semantic' | 'clauses' | 'builder' | 'diff' | 'audit'

interface ContractEditorUiState {
  activeTab: ContractEditorTabId
  tabs: [
    { id: 'details'; label: string },
    { id: 'content'; label: string },
    { id: 'semantic'; label: string },
    { id: 'clauses'; label: string },
    { id: 'builder'; label: string },
    { id: 'diff'; label: string },
    { id: 'audit'; label: string },
  ]
}

export type { ContractEditorUiState }
