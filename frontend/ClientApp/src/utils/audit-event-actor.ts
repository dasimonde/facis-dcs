/** Prefer these actor fields when several `*_by` values are present. */
const preferredActorFieldKeys = [
  'offered_by',
  'withdrawn_by',
  'created_by',
  'updated_by',
  'submitted_by',
  'retrieved_by',
  'verified_by',
  'negotiated_by',
  'accepted_by',
  'rejected_by',
  'approved_by',
  'terminated_by',
  'renewed_by',
  'recorded_by',
  'audited_by',
  'reviewed_by',
  'exported_by',
  'stored_by',
  'deleted_by',
  'shredded_by',
  'annotated_by',
  'archived_by',
  'registered_by',
  'published_by',
  'copied_by',
  'applied_by',
  'validated_by',
  'revoked_by',
  'checked_by',
  'generated_by',
  'monitored_by',
  'reported_by',
] as const

/**
 * Participant DID attributed on an audit event payload.
 */
export function auditEventActor(eventData?: unknown): string | undefined {
  if (!eventData || typeof eventData !== 'object' || Array.isArray(eventData)) {
    return undefined
  }
  const data = eventData as Record<string, unknown>

  for (const key of preferredActorFieldKeys) {
    const value = data[key]
    if (typeof value === 'string' && value.trim()) {
      return value.trim()
    }
  }

  for (const [key, value] of Object.entries(data)) {
    if (!key.endsWith('_by')) continue
    if (typeof value === 'string' && value.trim()) {
      return value.trim()
    }
  }

  return undefined
}
