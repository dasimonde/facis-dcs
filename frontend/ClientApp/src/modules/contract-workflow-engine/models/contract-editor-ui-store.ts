export type ContractEditorTabId =
  | 'details'
  | 'content'
  | 'clauses'
  | 'builder'
  | 'diff'
  | 'proposals'
  | 'audit'
  | 'history'
  | 'structure'

interface ContractEditorUiState {
  activeTab: ContractEditorTabId
  tabs: [
    { id: 'details'; label: string },
    { id: 'content'; label: string },
    { id: 'clauses'; label: string },
    { id: 'builder'; label: string },
    { id: 'diff'; label: string },
    { id: 'proposals'; label: string },
    { id: 'audit'; label: string },
    { id: 'history'; label: string },
    { id: 'structure'; label: string },
  ]
}

export type { ContractEditorUiState }
