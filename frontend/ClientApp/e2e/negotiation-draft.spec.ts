import { expect, test } from './dcs-test'
import { buildDraftContract, gotoAs } from './lifecycle-helpers'

/**
 * SRS §3.1.1 Contract Negotiation UI "Save draft": a negotiator stages a
 * counter-offer privately, leaves, comes back to find it restored, and only
 * "Change Proposal" makes it real for the counterparty — which also consumes
 * the stored draft. Driven end-to-end through the real Negotiate view buttons
 * against a UI-authored contract in NEGOTIATION.
 */

test('a staged counter-offer survives navigation and is consumed by proposing it', async ({ page, loginAs }) => {
  test.setTimeout(60_000)

  const { contractDid } = await buildDraftContract(page, loginAs)

  await test.step('fill the required Payment Amount and submit into negotiation', async () => {
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
    const submit = page.getByRole('button', { name: 'Submit', exact: true })
    await expect(submit).toBeEnabled({ timeout: 15_000 })
    const submitted = page.waitForResponse(
      (r) => r.url().includes('/contract/submit') && r.request().method() === 'POST',
    )
    await submit.click()
    const resp = await submitted
    expect(resp.ok(), `contract submit ${resp.status()}: ${await resp.text()}`).toBeTruthy()
  })

  await test.step('stage a counter-offer and save it as a private draft', async () => {
    await gotoAs(page, loginAs, 'Contract Creator', `/ui/contracts/negotiate/${contractDid}`)
    await page
      .getByRole('tab', { name: /content/i })
      .or(page.getByText('Contract Content', { exact: true }))
      .first()
      .click()
    const amount = page
      .getByRole('spinbutton', { name: /amount/i })
      .or(page.getByRole('textbox', { name: /amount/i }))
      .first()
    await expect(amount).toBeVisible({ timeout: 30_000 })
    await amount.fill('300')
    await amount.blur()
    const saved = page.waitForResponse(
      (r) => r.url().includes('/contract/negotiation_draft') && r.request().method() === 'PUT',
    )
    await page.getByRole('button', { name: 'Save draft', exact: true }).click()
    const resp = await saved
    expect(resp.ok(), `draft save ${resp.status()}: ${await resp.text()}`).toBeTruthy()
    await expect(page.getByRole('button', { name: 'Discard draft', exact: true })).toBeVisible()
  })

  await test.step('the staged draft is restored after navigating away and back', async () => {
    await gotoAs(page, loginAs, 'Contract Creator', '/ui/contracts')
    await gotoAs(page, loginAs, 'Contract Creator', `/ui/contracts/negotiate/${contractDid}`)
    await page
      .getByRole('tab', { name: /content/i })
      .or(page.getByText('Contract Content', { exact: true }))
      .first()
      .click()
    const amount = page
      .getByRole('spinbutton', { name: /amount/i })
      .or(page.getByRole('textbox', { name: /amount/i }))
      .first()
    await expect(amount).toBeVisible({ timeout: 30_000 })
    await expect(amount).toHaveValue('300')
    await expect(page.getByRole('button', { name: 'Discard draft', exact: true })).toBeVisible()
  })

  await test.step('proposing the staged draft consumes it', async () => {
    const proposed = page.waitForResponse(
      (r) => r.url().includes('/contract/negotiate') && r.request().method() === 'POST' && r.ok(),
      { timeout: 30_000 },
    )
    await page.getByRole('button', { name: 'Change Proposal', exact: true }).click()
    await proposed
    // The propose consumed the server-side draft: a fresh visit restores
    // nothing and offers no draft to discard.
    await gotoAs(page, loginAs, 'Contract Creator', `/ui/contracts/negotiate/${contractDid}`)
    await expect(page.getByRole('button', { name: 'Change Proposal', exact: true })).toBeVisible({ timeout: 30_000 })
    await expect(page.getByRole('button', { name: 'Discard draft', exact: true })).toBeHidden()
  })
})
