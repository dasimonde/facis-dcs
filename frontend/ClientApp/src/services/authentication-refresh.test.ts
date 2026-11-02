import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, expect, it, vi } from 'vitest'

const post = vi.hoisted(() => vi.fn())
vi.mock('@/api/auth-http', () => ({ default: { post } }))

import { authenticationService } from './authentication-service'

// A header.payload.signature token whose payload names a holder.
const token = `x.${btoa(JSON.stringify({ sub: 'did:web:holder', exp: 4102444800 }))}.y`

beforeEach(() => {
  setActivePinia(createPinia())
  post.mockReset()
})

// Hydra refuses a second refresh with the same refresh token, so requests that
// fail at the same moment must share one /auth/refresh call.
it('shares one refresh between concurrent callers', async () => {
  let resolve!: (value: unknown) => void
  post.mockReturnValue(new Promise((r) => (resolve = r)))
  const calls = [authenticationService.refresh(), authenticationService.refresh(), authenticationService.refresh()]
  resolve({ data: { token_type: 'Bearer', access_token: token } })
  await Promise.all(calls)
  expect(post).toHaveBeenCalledTimes(1)
})

it('starts a new refresh after the previous one has ended', async () => {
  post.mockResolvedValue({ data: { token_type: 'Bearer', access_token: token } })
  await authenticationService.refresh()
  await authenticationService.refresh()
  expect(post).toHaveBeenCalledTimes(2)
})
