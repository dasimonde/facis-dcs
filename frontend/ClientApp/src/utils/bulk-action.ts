import { useErrorStore } from '@/stores/error-store'
import { claimReportedHttpError } from '@/utils/report-action-error'

export interface BulkResult {
  succeeded: string[]
  failed: { did: string; message: string }[]
}

/**
 * Runs one existing per-contract call for each selected contract, one after the
 * other, and collects the outcome per contract (DCS-FR-CWE-24, DCS-FR-CSA-23).
 * The per-call error toasts are replaced by one summary.
 */
export async function runBulk<T extends { did: string }>(
  items: T[],
  action: (item: T) => Promise<unknown>,
): Promise<BulkResult> {
  const result: BulkResult = { succeeded: [], failed: [] }
  for (const item of items) {
    try {
      await action(item)
      result.succeeded.push(item.did)
    } catch (err) {
      claimReportedHttpError(err)
      result.failed.push({ did: item.did, message: err instanceof Error ? err.message : String(err) })
    }
  }
  return result
}

export function bulkSummary(label: string, result: BulkResult): string {
  const failures = result.failed.map((failure) => `${failure.did}: ${failure.message}`).join('; ')
  return `${label}: ${result.succeeded.length} succeeded, ${result.failed.length} failed${failures ? ` (${failures})` : ''}.`
}

/** Shows the summary, as an error when any contract failed. */
export function reportBulk(label: string, result: BulkResult): void {
  useErrorStore().add(bulkSummary(label, result), result.failed.length ? 'error' : 'info', 8000)
}
