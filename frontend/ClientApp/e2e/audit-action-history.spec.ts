import { expect, type Page, type Route, test } from '@playwright/test'
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

function jwt(role: string): string {
  const encode = (value: object) => Buffer.from(JSON.stringify(value)).toString('base64url')
  return `${encode({ alg: 'none', typ: 'JWT' })}.${encode({
    sub: 'did:web:auditor.example',
    exp: Math.floor(Date.now() / 1000) + 3600,
    roles: [role],
    ext: { iss: 'did:web:example.test', roles: [role] },
  })}.unsigned`
}

async function json(route: Route, body: unknown, status = 200): Promise<void> {
  await route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })
}

async function authenticate(page: Page, role: string): Promise<void> {
  const accessToken = jwt(role)
  await page.route('**/auth/refresh', (route) => json(route, { token_type: 'Bearer', access_token: accessToken }))
  await page.route('**/semantic/schema/list', (route) => json(route, []))
  await page.route('**/semantic/ontology/dcs-odrl-profile', (route) => json(route, { content: odrlProfileFixture }))
  await withholdUnsignedToken(page)
  await page.addInitScript(
    ([accessToken]) => {
      localStorage.setItem('token_type', 'Bearer')
      localStorage.setItem('access_token', accessToken)
    },
    [accessToken],
  )
}

const contractDid = '3507fd12-5b05-475d-89b5-29cbde761e4e'
const templateDid = 'c51878c9-6d3d-4b78-b4cc-29dbd5c10169'

test.describe('Audit Action History', () => {
  test.use({ viewport: { width: 1280, height: 800 } })

  test('switches to Action History and loads contract activity with expandable JSON', async ({ page }) => {
    await authenticate(page, 'Auditor')
    await page.route('**/contract/audit**', (route) => {
      if (route.request().method() !== 'POST') return route.fallback()
      return json(route, [
        {
          id: 1,
          component: 'CONTRACT_WORKFLOW_ENGINE',
          event_type: 'CREATE_CONTRACT',
          created_at: '2026-10-12T14:05:28.000Z',
          did: contractDid,
          event_data: {
            occurred_at: '2026-10-12T14:05:28.000Z',
            created_by: 'did:web:localhost%3A8991',
            did: contractDid,
          },
        },
        {
          id: 2,
          component: 'CONTRACT_WORKFLOW_ENGINE',
          event_type: 'SUBMIT_CONTRACT',
          created_at: '2026-10-12T14:10:00.000Z',
          did: contractDid,
          event_data: {
            occurred_at: '2026-10-12T14:10:00.000Z',
            submitted_by: 'did:web:localhost%3A8991',
            previous_state: 'NEGOTIATION',
            new_state: 'SUBMITTED',
          },
        },
      ])
    })

    await page.goto('/ui/audit')
    await expect(page.getByRole('heading', { name: 'Audit', exact: true })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Execute Audit' })).toBeVisible()

    await page.getByRole('tab', { name: 'Action History', exact: true }).click()
    await expect(page.getByRole('button', { name: 'View activity history' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Execute Audit' })).toBeHidden()

    await page.locator('#select-action-history-type').selectOption('contract')
    await page.locator('#input-action-history-did').fill(contractDid)
    await page.locator('#input-action-history-justification').fill('Action history review')
    await page.getByRole('button', { name: 'View activity history' }).click()

    await expect(page.getByRole('heading', { name: 'Activity' })).toBeVisible()
    await expect(page.getByText('Contract created', { exact: true })).toBeVisible()
    await expect(page.getByText('By: did:web:localhost%3A8991').first()).toBeVisible()
    await expect(page.getByText('Submitted for review', { exact: true })).toBeVisible()

    await page.getByRole('button', { name: 'Show activity details' }).first().click()
    await expect(page.getByText('Event data (JSON)').first()).toBeVisible()
    await page.locator('summary').filter({ hasText: 'Event data (JSON)' }).first().click()
    await expect(page.getByText('"created_by"').first()).toBeVisible()

    expect(new URL(page.url()).search).toBe('')
  })

  test('loads template activity history', async ({ page }) => {
    await authenticate(page, 'Auditor')
    await page.route('**/template/audit**', (route) => {
      if (route.request().method() !== 'GET') return route.fallback()
      return json(route, [
        {
          id: 11,
          component: 'CONTRACT_TEMPLATE_REPO',
          event_type: 'CREATE_CONTRACT_TEMPLATE',
          created_at: '2026-10-12T12:00:00.000Z',
          did: templateDid,
          event_data: {
            created_by: 'did:web:localhost%3A8991',
            did: templateDid,
          },
        },
      ])
    })

    await page.goto('/ui/audit')
    await page.getByRole('tab', { name: 'Action History', exact: true }).click()
    await page.locator('#select-action-history-type').selectOption('template')
    await page.locator('#input-action-history-did').fill(templateDid)
    await page.locator('#input-action-history-justification').fill('Action history review')
    await page.getByRole('button', { name: 'View activity history' }).click()

    await expect(page.getByText('Template created', { exact: true })).toBeVisible()
    await expect(page.getByText('By: did:web:localhost%3A8991')).toBeVisible()
  })

  test('keeps Compliance Audit available as the default tab', async ({ page }) => {
    await authenticate(page, 'Auditor')
    await page.goto('/ui/audit')
    await expect(page.getByRole('tab', { name: 'Compliance Audit', exact: true })).toBeVisible()
    await expect(page.getByRole('tab', { name: 'Action History', exact: true })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Execute Audit' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'JSON', exact: true })).toBeVisible()
  })
})
