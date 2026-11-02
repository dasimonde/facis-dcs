import { applySession, expect, mintSession, test } from './dcs-test'
import { buildDraftContractFixture, gotoAs, type LoginAs } from './lifecycle-helpers'
import { E2E_FRONTEND_ORIGIN } from '../playwright.config'

/**
 * Live updates of the contract dashboard (DCS-FR-CWE-24) against the real
 * backend: a Contract Manager keeps the contract list open in one browser
 * context while a Contract Creator renames the contract in another. The list
 * receives the change over GET /contract/events, fetches that contract again
 * and shows the new name without a page reload.
 */

test('a change in one browser shows in the contract list of another without a reload', async ({
  browser,
  page,
  loginAs,
}) => {
  // DCS-FR-CWE-24
  test.setTimeout(300_000)
  page.setDefaultTimeout(15_000)
  const fixture = await buildDraftContractFixture(browser)
  const renamed = `Live renamed ${Date.now()}`
  const rows = page.locator('li.list-row')

  await test.step('the manager opens the contract list on the draft', async () => {
    const connected = page.waitForResponse((r) => r.url().includes('/contract/events') && r.ok(), { timeout: 60_000 })
    await gotoAs(page, loginAs, 'Contract Manager', '/ui/contracts')
    await connected
    const search = page.getByRole('combobox', { name: 'Search contracts' })
    const searched = page.waitForResponse((r) => r.url().includes('/contract/search') && r.ok())
    await search.fill(fixture.contractDid)
    await search.press('Enter')
    await searched
    await expect(rows).toHaveCount(1)
    await expect(rows.first()).toContainText(fixture.contractTemplateName)
    // Survives only while the page is not loaded again.
    await page.evaluate(() => {
      ;(window as unknown as { liveMarker?: string }).liveMarker = 'still here'
    })
  })

  await test.step('the creator renames the contract in another browser context', async () => {
    const context = await browser.newContext({ baseURL: E2E_FRONTEND_ORIGIN })
    const creatorPage = await context.newPage()
    const loginAsCreator: LoginAs = async (role) => {
      await applySession(context, creatorPage, E2E_FRONTEND_ORIGIN, mintSession(role))
    }
    try {
      await gotoAs(creatorPage, loginAsCreator, 'Contract Creator', `/ui/contracts/edit/${fixture.contractDid}`)
      const name = creatorPage.getByTestId('contract-global-name')
      await expect(name).toHaveValue(fixture.contractTemplateName)
      await name.fill(renamed)
      const updated = creatorPage.waitForResponse(
        (r) => r.url().includes('/contract/update') && r.request().method() === 'PUT',
      )
      await creatorPage.getByRole('button', { name: 'Update', exact: true }).click()
      const resp = await updated
      expect(resp.ok(), `contract update ${resp.status()}: ${await resp.text()}`).toBeTruthy()
    } finally {
      await context.close()
    }
  })

  await test.step('the manager list shows the new name without a reload', async () => {
    await expect(rows).toHaveCount(1)
    await expect(rows.first()).toContainText(renamed, { timeout: 30_000 })
    expect(await page.evaluate(() => (window as unknown as { liveMarker?: string }).liveMarker)).toBe('still here')
  })
})
