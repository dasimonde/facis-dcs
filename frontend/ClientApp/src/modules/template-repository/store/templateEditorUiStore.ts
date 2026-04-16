import { defineStore } from 'pinia'
import type {
  TemplateEditorUiState,
  TemplateEditorTabId,
  BlockMovementPreview,
  ClausePlaceholderHighlight,
} from '@template-repository/models/template-editor-ui-store'
import { TemplateType, type TemplateTypeValue } from '../models/contract-templace'

const storeId = 'templateEditorUi'
const defaultState: Readonly<TemplateEditorUiState> = {
  activeTab: 'details',
  tabs: [
    { id: 'details', label: 'Details' },
    { id: 'semantic', label: 'Semantic Rules' },
    { id: 'clauses', label: 'Clauses' },
    { id: 'builder', label: 'Builder' },
    { id: 'meta', label: 'Meta Data' },
  ],
  addBlockModalContext: null,
  blockMovementPreview: null,
  selectedBlockId: null,
  clausePlaceholderHighlight: null,
  isPreviewDialogOpen: false,
  isTemplateEditable: false
}

export const useTemplateEditorUiStore = defineStore(storeId, {
  state: (): TemplateEditorUiState => getInitialState(),
  getters: {},
  actions: {
    setActiveTab(tab: TemplateEditorTabId) {
      this.activeTab = tab
      // Clear clause chip highlight when leaving Clauses tab
      this.clausePlaceholderHighlight = null
    },
    openAddBlockModal(parentBlockId: string, insertIndex: number) {
      this.addBlockModalContext = { parentBlockId, insertIndex }
    },
    closeAddBlockModal() {
      this.addBlockModalContext = null
    },
    setBlockMovementPreview(value: BlockMovementPreview | null) {
      this.blockMovementPreview = value
    },
    setSelectedBlockId(blockId: string | null) {
      this.selectedBlockId = blockId
    },
    setClausePlaceholderHighlight(value: ClausePlaceholderHighlight) {
      this.clausePlaceholderHighlight = value
    },
    togglePreviewDialog() {
      this.isPreviewDialogOpen = !this.isPreviewDialogOpen
    },
    availableTabs(templateType: TemplateTypeValue) {
      if (templateType === TemplateType.subContract) return this.tabs
      return this.tabs.filter(tab => !['semantic', 'clauses'].includes(tab.id))
    },
    setTemplateEditable(isEditable: boolean) {
      this.isTemplateEditable = isEditable
    },
    reset(overrides?: Partial<TemplateEditorUiState>) {
      Object.assign(this, getInitialState())
      if (overrides) Object.assign(this, overrides)
    }
  }
})

function getInitialState(): TemplateEditorUiState {
  return {
    ...defaultState,
    tabs: [...defaultState.tabs]
  }
}