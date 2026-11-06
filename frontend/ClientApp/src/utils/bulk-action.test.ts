import { expect, it, vi } from 'vitest'
import { bulkSummary, runBulk } from './bulk-action'

vi.mock('@/utils/report-action-error', () => ({ claimReportedHttpError: vi.fn() }))

// DCS-FR-CWE-24, DCS-FR-CSA-23: a bulk action calls the per-contract endpoint
// for each selected contract and a failure does not stop the others.
it('runs every item and summarizes the outcome', async () => {
  const action = vi.fn((item: { did: string }) =>
    item.did === 'b' ? Promise.reject(new Error('contract is terminated')) : Promise.resolve(),
  )
  const result = await runBulk([{ did: 'a' }, { did: 'b' }, { did: 'c' }], action)

  expect(action).toHaveBeenCalledTimes(3)
  expect(result).toEqual({ succeeded: ['a', 'c'], failed: [{ did: 'b', message: 'contract is terminated' }] })
  expect(bulkSummary('Terminate', result)).toBe('Terminate: 2 succeeded, 1 failed (b: contract is terminated).')
})
