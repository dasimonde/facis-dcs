import { expect, test } from './dcs-test'

/**
 * The Semantic Hub is manageable through the UI (DCS-FR-TR-03, ADR-8): an
 * operator publishes a brand-new vocabulary entry (the Gaia-X case — an
 * external shapes graph enters the running instance without a rebuild) and
 * registers/activates new versions of existing entries, all from the
 * dashboard.
 */

const E2E_SHAPES_TTL = `@prefix sh: <http://www.w3.org/ns/shacl#> .
@prefix ex: <https://example.org/e2e#> .

ex:E2EPublishedShape
  a sh:NodeShape ;
  sh:targetClass ex:E2EThing ;
  sh:property [ sh:path ex:name ; sh:minCount 1 ] .
`

test('an operator publishes a new vocabulary entry through the hub UI', async ({ page, loginAs }) => {
  await loginAs('Template Manager')
  await page.goto('/ui/semantic-hub')
  await expect(page.getByRole('heading', { name: 'Semantic Hub' })).toBeVisible()

  const name = `e2e-published-${Date.now()}`
  await page.getByLabel('Entry name').fill(name)
  await page.getByLabel('Entry kind').selectOption('shapes')
  await page.getByLabel('Entry content').fill(E2E_SHAPES_TTL)
  await page.getByRole('button', { name: 'Publish entry' }).click()

  // The new entry appears in the inventory, selected, at version 1, active.
  await expect(page.getByRole('heading', { name })).toBeVisible()
  await expect(page.getByText('active').first()).toBeVisible()

  // And it resolves through the hub's public route — the published
  // vocabulary is live for validation and form generation immediately.
  const resolved = await page.request.get(`/api/semantic/shapes/${name}`)
  expect(resolved.ok()).toBeTruthy()
  expect(await resolved.text()).toContain('E2EPublishedShape')
})

test('an operator registers and activates a new version of an existing entry', async ({ page, loginAs }) => {
  await loginAs('Template Manager')
  await page.goto('/ui/semantic-hub')

  // Work on the facis-sla ontology entry.
  await page
    .getByRole('button', { name: /facis-sla/ })
    .first()
    .click()

  const current = await (await page.request.get('/api/semantic/ontology/facis-sla')).json()
  const nextContent = `${current.content}\n# e2e version bump ${Date.now()}\n`

  await page.getByPlaceholder(/Paste the new version/).fill(nextContent)
  await page.getByRole('button', { name: 'Register version' }).click()

  // The version list grows and the newest version is the active one.
  await expect(page.getByText(`v${current.version + 1}`).first()).toBeVisible()
  const after = await (await page.request.get('/api/semantic/ontology/facis-sla')).json()
  expect(after.version).toBe(current.version + 1)
  expect(after.content).toContain('e2e version bump')
})
