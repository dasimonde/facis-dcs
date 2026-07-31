export type UserRole =
  | 'TEMPLATE_CREATOR'
  | 'TEMPLATE_REVIEWER'
  | 'TEMPLATE_APPROVER'
  | 'TEMPLATE_MANAGER'
  | 'CONTRACT_CREATOR'
  | 'CONTRACT_REVIEWER'
  | 'CONTRACT_APPROVER'
  | 'CONTRACT_MANAGER'
  | 'CONTRACT_NEGOTIATOR'
  | 'CONTRACT_SIGNER'
  | 'CONTRACT_OBSERVER'
  | 'ARCHIVE_MANAGER'
  | 'AUDITOR'
  | 'SYSTEM_ADMINISTRATOR'
  | 'COMPLIANCE_OFFICER'
  | 'INTEGRATION_MANAGER'
  | 'PROCESS_ORCHESTRATOR'
  | 'VALIDATOR'

/** Maps access-token role claim labels to UserRole ids. */
const ROLE_LABEL_TO_USER_ROLE: Record<string, UserRole> = {
  'Template Creator': 'TEMPLATE_CREATOR',
  'Template Reviewer': 'TEMPLATE_REVIEWER',
  'Template Approver': 'TEMPLATE_APPROVER',
  'Template Manager': 'TEMPLATE_MANAGER',
  'Contract Creator': 'CONTRACT_CREATOR',
  'Contract Reviewer': 'CONTRACT_REVIEWER',
  'Contract Approver': 'CONTRACT_APPROVER',
  'Contract Manager': 'CONTRACT_MANAGER',
  'Contract Negotiator': 'CONTRACT_NEGOTIATOR',
  'Contract Signer': 'CONTRACT_SIGNER',
  'Contract Observer': 'CONTRACT_OBSERVER',
  'Archive Manager': 'ARCHIVE_MANAGER',
  Auditor: 'AUDITOR',
  'Sys. Administrator': 'SYSTEM_ADMINISTRATOR',
  'Compliance Officer': 'COMPLIANCE_OFFICER',
  'Integration Manager': 'INTEGRATION_MANAGER',
  'Process Orchestrator': 'PROCESS_ORCHESTRATOR',
  Validator: 'VALIDATOR',
}

/** Reads roles from a JWT payload. */
export function rolesFromJwtPayload(payload: Record<string, unknown> | null | undefined): unknown {
  if (!payload) return []
  if (Array.isArray(payload.roles)) return payload.roles
  const ext = payload.ext
  if (ext && typeof ext === 'object' && Array.isArray((ext as Record<string, unknown>).roles)) {
    return (ext as Record<string, unknown>).roles
  }
  return []
}

export function mapRoleLabelsToUserRoles(roles: unknown): UserRole[] {
  if (!Array.isArray(roles)) return []
  const mapped: UserRole[] = []
  for (const r of roles) {
    if (typeof r !== 'string') continue
    const role = ROLE_LABEL_TO_USER_ROLE[r]
    if (role) mapped.push(role)
  }
  return mapped
}
