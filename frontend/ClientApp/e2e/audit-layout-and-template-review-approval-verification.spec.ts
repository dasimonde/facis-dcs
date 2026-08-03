import { expect, type Page, type Route, test } from '@playwright/test'

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

test.describe('audit layout at 1280px with expanded navigation', () => {
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
})
