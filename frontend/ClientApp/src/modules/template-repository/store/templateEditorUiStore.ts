import { defineStore } from 'pinia'
import { TemplateType, type TemplateTypeValue } from '@template-repository/models/contract-template'
import { useAuthStore } from '@/stores/auth-store'
import type { UserRole } from '@/types/user-role'
import type {
  BlockMovementPreview,
  ClausePlaceholderHighlight,
  PendingClauseDraft,
  TemplateEditorTabId,
  TemplateEditorUiState,
} from '@template-repository/models/template-editor-ui-store'

const storeId = 'templateEditorUi'
const defaultState: Readonly<TemplateEditorUiState> = {
  activeTab: 'details',
  tabs: [
    { id: 'details', label: 'Details' },
    { id: 'clauses', label: 'Clauses' },
    { id: 'builder', label: 'Builder' },
    { id: 'data', label: 'Data' },
    { id: 'meta', label: 'Meta Data' },
    { id: 'audit', label: 'Audit History' },
  ],
  addBlockModalContext: null,
  blockMovementPreview: null,
  selectedBlockId: null,
  clausePlaceholderHighlight: null,
  pendingClauseDraft: null,
  pendingPlacementClauseBlockId: null,
  isPreviewDialogOpen: false,
  isTemplateEditable: false,
  workflow: 'template',
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
      this.pendingPlacementClauseBlockId = null
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
    startClauseDraft(value: PendingClauseDraft) {
      this.pendingClauseDraft = value
      this.activeTab = 'clauses'
      this.clausePlaceholderHighlight = null
    },
    clearPendingClauseDraft() {
      this.pendingClauseDraft = null
    },
    startClausePlacement(blockId: string) {
      this.pendingPlacementClauseBlockId = blockId
      this.activeTab = 'builder'
    },
    clearPendingClausePlacement() {
      this.pendingPlacementClauseBlockId = null
    },
    togglePreviewDialog() {
      this.isPreviewDialogOpen = !this.isPreviewDialogOpen
    },
    availableTabs(templateType: TemplateTypeValue) {
      const isAuditingAuthorized =
        (['AUDITOR', 'COMPLIANCE_OFFICER'] as UserRole[]).some((role) => useAuthStore().user?.roles?.includes(role)) ??
        false
      const tabs = this.tabs.filter((tab) => tab.id !== 'audit' || isAuditingAuthorized)
      if (templateType === TemplateType.component) return tabs
      return tabs.filter((tab) => tab.id !== 'clauses')
    },
    setTemplateEditable(isEditable: boolean) {
      this.isTemplateEditable = isEditable
    },
    reset(overrides?: Partial<TemplateEditorUiState>) {
      Object.assign(this, getInitialState())
      if (overrides) Object.assign(this, overrides)
    },
  },
})

function getInitialState(): TemplateEditorUiState {
  return {
    ...defaultState,
    tabs: [...defaultState.tabs],
  }
}
