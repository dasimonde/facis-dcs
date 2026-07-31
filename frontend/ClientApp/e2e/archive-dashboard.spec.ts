import { expect, test } from './dcs-test'
import { buildApprovedContract, signApprovedContractViaViewer } from './lifecycle-helpers'

/**
 * Contract Archive Dashboard — DCS-FR-CSA-21.
 *
 * The SRS requires a dashboard giving an overview of archived contract
 * statistics, recent actions, storage volume, expiring contracts, and
 * compliance status, with drill-down into contract details. A contract is
 * archived when it reaches SIGNED (signing apply creates the archive entry
 * with snapshot, content hash, signature metadata, and TSA receipt), so the
 * spec drives a real contract to SIGNED through the UI, then reads the
 * dashboard as the Auditor and drills down into the contract's archive
 * audit trail.
 */

test('the archive dashboard shows the SRS overview and drills down into a contract', async ({ page, loginAs }) => {
  // DCS-FR-UC-07-3
  test.setTimeout(600_000)

  let contractDid = ''
  await test.step('a contract reaches SIGNED and is archived', async () => {
    contractDid = await buildApprovedContract(page, loginAs)
    await signApprovedContractViaViewer(page, loginAs, contractDid)
  })

  await test.step('the dashboard shows statistics, storage volume, and compliance status', async () => {
    await loginAs('Auditor')
    await page.goto('/ui/archive/dashboard')
    await expect(page.getByTestId('archive-dashboard')).toBeVisible()

    const statistics = page.getByTestId('archive-statistics')
    await expect(statistics).toBeVisible()

    const archivedContracts = Number(await page.getByTestId('stat-archived-contracts').innerText())
    expect(archivedContracts, 'at least the just-signed contract is archived').toBeGreaterThanOrEqual(1)

    await expect(page.getByTestId('stat-storage-volume'), 'archived snapshots occupy storage').not.toHaveText('0 B')

    const compliant = Number(await page.getByTestId('stat-compliant').innerText())
    const flagged = Number(await page.getByTestId('stat-flagged').innerText())
    expect(
      compliant,
      'the freshly signed entry carries complete evidence (snapshot, hash, signature metadata, TSA receipt)',
    ).toBeGreaterThanOrEqual(1)
    expect(flagged, 'the flagged tile renders a count').toBeGreaterThanOrEqual(0)
  })

  await test.step('recent actions list the archive operation on this contract', async () => {
    const recent = page.getByTestId('archive-recent-actions')
    await expect(recent).toBeVisible()
    await expect(recent.getByRole('link', { name: new RegExp(contractDid.slice(0, 12)) }).first()).toBeVisible()
  })

  await test.step('the expiring-contracts overview renders', async () => {
    // The e2e fixture cannot set an expiration date through the UI yet
    // (CSA-23's expiration-management surface), so this asserts the SRS
    // element itself: the overview renders with rows or its explicit
    // empty state. The expiring-window logic is proven API-level by
    // features/07_contract_storage_security/archive_management.feature.
    const expiring = page.getByTestId('archive-expiring')
    await expect(expiring).toBeVisible()
    await expect(expiring.getByText(/Expiring within 30 days/)).toBeVisible()
  })

  await test.step('a recent action drills down into the contract archive trail', async () => {
    await page
      .getByTestId('archive-recent-actions')
      .getByRole('link', { name: new RegExp(contractDid.slice(0, 12)) })
      .first()
      .click()
    await expect(page).toHaveURL(/\/ui\/audit\?.*did=/)

    // The audit view lands pre-parameterized on this contract's archive
    // scope; executing it renders the contract's archive trail.
    await expect(page.locator('#select-scope')).toHaveValue('archive')
    await expect(page.locator('#input-did')).toHaveValue(contractDid)
  })
})
