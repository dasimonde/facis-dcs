import { expect, type Page, type Route, test } from '@playwright/test'

const templateDid = 'did:web:example.test:templates:review-layout'
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

function jwt(role: string): string {
  const encode = (value: object) => Buffer.from(JSON.stringify(value)).toString('base64url')
  return `${encode({ alg: 'none', typ: 'JWT' })}.${encode({
    sub: 'did:web:reviewer.example',
    exp: Math.floor(Date.now() / 1000) + 3600,
    roles: [role],
    ext: { iss: 'did:web:example.test', roles: [role] },
  })}.unsigned`
}

async function authenticate(page: Page, role: string): Promise<void> {
  const accessToken = jwt(role)
  await page.route('**/auth/refresh', (route) => json(route, { token_type: 'Bearer', access_token: accessToken }))
  await page.route('**/semantic/schema/list', (route) => json(route, []))
  await page.route('**/semantic/ontology/dcs-odrl-profile', (route) => json(route, { content: odrlProfileFixture }))
  await page.addInitScript(
    ([accessToken]) => {
      localStorage.setItem('token_type', 'Bearer')
      localStorage.setItem('access_token', accessToken)
    },
    [accessToken],
  )
}

async function json(route: Route, body: unknown, status = 200): Promise<void> {
  await route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })
}

async function mockTemplateReview(page: Page): Promise<void> {
  await authenticate(page, 'Template Reviewer')
  await page.route('**/template/retrieve/**', (route) =>
    json(route, {
      did: templateDid,
      version: 1,
      state: 'SUBMITTED',
      template_type: 'COMPONENT',
      name: 'Approval verification fixture',
      description: 'A submitted template awaiting review.',
      created_by: 'template-creator',
      created_at: '2026-07-29T08:00:00Z',
      updated_at: '2026-07-29T08:00:00Z',
      template_data: {
        '@type': 'dcs:ContractTemplate',
        'dcs:metadata': {
          '@type': 'dcs:TemplateMetadata',
          'dcs:title': 'Approval verification fixture',
          'dcs:description': 'A submitted template awaiting review.',
          'dcs:templateType': 'COMPONENT',
        },
        'dcs:documentStructure': {
          '@type': 'dcs:DocumentStructure',
          'dcs:blocks': { '@list': [] },
          'dcs:layout': { '@list': [] },
        },
        'dcs:contractData': [],
        'dcs:contractFields': [],
        'dcs:policies': {
          '@id': `${templateDid}#policy`,
          '@type': 'odrl:Offer',
          'odrl:profile': { '@id': 'dcs:profile' },
        },
      },
    }),
  )
}

async function gotoReview(page: Page): Promise<void> {
  await page.goto(`/ui/templates/review/${encodeURIComponent(templateDid)}`)
  await expect(page.getByText('Review Template', { exact: true })).toBeVisible()
}

test.describe('audit and compliance layout at 1280px with expanded navigation', () => {
  test.use({ viewport: { width: 1280, height: 800 } })

  test('AC1/AC2 Audit has no page overflow and keeps JSON/CSV/PDF export in one visible row', async ({ page }) => {
    await authenticate(page, 'Auditor')
    await page.goto('/ui/audit')
    await expect(page.getByRole('heading', { name: 'Audit' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Collapse sidebar' })).toBeVisible()

    const metrics = await page.evaluate(() => ({
      documentScrollWidth: document.documentElement.scrollWidth,
      documentClientWidth: document.documentElement.clientWidth,
    }))
    expect(metrics.documentScrollWidth).toBeLessThanOrEqual(metrics.documentClientWidth)

    const buttons = ['JSON', 'CSV', 'PDF'].map((name) => page.getByRole('button', { name, exact: true }))
    for (const button of buttons) await expect(button).toBeVisible()
    const boxes = await Promise.all(buttons.map((button) => button.boundingBox()))
    expect(new Set(boxes.map((box) => Math.round(box?.y ?? -1))).size).toBe(1)
  })

  test('AC4 Audit enables export only after a successful current-scope run and clears filtered selection', async ({
    page,
  }) => {
    await authenticate(page, 'Auditor')
    let failAudit = true
    await page.route('**/pac/audit', (route) => {
      if (failAudit) return json(route, { message: 'executor unavailable' }, 500)
      return json(route, {
        audit_id: 'audit-1',
        correlation_id: 'correlation-1',
        executed_at: '2026-07-30T08:00:00Z',
        scope: 'contracts',
        executor: { id: 'policy-engine', version: '1' },
        resource: { did: 'did:web:example.test:contract-1' },
        findings: [{ rule_id: 'RULE-1', result: 'FAILED', reason: 'Policy mismatch' }],
      })
    })
    await page.goto('/ui/audit')
    await page.getByLabel('Audit justification').fill('Focused verification')
    await expect(page.getByText('Failed Checks')).toHaveCount(0)
    const exports = page.getByRole('button', { name: /^(JSON|CSV|PDF)$/ })
    for (let index = 0; index < (await exports.count()); index += 1) await expect(exports.nth(index)).toBeDisabled()

    await page.getByRole('button', { name: 'Execute Audit' }).click()
    await expect(page.getByRole('alert')).toContainText(/executor unavailable/i)
    await expect(page.getByText('Failed Checks')).toHaveCount(0)
    for (let index = 0; index < (await exports.count()); index += 1) await expect(exports.nth(index)).toBeDisabled()

    failAudit = false
    await page.getByRole('button', { name: 'Execute Audit' }).click()
    await expect(page.getByText('Policy mismatch')).toBeVisible()
    await expect(page.getByText('Failed Checks').locator('..')).toContainText('1')
    for (let index = 0; index < (await exports.count()); index += 1) await expect(exports.nth(index)).toBeEnabled()
    await page.getByText('Policy mismatch').click()
    await expect(page.getByRole('heading', { name: 'Finding Details' }).locator('..')).toContainText('RULE-1')

    await page.getByRole('button', { name: /Component/ }).click()
    await page.getByRole('button', { name: 'None' }).click()
    await expect(page.getByText('Select a row to inspect the corresponding audit evidence.')).toBeVisible()

    await page.getByLabel('DID (optional)').fill('did:web:example.test:other')
    for (let index = 0; index < (await exports.count()); index += 1) await expect(exports.nth(index)).toBeDisabled()
  })

  test('AC1/AC2/AC3 Compliance keeps list bounded and JSON/PDF exports visible in one row', async ({ page }) => {
    await authenticate(page, 'Auditor')
    const longDid = `did:web:example.test:${'contract-segment-'.repeat(14)}`
    await page.route('**/signature/retrieve', (route) =>
      json(route, {
        contracts: [{ did: longDid, name: 'Long compliance contract', state: 'SIGNED' }],
        signing_tasks: [],
      }),
    )
    await page.route('**/signature/view**', (route) =>
      json(route, { contract_state: 'SIGNED', integrity_findings: [], signatures: [] }),
    )

    await page.goto('/ui/compliance')
    await page.getByRole('button', { name: /Long compliance contract/ }).click()

    const metrics = await page.evaluate(() => ({
      documentScrollWidth: document.documentElement.scrollWidth,
      documentClientWidth: document.documentElement.clientWidth,
    }))
    expect(metrics.documentScrollWidth).toBeLessThanOrEqual(metrics.documentClientWidth)

    const exports = ['Export JSON', 'Export PDF'].map((name) => page.getByRole('button', { name, exact: true }))
    for (const button of exports) await expect(button).toBeVisible()
    const boxes = await Promise.all(exports.map((button) => button.boundingBox()))
    expect(new Set(boxes.map((box) => Math.round(box?.y ?? -1))).size).toBe(1)

    const contractList = page.getByRole('list').filter({ hasText: 'Long compliance contract' })
    const listOverflow = await contractList.evaluate((element) => ({
      scrollWidth: element.scrollWidth,
      clientWidth: element.clientWidth,
    }))
    expect(listOverflow.scrollWidth).toBeLessThanOrEqual(listOverflow.clientWidth)
    const listBox = await contractList.boundingBox()
    const detailTextBox = await page.getByText('No integrity findings recorded.').boundingBox()
    expect((listBox?.x ?? 0) + (listBox?.width ?? 0)).toBeLessThanOrEqual(detailTextBox?.x ?? 0)
  })
})

test.describe('Template Review approval performs mandatory verification', () => {
  test('AC4 Approve is the only verification action and invokes verify exactly once before a dialog', async ({
    page,
  }) => {
    await mockTemplateReview(page)
    let verifyCalls = 0
    await page.route('**/template/verify', async (route) => {
      verifyCalls += 1
      await json(route, { did: templateDid, findings: [] })
    })
    await gotoReview(page)

    await expect(page.getByRole('button', { name: 'Verify', exact: true })).toHaveCount(0)
    await page.getByRole('button', { name: 'Approve', exact: true }).click()
    await expect(page.getByRole('dialog')).toBeVisible()
    expect(verifyCalls).toBe(1)
  })

  test('AC5 distinguishes findings from an explicit no-findings result', async ({ page }) => {
    await mockTemplateReview(page)
    await page.route('**/template/verify', (route) =>
      json(route, { did: templateDid, findings: ['Missing semantic classification'] }),
    )
    await gotoReview(page)
    await page.getByRole('button', { name: 'Approve', exact: true }).click()
    await expect(page.getByRole('dialog')).toContainText('Missing semantic classification')

    await page.reload()
    await page.unroute('**/template/verify')
    await page.route('**/template/verify', (route) => json(route, { did: templateDid, findings: [] }))
    await page.getByRole('button', { name: 'Approve', exact: true }).click()
    await expect(page.getByRole('dialog')).toContainText(/no findings/i)
  })

  test('AC5 exposes verify 500 as an alert and allows retry', async ({ page }) => {
    await mockTemplateReview(page)
    let attempts = 0
    await page.route('**/template/verify', async (route) => {
      attempts += 1
      await json(
        route,
        attempts === 1 ? { error: 'verification unavailable' } : { did: templateDid, findings: [] },
        attempts === 1 ? 500 : 200,
      )
    })
    await gotoReview(page)
    await page.getByRole('button', { name: 'Approve', exact: true }).click()
    await expect(page.getByRole('dialog').getByRole('alert')).toContainText(/verification|retry|unavailable/i)
    await page.getByRole('button', { name: /retry/i }).click()
    expect(attempts).toBe(2)
  })

  test('AC6 every returned string finding blocks forwarding and no submit is sent', async ({ page }) => {
    await mockTemplateReview(page)
    let submits = 0
    await page.route('**/template/verify', (route) =>
      json(route, { did: templateDid, findings: ['Informational note'] }),
    )
    await page.route('**/template/submit', async (route) => {
      submits += 1
      await json(route, { did: templateDid })
    })
    await gotoReview(page)
    await page.getByRole('button', { name: 'Approve', exact: true }).click()
    await expect(page.getByRole('dialog')).toContainText('Informational note')
    await expect(page.getByRole('dialog').getByRole('button', { name: /confirm|submit|approve/i })).toHaveCount(0)
    expect(submits).toBe(0)
  })

  test('AC7 zero findings submits once to APPROVAL; cancel and Escape never submit and restore focus', async ({
    page,
  }) => {
    await mockTemplateReview(page)
    const submissions: unknown[] = []
    await page.route('**/template/verify', (route) => json(route, { did: templateDid, findings: [] }))
    await page.route('**/template/submit', async (route) => {
      submissions.push(route.request().postDataJSON())
      await json(route, { did: templateDid })
    })
    await gotoReview(page)
    const approve = page.getByRole('button', { name: 'Approve', exact: true })

    await approve.click()
    await page.getByRole('button', { name: 'Cancel', exact: true }).click()
    expect(submissions).toHaveLength(0)
    await expect(approve).toBeFocused()

    await approve.click()
    await page.keyboard.press('Escape')
    expect(submissions).toHaveLength(0)
    await expect(approve).toBeFocused()

    await approve.click()
    await page.getByRole('button', { name: /confirm|submit/i }).click()
    await expect.poll(() => submissions.length).toBe(1)
    expect(submissions[0]).toMatchObject({ did: templateDid, forward_to: 'APPROVAL' })
  })
})
