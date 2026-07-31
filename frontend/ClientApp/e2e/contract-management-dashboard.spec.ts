import { applySession, expect, mintSession, test } from './dcs-test'
import {
  buildApprovedContract,
  buildDraftContract,
  type DraftContractFixture,
  gotoAs,
  type LoginAs,
} from './lifecycle-helpers'
import { E2E_FRONTEND_ORIGIN } from '../playwright.config'

/**
 * Contract Management dashboard (the UI half of the manager requirements whose
 * API half the BDD suite covers):
 *
 *   - DCS-IR-CWE-11: Managers retrieve and search contracts across lifecycle
 *     states — the dashboard search box and the lifecycle-state filter narrow
 *     the list to the expected contracts.
 *   - DCS-IR-CWE-07: the search locates contracts by metadata (name) and by
 *     identifier (DID).
 *   - DCS-IR-CWE-12: Managers terminate a contract — the Terminate action
 *     records a mandatory reason and the contract ends in TERMINATED.
 *
 * Both fixtures are authored through the real UI: one contract left in DRAFT
 * and one driven to APPROVED, so search and state filter genuinely
 * discriminate between lifecycle states.
 */

test.describe('contract management dashboard backed by contracts in two lifecycle states', () => {
  let draft: DraftContractFixture
  let approvedDid: string

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(240_000)
    // A throwaway context authors the fixtures so the tests' own page fixture
    // stays untouched (same pattern as buildDraftContractFixture).
    const context = await browser.newContext({ baseURL: E2E_FRONTEND_ORIGIN })
    const page = await context.newPage()
    const loginAs: LoginAs = async (role) => {
      await applySession(context, page, E2E_FRONTEND_ORIGIN, mintSession(role))
    }
    try {
      draft = await buildDraftContract(page, loginAs)
      approvedDid = await buildApprovedContract(page, loginAs)
    } finally {
      await context.close()
    }
  })

  test('the manager locates contracts by name and by DID through the dashboard search', async ({ page, loginAs }) => {
    // DCS-IR-CWE-11 // DCS-IR-CWE-07
    await loginAs('Contract Manager')
    const rows = page.locator('li.list-row')

    await test.step('a name search narrows the list to the DRAFT fixture', async () => {
      await page.goto('/ui/contracts')
      // The search box defaults to the DID field; pick Name from its filter
      // popover for a metadata search.
      await page.locator('#list-btn-search').click()
      await page.locator('#list-popover-search').getByText('Name', { exact: true }).click()
      const searched = page.waitForResponse((r) => r.url().includes('/contract/search') && r.ok())
      await page.getByLabel('Search contracts').fill(draft.contractTemplateName)
      await searched
      await page.getByRole('button', { name: 'Search', exact: true }).click()
      await expect(rows).toHaveCount(1)
      await expect(rows.first()).toContainText(draft.contractTemplateName)
      await expect(rows.first()).toContainText('DRAFT')
    })

    await test.step('a DID search narrows the list to the APPROVED contract', async () => {
      await page.goto('/ui/contracts')
      const searched = page.waitForResponse((r) => r.url().includes('/contract/search') && r.ok())
      await page.getByLabel('Search contracts').fill(approvedDid)
      await searched
      await page.getByRole('button', { name: 'Search', exact: true }).click()
      await expect(rows).toHaveCount(1)
      await expect(rows.first()).toContainText('APPROVED')
      await expect(rows.first()).not.toContainText(draft.contractTemplateName)
    })
  })

  test('the manager filters the contract list by lifecycle state', async ({ page, loginAs }) => {
    // DCS-IR-CWE-11
    await loginAs('Contract Manager')
    await page.goto('/ui/contracts')
    const rows = page.locator('li.list-row')
    const filterPopover = page.locator('#filter-popover')

    await test.step('the DRAFT filter keeps the draft fixture and hides approved contracts', async () => {
      await expect(rows.filter({ hasText: draft.contractTemplateName }).first()).toBeVisible()
      await page.locator('#popover-btn').click()
      await filterPopover.getByText('DRAFT', { exact: true }).click()
      await page.keyboard.press('Escape')
      await expect(rows.filter({ hasText: draft.contractTemplateName }).first()).toBeVisible()
      await expect(rows.filter({ hasText: 'APPROVED' })).toHaveCount(0)
    })

    await test.step('the APPROVED filter hides the draft fixture', async () => {
      await page.locator('#popover-btn').click()
      // Deselecting DRAFT restores the full state list inside the popover.
      await filterPopover.getByText('DRAFT', { exact: true }).click()
      await filterPopover.getByText('APPROVED', { exact: true }).click()
      await page.keyboard.press('Escape')
      await expect(rows.filter({ hasText: 'APPROVED' }).first()).toBeVisible()
      await expect(rows.filter({ hasText: draft.contractTemplateName })).toHaveCount(0)
    })
  })

  test('the manager terminates an approved contract with a recorded reason', async ({ page, loginAs }) => {
    // DCS-IR-CWE-12
    await gotoAs(page, loginAs, 'Contract Manager', `/ui/contracts/view/${approvedDid}`)
    await page.getByRole('button', { name: 'Terminate', exact: true }).click()

    // The confirmation modal demands a reason before Submit enables.
    const dialog = page.getByRole('dialog').filter({ hasText: 'Confirmation' })
    await expect(dialog.getByText('Proceed with terminating?')).toBeVisible()
    await dialog.getByPlaceholder('Reason').fill('Service agreement superseded by a renegotiated successor.')
    const terminated = page.waitForResponse(
      (r) => r.url().includes('/contract/terminate') && r.request().method() === 'POST',
    )
    await dialog.getByRole('button', { name: 'Submit', exact: true }).click()
    const resp = await terminated
    expect(resp.ok(), `contract terminate ${resp.status()}: ${await resp.text()}`).toBeTruthy()
    // A successful terminate navigates back to the contract list.
    await expect(page).toHaveURL(/\/ui\/contracts\/?$/)

    await gotoAs(page, loginAs, 'Contract Manager', `/ui/contracts/view/${approvedDid}`)
    await expect(page.getByText('TERMINATED', { exact: true }).locator('visible=true').first()).toBeVisible()
  })
})
