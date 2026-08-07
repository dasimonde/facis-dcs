import { expect, type Page, type Route, test } from '@playwright/test'

const contractDid = 'did:web:example.test:contracts:integrated-review'
const fieldDid = `${contractDid}#field-service-code`
const updatedAt = '2026-07-29T09:30:00Z'
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

interface ContractFixtureOptions {
  value?: string
  pattern?: string
}

function jwt(role: string): string {
  const encode = (value: object) => Buffer.from(JSON.stringify(value)).toString('base64url')
  return `${encode({ alg: 'none', typ: 'JWT' })}.${encode({
    sub: 'did:web:reviewer.example',
    exp: Math.floor(Date.now() / 1000) + 3600,
    roles: [role],
    ext: { iss: 'did:web:example.test', roles: [role] },
  })}.unsigned`
}

async function json(route: Route, body: unknown, status = 200): Promise<void> {
  await route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })
}

function contractFixture({ value = 'FACIS-42', pattern = '^FACIS-\\d+$' }: ContractFixtureOptions = {}) {
  return {
    did: contractDid,
    contract_version: 3,
    state: 'SUBMITTED',
    name: 'Integrated review fixture',
    description: 'A submitted contract awaiting local semantic review.',
    created_by: 'contract-creator',
    created_at: '2026-07-29T09:00:00Z',
    updated_at: updatedAt,
    contract_data: {
      '@id': contractDid,
      '@type': 'dcs:Contract',
      'dcs:metadata': {
        '@type': 'dcs:ContractMetadata',
        'dcs:title': 'Integrated review fixture',
        'dcs:description': 'A submitted contract awaiting local semantic review.',
      },
      'dcs:documentStructure': {
        '@type': 'dcs:DocumentStructure',
        'dcs:blocks': {
          '@list': [
            {
              '@id': `${contractDid}#clause-service`,
              '@type': 'dcs:Clause',
              'dcs:title': 'Service identification',
              'dcs:content': { '@list': ['Service code: ', { '@id': fieldDid }] },
            },
          ],
        },
        'dcs:layout': { '@list': [] },
      },
      'dcs:contractData': [],
      'dcs:contractFields': [
        {
          '@id': fieldDid,
          '@type': 'dcs:ContractField',
          'dcs:label': 'Service code',
          'dcs:datatype': 'xsd:string',
          'dcs:required': true,
          'dcs:value': { '@value': value, '@type': 'xsd:string' },
          'dcs:valueConstraint': { pattern },
        },
      ],
      'dcs:policies': {
        '@id': `${contractDid}#policy`,
        '@type': 'odrl:Agreement',
        'odrl:profile': { '@id': 'https://w3id.org/facis/dcs/ontology/v1/odrl-profile' },
      },
    },
  }
}

async function mockContractReview(page: Page, options: ContractFixtureOptions = {}): Promise<void> {
  const accessToken = jwt('Contract Reviewer')
  await page.route('**/auth/refresh', (route) => json(route, { token_type: 'Bearer', access_token: accessToken }))
  // The draft store hydrates its semantic catalog during startup, even though
  // this scenario deliberately verifies the contract document locally.
  await page.route('**/semantic/schema/list', (route) => json(route, []))
  await page.route('**/semantic/ontology/dcs-odrl-profile', (route) => json(route, { content: odrlProfileFixture }))
  await page.route('**/contract/retrieve/**', (route) => json(route, contractFixture(options)))
  await page.addInitScript(
    ([token]) => {
      localStorage.setItem('token_type', 'Bearer')
      localStorage.setItem('access_token', token)
    },
    [accessToken],
  )
}

async function gotoReview(page: Page): Promise<void> {
  await page.goto(`/ui/contracts/review/${encodeURIComponent(contractDid)}`)
  await expect(page.getByText('Review Contract', { exact: true })).toBeVisible()
}

function localPrecheckDialog(page: Page) {
  return page.getByRole('dialog', { name: /local semantic precheck/i })
}

test.describe('integrated Contract Review verification', () => {
  test('AC1 has no separate Verify action and keeps Approve actionable when the submitted contract has findings', async ({
    page,
  }) => {
    await mockContractReview(page, { value: 'wrong-service-code' })
    await gotoReview(page)

    await expect(page.getByRole('button', { name: 'Verify', exact: true })).toHaveCount(0)
    await expect(page.getByRole('button', { name: 'Approve', exact: true })).toBeEnabled()
  })

  test('AC2 Approve runs the current local semantic check once and opens the accessible local-precheck dialog', async ({
    page,
  }) => {
    await mockContractReview(page)
    await gotoReview(page)

    const approve = page.getByRole('button', { name: 'Approve', exact: true })
    await approve.click()

    const dialog = localPrecheckDialog(page)
    await expect(dialog).toBeVisible()
    await expect(dialog).toContainText(/local semantic precheck/i)
    await expect(dialog.getByText(/no findings/i)).toHaveCount(1)
  })

  test('AC3 lists every local finding, offers no confirmation, and never submits', async ({ page }) => {
    await mockContractReview(page, { value: 'wrong-service-code' })
    const submissions: unknown[] = []
    await page.route('**/contract/submit', async (route) => {
      submissions.push(route.request().postDataJSON())
      await json(route, { did: contractDid })
    })
    await gotoReview(page)

    await page.getByRole('button', { name: 'Approve', exact: true }).click()
    const dialog = localPrecheckDialog(page)
    await expect(dialog).toContainText('Service code')
    await expect(dialog).toContainText('Expected format')
    await expect(dialog.getByRole('button', { name: /confirm/i })).toHaveCount(0)
    expect(submissions).toHaveLength(0)
  })

  test('AC4 explicitly reports zero findings and provides an optional labeled comment plus Confirm', async ({
    page,
  }) => {
    await mockContractReview(page)
    await gotoReview(page)

    await page.getByRole('button', { name: 'Approve', exact: true }).click()
    const dialog = localPrecheckDialog(page)
    await expect(dialog).toContainText(/no findings/i)
    await expect(dialog.getByLabel(/comment \(optional\)/i)).toBeVisible()
    await expect(dialog.getByRole('button', { name: /confirm/i })).toBeEnabled()
  })

  test('AC5 Confirm submits the trimmed decision exactly once and double click cannot submit twice', async ({
    page,
  }) => {
    await mockContractReview(page)
    const submissions: unknown[] = []
    await page.route('**/contract/submit', async (route) => {
      submissions.push(route.request().postDataJSON())
      await new Promise((resolve) => setTimeout(resolve, 150))
      await json(route, { did: contractDid })
    })
    await gotoReview(page)

    await page.getByRole('button', { name: 'Approve', exact: true }).click()
    const dialog = localPrecheckDialog(page)
    await dialog.getByLabel(/comment \(optional\)/i).fill('  Ready for approval.  ')
    await dialog.getByRole('button', { name: /confirm/i }).dblclick()

    await expect.poll(() => submissions.length).toBe(1)
    expect(submissions[0]).toEqual({
      did: contractDid,
      updated_at: updatedAt,
      forward_to: 'APPROVAL',
      comments: ['Ready for approval.'],
    })
    await page.waitForTimeout(250)
    expect(submissions).toHaveLength(1)
  })

  test('AC6 Cancel and Escape do not submit and restore focus to Approve', async ({ page }) => {
    await mockContractReview(page)
    const submissions: unknown[] = []
    await page.route('**/contract/submit', async (route) => {
      submissions.push(route.request().postDataJSON())
      await json(route, { did: contractDid })
    })
    await gotoReview(page)
    const approve = page.getByRole('button', { name: 'Approve', exact: true })

    await approve.click()
    await localPrecheckDialog(page).getByRole('button', { name: 'Cancel', exact: true }).click()
    await expect(approve).toBeFocused()
    expect(submissions).toHaveLength(0)

    await approve.click()
    await page.keyboard.press('Escape')
    await expect(approve).toBeFocused()
    expect(submissions).toHaveLength(0)
  })

  test('AC7 local verification failure is visible, guarded while busy, and retryable', async ({ page }) => {
    await mockContractReview(page, { pattern: '[' })
    await gotoReview(page)

    const approve = page.getByRole('button', { name: 'Approve', exact: true })
    await approve.dblclick()
    const dialog = localPrecheckDialog(page)
    await expect(dialog.getByRole('alert')).toContainText(/verification|semantic/i)
    await expect(dialog.getByRole('button', { name: /confirm/i })).toHaveCount(0)
    await expect(dialog.getByRole('button', { name: /retry/i })).toBeEnabled()
  })

  test('AC7 submit failure is distinct from verification failure, keeps one request in flight, and allows retry', async ({
    page,
  }) => {
    await mockContractReview(page)
    let attempts = 0
    let concurrent = 0
    let maxConcurrent = 0
    await page.route('**/contract/submit', async (route) => {
      attempts += 1
      concurrent += 1
      maxConcurrent = Math.max(maxConcurrent, concurrent)
      await new Promise((resolve) => setTimeout(resolve, 150))
      concurrent -= 1
      await json(
        route,
        attempts === 1 ? { error: 'review submission unavailable' } : { did: contractDid },
        attempts === 1 ? 500 : 200,
      )
    })
    await gotoReview(page)

    await page.getByRole('button', { name: 'Approve', exact: true }).click()
    const dialog = localPrecheckDialog(page)
    const confirm = dialog.getByRole('button', { name: /confirm/i })
    await confirm.dblclick()

    await expect(dialog.getByRole('alert')).toContainText(/submit|submission/i)
    await expect(dialog.getByRole('alert')).not.toContainText(/verification.*failed/i)
    expect(attempts).toBe(1)
    expect(maxConcurrent).toBe(1)

    await dialog.getByRole('button', { name: /retry/i }).click()
    await expect.poll(() => attempts).toBe(2)
    expect(maxConcurrent).toBe(1)
  })
})
