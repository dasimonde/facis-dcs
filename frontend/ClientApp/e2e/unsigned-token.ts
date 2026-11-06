import type { Page } from '@playwright/test'

/**
 * For specs that sign in with an unsigned token and mock the calls they need.
 *
 * An API call the spec does not mock still reaches the backend, but without
 * the unsigned token. The backend counts each invalid token from an address
 * toward a lockout of that address (FR-UC-01-4), and every spec of the run
 * shares one address, so sending the token would lock out the specs that sign
 * in for real. Without a token, a protected call is refused exactly as before
 * and a public one still answers.
 *
 * Registered on the context, so every route the spec registers on the page
 * takes precedence, whenever it is registered.
 */
export async function withholdUnsignedToken(page: Page): Promise<void> {
  await page.context().route(
    (url) => url.pathname.startsWith('/api/'),
    async (route) => {
      const headers = { ...route.request().headers() }
      delete headers.authorization
      await route.continue({ headers })
    },
  )
}
