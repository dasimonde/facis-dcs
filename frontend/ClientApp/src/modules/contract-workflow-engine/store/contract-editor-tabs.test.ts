import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import { useContractEditorUiStore } from '@contract-workflow-engine/store/contractEditorUiStore'
import { useAuthStore } from '@/stores/auth-store'
import { ContractState } from '@/types/contract-state'
import type { UserRole } from '@/types/user-role'

/**
 * A tab strip is a control: selecting a tab the calling view has no pane for
 * leaves the page body empty. availableTabs is shared by five views with
 * different panes, so each one states what it renders.
 */

const tabIds = (
  state: ContractState,
  rendered: Parameters<ReturnType<typeof useContractEditorUiStore>['availableTabs']>[1],
) =>
  useContractEditorUiStore()
    .availableTabs(state, rendered)
    .map((tab) => tab.id)

function signIn(roles: UserRole[]) {
  useAuthStore().user = { issuer: 'did:web:example.com:org', holder: 'user', roles }
}

describe('contract editor tabs', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  // Only ViewContractView has a structure pane; Negotiate/Review/Approve
  // rendered the tab and then showed nothing.
  it('withholds a tab the calling view cannot render', () => {
    signIn(['CONTRACT_CREATOR'])

    expect(tabIds(ContractState.negotiation, ['details', 'content', 'diff', 'proposals', 'audit'])).toEqual([
      'details',
      'content',
      'diff',
      'proposals',
    ])
    expect(tabIds(ContractState.draft, ['details', 'content', 'audit', 'structure'])).toEqual([
      'details',
      'content',
      'structure',
    ])
  })

  it('offers Diff View for submitted and draft when the view renders it', () => {
    signIn(['CONTRACT_REVIEWER'])

    expect(tabIds(ContractState.submitted, ['details', 'content', 'diff', 'audit'])).toEqual([
      'details',
      'content',
      'diff',
    ])
    expect(tabIds(ContractState.draft, ['details', 'content', 'diff'])).toContain('diff')
    expect(tabIds(ContractState.submitted, ['details', 'content', 'audit'])).not.toContain('diff')
  })

  it('offers Proposals when the view renders it', () => {
    signIn(['CONTRACT_CREATOR'])

    expect(tabIds(ContractState.offered, ['details', 'content', 'proposals'])).toContain('proposals')
    expect(tabIds(ContractState.negotiation, ['details', 'content'])).not.toContain('proposals')
  })

  it('offers the audit tab to the roles /contract/audit scopes', () => {
    signIn(['CONTRACT_CREATOR', 'AUDITOR'])

    expect(tabIds(ContractState.draft, ['details', 'content', 'audit'])).toContain('audit')
  })

  // U3: every party that can open the contract sees who moved it through its lifecycle.
  it.each<UserRole>(['CONTRACT_OBSERVER', 'CONTRACT_REVIEWER', 'AUDITOR'])(
    'offers the lifecycle history to %s',
    (role) => {
      signIn([role])

      expect(tabIds(ContractState.signed, ['details', 'content', 'audit', 'history', 'structure'])).toContain('history')
    },
  )

  it('keeps the audit tab from a party role without audit scope', () => {
    signIn(['CONTRACT_OBSERVER'])

    expect(tabIds(ContractState.signed, ['details', 'content', 'audit', 'history', 'structure'])).toEqual([
      'details',
      'content',
      'history',
      'structure',
    ])
  })

  // Sys. Administrator is in no contract route's roles and in no /contract/audit
  // scope, so the branch could never produce a working tab.
  it('does not offer the audit tab to a system administrator', () => {
    signIn(['CONTRACT_CREATOR', 'SYSTEM_ADMINISTRATOR'])

    expect(tabIds(ContractState.draft, ['details', 'content', 'audit'])).not.toContain('audit')
  })
})
