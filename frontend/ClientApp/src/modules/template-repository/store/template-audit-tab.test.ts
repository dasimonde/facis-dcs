import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it } from 'vitest'
import { TemplateType } from '@template-repository/models/contract-template'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'
import { useAuthStore } from '@/stores/auth-store'
import type { UserRole } from '@/types/user-role'

/** DCS-IR-TR-07: the Template Manager audits templates from the dashboard. */
function hasAuditTab(roles: UserRole[]) {
  setActivePinia(createPinia())
  useAuthStore().user = { issuer: 'did:web:example.com:org', holder: 'user', roles }
  return useTemplateEditorUiStore()
    .availableTabs(TemplateType.contractTemplate)
    .some((tab) => tab.id === 'audit')
}

describe('template audit tab', () => {
  it('is shown to the Template Manager', () => {
    expect(hasAuditTab(['TEMPLATE_MANAGER'])).toBe(true)
  })

  it('is hidden from the Template Creator', () => {
    expect(hasAuditTab(['TEMPLATE_CREATOR'])).toBe(false)
  })
})
