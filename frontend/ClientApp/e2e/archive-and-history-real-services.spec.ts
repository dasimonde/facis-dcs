import { createHash } from 'node:crypto'
import { readFile } from 'node:fs/promises'
import { E2E_ORG_B, expect, mintSession, test } from './dcs-test'
import {
  buildApprovedContract,
  buildDraftContract,
  FIXTURE_CLAUSE_PROSE,
  gotoAs,
  signApprovedContractViaViewer,
  submitDraftIntoNegotiation,
} from './lifecycle-helpers'
import { E2E_API_BASE } from '../playwright.config'

test('archive annotations, exact snapshots, evidence and complete reports persist through real services', async ({
  page,
  loginAs,
}) => {
  test.setTimeout(900_000)
  page.setDefaultTimeout(30_000)
  const did = await buildApprovedContract(page, loginAs)
  await signApprovedContractViaViewer(page, loginAs, did)

  await test.step('manager stores and downloads the exact evidence after reload', async () => {
    await gotoAs(page, loginAs, 'Contract Manager', `/ui/contracts/view/${encodeURIComponent(did)}`)
    await page.getByRole('tab', { name: 'Audit History' }).click()
    await page.getByRole('button', { name: 'Store evidence', exact: true }).click()
    const bytes = Buffer.from([0, 1, 128, 255, 10, 13, 65])
    const hash = createHash('sha256').update(bytes).digest('hex')
    const fileName = 'connected-proof.bin'
    await page.getByLabel('Evidence file', { exact: true }).setInputFiles({
      name: fileName,
      mimeType: 'application/octet-stream',
      buffer: bytes,
    })
    await page.getByLabel('Evidence description').fill('Connected QA evidence')
    const stored = page.waitForResponse((response) => response.url().endsWith('/contract/store'))
    await page.getByRole('button', { name: 'Save evidence', exact: true }).click()
    const response = await stored
    expect(response.ok(), await response.text()).toBeTruthy()
    await page.reload()
    await page.getByRole('tab', { name: 'Audit History' }).click()
    await expect(page.getByText(`SHA-256: ${hash}`)).toBeVisible()
    await expect(page.getByText('Connected QA evidence', { exact: true })).toBeVisible()
    const downloading = page.waitForEvent('download')
    // The panel labels each download button with the stored file name.
    await page.getByRole('button', { name: `Download ${fileName}` }).click()
    const download = await downloading
    expect(download.suggestedFilename()).toBe(fileName)
    const file = await download.path()
    expect(file).not.toBeNull()
    expect(await readFile(file)).toEqual(bytes)
  })

  let version = ''
  await test.step('archive summary survives reload and a tags-only update', async () => {
    await gotoAs(page, loginAs, 'Archive Manager', '/ui/archive/dashboard')
    await page.getByRole('button', { name: 'Additional filters' }).click()
    await page.getByLabel('DID', { exact: true }).fill(did)
    await page.getByRole('button', { name: 'Search archive', exact: true }).click()
    const row = page.getByTestId(`archive-entry-${did}`).last()
    await expect(row).toBeVisible()
    version = (await row.getByRole('cell').nth(1).innerText()).trim()
    const summary = `Connected QA summary ${Date.now()}`
    await row.getByRole('button', { name: 'Annotate', exact: true }).click()
    await expect(page.getByRole('dialog')).toHaveAccessibleName('Edit annotation')
    await page.getByLabel('Summary', { exact: true }).fill(summary)
    const annotated = page.waitForResponse((response) => response.url().endsWith('/archive/annotate'))
    await page.getByRole('dialog').getByRole('button', { name: 'Save annotation', exact: true }).click()
    const response = await annotated
    expect(response.ok(), await response.text()).toBeTruthy()
    await expect(page.getByRole('status').filter({ hasText: 'Annotation saved.' })).toBeVisible()
    await expect(page.getByRole('dialog')).not.toBeVisible()
    await page.reload()
    await expect(page.getByTestId(`archive-annotation-${did}`).last()).toContainText(summary)
    await page.getByTestId(`archive-annotate-${did}`).last().click()
    await expect(page.getByLabel('Summary', { exact: true })).toHaveValue(summary)
    await page.getByLabel('Summary', { exact: true }).fill('')
    await page.getByLabel('Tags', { exact: true }).fill('connected-qa')
    const tagged = page.waitForResponse((response) => response.url().endsWith('/archive/annotate'))
    await page.getByRole('dialog').getByRole('button', { name: 'Save annotation', exact: true }).click()
    const tagsResponse = await tagged
    expect(tagsResponse.ok(), await tagsResponse.text()).toBeTruthy()
    expect(tagsResponse.request().postDataJSON()).not.toHaveProperty('summary')
    await expect(page.getByRole('dialog')).not.toBeVisible()
    await page.reload()
    await expect(page.getByTestId(`archive-annotation-${did}`).last()).toContainText(summary)
    await expect(page.getByTestId(`archive-annotation-${did}`).last()).toContainText('connected-qa')
  })

  await test.step('observer searches and opens the immutable exact archive version', async () => {
    await gotoAs(page, loginAs, 'Contract Observer', '/ui/archive/dashboard')
    await expect(page.getByTestId('archive-statistics')).toHaveCount(0)
    await page.getByRole('button', { name: 'Additional filters' }).click()
    await page.getByLabel('DID', { exact: true }).fill(did)
    await page.getByLabel('Version', { exact: true }).fill(version)
    await page.getByLabel('Tag', { exact: true }).fill('connected-qa')
    await page.getByRole('button', { name: 'Search archive', exact: true }).click()
    const row = page.getByTestId(`archive-entry-${did}`)
    await expect(row).toHaveCount(1)
    await expect(row.getByRole('button', { name: 'Annotate', exact: true })).toHaveCount(0)
    const detail = page.waitForResponse((response) => new URL(response.url()).pathname.endsWith('/archive/detail'))
    await row.getByRole('link').first().click()
    const response = await detail
    expect(response.ok(), await response.text()).toBeTruthy()
    const snapshot = await response.json()
    expect(snapshot.did).toBe(did)
    expect(snapshot.contract_version).toBe(Number(version))
    await expect(page.getByText('This is the immutable archived version.')).toBeVisible()
    await expect(page.getByText(FIXTURE_CLAUSE_PROSE, { exact: true })).toBeVisible()
    const archivedContent = page.getByRole('region', { name: 'Archived contract content' })
    const amount = archivedContent.getByRole('textbox', { name: 'Payment Amount', exact: true })
    await expect(amount).toHaveValue('250')
    await expect(amount).toBeDisabled()
    await expect(amount).not.toBeEditable()
    await expect(page.getByRole('button', { name: /^(Save|Update)$/ })).toHaveCount(0)
    await page.getByTestId('machine-readable-toggle').click()
    await expect(page.getByTestId('machine-readable-payload')).toContainText(FIXTURE_CLAUSE_PROSE)
  })

  await test.step('auditor downloads JSON, CSV and PDF containing the lifecycle', async () => {
    await gotoAs(page, loginAs, 'Auditor', '/ui/audit')
    await page.locator('#select-scope').selectOption('contracts')
    await page.getByLabel('DID (optional)').fill(did)
    await page.getByLabel('Audit justification').fill('Connected QA report acceptance')
    const audited = page.waitForResponse((response) => response.url().endsWith('/pac/audit'), { timeout: 120_000 })
    await page.getByRole('button', { name: 'Execute Audit', exact: true }).click()
    const response = await audited
    expect(response.ok(), await response.text()).toBeTruthy()
    for (const format of ['json', 'csv', 'pdf']) {
      const downloading = page.waitForEvent('download')
      await page.getByRole('button', { name: format.toUpperCase(), exact: true }).click()
      const download = await downloading
      expect(download.suggestedFilename()).toBe(`audit-report-contracts.${format}`)
      const file = await download.path()
      expect(file).not.toBeNull()
      const bytes = await readFile(file)
      await test.info().attach(`connected-report.${format}`, { path: file })
      if (format === 'json') {
        const report = JSON.parse(bytes.toString())
        expect(report.did).toBe(did)
        expect(report.resources).toEqual(expect.arrayContaining([expect.objectContaining({ did })]))
        expect(report.events.length).toBeGreaterThan(0)
        expect(report.events).toEqual(expect.arrayContaining([expect.objectContaining({ did })]))
        expect(report.summary.totalEvents).toBe(report.events.length)
        expect(report.executor.id).toBeTruthy()
      } else if (format === 'csv') {
        expect(bytes.toString().split(/\r?\n/, 1)[0]?.split(',')).toContain('eventType')
        expect(bytes.toString()).toContain(did)
      } else {
        expect(bytes.subarray(0, 5).toString()).toBe('%PDF-')
        expect(bytes.length).toBeGreaterThan(1000)
      }
    }
  })
})

test('lifecycle history, version actors and template history show through real services', async ({
  page,
  loginAs,
  request,
}) => {
  test.setTimeout(600_000)
  page.setDefaultTimeout(30_000)
  const { contractDid: did } = await buildDraftContract(page, loginAs)
  await submitDraftIntoNegotiation(page, loginAs, did)
  const lifecycleURL = `${E2E_API_BASE}/contract/lifecycle/${encodeURIComponent(did)}`
  const bearer = (organization?: string) => ({
    Authorization: `Bearer ${mintSession('Contract Manager', E2E_API_BASE, organization).token}`,
  })

  await test.step('version picker names who replaced each version and when', async () => {
    await gotoAs(page, loginAs, 'Contract Creator', `/ui/contracts/negotiate/${did}`)
    await page.getByRole('tab', { name: 'Diff View' }).click()
    await expect(
      page.locator('#diff-left-pick option').filter({ hasText: /replaced by \S+.* at \d{4}-\d{2}-\d{2} / }),
    ).not.toHaveCount(0)
  })

  await test.step('party sees the lifecycle history with actor', async () => {
    await gotoAs(page, loginAs, 'Contract Manager', `/ui/contracts/view/${did}`)
    await page.getByRole('tab', { name: 'Lifecycle History' }).click()
    const history = page.getByTestId('lifecycle-history')
    await expect(history.getByRole('listitem').first()).toBeVisible()
    await expect(history.getByText(/^By: \S/).first()).toBeVisible()
  })

  await test.step('lifecycle is refused to a foreign organization', async () => {
    const own = await request.get(lifecycleURL, { headers: bearer() })
    expect(own.status(), await own.text()).toBe(200)
    const foreign = await request.get(lifecycleURL, { headers: bearer(E2E_ORG_B) })
    expect(foreign.status(), await foreign.text()).toBe(403)
  })

  await test.step('Meta Data lists the template version history', async () => {
    const contract = await request.get(`${E2E_API_BASE}/contract/retrieve/${encodeURIComponent(did)}`, {
      headers: bearer(),
    })
    expect(contract.ok(), await contract.text()).toBeTruthy()
    const templateDid = String((await contract.json()).template_did)
    await gotoAs(page, loginAs, 'Template Manager', `/ui/templates/view/${encodeURIComponent(templateDid)}`)
    await page.getByRole('tab', { name: 'Meta Data' }).click()
    const entries = page.getByTestId('template-history-entry')
    await expect.poll(() => entries.count()).toBeGreaterThan(1)
    await expect(page.getByTestId('template-history')).toContainText('APPROVED')
  })
})
