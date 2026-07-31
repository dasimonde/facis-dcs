import { expect, test } from './dcs-test'
import {
  buildApprovedContract,
  buildDraftContractFixture,
  type DraftContractFixture,
  gotoAs,
  signApprovedContractViaViewer,
} from './lifecycle-helpers'

/**
 * The workspace dashboards render their backing APIs' data for the roles the
 * SRS assigns them (DCS-FR-TR-28 template overview, DCS-FR-CWE-24 contract
 * overview, DCS-FR-SM-22 signer dashboard, DCS-FR-CSA-21/-24 archive/audit
 * views) — the UI half of requirements whose API half the BDD suite covers.
 *
 * The template/contract lists and the human-readable render are asserted
 * against a fixture authored by clicking a registered contract template and a
 * DRAFT contract into existence through the real UI.
 */

test.describe('dashboards backed by an authored fixture', () => {
  let fixture: DraftContractFixture

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(180_000)
    fixture = await buildDraftContractFixture(browser)
  })

  test('template dashboard lists registered templates', async ({ page, loginAs }) => {
    await loginAs('Template Creator')
    await page.goto('/ui/templates')

    await expect(page.getByText(fixture.contractTemplateName).first()).toBeVisible()
  })

  test('contract dashboard lists contracts with their lifecycle state', async ({ page, loginAs }) => {
    // DCS-FR-CWE-27 // DCS-FR-UC-06-1 // DCS-IR-CWE-13 (comment citation)
    await loginAs('Contract Manager')
    await page.goto('/ui/contracts')

    // The derived contract inherits its template's name at creation; the state
    // chip is asserted among VISIBLE matches (a bare .first() can land on
    // hidden filter options).
    await expect(page.getByText(fixture.contractTemplateName).first()).toBeVisible()
    await expect(page.getByText('DRAFT', { exact: true }).locator('visible=true').first()).toBeVisible()
  })

  test('contract view renders the human-readable document', async ({ page, loginAs }) => {
    await loginAs('Contract Manager')
    await page.goto(`/ui/contracts/view/${fixture.contractDid}`)

    // The human-readable rendering lives under the Contract Content tab.
    await page
      .getByRole('tab', { name: /content/i })
      .or(page.getByText('Contract Content', { exact: true }))
      .first()
      .click()
    // The authored clause's prose, rendered from the machine-readable JSON-LD —
    // the human-readable representation the SRS demands alongside it.
    await expect(page.getByText(fixture.clauseProse).first()).toBeVisible()
  })
})

test('signer dashboard shows pending, signed, and revoked status with credential + timestamp', async ({
  page,
  loginAs,
}) => {
  // DCS-FR-SM-22: the signer dashboard covers the full signing lifecycle —
  // pending, completed, and revoked — with the credential used, signer,
  // timestamps, and validation results per signature.
  test.setTimeout(300_000)
  page.setDefaultTimeout(15_000)

  const contractDid = await buildApprovedContract(page, loginAs)

  await test.step('an approved contract awaits signature as PENDING', async () => {
    await gotoAs(page, loginAs, 'Contract Signer', '/ui/signing')
    await expect(page.getByRole('heading', { name: 'Signing', exact: true })).toBeVisible()

    const row = page.getByTestId('signing-row').filter({ hasText: contractDid })
    await expect(row).toBeVisible()
    await expect(row.getByTestId('signing-status')).toHaveText('PENDING')
  })

  await signApprovedContractViaViewer(page, loginAs, contractDid)

  await test.step('the signed contract shows SIGNED with credential, signer, and timestamp', async () => {
    await gotoAs(page, loginAs, 'Contract Signer', '/ui/signing')
    const row = page.getByTestId('signing-row').filter({ hasText: contractDid })
    await expect(row).toBeVisible()
    await expect(row.getByTestId('signing-status')).toHaveText('SIGNED')

    // Expanding the row loads GET /signature/view: per-signature credential,
    // signer, timestamps, plus the contract's validation results.
    const viewLoaded = page.waitForResponse(
      (r) => r.url().includes('/signature/view') && r.request().method() === 'GET' && r.ok(),
      { timeout: 60_000 },
    )
    await row.getByTestId('signing-details-toggle').click()
    const view = await (await viewLoaded).json()

    const details = page.getByTestId('signing-signatures')
    const entry = details.getByTestId('signature-entry').first()
    await expect(entry).toBeVisible()
    // The credential badge must show the level the ceremony actually
    // established, and say so plainly when none was — asserting a fixed 'AES'
    // passed just as well when the UI defaulted an absent level to AES, which
    // is the false attestation the backend stopped writing.
    const credentialType: string = view.signatures?.[0]?.credential_type ?? ''
    await expect(entry.getByTestId('signature-credential')).toHaveText(
      credentialType ? credentialType.toUpperCase() : 'NOT ESTABLISHED',
    )
    // This ceremony completed with a wallet AES, so that is what it must read.
    expect(credentialType.toUpperCase()).toBe('AES')
    await expect(entry.getByTestId('signature-status')).toHaveText('SIGNED')
    await expect(entry.getByTestId('signature-signer')).not.toBeEmpty()
    // The signing timestamp renders as the recorded RFC 3339 instant.
    await expect(entry.getByTestId('signature-timestamp')).toContainText(/Signed at: \d{4}-\d{2}-\d{2}T/)
    await expect(details.getByTestId('signing-validation')).toBeVisible()
  })

  await test.step('revoke the signature through the compliance viewer', async () => {
    // The revocation action lives in the Signature Compliance Viewer's
    // Revocation tab (Contract Manager) — the same UI flow
    // signature-compliance-viewer.spec.ts exercises.
    await gotoAs(page, loginAs, 'Contract Manager', '/ui/compliance')
    await page.getByPlaceholder('Search DID or name…').fill(contractDid)
    const item = page.getByRole('button').filter({ hasText: contractDid })
    await expect(item.first()).toBeVisible()
    await item.first().click()
    await page.getByRole('tab', { name: 'Revocation' }).click()

    const sigRow = page.getByRole('row').filter({ hasText: 'ACTIVE' }).first()
    await expect(sigRow).toBeVisible()
    const revoked = page.waitForResponse(
      (r) => r.url().includes('/signature/revoke') && r.request().method() === 'POST' && r.ok(),
    )
    await sigRow.getByRole('button', { name: 'Revoke', exact: true }).click()
    const confirmation = page.getByRole('dialog', { name: 'Confirmation' })
    await confirmation.getByPlaceholder('Reason for revocation').fill('Revoked from the contract dashboard test')
    await confirmation.getByRole('button', { name: 'Submit' }).click()
    await revoked
  })

  await test.step('the revoked contract stays on the dashboard, marked REVOKED', async () => {
    await gotoAs(page, loginAs, 'Contract Signer', '/ui/signing')
    const row = page.getByTestId('signing-row').filter({ hasText: contractDid })
    await expect(row).toBeVisible()
    await expect(row.getByTestId('signing-status')).toHaveText('REVOKED')

    const viewLoaded = page.waitForResponse(
      (r) => r.url().includes('/signature/view') && r.request().method() === 'GET' && r.ok(),
      { timeout: 60_000 },
    )
    await row.getByTestId('signing-details-toggle').click()
    await viewLoaded

    const entry = page.getByTestId('signing-signatures').getByTestId('signature-entry').first()
    await expect(entry.getByTestId('signature-status')).toHaveText('REVOKED')
    await expect(entry.getByTestId('signature-timestamp')).toContainText(/Revoked at: \d{4}-\d{2}-\d{2}T/)
  })
})

test('audit view renders scoped audits for the auditor role', async ({ page, loginAs }) => {
  await loginAs('Auditor')
  await page.goto('/ui/audit')

  await expect(page.getByRole('heading', { level: 2 }).first()).toBeVisible()
  // The scoped-audit workstation the /pac endpoints back.
  await expect(page.getByText('Execute Audit').first()).toBeVisible()
  await expect(page.getByText('Scope', { exact: true }).first()).toBeVisible()
})
