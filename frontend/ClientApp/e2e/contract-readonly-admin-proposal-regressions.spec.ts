import { expect, test } from './dcs-test'
import { buildApprovedContract, buildDraftContract, FIXTURE_CLAUSE_PROSE, gotoAs } from './lifecycle-helpers'
import type { Page } from '@playwright/test'

async function expectNoPageOverflow(page: Page) {
  const dimensions = await page.evaluate(() => ({
    clientWidth: document.documentElement.clientWidth,
    scrollWidth: document.documentElement.scrollWidth,
  }))
  expect(dimensions.scrollWidth).toBeLessThanOrEqual(dimensions.clientWidth)
}

test('@REQ-contract-readonly-event-admin-diff-regressions-AC1 terminal View Contract disables every placeholder control', async ({
  page,
  loginAs,
}) => {
  test.setTimeout(120_000)
  const contractDid = await buildApprovedContract(page, loginAs)

  await gotoAs(page, loginAs, 'Contract Manager', `/ui/contracts/view/${contractDid}`)
  await page.getByRole('button', { name: 'Terminate', exact: true }).click()
  const dialog = page.getByRole('dialog').filter({ hasText: 'Confirmation' })
  await dialog.getByPlaceholder('Reason').fill('Read-only contract viewer acceptance regression.')
  const terminated = page.waitForResponse(
    (response) => response.url().includes('/contract/terminate') && response.request().method() === 'POST',
  )
  await dialog.getByRole('button', { name: 'Submit', exact: true }).click()
  expect((await terminated).ok()).toBeTruthy()

  await gotoAs(page, loginAs, 'Contract Manager', `/ui/contracts/view/${contractDid}`)
  await page
    .getByRole('tab', { name: /content/i })
    .or(page.getByText('Contract Content', { exact: true }))
    .first()
    .click()

  const placeholderControls = page
    .getByTestId('contract-readonly-preview')
    .locator('input[aria-label], select[aria-label]')
  expect(await placeholderControls.count()).toBeGreaterThan(0)
  for (const control of await placeholderControls.all()) {
    await expect(control).toBeDisabled()
  }
})

test('@REQ-contract-readonly-event-admin-diff-regressions-AC3 admin groups remain labelled and page-contained at 375 and 1280', async ({
  page,
  loginAs,
}) => {
  await loginAs('Sys. Administrator')

  const pages = [
    {
      path: '/ui/admin/targets',
      root: 'target-admin',
      headings: ['Target system configuration', 'Registered target systems'],
    },
    {
      path: '/ui/admin/system-users',
      root: 'machine-identity-admin',
      headings: ['System user configuration', 'Registered system users'],
    },
    {
      path: '/ui/admin/hsm-keys',
      root: 'key-inventory',
      headings: ['Active key versions'],
    },
  ]

  for (const width of [1280, 375]) {
    await page.setViewportSize({ width, height: 800 })
    for (const adminPage of pages) {
      await page.goto(adminPage.path)
      await expect(page.getByTestId(adminPage.root)).toBeVisible()
      for (const heading of adminPage.headings) {
        await expect(page.getByRole('heading', { name: heading })).toBeVisible()
      }
      await expectNoPageOverflow(page)
    }
  }
})

test('@REQ-contract-readonly-event-admin-diff-regressions-AC4 selected proposal shows complete Current and Proposed snapshots responsively', async ({
  page,
  loginAs,
}) => {
  test.setTimeout(120_000)
  const { contractDid } = await buildDraftContract(page, loginAs)

  await gotoAs(page, loginAs, 'Contract Creator', `/ui/contracts/edit/${contractDid}`)
  await page
    .getByRole('tab', { name: /content/i })
    .or(page.getByText('Contract Content', { exact: true }))
    .first()
    .click()
  const amount = page
    .getByRole('spinbutton', { name: /amount/i })
    .or(page.getByRole('textbox', { name: /amount/i }))
    .first()
  await amount.fill('250')
  const submitted = page.waitForResponse(
    (response) => response.url().includes('/contract/submit') && response.request().method() === 'POST',
  )
  await page.getByRole('button', { name: 'Submit', exact: true }).click()
  expect((await submitted).ok()).toBeTruthy()

  await gotoAs(page, loginAs, 'Contract Creator', `/ui/contracts/negotiate/${contractDid}`)
  await page
    .getByRole('tab', { name: /content/i })
    .or(page.getByText('Contract Content', { exact: true }))
    .first()
    .click()
  const proposedAmount = page
    .getByRole('spinbutton', { name: /amount/i })
    .or(page.getByRole('textbox', { name: /amount/i }))
    .first()
  await proposedAmount.fill('300')
  const proposed = page.waitForResponse(
    (response) => response.url().includes('/contract/negotiate') && response.request().method() === 'POST',
  )
  await page.getByRole('button', { name: 'Change Proposal', exact: true }).click()
  const proposedResponse = await proposed
  expect(proposedResponse.ok()).toBeTruthy()
  const proposalPayload = proposedResponse.request().postDataJSON() as {
    change_request?: {
      contract_data?: Record<string, unknown>
    }
  }
  const proposedDocument = proposalPayload.change_request?.contract_data
  expect(proposedDocument, 'the proposal carries a complete contract snapshot').toMatchObject({
    '@type': 'dcs:Contract',
    'dcs:documentStructure': {
      'dcs:blocks': { '@list': expect.any(Array) },
    },
    'dcs:contractFields': expect.any(Array),
    'dcs:contractData': expect.any(Array),
  })
  expect(proposedDocument?.['dcs:policies']).toBeTruthy()

  // Off-canvas by design (NegotiateContractView parks the list at -100vw):
  // a coordinate click cannot land, so the event is dispatched directly.
  await page.getByRole('button', { name: 'Show', exact: true }).first().dispatchEvent('click')
  const comparison = page.getByTestId('proposal-comparison')
  await expect(comparison).toBeVisible()
  await expect(comparison.getByRole('heading', { name: 'Current contract' })).toBeVisible()
  await expect(comparison.getByRole('heading', { name: 'Proposed contract' })).toBeVisible()
  await expect(comparison.getByText('Current contract content', { exact: true })).toBeVisible()
  await expect(comparison.getByText('Proposed contract content', { exact: true })).toBeVisible()
  await expect(comparison.getByText(FIXTURE_CLAUSE_PROSE, { exact: false })).toHaveCount(2)
  await expect(comparison.getByText('250', { exact: true })).toBeVisible()
  await expect(comparison.getByText('300', { exact: true })).toBeVisible()

  await page.setViewportSize({ width: 1280, height: 800 })
  const currentDesktop = await page.getByTestId('proposal-current').boundingBox()
  const proposedDesktop = await page.getByTestId('proposal-proposed').boundingBox()
  expect(currentDesktop?.y).toBe(proposedDesktop?.y)

  await page.setViewportSize({ width: 375, height: 800 })
  const currentNarrow = await page.getByTestId('proposal-current').boundingBox()
  const proposedNarrow = await page.getByTestId('proposal-proposed').boundingBox()
  expect((proposedNarrow?.y ?? 0) > (currentNarrow?.y ?? 0)).toBeTruthy()
  await expectNoPageOverflow(page)
})
