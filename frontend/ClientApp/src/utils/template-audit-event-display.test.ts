import { expect, it } from 'vitest'
import { templateAuditEventDisplayText } from './template-audit-event-display'

/** DCS-FR-TR-21: the audit view tells a deletion from a deprecation. */
it('labels deprecation and deletion apart', () => {
  expect(templateAuditEventDisplayText('ARCHIVE_CONTRACT_TEMPLATE')).toBe('Template deprecated')
  expect(templateAuditEventDisplayText('DELETE_CONTRACT_TEMPLATE')).toBe('Template deleted')
})
