import { AxiosError } from 'axios'
import { expect, it, vi } from 'vitest'
import { contractWorkflowService } from './contract-workflow-service'

const get = vi.hoisted(() => vi.fn())
vi.mock('@/api/http', () => ({ default: { get } }))

it('preserves the original retrieve failure for caller-owned feedback', async () => {
  const error = new AxiosError('Denied')
  get.mockRejectedValueOnce(error)
  await expect(contractWorkflowService.retrieveById({ did: 'did:test:denied' })).rejects.toBe(error)
  expect(get).toHaveBeenCalledWith('/contract/retrieve/did:test:denied')
})
