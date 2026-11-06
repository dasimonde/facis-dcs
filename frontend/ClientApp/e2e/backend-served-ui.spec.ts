import { expect, test } from './dcs-test'
import { E2E_API_BASE } from '../playwright.config'

/**
 * The other specs use the UI that `vite` serves. This spec opens the UI that
 * the DCS backend itself serves (backend/cmd/dcs/frontend.go) on the deployed
 * stack: the base href and config.js that the backend writes at serve time,
 * the security headers, and a page that loads its data under that CSP.
 */
const apiBase = new URL(E2E_API_BASE)
const uiPath = apiBase.pathname.replace(/\/api\/?$/, '/ui/')

test.use({ baseURL: apiBase.origin })

test('the backend serves the built UI with its deployment settings and CSP', async ({ page, loginAs }) => {
  const cspViolations: string[] = []
  page.on('console', (message) => {
    if (message.type() === 'error' && message.text().includes('Content Security Policy')) {
      cspViolations.push(message.text())
    }
  })

  await loginAs('Contract Creator')
  const loaded = page.waitForResponse(
    (r) => r.url().startsWith(apiBase.origin + apiBase.pathname + '/contract/') && r.ok(),
  )
  const response = await page.goto(uiPath + 'contracts')

  expect(response?.status()).toBe(200)
  expect(response?.headers()['content-security-policy']).toContain("script-src 'self'")
  expect(response?.headers()['x-frame-options']).toBe('DENY')
  await expect(page.locator('base')).toHaveAttribute('href', uiPath)

  const config = await page.request.get(uiPath + 'config.js')
  expect(await config.text()).toContain(JSON.stringify(apiBase.pathname))

  await loaded
  await expect(page.getByLabel('Search contracts')).toBeVisible()
  expect(cspViolations).toEqual([])
})
