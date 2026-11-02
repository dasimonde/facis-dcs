import { readFile } from 'node:fs/promises'
import { expect, type Page, type Route, test } from '@playwright/test'
import { withholdUnsignedToken } from './unsigned-token'

// HTTP is mocked: these checks prove browser behavior, not backend persistence or authorization.
const did = 'did:web:example.test:contract:qa'
const profile = `@prefix dcs: <https://w3id.org/facis/dcs/ontology/v1#> .
@prefix odrl: <http://www.w3.org/ns/odrl/2/> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
<https://w3id.org/facis/dcs/ontology/v1/odrl-profile> a odrl:Profile ; dcs:defaultConstraintAction dcs:provideCompliantValue .
odrl:eq a odrl:Operator ; rdfs:label "Must equal" ; dcs:appliesToParameterType "string" .`
async function json(route: Route, body: unknown, status = 200) {
  await route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })
}
async function authenticate(page: Page, role: string) {
  const encode = (value: object) => Buffer.from(JSON.stringify(value)).toString('base64url')
  const token = `${encode({ alg: 'none', typ: 'JWT' })}.${encode({ sub: 'did:web:auditor.test', exp: Math.floor(Date.now() / 1000) + 3600, roles: [role], ext: { iss: 'did:web:example.test', roles: [role] } })}.unsigned`
  await withholdUnsignedToken(page)
  await page.addInitScript((token) => {
    localStorage.setItem('token_type', 'Bearer')
    localStorage.setItem('access_token', token)
  }, token)
  await page.route('**/auth/refresh', (route) => json(route, { token_type: 'Bearer', access_token: token }))
  await page.route('**/semantic/schema/list', (route) => json(route, []))
  await page.route('**/semantic/ontology/dcs-odrl-profile', (route) => json(route, { content: profile }))
}
async function archive(page: Page) {
  await authenticate(page, 'Archive Manager')
  await page.route('**/archive/statistics', (route) =>
    json(route, {
      archived_total: 1,
      archived_contracts: 1,
      deleted_total: 0,
      storage_bytes: 100,
      compliant_total: 1,
      flagged_total: 0,
      recent_actions: [],
      expiring_contracts: [],
      expiring_window_days: 30,
    }),
  )
  await page.route('**/archive/erasure-status**', (route) => json(route, { did, local_status: 'live', peers: [] }))
}
async function openAnnotation(page: Page, summary?: string) {
  await page.getByTestId(`archive-annotate-${did}`).click()
  await expect(page.getByRole('dialog')).toHaveAccessibleName('Edit annotation')
  await expect(page.getByRole('dialog')).toContainText(
    'Leave the summary empty to keep the existing summary. If none exists, one is generated from the contract metadata. Leave the tags empty to keep the current set.',
  )
  await expect(page.getByLabel('Tags', { exact: true })).toBeVisible()
  if (summary !== undefined) await page.getByLabel('Summary', { exact: true }).fill(summary)
}
for (const width of [390, 1440]) {
  for (const failed of [false, true]) {
    test(`archive annotation ${failed ? 'failure is visible and keeps the input' : 'saves in one dialog and survives reload'}; cancellation sends no POST at ${width}px`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: width === 390 ? 844 : 900 })
      await archive(page)
      let storedSummary = 'Original summary'
      const posts: unknown[] = []
      await page.route('**/archive/retrieve', (route) =>
        json(route, {
          contracts: [
            {
              did,
              name: 'QA archive contract',
              state: 'ARCHIVED',
              contract_version: 1,
              archive_summary: storedSummary,
              archive_tags: ['original', 'qa'],
            },
          ],
        }),
      )
      await page.route('**/archive/annotate', async (route) => {
        const body = route.request().postDataJSON() as { summary: string }
        posts.push(body)
        if (failed) return json(route, { message: 'Annotation storage unavailable' }, 500)
        storedSummary = body.summary
        await json(route, { did, summary: storedSummary, tags: [] })
      })
      await page.goto('/ui/archive/dashboard')
      await openAnnotation(page)
      await expect(page.getByLabel('Summary', { exact: true })).toHaveValue('Original summary')
      await expect(page.getByLabel('Tags', { exact: true })).toHaveValue('original, qa')
      await page.getByLabel('Summary', { exact: true }).fill('  Retained QA archive summary  ')
      await page.getByLabel('Tags', { exact: true }).fill('')
      expect(posts).toHaveLength(0)
      await page.getByRole('dialog').getByRole('button', { name: 'Save annotation' }).press('Enter')
      await expect.poll(() => posts.length).toBe(1)
      expect(posts[0]).toEqual({ did, summary: 'Retained QA archive summary' })
      if (failed) {
        await expect(page.getByRole('dialog').getByRole('alert')).toContainText('Annotation storage unavailable')
        await expect(page.getByLabel('Summary', { exact: true })).toHaveValue('  Retained QA archive summary  ')
        await expect(page.getByText('Annotation saved.')).toHaveCount(0)
        await page.getByRole('dialog').getByRole('button', { name: 'Cancel' }).click()
      } else {
        await expect(page.getByRole('status').filter({ hasText: 'Annotation saved.' })).toBeVisible()
      }
      await expect(page.getByRole('dialog')).not.toBeVisible()
      expect(posts).toHaveLength(1)
      const expected = failed ? 'Original summary' : 'Retained QA archive summary'
      await expect(page.getByTestId(`archive-annotation-${did}`).locator('div').first()).toHaveText(expected)
      await page.reload()
      await expect(page.getByTestId(`archive-annotation-${did}`).locator('div').first()).toHaveText(expected)
      for (const cancel of ['button', 'escape']) {
        const annotate = page.getByTestId(`archive-annotate-${did}`)
        await annotate.press('Enter')
        await expect(page.getByRole('dialog')).toHaveAccessibleName('Edit annotation')
        await expect(page.getByLabel('Summary', { exact: true })).toHaveValue(expected)
        if (cancel === 'button') await page.getByRole('dialog').getByRole('button', { name: 'Cancel' }).click()
        else await page.keyboard.press('Escape')
        await expect(page.getByRole('dialog')).not.toBeVisible()
        expect(posts).toHaveLength(1)
        await expect(annotate).toBeFocused()
        await openAnnotation(page, '  Discarded QA archive summary  ')
        if (cancel === 'button') await page.getByRole('dialog').getByRole('button', { name: 'Cancel' }).press('Enter')
        else await page.keyboard.press('Escape')
        await expect(page.getByRole('dialog')).not.toBeVisible()
        expect(posts).toHaveLength(1)
        await expect(annotate).toBeFocused()
      }
    })
  }
}
for (const width of [390, 1440]) {
  test(`archive entries without annotations start empty and preserve omission semantics at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: width === 390 ? 844 : 900 })
    await archive(page)
    await page.route('**/archive/retrieve', (route) =>
      json(route, { contracts: [{ did, name: 'Without annotation', state: 'ARCHIVED', contract_version: 1 }] }),
    )
    const posts: unknown[] = []
    await page.route('**/archive/annotate', async (route) => {
      posts.push(route.request().postDataJSON())
      await json(route, { did, summary: 'Generated summary', tags: [] })
    })
    await page.goto('/ui/archive/dashboard')
    await page.getByTestId(`archive-annotate-${did}`).press('Enter')
    await expect(page.getByRole('dialog')).toHaveAccessibleName('Edit annotation')
    await expect(page.getByLabel('Summary', { exact: true })).toHaveValue('')
    await expect(page.getByLabel('Tags', { exact: true })).toHaveValue('')
    expect(posts).toHaveLength(0)
    await page.getByRole('dialog').getByRole('button', { name: 'Save annotation', exact: true }).press('Enter')
    await expect.poll(() => posts.length).toBe(1)
    expect(posts[0]).toEqual({ did })
    await expect(page.getByRole('status').filter({ hasText: 'Annotation saved.' })).toBeVisible()
    await expect(page.getByRole('dialog')).not.toBeVisible()
  })
}
async function audit(page: Page, findings: Record<string, unknown>[] = []) {
  await authenticate(page, 'Auditor')
  await page.route('**/pac/audit', (route) =>
    json(route, {
      audit_id: 'qa-audit',
      correlation_id: 'qa-correlation',
      executed_at: '2026-10-15T10:00:00Z',
      scope: 'contracts',
      executor: { id: 'policy-engine', version: '1' },
      findings,
    }),
  )
  await page.goto('/ui/audit')
  await page.getByLabel('Audit justification').fill('Investigate QA report')
  await page.getByRole('button', { name: 'Execute Audit' }).click()
  await expect(page.getByRole('button', { name: 'JSON', exact: true })).toBeEnabled()
}
test('audit: a DID change hides prior findings, filters and opened details', async ({ page }) => {
  await authenticate(page, 'Auditor')
  await page.route('**/pac/audit', (route) =>
    json(route, {
      audit_id: 'context-audit',
      correlation_id: 'context-correlation',
      executed_at: '2026-10-26T10:00:00Z',
      scope: 'contracts',
      resource: { did },
      executor: { id: 'policy-engine', version: '1' },
      findings: [{ rule_id: 'Context A check', result: 'PASSED', reason: 'Only contract A was checked.' }],
    }),
  )
  await page.goto('/ui/audit')
  await page.getByLabel('DID (optional)').fill(did)
  await page.getByLabel('Audit justification').fill('Check audit context')
  await page.getByRole('button', { name: 'Execute Audit' }).click()
  await expect(page.getByText('Audit passed.', { exact: false })).toBeVisible()
  const row = page.getByRole('row').filter({ hasText: 'Context A check' })
  await row.press('Enter')
  await expect(page.locator('main aside')).toContainText('Only contract A was checked.')
  await page.getByLabel('DID (optional)').fill('did:test:different-contract')
  await expect(page.getByText('Context A check', { exact: true })).toHaveCount(0)
  await expect(page.locator('main aside')).toHaveCount(0)
  await expect(page.getByText('Select a scope and execute an audit.', { exact: true })).toBeVisible()
  await expect(page.locator('main .dropdown input[type="checkbox"]')).toHaveCount(0)
  await expect(page.locator('main .dropdown')).toHaveCount(0)
  for (const format of ['JSON', 'CSV', 'PDF'])
    await expect(page.getByRole('button', { name: format, exact: true })).toBeDisabled()
})
for (const [format, contentType, bytes] of [
  ['json', 'application/json', '{"reportId":"qa-audit","events":[{"eventType":"ARCHIVED"}]}'],
  ['csv', 'text/csv', 'eventType,occurredAt\r\nARCHIVED,2026-10-15T10:00:00Z\r\n'],
  ['pdf', 'application/pdf', '%PDF-1.4\nQA transport fixture\n%%EOF\n'],
] as const) {
  test(`audit report ${format} download preserves server bytes and filename`, async ({ page }) => {
    await audit(page)
    await page.route('**/pac/report?**', async (route) => {
      const params = new URL(route.request().url()).searchParams
      expect(params.get('format')).toBe(format)
      expect(params.get('scope')).toBe('contracts')
      expect(params.get('justification')).toBe('Investigate QA report')
      await route.fulfill({ contentType, body: bytes })
    })
    const promise = page.waitForEvent('download')
    await page.getByRole('button', { name: format.toUpperCase(), exact: true }).click()
    const download = await promise
    expect(download.suggestedFilename()).toBe(`audit-report-contracts.${format}`)
    expect(await readFile(await download.path(), 'utf8')).toBe(bytes)
  })
}
for (const width of [390, 1440]) {
  for (const status of [404, 500]) {
    test(`audit report error ${status} preserves findings and details at ${width}px`, async ({ page }) => {
      await page.setViewportSize({ width, height: width === 390 ? 844 : 900 })
      await audit(page, [{ rule_id: 'Preserved check', result: 'PASSED', reason: 'Preserved evidence' }])
      await page.getByRole('row').filter({ hasText: 'Preserved check' }).press('Enter')
      let attempts = 0
      await page.route('**/pac/report?**', async (route) => {
        attempts++
        if (attempts === 1) return json(route, { message: 'Report unavailable' }, status)
        await route.fulfill({ contentType: 'application/pdf', body: '%PDF-1.4\nRetry report\n%%EOF' })
      })
      const downloads = page.getByRole('group', { name: 'Download report', exact: true })
      await downloads.getByRole('button', { name: 'PDF', exact: true }).click()
      await expect(page.getByRole('alert')).toHaveCount(1)
      await expect(downloads.getByRole('alert')).toHaveText('Audit report could not be generated.')
      await expect(page.getByText('Audit passed.', { exact: false })).toBeVisible()
      await expect(page.locator('main aside')).toContainText('Preserved evidence')
      await expect(page.getByRole('row').filter({ hasText: 'Preserved check' })).toHaveCount(1)
      await expect(page.locator('main .dropdown')).toHaveCount(4)
      const promise = page.waitForEvent('download')
      await downloads.getByRole('button', { name: 'PDF', exact: true }).click()
      await promise
      await expect(page.getByRole('alert')).toHaveCount(0)
      expect(attempts).toBe(2)
    })
  }
  test(`audit report keeps format labels and announces report loading at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: width === 390 ? 844 : 900 })
    await audit(page)
    let finish!: () => void
    const pending = new Promise<void>((resolve) => {
      finish = resolve
    })
    await page.route('**/pac/report?**', async (route) => {
      await pending
      await json(route, { reportId: 'qa-report', events: [] })
    })
    const group = page.getByRole('group', { name: 'Download report', exact: true })
    await group.getByRole('button', { name: 'JSON', exact: true }).click()
    await expect(group.getByRole('button', { name: 'JSON', exact: true })).toBeDisabled()
    await expect(group.getByRole('status')).toHaveText('Preparing JSON report…')
    await expect(group.locator('.loading')).toHaveAttribute('aria-hidden', 'true')
    for (const format of ['JSON', 'CSV', 'PDF'])
      await expect(group.getByRole('button', { name: format, exact: true })).toHaveText(format)
    const download = page.waitForEvent('download')
    finish()
    await download
    await expect(group.getByRole('status')).toHaveCount(0)
  })
  test(`audit empty states show only relevant controls at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: width === 390 ? 844 : 900 })
    await authenticate(page, 'Auditor')
    let executions = 0
    await page.route('**/pac/audit', (route) =>
      json(route, {
        audit_id: 'empty-audit',
        correlation_id: 'empty-correlation',
        executed_at: '2026-10-26T10:00:00Z',
        scope: 'contracts',
        executor: { id: 'policy-engine', version: '1' },
        findings: executions++ === 0 ? [] : [{ rule_id: 'Filter check', result: 'PASSED' }],
      }),
    )
    await page.goto('/ui/audit')
    await expect(page.getByText('Select a scope and execute an audit.', { exact: true })).toBeVisible()
    await expect(page.locator('main .dropdown')).toHaveCount(0)
    await expect(page.locator('main aside')).toHaveCount(0)
    await page.getByLabel('Audit justification').fill('Review empty states')
    await page.getByRole('button', { name: 'Execute Audit' }).click()
    await expect(page.getByText('Audit completed. No matching entries were found.', { exact: true })).toBeVisible()
    await expect(page.getByText('No findings match the selected filters.', { exact: true })).toHaveCount(0)
    await expect(page.locator('main .dropdown')).toHaveCount(0)
    await expect(page.locator('main aside')).toHaveCount(0)
    await page.getByRole('button', { name: 'Execute Audit' }).click()
    await expect(page.getByRole('row').filter({ hasText: 'Filter check' })).toBeVisible()
    await expect(page.locator('main aside')).toHaveCount(0)
    const statusFilter = page.locator('main .dropdown').first()
    await statusFilter.locator('summary').click()
    await statusFilter.getByRole('button', { name: 'None', exact: true }).click()
    await expect(page.getByText('No findings match the selected filters.', { exact: true })).toBeVisible()
    await expect(page.locator('main aside')).toHaveCount(0)
    await expect(page.locator('main .dropdown')).toHaveCount(4)
  })
}
const denied = {
  id: 9,
  attempted_at: '2026-10-15T10:00:00Z',
  attempt_by: '<img src=x onerror=alert(1)>',
  roles: ['ARCHIVE_MANAGER'],
  service: 'ProcessAuditAndCompliance',
  method: 'Audit',
  scope: 'contracts',
  did,
  justification: 'Denied audit request',
  outcome: 'DENIED',
}
for (const width of [390, 1280]) {
  test(`access logs load, page, clear on failure and retry at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 })
    await authenticate(page, 'Auditor')
    const requests: URLSearchParams[] = []
    let pending: Route | undefined
    await page.route('**/pac/access-attempts?**', (route) => {
      requests.push(new URL(route.request().url()).searchParams)
      pending = route
    })
    await page.goto('/ui/audit')
    const summary = page.locator('summary').filter({ hasText: /^Access attempts$/ })
    await summary.focus()
    await page.keyboard.press('Enter')
    const section = page.locator('details').filter({ has: summary })
    const load = section.getByRole('button', { name: 'Load access attempts' })
    await expect(load).toBeDisabled()
    expect(requests).toHaveLength(0)
    await page.getByLabel('Access log justification').fill('  Investigate access denials  ')
    await load.click()
    await expect(section.getByRole('status')).toHaveText('Loading access attempts…')
    await expect(load).toBeDisabled()
    await expect.poll(() => requests.length).toBe(1)
    expect(requests[0].get('justification')).toBe('Investigate access denials')
    await json(pending!, { attempts: [denied], next_cursor: 'opaque:first/+' })
    const region = section.getByRole('region', { name: 'Access attempts table' })
    await expect(region).toContainText('DENIED')
    await expect(region).toContainText(denied.attempt_by)
    await expect(region.locator('img')).toHaveCount(0)
    await region.focus()
    await expect(region).toBeFocused()
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth),
    ).toBe(true)
    await page.getByLabel('Access log justification').fill('Changed purpose')
    await section.getByRole('button', { name: 'Go to next page' }).click()
    await expect.poll(() => requests.length).toBe(2)
    expect(requests[1].get('cursor')).toBe('opaque:first/+')
    expect(requests[1].get('justification')).toBe('Investigate access denials')
    await json(pending!, { attempts: [{ ...denied, id: 8, outcome: 'FAILED' }] })
    await expect(region).toContainText('FAILED')
    await section.getByRole('button', { name: 'Go to previous page' }).click()
    await expect.poll(() => requests.length).toBe(3)
    expect(requests[2].has('cursor')).toBe(false)
    await json(pending!, { attempts: [denied], next_cursor: 'opaque:first/+' })
    await expect(region).toContainText('DENIED')
    await load.click()
    await expect.poll(() => requests.length).toBe(4)
    expect(requests[3].get('justification')).toBe('Changed purpose')
    expect(requests[3].has('cursor')).toBe(false)
    await json(pending!, { message: 'Access unavailable' }, 500)
    await expect(page.getByRole('alert')).toHaveText('Access attempts could not be loaded. Please try again.')
    await expect(region).toHaveCount(0)
    await expect(section.getByText('No access attempts found.')).toHaveCount(0)
    await load.click()
    await expect.poll(() => requests.length).toBe(5)
    await json(pending!, { attempts: [] })
    await expect(section.getByRole('status')).toHaveText('No access attempts found.')
    await expect(page.getByRole('alert')).toHaveCount(0)
  })
}
for (const role of ['Archive Manager', 'Compliance Officer']) {
  test(`${role} cannot see or request the access log panel`, async ({ page }) => {
    await authenticate(page, role)
    let requests = 0
    await page.route('**/pac/access-attempts**', (route) => {
      requests++
      return json(route, { attempts: [] })
    })
    await page.goto('/ui/audit')
    await expect(page.getByRole('heading', { name: 'Audit', exact: true })).toBeVisible()
    await expect(page.locator('summary').filter({ hasText: /^Access attempts$/ })).toHaveCount(0)
    expect(requests).toBe(0)
  })
}

for (const role of ['Archive Manager', 'Contract Observer', 'Auditor']) {
  for (const width of [390, 768, 1280, 3440]) {
    test(`Archive layout ${role} at ${width}px`, async ({ page }, testInfo) => {
      await page.setViewportSize({ width, height: 1000 })
      await authenticate(page, role)
      const longName = 'Archived supply agreement — '.repeat(5)
      await page.route('**/archive/statistics', (route) =>
        json(route, {
          archived_total: 2,
          archived_contracts: 1,
          storage_bytes: 6250,
          compliant_total: 1,
          flagged_total: 1,
          deleted_total: 0,
          expiring_contracts: [{ did, name: longName, exp_date: '2026-11-01T00:00:00Z' }],
          expiring_window_days: 30,
          recent_actions: [
            {
              did,
              actor: 'did:web:long-actor.test:'.repeat(5),
              event_type: 'STORE_ARCHIVED_CONTRACT',
              occurred_at: '2026-10-20T10:00:00Z',
            },
          ],
        }),
      )
      await page.route('**/archive/retrieve', (route) =>
        json(route, {
          contracts: [1, 2].map((contract_version) => ({
            did,
            name: longName,
            // Delete is offered only for a contract out of force (DCS-FR-CSA-17).
            state: contract_version === 1 ? 'TERMINATED' : 'SIGNED',
            contract_version,
            archive_summary: 'Long archive summary '.repeat(15),
            archive_tags: ['long-tag'.repeat(25)],
          })),
        }),
      )
      await page.route('**/archive/erasure-status**', (route) =>
        json(route, { did, local_status: width === 768 ? 'shredded' : 'live', peers: [] }),
      )
      await page.goto('/ui/archive/dashboard')
      const dashboard = page.getByTestId('archive-dashboard')
      await expect(dashboard).toBeVisible()
      if (role !== 'Auditor') {
        const toggle = page.getByRole('button', { name: 'Additional filters' })
        await toggle.focus()
        await page.keyboard.press('Enter')
        await expect(toggle).toHaveAttribute('aria-expanded', 'true')
        await page.getByLabel('Tag', { exact: true }).fill('retained')
        await toggle.focus()
        await page.keyboard.press('Space')
        await expect(toggle).toBeFocused()
        await expect(toggle).toHaveAttribute('aria-expanded', 'false')
        await expect(page.getByLabel('Tag', { exact: true })).not.toBeVisible()
        await page.keyboard.press('Tab')
        await expect(page.getByRole('button', { name: 'Reset search' })).toBeFocused()
        // Saved queries and export (DCS-FR-CSA-22) follow the search; Save
        // query stays disabled until the query has a name.
        await page.keyboard.press('Tab')
        await expect(page.getByLabel('Name for this query')).toBeFocused()
        await page.keyboard.press('Tab')
        await expect(page.getByRole('button', { name: 'Export CSV' })).toBeFocused()
        await page.keyboard.press('Tab')
        await expect(page.getByRole('button', { name: 'Export JSON' })).toBeFocused()
        if (role === 'Archive Manager') {
          // Row selection for bulk termination (DCS-FR-CSA-23).
          await page.keyboard.press('Tab')
          await expect(page.getByRole('checkbox', { name: `Select ${longName}` }).first()).toBeFocused()
        }
        await page.keyboard.press('Tab')
        await expect(page.getByRole('link', { name: `${longName}, version 1`, exact: true })).toBeFocused()
        const open = page.locator('.archive-entry-link')
        await expect(open).toHaveCount(2)
        for (const version of [1, 2]) {
          const link = open.nth(version - 1)
          await expect(link).toBeVisible()
          const url = new URL((await link.getAttribute('href')) ?? '', page.url())
          expect(url.searchParams.get('did')).toBe(did)
          expect(url.searchParams.get('contract_version')).toBe(String(version))
          const box = await link.boundingBox()
          expect(box!.x).toBeGreaterThanOrEqual(0)
          expect(box!.x + box!.width).toBeLessThanOrEqual(width)
        }
        if (role === 'Archive Manager') {
          const list = await page.getByTestId('archive-entries').boundingBox()
          const stats = await page.getByTestId('archive-statistics').boundingBox()
          expect(list!.y).toBeLessThan(stats!.y)
          await page.getByTestId(`erasure-badge-${did}`).first().click()
          await expect(page.getByTestId(`erasure-details-${did}`).first()).toContainText(
            width === 768 ? 'Keys destroyed' : 'Encryption keys are live',
          )
          await page.getByTestId(`archive-delete-${did}`).first().click()
          await expect(page.getByPlaceholder('Justification')).toBeVisible()
          await expect(page.getByRole('dialog').getByRole('checkbox')).toBeVisible()
          await expect(page.getByRole('dialog')).toContainText(
            'Encryption keys will be destroyed on both instances (irreversible)',
          )
          await expect(page.getByTestId('confirmation-confirm')).toBeDisabled()
          await page.getByPlaceholder('Justification').fill('Layout verification')
          await expect(page.getByTestId('confirmation-confirm')).toBeDisabled()
          await page.getByRole('dialog').getByRole('checkbox').check()
          await expect(page.getByTestId('confirmation-confirm')).toBeEnabled()
          await page.getByRole('dialog').getByRole('button', { name: 'Cancel' }).click()
        }
      }
      if (role !== 'Contract Observer') {
        await expect(page.getByTestId('stat-flagged')).toHaveText('1')
        const audit = page.getByTestId('archive-recent-actions').getByRole('link')
        expect(await audit.getAttribute('href')).toContain('scope=archive')
      }
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth),
      ).toBe(true)
      const bounds = await dashboard.boundingBox()
      expect(bounds!.width).toBeLessThanOrEqual(1536)
      if (width === 3440) {
        const parent = await dashboard.locator('..').boundingBox()
        expect(Math.abs(bounds!.x - parent!.x - (parent!.x + parent!.width - bounds!.x - bounds!.width))).toBeLessThan(
          2,
        )
      }
      await page.getByRole('heading', { name: 'Contract Archive Dashboard' }).scrollIntoViewIfNeeded()
      await page.screenshot({
        path: testInfo.outputPath(`archive-${role.replaceAll(' ', '-')}-${width}.png`),
        fullPage: true,
      })
      if (role !== 'Contract Observer') {
        await page.getByTestId('archive-recent-actions').scrollIntoViewIfNeeded()
        await page.screenshot({
          path: testInfo.outputPath(`overview-${role.replaceAll(' ', '-')}-${width}.png`),
          fullPage: true,
        })
      }
      if (role === 'Auditor') {
        await page.getByTestId('archive-recent-actions').getByRole('link').click()
        await expect(page.locator('#select-scope')).toHaveValue('archive')
        await expect(page.locator('#input-did')).toHaveValue(did)
      } else {
        await page.route('**/api/archive/detail**', (route) =>
          json(route, { did, name: longName, contract_version: 2, state: 'SIGNED' }),
        )
        await page.locator('.archive-entry-link').nth(1).click()
        await expect(page).toHaveURL(/archive\/detail\?.*contract_version=2/)
        expect(new URL(page.url()).searchParams.get('did')).toBe(did)
      }
    })
  }
}

for (const [role, width] of [
  ['Archive Manager', 1280],
  ['Archive Manager', 3440],
  ['Auditor', 1280],
] as const) {
  test(`Archive compact overview ${role} at ${width}px`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width, height: 1000 })
    await authenticate(page, role)
    await page.route('**/archive/statistics', (route) =>
      json(route, {
        archived_total: 1,
        archived_contracts: 1,
        storage_bytes: 6250,
        compliant_total: 1,
        flagged_total: 0,
        deleted_total: 0,
        expiring_contracts: [],
        expiring_window_days: 30,
        recent_actions: [],
      }),
    )
    await page.route('**/archive/retrieve', (route) =>
      json(route, {
        contracts: [
          {
            did,
            name: 'Supply agreement 2026',
            contract_version: 1,
            state: 'SIGNED',
            archive_summary: 'Signed supply agreement for the current year.',
            archive_tags: ['supply', '2026'],
          },
        ],
      }),
    )
    await page.route('**/archive/erasure-status**', (route) => json(route, { did, local_status: 'live', peers: [] }))
    await page.goto('/ui/archive/dashboard')
    await expect(page.getByTestId('archive-expiring')).toContainText(
      'No archived contracts expire within the next 30 days.',
    )
    await expect(page.getByTestId('archive-recent-actions')).toContainText('No archive operations recorded yet.')
    await expect(page.getByTestId('stat-storage-volume')).toHaveText('6.1 KiB')
    expect((await page.getByTestId('archive-expiring').boundingBox())!.height).toBeLessThan(160)
    await page.screenshot({
      path: testInfo.outputPath(`compact-${role.replaceAll(' ', '-')}-${width}.png`),
      fullPage: true,
    })
  })
}

for (const width of [390, 1280, 3440]) {
  for (const mode of ['observer', 'live', 'shredded']) {
    test(`Archive clickable entries ${mode} at ${width}px`, async ({ page }) => {
      await page.setViewportSize({ width, height: 1000 })
      await authenticate(page, mode === 'observer' ? 'Contract Observer' : 'Archive Manager')
      const name = 'Supply agreement '.repeat(12)
      await page.route('**/archive/statistics', (route) =>
        json(route, {
          archived_total: 2,
          archived_contracts: 1,
          storage_bytes: 100,
          compliant_total: 2,
          flagged_total: 0,
          deleted_total: 0,
          expiring_contracts: [],
          expiring_window_days: 30,
          recent_actions: [],
        }),
      )
      await page.route('**/archive/retrieve', (route) =>
        json(route, {
          contracts: [1, 2].map((contract_version) => ({
            did,
            name,
            // Delete is offered only for a contract out of force (DCS-FR-CSA-17).
            state: contract_version === 1 ? 'TERMINATED' : 'SIGNED',
            contract_version,
            archive_summary: contract_version === 1 ? 'Summary '.repeat(30) : undefined,
            archive_tags: ['long-tag'.repeat(30)],
          })),
        }),
      )
      await page.route('**/archive/erasure-status**', (route) => json(route, { did, local_status: mode, peers: [] }))
      await page.route('**/api/archive/detail**', (route) =>
        json(route, {
          did,
          name,
          contract_version: Number(new URL(route.request().url()).searchParams.get('contract_version')),
          state: 'SIGNED',
        }),
      )
      await page.goto('/ui/archive/dashboard')
      const rows = page.getByTestId(`archive-entry-${did}`)
      await expect(rows).toHaveCount(2)
      const link = rows.first().getByRole('link', { name: `${name}, version 1`, exact: true })
      await expect(rows.first().locator('a[href*="/archive/detail"]')).toHaveCount(1)
      await expect(page.getByText('Open contract', { exact: true })).toHaveCount(0)
      await expect(link).toHaveCSS('text-decoration-line', 'none')
      await link.hover()
      await expect(link).toHaveCSS('text-decoration-line', 'none')
      await page.screenshot({ path: `/tmp/facis-archive-clickable/hover-${mode}-${width}.png` })
      await link.focus()
      await expect(link).toBeFocused()
      expect(await link.evaluate((element) => getComputedStyle(element, '::after').outlineStyle)).toBe('solid')
      await page.screenshot({ path: `/tmp/facis-archive-clickable/focus-${mode}-${width}.png` })
      await link.press('Enter')
      await expect(page).toHaveURL(/archive\/detail\?.*contract_version=1/)
      expect(new URL(page.url()).searchParams.get('did')).toBe(did)
      await page.goBack()
      await rows
        .first()
        .getByRole('link', { name: `${name}, version 1`, exact: true })
        .click()
      await expect(page).toHaveURL(/archive\/detail\?.*contract_version=1/)
      await page.goBack()
      const second = rows.nth(1)
      await second.scrollIntoViewIfNeeded()
      const box = await second.boundingBox()
      await second.click({ position: { x: box!.width - 3, y: box!.height - 3 } })
      await expect(page).toHaveURL(/archive\/detail\?.*contract_version=2/)
      expect(new URL(page.url()).searchParams.get('did')).toBe(did)
      await page.goBack()
      for (const [label, target, version, corner] of [
        ['DID', rows.first().locator('.font-mono'), 1, false],
        ['Version', rows.first().locator('td').nth(1), 1, false],
        ['State', rows.first().locator('td').nth(2).locator('.badge'), 1, false],
        ['Summary', rows.first().getByTestId(`archive-annotation-${did}`).locator('div').first(), 1, false],
        ['Not annotated', rows.nth(1).getByText('Not annotated'), 2, false],
        ['Tags', rows.first().getByTestId(`archive-annotation-${did}`).locator('.badge'), 1, false],
        ['First row edge', rows.first(), 1, true],
        ['Second row edge', rows.nth(1), 2, true],
      ] as const) {
        await test.step(`coordinate click on ${label}`, async () => {
          await target.scrollIntoViewIfNeeded()
          const bounds = await target.boundingBox()
          await page.mouse.click(
            bounds!.x + (corner ? 2 : bounds!.width / 2),
            bounds!.y + (corner ? 2 : bounds!.height / 2),
          )
          await expect
            .soft(page, `${label} opens version ${version}`)
            .toHaveURL(new RegExp(`archive/detail\\?.*contract_version=${version}`), { timeout: 2000 })
          if (page.url().includes('/archive/detail')) expect(new URL(page.url()).searchParams.get('did')).toBe(did)
          await page.goto('/ui/archive/dashboard')
        })
      }
      if (mode !== 'observer') {
        for (const keyboard of [false, true]) {
          const activate = async (control: ReturnType<Page['getByRole']>) => {
            if (keyboard) await control.press('Enter')
            else await control.click()
          }
          await activate(rows.first().getByTestId(`erasure-badge-${did}`))
          await expect(page.getByTestId(`erasure-details-${did}`).first()).toContainText(
            mode === 'live' ? 'Encryption keys are live' : 'Keys destroyed',
          )
          await expect(page).toHaveURL(/archive\/dashboard$/)
          const details = page.getByTestId(`erasure-details-${did}`).first()
          await details.scrollIntoViewIfNeeded()
          const detailBox = await details.boundingBox()
          await page.mouse.click(detailBox!.x + detailBox!.width / 2, detailBox!.y + detailBox!.height / 2)
          await expect(page).toHaveURL(/archive\/dashboard$/)
          await activate(rows.first().getByTestId(`erasure-badge-${did}`))
          for (const action of ['annotate', 'delete']) {
            await activate(rows.first().getByTestId(`archive-${action}-${did}`))
            await expect(page.getByRole('dialog')).toBeVisible()
            await expect(page).toHaveURL(/archive\/dashboard$/)
            await page.getByRole('dialog').getByRole('button', { name: 'Cancel' }).click()
          }
          await activate(rows.first().getByRole('link', { name: 'Audit', exact: true }))
          await expect(page).toHaveURL(/audit\?scope=archive/)
          expect(new URL(page.url()).searchParams.get('did')).toBe(did)
          await page.goBack()
        }
      }
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth),
      ).toBe(true)
    })
  }
}

for (const width of [390, 1440]) {
  test(`long access logs preserve audit priority and visible outcomes at ${width}px`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width, height: width === 390 ? 844 : 900 })
    await authenticate(page, 'Auditor')
    await page.route('**/pac/access-attempts?**', (route) =>
      json(route, {
        attempts: Array.from({ length: 25 }, (_, index) => ({
          ...denied,
          id: index + 1,
          attempt_by: 'did:web:long-actor-'.repeat(30),
          roles: Array.from({ length: 25 }, (_, role) => `LONG_DEVELOPMENT_ROLE_${role}`),
          did: 'did:web:long-contract-'.repeat(30),
          justification: 'Investigate access attempts '.repeat(30),
        })),
      }),
    )
    await page.goto('/ui/audit')
    const execute = page.getByRole('button', { name: 'Execute Audit', exact: true })
    const executeTop = () =>
      execute.evaluate((element) => {
        const main = element.closest('main')!
        return element.getBoundingClientRect().top - main.getBoundingClientRect().top + main.scrollTop
      })
    const originalTop = await executeTop()
    const summary = page.locator('summary').filter({ hasText: /^Access attempts$/ })
    await summary.press('Enter')
    const purpose = page.getByLabel('Access log justification')
    const loadButton = page.getByRole('button', { name: 'Load access attempts' })
    const purposeBox = (await purpose.boundingBox())!
    const buttonBox = (await loadButton.boundingBox())!
    if (width === 390) {
      expect(buttonBox.y).toBeGreaterThanOrEqual(purposeBox.y + purposeBox.height)
      expect(Math.abs(buttonBox.x - purposeBox.x)).toBeLessThanOrEqual(1)
      expect(Math.abs(buttonBox.width - purposeBox.width)).toBeLessThanOrEqual(1)
    } else {
      expect(buttonBox.x).toBeGreaterThanOrEqual(purposeBox.x + purposeBox.width)
    }
    await purpose.fill('Review long access log')
    await page.getByRole('button', { name: 'Load access attempts' }).press('Enter')
    const region = page.getByRole('region', { name: 'Access attempts table' })
    await expect(region.locator('tbody tr')).toHaveCount(25)
    expect(await executeTop()).toBe(originalTop)
    expect(
      await summary.evaluate((element) => {
        const findings = Array.from(document.querySelectorAll('h3')).find(
          (heading) => heading.textContent === 'Findings',
        )!
        return !!(findings.compareDocumentPosition(element) & Node.DOCUMENT_POSITION_FOLLOWING)
      }),
    ).toBe(true)
    await region.scrollIntoViewIfNeeded()
    const size = await region.evaluate((element) => ({
      height: element.clientHeight,
      scrollHeight: element.scrollHeight,
    }))
    expect(size.height).toBeLessThanOrEqual(384)
    expect(size.scrollHeight).toBeGreaterThan(size.height)
    const outcome = region.locator('tbody tr').first().locator('td').first()
    await expect(outcome).toHaveText('DENIED')
    const regionBox = (await region.boundingBox())!
    const outcomeBox = (await outcome.locator('.badge').boundingBox())!
    expect(outcomeBox.x).toBeGreaterThanOrEqual(regionBox.x)
    expect(outcomeBox.x + outcomeBox.width).toBeLessThanOrEqual(regionBox.x + regionBox.width)
    expect(outcomeBox.y + outcomeBox.height).toBeLessThanOrEqual(regionBox.y + regionBox.height)
    const actor = region.locator('tbody tr').first().locator('td').nth(2).locator('details')
    const actorSummary = actor.locator('summary')
    const fullActor = 'did:web:long-actor-'.repeat(30)
    await expect(actorSummary).toHaveAttribute('title', fullActor)
    await expect(actorSummary).toContainText('…')
    await actorSummary.press('Enter')
    await expect(actor).toHaveAttribute('open', '')
    await expect(actor.locator('div')).toHaveText(fullActor)
    await expect(actor.locator('div')).toBeVisible()
    await actorSummary.press('Space')
    await expect(actor).not.toHaveAttribute('open', '')
    await expect(actor.locator('div')).not.toBeVisible()
    const time = region.locator('tbody tr').first().locator('time')
    await expect(time).toHaveAttribute('datetime', denied.attempted_at)
    await expect(time).toHaveAttribute('title', denied.attempted_at)
    await expect(time).not.toHaveText(denied.attempted_at)
    await region.evaluate((element) => {
      element.scrollLeft = 0
    })
    await region.focus()
    await expect(region).toBeFocused()
    await page.keyboard.press('ArrowDown')
    await expect.poll(() => region.evaluate((element) => element.scrollTop)).toBeGreaterThan(0)
    await region.evaluate((element) => {
      element.scrollTop = 200
    })
    await expect
      .poll(async () => {
        const header = (await region.locator('th').first().boundingBox())!
        const box = (await region.boundingBox())!
        return Math.abs(header.y - box.y)
      })
      .toBeLessThanOrEqual(1)
    await expect(region.locator('th').first()).toHaveText('Outcome')
    expect(await region.locator('tbody tr').last().textContent()).toContain('LONG_DEVELOPMENT_ROLE_24')
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth),
    ).toBe(true)
    await page.screenshot({ path: testInfo.outputPath(`access-log-stress-${width}.png`), fullPage: true })
  })
}
