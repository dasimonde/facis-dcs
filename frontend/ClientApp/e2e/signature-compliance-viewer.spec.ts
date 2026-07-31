import { readFileSync } from 'node:fs'
import { type DcsRole, expect, test } from './dcs-test'
import { buildApprovedContract, gotoAs, signApprovedContractViaViewer } from './lifecycle-helpers'
import type { Page } from '@playwright/test'

/**
 * Signature Compliance Viewer (DCS-FR-SM-05/-07/-08, DCS-FR-SM-18/-21/-26).
 *
 * Derives a fresh contract, takes it to SIGNED through the real signing flow
 * (the same lifecycle + wallet/SCA path full-vertical.spec.ts uses), then opens
 * the dedicated tabbed viewer and exercises every tab — asserting the DSS-report
 * + embedded-VC metadata this feature surfaces (signer identity, signature
 * level, timestamp, cryptographic integrity, QES/AES + credential-status flags)
 * actually renders, plus the PDF + JSON report export.
 *
 * NOTE: this asserts the enriched validate/compliance responses and the new
 * /signature/view fields — it must be run against a build carrying those
 * backend changes (merge + redeploy first).
 */

type LoginAs = (role: DcsRole) => Promise<void>

/** Opens the viewer for the given role and selects the contract in the list. */
async function openViewer(page: Page, loginAs: LoginAs, role: DcsRole, contractDid: string): Promise<void> {
  await gotoAs(page, loginAs, role, '/ui/compliance')
  await page.getByPlaceholder('Search DID or name…').fill(contractDid)
  const item = page.getByRole('button').filter({ hasText: contractDid })
  await expect(item.first()).toBeVisible()
  const viewLoaded = page.waitForResponse(
    (r) => r.url().includes('/signature/view') && r.request().method() === 'GET' && r.ok(),
  )
  await item.first().click()
  await viewLoaded
}

test('signature compliance viewer surfaces DSS + embedded-VC metadata', async ({ page, loginAs }) => {
  test.setTimeout(120_000)
  page.setDefaultTimeout(15_000)

  // Reach SIGNED via the same proven lifecycle the other specs use — derive a
  // fresh APPROVED contract, then sign it through the Secure Contract Viewer
  // (ADR-12). The compliance viewer's own tabs are what this spec asserts.
  const contractDid = await buildApprovedContract(page, loginAs)
  await signApprovedContractViaViewer(page, loginAs, contractDid)

  // ---- Contract Manager: Validation, Compliance Checks, export, Revocation ----
  await test.step('validation tab surfaces DSS report + cryptographic integrity', async () => {
    await openViewer(page, loginAs, 'Contract Manager', contractDid)

    const validated = page.waitForResponse(
      (r) => r.url().includes('/signature/validate') && r.request().method() === 'POST' && r.ok(),
    )
    await page.getByRole('button', { name: 'Validate', exact: true }).click()
    await validated

    // The DSS report (ETSI EN 319 102-1) — signer identity, signature level,
    // timestamp — is exactly the metadata FR-SM-26 requires.
    await expect(page.getByText('EU DSS Validation (ETSI EN 319 102-1)')).toBeVisible()
    await expect(page.getByText('Signer identity:')).toBeVisible()
    await expect(page.getByText('Signature level:')).toBeVisible()
    await expect(page.getByText('Timestamp:')).toBeVisible()

    // The cryptographic-integrity findings section renders with pass/fail rows.
    await expect(page.getByRole('heading', { name: 'Cryptographic Integrity' })).toBeVisible()
    await expect(
      page
        .locator('.badge')
        .filter({ hasText: /PASS|FAIL/ })
        .first(),
    ).toBeVisible()
  })

  await test.step('compliance checks tab flags signature level + credential status', async () => {
    // DCS-IR-SM-07
    await page.getByRole('tab', { name: 'Compliance Checks' }).click()

    const checked = page.waitForResponse(
      (r) => r.url().includes('/signature/compliance') && r.request().method() === 'POST' && r.ok(),
    )
    await page.getByRole('button', { name: 'Run Compliance', exact: true }).click()
    await checked

    // FR-SM-21: signature level (AES) and credential status (ACTIVE) per signature.
    await expect(page.locator('.badge').filter({ hasText: 'AES' }).first()).toBeVisible()
    await expect(page.locator('.badge').filter({ hasText: 'ACTIVE' }).first()).toBeVisible()
  })

  await test.step('compliance table surfaces the level-aware fields ADR-20 added (SM-01/SM-26)', async () => {
    // The signatures table is populated from /signature/view (already loaded
    // by openViewer above), independent of the "Run Compliance" POST above —
    // these columns render as soon as the viewer opens.
    const row = page
      .getByRole('row')
      .filter({ has: page.locator('td') })
      .first()
    await expect(row).toBeVisible()

    // Achieved level (this contract has no declared requirement, so it
    // defaults to AES): the achieved-level badge from the pre-existing column.
    await expect(row.locator('td').nth(1).locator('.badge')).toHaveText('AES')
    // Required level: same default, AES.
    await expect(row.locator('td').nth(2).locator('.badge')).toHaveText('AES')
    // Level compliance: achieved (AES) meets required (AES) -> PASS.
    await expect(row.locator('td').nth(3).locator('.badge')).toHaveText('PASS')
    await expect(row.locator('td').nth(3).locator('.badge')).toHaveClass(/badge-success/)
    // Qualification: N/A for an AES (non-QES) signature.
    await expect(row.locator('td').nth(4)).toContainText('N/A (AES)')
    // Certificate subject: non-empty — the signatory's own certificate, not
    // a DCS/org key (ADR-20 sole-control evidence, DCS-FR-SM-26).
    const certCell = row.locator('td').nth(5)
    await expect(certCell).not.toHaveText('—')
    await expect(certCell).not.toHaveText('')
  })

  await test.step('export compliance report as JSON and PDF', async () => {
    // DCS-IR-SM-08
    const jsonDownload = page.waitForEvent('download')
    await page.getByRole('button', { name: 'Export JSON' }).click()
    const jsonPath = await (await jsonDownload).path()
    const report = JSON.parse(readFileSync(jsonPath, 'utf-8')) as {
      contract_did: string
      signatures: unknown[]
    }
    expect(report.contract_did).toBe(contractDid)
    expect(report.signatures.length).toBeGreaterThan(0)

    // PDF export opens a print window (dependency-free, browser "Save as PDF").
    const popup = page.waitForEvent('popup')
    await page.getByRole('button', { name: 'Export PDF' }).click()
    expect(await popup).toBeTruthy()
  })

  // ---- Auditor: Audit Reports tab ----
  await test.step('audit reports tab loads the audit trail', async () => {
    await openViewer(page, loginAs, 'Auditor', contractDid)
    await page.getByRole('tab', { name: 'Audit Reports' }).click()

    const audited = page.waitForResponse(
      (r) => r.url().includes('/signature/audit') && r.request().method() === 'GET',
      { timeout: 90_000 },
    )
    await page.getByRole('button', { name: 'Load Audit Report', exact: true }).click()
    const resp = await audited
    // The audit trail lives only in IPFS and intermittently loses a just-written
    // entry ("DataIdentifier not found") — tolerate that one infra flake, stay
    // strict on any other failure (mirrors full-vertical's audit stage).
    if (resp.ok()) {
      await expect(page.getByRole('table')).toBeVisible()
    } else {
      const body = await resp.text()
      const ipfsTrailMiss = body.includes('ipfs could not find') || body.includes('DataIdentifier not found')
      expect(ipfsTrailMiss, `audit ${resp.status()}: ${body}`).toBeTruthy()
      test.info().annotations.push({ type: 'known-flake', description: `audit tolerated an IPFS trail miss: ${body}` })
    }
  })

  // ---- Revocation (destructive — last) ----
  await test.step('revocation tab revokes the signature', async () => {
    // DCS-IR-SM-06
    await openViewer(page, loginAs, 'Contract Manager', contractDid)
    await page.getByRole('tab', { name: 'Revocation' }).click()

    const sigRow = page.getByRole('row').filter({ hasText: 'ACTIVE' }).first()
    await expect(sigRow).toBeVisible()
    const revoked = page.waitForResponse(
      (r) => r.url().includes('/signature/revoke') && r.request().method() === 'POST' && r.ok(),
    )
    await sigRow.getByRole('button', { name: 'Revoke', exact: true }).click()
    const confirmation = page.getByRole('dialog', { name: 'Confirmation' })
    await confirmation.getByPlaceholder('Reason for revocation').fill('Superseded during compliance review')
    await confirmation.getByRole('button', { name: 'Submit' }).click()
    await revoked
    await expect(page.locator('.badge').filter({ hasText: 'REVOKED' }).first()).toBeVisible()
  })

  await test.step('the contract PDF still exports after revocation — revocation is not erasure', async () => {
    // DCS-NFR-BR-06 voids the agreement; the content keys stay live (only an
    // archive deletion shreds them, ADR-28) — the revoked contract's PDF must
    // remain readable to authorized users.
    await gotoAs(page, loginAs, 'Contract Manager', `/ui/contracts/view/${contractDid}`)
    const exported = page.waitForResponse((r) => r.url().includes(`/pdf/export/contract/${contractDid}`) && r.ok(), {
      timeout: 120_000,
    })
    await page.getByRole('button', { name: 'Export PDF' }).click()
    await exported
    await expect(page.getByText('Content erased — encryption keys destroyed')).toHaveCount(0)
  })
})
