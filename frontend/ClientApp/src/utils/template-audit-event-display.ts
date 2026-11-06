import { auditEventActor } from './audit-event-actor'
import { toProperCase } from './string'

const templateEventLabels: Record<string, string> = {
  CREATE_CONTRACT_TEMPLATE: 'Template created',
  UPDATE_CONTRACT_TEMPLATE: 'Template updated',
  SUBMIT_CONTRACT_TEMPLATE: 'Template submitted',
  APPROVE_CONTRACT_TEMPLATE: 'Template approved',
  REJECT_CONTRACT_TEMPLATE: 'Template rejected',
  VERIFY_CONTRACT_TEMPLATE: 'Template verified',
  ARCHIVE_CONTRACT_TEMPLATE: 'Template deprecated',
  DELETE_CONTRACT_TEMPLATE: 'Template deleted',
  REGISTER_CONTRACT_TEMPLATE: 'Template registered',
  PUBLISH_CONTRACT_TEMPLATE: 'Template published',
  COPY_CONTRACT_TEMPLATE: 'Template copied',
  AUDIT_CONTRACT_TEMPLATE: 'Template audited',
  SEARCH_CONTRACT_TEMPLATE: 'Templates searched',
  RETRIEVE_ALL_CONTRACT_TEMPLATES: 'Templates retrieved',
  RETRIEVE_CONTRACT_TEMPLATE_BY_ID: 'Template retrieved',
  RETRIEVE_CONTRACT_TEMPLATE_PROVENANCE: 'Template provenance retrieved',
  TEMPLATE_POLICY_AUDIT_FINDING: 'Policy finding',
}

export function templateAuditEventDisplayText(eventType?: string, eventData?: unknown): string {
  const normalized = eventType?.trim().toUpperCase()
  if (!normalized) return 'Audit event'

  if (normalized === 'TEMPLATE_POLICY_AUDIT_FINDING') {
    const title = stringField(eventData, 'title')
    if (title) return title
  }

  return templateEventLabels[normalized] ?? toProperCase(normalized)
}

export function templateAuditEventActor(eventData?: unknown): string | undefined {
  return auditEventActor(eventData)
}

function stringField(value: unknown, key: string): string | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return undefined
  }
  const raw = (value as Record<string, unknown>)[key]
  return typeof raw === 'string' && raw.trim() ? raw : undefined
}
