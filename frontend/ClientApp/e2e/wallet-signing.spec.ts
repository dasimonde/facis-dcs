import { expect, test } from './dcs-test'
import { buildApprovedContract, gotoAs, signApprovedContractWithWallet } from './lifecycle-helpers'

/**
 * Review, approval and wallet signing of one contract against the real backend.
 *
 * - DCS-FR-CWE-25, DCS-IR-CWE-05, DCS-IR-CWE-06: the review and approval pages
 *   name the role the user acts in; the reviewer marks a clause, which shows
 *   highlighted, and forwards the annotation with the approval. The contract's
 *   audit history records the annotation and the role of the actor
 *   (DCS-FR-CWE-27 action history).
 * - DCS-FR-SM-06, DCS-FR-SM-23: "Sign with wallet" in the Secure Contract
 *   Viewer publishes the OID4VP signing request after the PID ceremony; the
 *   test wallet signs through Document Retrieval and the viewer shows SIGNED.
 */

test('reviewer annotations with role attribution, then signing with the wallet', async ({ page, loginAs }) => {
  // DCS-FR-CWE-25 // DCS-FR-CWE-27 // DCS-IR-CWE-05 // DCS-IR-CWE-06 // DCS-FR-SM-06 // DCS-FR-SM-23
  test.setTimeout(360_000)
  page.setDefaultTimeout(15_000)
  const finding = `Check the payment ceiling ${Date.now()}`

  const contractDid = await buildApprovedContract(page, loginAs, { reviewFinding: finding })

  await test.step('the audit history shows the review annotation and the reviewer role', async () => {
    await gotoAs(page, loginAs, 'Contract Manager', `/ui/contracts/view/${contractDid}`)
    await page.getByRole('tab', { name: 'Audit History' }).click()
    await page.getByLabel('Audit justification').fill('e2e check of the review findings')
    const loaded = page.waitForResponse(
      (r) => r.url().includes('/contract/audit') && r.request().method() === 'POST' && r.ok(),
    )
    await page.getByRole('button', { name: 'Load audit trail', exact: true }).click()
    await loaded

    const annotations = page.getByTestId('audit-annotations').filter({ hasText: finding }).first()
    await expect(annotations).toBeVisible()
    // The annotation names the clause it marks, as the contract content shows it.
    await expect(annotations).toContainText(/provider invoices/)
    // The same audit entry: the roles the reviewer held for the forward.
    await expect(annotations.locator('xpath=..').getByTestId('audit-roles')).toContainText('Contract Reviewer')
  })

  await test.step('sign with the wallet through the published OID4VP signing request', () =>
    signApprovedContractWithWallet(page, loginAs, contractDid))

  await test.step('the signing list shows the contract as SIGNED', async () => {
    await gotoAs(page, loginAs, 'Contract Signer', '/ui/signing')
    const row = page.getByTestId('signing-row').filter({ hasText: contractDid })
    await expect(row.getByTestId('signing-status')).toHaveText('SIGNED')
  })
})
