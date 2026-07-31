import { expect, test } from './dcs-test'
import { buildApprovedContract, gotoAs } from './lifecycle-helpers'

/**
 * Secure Contract Viewer UI (SRS DCS-IR-SM-01..04). Focuses on what is unique to
 * the viewer — reading the real contract content and the guided signing steps —
 * against the real backend. The full wallet-driven signing ceremony
 * (prepare → external sign → submit → validate) is exercised end-to-end by
 * full-vertical.spec.ts, so it is not repeated here.
 */

test('secure contract viewer shows the real contract content and guides the signer', async ({ page, loginAs }) => {
  // DCS-FR-CWE-26 // DCS-FR-SM-09
  test.setTimeout(60_000)
  page.setDefaultTimeout(15_000)
  page.on('console', (m) => {
    if (m.text().includes('SUBMIT-GATE')) console.log(m.text())
  })

  const contractDid = await buildApprovedContract(page, loginAs)

  await test.step('open the approved contract in the split-view viewer', async () => {
    await gotoAs(page, loginAs, 'Contract Signer', '/ui/signing')
    const row = page.getByRole('row').filter({ hasText: contractDid })
    await expect(row).toBeVisible()
    await row.getByRole('link', { name: /Open/ }).click()

    await expect(page).toHaveURL(/\/signing\/.+/)
    await expect(page.getByRole('heading', { name: 'Contract document' })).toBeVisible()
  })

  await test.step('left panel renders the actual clauses, not just a summary', async () => {
    // The Contract Signer can now read the approved-unsigned contract's content
    // (the read-gap fix): the clause prose authored into the component renders.
    await expect(page.getByText(/provider invoices the agreed payment amount/i)).toBeVisible()
  })

  await test.step('the guided steps gate signing until the contract is verified', async () => {
    const downloadToSign = page.getByRole('button', { name: /download document to sign/ })
    // Step 3 (verify identity & download) is blocked until step 2 (verify) passes.
    await expect(downloadToSign).toBeDisabled()
    await expect(page.getByText('Complete step 3 first to get the document to sign.')).toBeVisible()

    await page.getByRole('button', { name: 'Verify', exact: true }).click()
    await expect(page.getByText('Verified', { exact: true })).toBeVisible()

    // Once verified, the signer can download the to-be-signed document and the
    // upload affordance is present for the externally-signed PDF.
    await expect(downloadToSign).toBeEnabled()
    await expect(page.getByText('Upload signed document', { exact: true })).toBeVisible()
  })
})
