export type TemplateEditorTabId = 'details' | 'semantic' | 'clauses' | 'builder' | 'meta'
export interface AddBlockModalContext {
  parentBlockId: string
  /** Index in the parent's children array where the new block will be inserted */
  insertIndex: number
}

export type BlockMovementPreview =
  | { type: 'vertical'; sourceBlockId: string; targetBlockId: string }
  | { type: 'horizontal'; blockId: string; direction: 'left' | 'right' }

/**
 * When non-null: clause editor highlights chips that match.
 * - conditionId only: highlight all placeholders for that semantic rule
 * - conditionId + parameterName: highlight placeholders for that param
 */
export type ClausePlaceholderHighlight =
  | { conditionId: string; parameterName?: string }
  | null

/** UI state for template create/edit page */
interface TemplateEditorUiState {
  activeTab: TemplateEditorTabId
  tabs: [
    { id: 'details', label: string },
    { id: 'semantic', label: string },
    { id: 'clauses', label: string },
    { id: 'builder', label: string },
    { id: 'meta', label: string },
  ],
  /**
   * When non-null: add-block modal is open
   */
  addBlockModalContext: AddBlockModalContext | null
  /** When non-null: movement preview is active */
  blockMovementPreview: BlockMovementPreview | null
  selectedBlockId: string | null
  /** When non-null: clause legal-text editor highlights matching placeholder chips */
  clausePlaceholderHighlight: ClausePlaceholderHighlight
  /** When true: builder preview dialog is open */
  isPreviewDialogOpen: boolean
  /** Whether the current template is in an editable state */
  isTemplateEditable: boolean
}

export type { TemplateEditorUiState }