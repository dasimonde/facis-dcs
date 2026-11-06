import { afterEach, expect, it, vi } from 'vitest'
import { loadShapeLibraries } from './shape-library'

afterEach(() => {
  delete window.DCS_CONFIG
  vi.unstubAllGlobals()
})

// A backend-served UI reaches the API under its deployment path, so the
// Semantic Hub calls use the configured API base like every other call.
it('reads the Semantic Hub under the configured API base', async () => {
  window.DCS_CONFIG = { API_BASE_URL: '/digital-contracting-service/api' }
  const fetchMock = vi.fn().mockResolvedValue(new Response('[]', { status: 200 }))
  vi.stubGlobal('fetch', fetchMock)
  await loadShapeLibraries()
  expect(fetchMock).toHaveBeenCalledWith('/digital-contracting-service/api/semantic/schema/list', expect.anything())
})
