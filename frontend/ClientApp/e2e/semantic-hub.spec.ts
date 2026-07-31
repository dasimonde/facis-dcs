import { expect, test } from './dcs-test'

/**
 * The Semantic Hub dashboard renders the running instance's registered
 * semantic artifacts — the genesis-seeded vocabulary a fresh instance
 * serves (context, SHACL shapes, validation profile, ontologies, clause
 * catalog), fetched live from /semantic/* with no hardcoded vocabulary in
 * the client.
 */

test('semantic hub dashboard lists the registered artifacts', async ({ page, loginAs }) => {
  await loginAs('Template Manager')

  const inventory = await page.request.get('/api/semantic/schema/list')
  expect(inventory.ok()).toBeTruthy()
  const entries = (await inventory.json()) as { kind: string; name: string }[]
  const names = new Set(entries.map((s) => s.name))
  for (const required of ['facis-dcs', 'facis-sla', 'dcs-odrl-profile']) {
    expect(names, `hub registers ${required}`).toContain(required)
  }

  await page.goto('/ui/semantic-hub')
  await expect(page.getByRole('heading', { name: 'Semantic Hub' })).toBeVisible()
  for (const name of names) {
    await expect(page.getByText(name, { exact: false }).first()).toBeVisible()
  }
})

test('clause catalog palette is served from the hub with enforceable shapes', async ({ page, loginAs }) => {
  await loginAs('Template Manager')

  const response = await page.request.get('/api/semantic/clauses')
  expect(response.ok()).toBeTruthy()
  const catalog = (await response.json()) as { clauses?: { type: string; label: string; shape: string }[] }

  expect(catalog.clauses?.length, 'hub serves at least one typed clause').toBeGreaterThan(0)
  for (const clause of catalog.clauses ?? []) {
    expect(clause.type, 'clause types are (compacted) IRIs').toContain(':')
    expect(clause.label, 'every palette entry is labeled').toBeTruthy()
    expect(clause.shape, 'every palette entry names its NodeShape IRI').toBeTruthy()
  }
})
