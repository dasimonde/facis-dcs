import { expect, test } from './dcs-test'
import { buildDraftContract, gotoAs } from './lifecycle-helpers'

/**
 * The offer gate on the contract view: "Offer to counterparty" is the first
 * transmission of a draft to its counterparty (DRAFT -> OFFERED), and SRS §1.2
 * defines an offer as a clear and DEFINITE proposal — §2.2.2 requires Contract
 * Generation to end with a filled-out contract "ready to be sent to the
 * Responder". command/offer.go's validateOfferReady enforces contract
 * closedness server-side (HTTP 400, covered by the BDD state-machine pack),
 * and ContractManagerActions mirrors it in the UI: while a required
 * placeholder is unfilled the button is disabled and names the missing field;
 * once the Contract Creator fills it through the real edit UI, the same button
 * performs DRAFT -> OFFERED.
 *
 * The fixture walks the same path a user does — author a Payment component with
 * an inline required placeholder, drive it through its template lifecycle,
 * compose + approve + register a contract template from it, derive the contract
 * — all through real UI buttons (lifecycle-helpers).
 */

test('offer to counterparty is blocked until the draft is filled out', async ({ page, loginAs }) => {
  // DCS-FR-CWE-17
  test.setTimeout(60_000)

  const { contractDid } = await buildDraftContract(page, loginAs)

  await test.step('the offer action is disabled while the draft is unfilled, naming the missing field', async () => {
    await gotoAs(page, loginAs, 'Contract Creator', `/ui/contracts/view/${contractDid}`)
    await expect(page.getByText('DRAFT', { exact: true }).first()).toBeVisible({ timeout: 15_000 })
    const offerButton = page.getByRole('button', { name: 'Offer to counterparty' })
    await expect(offerButton).toBeVisible()
    await expect(offerButton).toBeDisabled()
    await expect(offerButton).toHaveAttribute('title', /required field.*amount/i)
  })

  await test.step('fill the required Payment Amount through the edit UI', async () => {
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
    await expect(amount).toBeVisible({ timeout: 15_000 })
    await amount.fill('250')
    await amount.blur()
    const updated = page.waitForResponse((r) => r.url().includes('/contract/update') && r.request().method() === 'PUT')
    await page.getByRole('button', { name: 'Update', exact: true }).click()
    const resp = await updated
    expect(resp.ok(), `contract update ${resp.status()}: ${await resp.text()}`).toBeTruthy()
  })

  await test.step('offering the filled draft succeeds: DRAFT -> OFFERED', async () => {
    await gotoAs(page, loginAs, 'Contract Creator', `/ui/contracts/view/${contractDid}`)
    await expect(page.getByRole('button', { name: 'Offer to counterparty' })).toBeEnabled({ timeout: 15_000 })
    const offered = page.waitForResponse(
      (r) => r.url().includes('/contract/offer') && r.request().method() === 'POST' && r.ok(),
      { timeout: 30_000 },
    )
    await page.getByRole('button', { name: 'Offer to counterparty' }).click()
    await offered
    // The view reloads on success; EventOffer is DRAFT-only, so the action is gone.
    await expect(page.getByText('OFFERED', { exact: true }).first()).toBeVisible({ timeout: 15_000 })
    await expect(page.getByRole('button', { name: 'Offer to counterparty' })).toBeHidden()
  })
})
