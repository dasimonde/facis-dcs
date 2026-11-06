import { expect, type Locator, type Page, test } from '@playwright/test'
import { withholdUnsignedToken } from './unsigned-token'

const odrlProfileFixture = `
  @prefix dcs: <https://w3id.org/facis/dcs/ontology/v1#> .
  @prefix odrl: <http://www.w3.org/ns/odrl/2/> .
  @prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
  <https://w3id.org/facis/dcs/ontology/v1/odrl-profile>
    a odrl:Profile ;
    dcs:defaultConstraintAction dcs:provideCompliantValue .
  odrl:eq
    a odrl:Operator ;
    rdfs:label "Must equal" ;
    dcs:appliesToParameterType "string" .
`

async function authenticate(page: Page) {
  const encode = (value: object) => Buffer.from(JSON.stringify(value)).toString('base64url')
  const token = `${encode({ alg: 'none', typ: 'JWT' })}.${encode({
    sub: 'did:web:auditor.test',
    exp: Math.floor(Date.now() / 1000) + 3600,
    roles: ['Auditor'],
    ext: { iss: 'did:web:example.test', roles: ['Auditor'] },
  })}.unsigned`
  await page.route('**/auth/refresh', (route) => route.fulfill({ json: { token_type: 'Bearer', access_token: token } }))
  await page.route('**/semantic/schema/list', (route) => route.fulfill({ json: [] }))
  await page.route('**/semantic/ontology/dcs-odrl-profile', (route) =>
    route.fulfill({ json: { content: odrlProfileFixture } }),
  )
  await withholdUnsignedToken(page)
  await page.addInitScript((token) => {
    localStorage.setItem('token_type', 'Bearer')
    localStorage.setItem('access_token', token)
  }, token)
}

function deferred() {
  let resolve!: () => void
  const promise = new Promise<void>((done) => (resolve = done))
  return { promise, resolve }
}

async function boxes(locators: Locator[]) {
  return Promise.all(
    locators.map((locator) =>
      locator.evaluate((element) => {
        const rect = element.getBoundingClientRect()
        let x = rect.x + window.scrollX
        let y = rect.y + window.scrollY
        for (
          let parent = element.parentElement;
          parent && parent !== document.documentElement;
          parent = parent.parentElement
        ) {
          x += parent.scrollLeft
          y += parent.scrollTop
        }
        return { x, y, width: rect.width, height: rect.height }
      }),
    ),
  )
}

for (const width of [1920, 1440, 390]) {
  test(`audit controls remain stable through request states at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 1000 })
    await authenticate(page)
    let auditGate = deferred()
    let failAudit = false
    await page.route('**/pac/audit', async (route) => {
      await auditGate.promise
      await route.fulfill(
        failAudit
          ? { status: 500, json: { message: 'Executor unavailable' } }
          : {
              json: {
                audit_id: 'stable-layout',
                correlation_id: 'layout-correlation',
                executed_at: '2026-10-26T10:00:00Z',
                scope: 'contracts',
                executor: { id: 'layout-engine', version: '1' },
                resource: { did: `did:web:example.test:${'long-contract-'.repeat(20)}` },
                findings: [
                  { rule_id: 'LAYOUT-1', result: 'FAILED', reason: 'A long contract needs policy review' },
                  { rule_id: 'LAYOUT-2', result: 'PASSED', reason: 'The evidence is valid' },
                ],
              },
            },
      )
    })
    const reportGate = deferred()
    await page.route('**/pac/report**', async (route) => {
      await reportGate.promise
      await route.fulfill({ status: 500, json: { message: 'Report unavailable' } })
    })
    await page.goto('/ui/audit')
    await page.getByLabel('Audit justification').fill('Stable layout verification')
    const execute = page.getByRole('button', { name: 'Execute Audit', exact: true })
    const downloads = ['JSON', 'CSV', 'PDF'].map((name) => page.getByRole('button', { name, exact: true }))
    const anchors = [
      page.getByRole('combobox', { name: 'Scope', exact: true }),
      page.getByLabel('DID (optional)'),
      page.getByLabel('Audit justification'),
      execute,
      page.getByRole('heading', { name: 'Audit result', exact: true }),
      ...downloads,
      page.getByRole('banner'),
    ]
    const initial = await boxes(anchors)
    const expectStable = async () => {
      const current = await boxes(anchors)
      for (let index = 0; index < initial.length; index++) {
        for (const dimension of ['x', 'y', 'width', 'height'] as const) {
          expect(
            Math.abs(current[index]![dimension] - initial[index]![dimension]),
            `anchor ${index} ${dimension}: ${initial[index]![dimension]} → ${current[index]![dimension]}`,
          ).toBeLessThanOrEqual(1)
        }
      }
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth)
      expect(overflow).toBe(false)
    }
    await expect(page.getByTestId('audit-counters')).toContainText('—')
    await anchors[0]!.focus()
    for (const next of [anchors[1]!, anchors[2]!, execute]) {
      await page.keyboard.press('Tab')
      await expect(next).toBeFocused()
    }
    await execute.press('Enter')
    await expect(page.getByText('Executing audit...', { exact: true })).toBeVisible()
    await expect(execute).toHaveText('Execute Audit')
    await expect(execute.locator('.loading')).toHaveAttribute('aria-hidden', 'true')
    await expectStable()
    auditGate.resolve()
    await expect(page.getByText('A long contract needs policy review', { exact: true })).toBeVisible()
    await expectStable()
    const counters = page.getByTestId('audit-counters')
    await expect(counters).not.toContainText('—')
    const counterBoxes = await boxes([0, 1, 2, 3].map((index) => counters.locator(':scope > div').nth(index)))
    expect(counterBoxes[0]!.y).toBe(counterBoxes[1]!.y)
    if (width === 390) expect(counterBoxes[2]!.y).toBeGreaterThan(counterBoxes[0]!.y)
    else expect(new Set(counterBoxes.map((box) => box.y)).size).toBe(1)
    for (const title of ['Failed Checks', 'Passed Checks', 'Needs Review', 'Not Evaluated']) {
      const metrics = await counters.getByText(title, { exact: true }).evaluate((element) => ({
        clientWidth: element.clientWidth,
        scrollWidth: element.scrollWidth,
      }))
      expect(metrics.scrollWidth).toBeLessThanOrEqual(metrics.clientWidth)
    }

    const firstRow = page.getByTestId('audit-findings-scroll').locator('tbody tr').first()
    await firstRow.focus()
    await firstRow.press('Enter')
    const details = page.getByRole('complementary').filter({ hasText: 'Finding Details' })
    await expect(details).toContainText('LAYOUT-1')
    const tableBox = (await page.getByTestId('audit-findings-scroll').boundingBox())!
    const detailBox = (await details.boundingBox())!
    if (width === 390) expect(detailBox.y).toBeGreaterThanOrEqual(tableBox.y + tableBox.height)
    else expect(detailBox.x).toBeGreaterThanOrEqual(tableBox.x + tableBox.width)
    const filters = page.locator('main .dropdown')
    for (let index = 0; index < 4; index++) {
      const filter = filters.nth(index)
      await filter.locator('summary').focus()
      await filter.locator('summary').press('Enter')
      const menu = filter.locator('.dropdown-content')
      await expect(menu).toBeVisible()
      const menuBox = (await menu.boundingBox())!
      expect(menuBox.x).toBeGreaterThanOrEqual(0)
      expect(menuBox.x + menuBox.width).toBeLessThanOrEqual(width)
      const checkbox = menu.getByRole('checkbox').first()
      await checkbox.scrollIntoViewIfNeeded()
      const visibleAtCenter = await checkbox.evaluate((element) => {
        const rect = element.getBoundingClientRect()
        return document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2) === element
      })
      expect(visibleAtCenter).toBe(true)
      await filter.locator('summary').press('Space')
      await expect(menu).not.toBeVisible()
    }
    await details.getByRole('button', { name: 'Close', exact: true }).click()
    // Return to the document origin before comparing document-position anchors.
    await page.evaluate(() => window.scrollTo(0, 0))
    await downloads[0]!.click()
    await expect(page.getByText('Preparing JSON report…', { exact: true })).toBeVisible()
    await expectStable()
    reportGate.resolve()
    await expect(page.getByRole('alert')).toHaveText('Audit report could not be generated.')
    await expectStable()
    await expect(page.getByText('A long contract needs policy review', { exact: true })).toBeVisible()

    failAudit = true
    auditGate = deferred()
    await execute.click()
    await expect(page.getByText('Executing audit...', { exact: true })).toBeVisible()
    await expectStable()
    auditGate.resolve()
    await expect(page.getByRole('alert')).toHaveText('Executor unavailable')
    await expectStable()
    await expect(counters).toContainText('—')
    for (const download of downloads) await expect(download).toBeDisabled()

    failAudit = false
    auditGate = deferred()
    await execute.click()
    auditGate.resolve()
    await expect(page.getByText('A long contract needs policy review', { exact: true })).toBeVisible()
    await expectStable()
    await page.getByLabel('DID (optional)').fill('did:web:different.test')
    await expectStable()
    await expect(counters).toContainText('—')
  })
}
