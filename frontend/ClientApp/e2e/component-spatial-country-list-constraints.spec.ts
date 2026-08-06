import { expect, test } from './dcs-test'
import { selectBilateralClauseRoles } from './lifecycle-helpers'
import type { Locator, Page } from '@playwright/test'
const COUNTRY_OPTIONS = ['Germany (DEU)', 'Austria (AUT)', 'Switzerland (CHE)', 'United States (USA)'] as const
const SPATIAL_OPTION_COUNT = 7

async function openSpatialConstraint(page: Page, loginAs: (role: 'Template Creator') => Promise<void>) {
  await loginAs('Template Creator')
  await page.goto('/ui/templates/new')
  await page.getByRole('button', { name: /Component/ }).click()
  await page.getByRole('tab', { name: /Clauses/ }).click()

  const editor = page.getByTestId('split-clause-editor')
  await expect(editor.getByText('Machine-readable meaning (ODRL)')).toBeVisible()
  await editor.getByRole('button', { name: '+ constraint' }).click()

  const row = editor.locator('.flex.flex-wrap.items-center.gap-1').last()
  await row.locator('select').nth(0).selectOption({ label: 'access region (spatial)' })
  return row
}
async function expectCountryOptions(valueSelect: Locator, includesPlaceholder = false) {
  const options = valueSelect.getByRole('option')
  if (includesPlaceholder) await expect(options.first()).toHaveText('choose value')
  for (const label of COUNTRY_OPTIONS)
    await expect(valueSelect.getByRole('option', { name: label, exact: true })).toHaveCount(1)
}

test('a Template Creator authors spatial country list constraints in a Component', async ({ page, loginAs }) => {
  page.setDefaultTimeout(15_000)

  let constraintRow: Locator
  await test.step('@REQ-component-spatial-country-list-constraints-AC1 opens the Component clause constraint editor', async () => {
    constraintRow = await openSpatialConstraint(page, loginAs)
  })

  await test.step('@REQ-component-spatial-country-list-constraints-AC2 spatial offers the ontology-backed countries and alpha-3 codes', async () => {
    await expectCountryOptions(constraintRow.locator('select').nth(3), true)
  })

  await test.step('@REQ-component-spatial-country-list-constraints-AC3 must equal uses a single-select country value', async () => {
    await constraintRow.locator('select').nth(1).selectOption('odrl:isAnyOf')
    await expect(constraintRow.getByTestId('constraint-value-multiselect').locator('summary')).toHaveText(
      'choose values',
    )

    await constraintRow.locator('select').nth(1).selectOption('odrl:eq')
    const valueSelect = constraintRow.locator('select').nth(3)
    await expect(valueSelect).not.toHaveAttribute('multiple', '')
    await valueSelect.selectOption({ label: 'Germany (DEU)' })
    await expect(valueSelect.locator('option:checked')).toHaveText('Germany (DEU)')
  })

  const assertCountryMultiSelect = async (
    operator: string,
    selectedLabels: [string, string],
    preservedLabel?: string,
  ) => {
    await constraintRow.locator('select').nth(1).selectOption(operator)
    const multiselect = constraintRow.getByTestId('constraint-value-multiselect')
    await expect(constraintRow.locator('select[multiple]')).toHaveCount(0)
    if ((await multiselect.getAttribute('open')) === null) await multiselect.locator('summary').click()
    await expect(multiselect.getByRole('checkbox')).toHaveCount(SPATIAL_OPTION_COUNT)
    if (preservedLabel) await expect(multiselect.getByRole('checkbox', { name: preservedLabel })).toBeChecked()
    for (const label of COUNTRY_OPTIONS) {
      const checkbox = multiselect.getByRole('checkbox', { name: label, exact: true })
      if (selectedLabels.includes(label)) await checkbox.check()
      else await checkbox.uncheck()
    }
    await expect(multiselect.locator('summary')).toHaveText('2 selected')
    return multiselect
  }

  await test.step('@REQ-component-spatial-country-list-constraints-AC4 must be one of selects multiple countries', async () => {
    const multiselect = await assertCountryMultiSelect('odrl:isAnyOf', ['Germany (DEU)', 'Austria (AUT)'])
    const austria = multiselect.getByRole('checkbox', { name: 'Austria (AUT)', exact: true })
    await austria.uncheck()
    await expect(multiselect.locator('summary')).toHaveText('1 selected')
    await multiselect.locator('summary').click()
    await multiselect.locator('summary').click()
    await expect(austria).not.toBeChecked()
    await expect(multiselect.getByRole('checkbox', { name: 'Germany (DEU)', exact: true })).toBeChecked()
  })

  await test.step('@REQ-component-spatial-country-list-constraints-AC5 must not be one of selects multiple countries', async () => {
    await assertCountryMultiSelect('odrl:isNoneOf', ['Switzerland (CHE)', 'United States (USA)'], 'Germany (DEU)')
  })

  await test.step('@REQ-component-spatial-country-list-constraints-AC6 must be all of selects multiple countries', async () => {
    await assertCountryMultiSelect('odrl:isAllOf', ['Germany (DEU)', 'Switzerland (CHE)'], 'Switzerland (CHE)')
  })

  await test.step('@REQ-component-spatial-country-list-constraints-AC7 the multiselect remains usable on a narrow viewport', async () => {
    await page.setViewportSize({ width: 375, height: 667 })
    const multiselect = constraintRow.getByTestId('constraint-value-multiselect')
    if ((await multiselect.getAttribute('open')) === null) await multiselect.locator('summary').click()
    const dropdownContent = multiselect.locator('.dropdown-content')
    const box = await dropdownContent.boundingBox()
    const styles = await dropdownContent.evaluate((element) => {
      const computed = getComputedStyle(element)
      return { maxHeight: computed.maxHeight, overflowY: computed.overflowY }
    })

    await expect(multiselect.locator('summary')).toBeVisible()
    await expect(multiselect.getByRole('checkbox', { name: 'Germany (DEU)', exact: true })).toBeVisible()
    expect(box).not.toBeNull()
    expect(box!.x).toBeGreaterThanOrEqual(0)
    expect(box!.x + box!.width).toBeLessThanOrEqual(375)
    expect(styles.overflowY).toBe('auto')
    expect(styles.maxHeight).not.toBe('none')
  })
})

test('catalog-backed purpose values are grouped and retain their concept IRIs', async ({ page, loginAs }) => {
  page.setDefaultTimeout(15_000)
  await loginAs('Template Creator')
  await page.goto('/ui/templates/new')
  await page.getByRole('button', { name: /Component/ }).click()
  await page
    .getByRole('group')
    .filter({ hasText: 'Global Name' })
    .getByRole('textbox')
    .fill(`Grouped purposes ${Date.now()}`)
  await page
    .getByRole('group')
    .filter({ hasText: 'Base Description' })
    .getByRole('textbox')
    .fill('Purpose catalog grouping fixture for the constraint-editor e2e.')
  await page.getByRole('tab', { name: /Clauses/ }).click()

  const editor = page.getByTestId('split-clause-editor')
  await editor.getByPlaceholder('Clause title').fill('Grouped purposes')
  await editor.locator('.clause-editor').first().click()
  await page.keyboard.type('Use is permitted for either selected purpose.')
  await editor.getByRole('button', { name: '+ constraint' }).click()

  const row = editor.locator('.flex.flex-wrap.items-center.gap-1').last()
  await row.locator('select').nth(0).selectOption('odrl:purpose')
  const multiselect = row.getByTestId('constraint-value-multiselect')
  const serviceOperations = multiselect.getByRole('checkbox', { name: 'Service Operations (OPERATIONS)' })
  const analyticsOperations = multiselect.getByRole('checkbox', { name: 'Operational Analytics (OPERATIONS)' })
  for (const operator of ['odrl:isAnyOf', 'odrl:isNoneOf', 'odrl:isAllOf']) {
    await row.locator('select').nth(1).selectOption(operator)
    if ((await multiselect.getAttribute('open')) === null) await multiselect.locator('summary').click()
    await expect(multiselect.getByRole('group', { name: 'Service Purposes' })).toBeVisible()
    await expect(multiselect.getByRole('group', { name: 'Data Use Purposes' })).toBeVisible()
    await expect(serviceOperations).toHaveValue('https://w3id.org/facis/dcs/taxonomy/v1#service-purpose-operations')
    await expect(analyticsOperations).toHaveValue('https://w3id.org/facis/dcs/taxonomy/v1#data-use-purpose-operations')
    await serviceOperations.check()
    await analyticsOperations.check()
    await expect(multiselect.locator('summary')).toHaveText('2 selected')
    if (operator === 'odrl:isAnyOf') {
      await multiselect.locator('summary').click()
      await multiselect.locator('summary').click()
      await expect(serviceOperations).toBeChecked()
      await expect(analyticsOperations).toBeChecked()
    }
    if (operator !== 'odrl:isAllOf') {
      await serviceOperations.uncheck()
      await analyticsOperations.uncheck()
    }
  }

  // A constraint only reaches the document inside a complete ODRL rule — the
  // builder emits nothing for a clause that names no rule type, exactly as the
  // other authoring specs select one before saving.
  const ruleSelect = (label: string) =>
    editor.locator('label.form-control').filter({ hasText: label }).locator('select')
  await ruleSelect('Rule').selectOption({ label: 'Permission: the assignee MAY' })
  await ruleSelect('Action').selectOption({ label: 'use' })
  await selectBilateralClauseRoles(editor)

  await editor.getByRole('button', { name: 'Add clause', exact: true }).click()
  const createdRequest = page.waitForRequest((request) => request.url().includes('/template/create'))
  const createdResponse = page.waitForResponse(
    (response) => response.url().includes('/template/create') && response.request().method() === 'POST',
  )
  await page.getByRole('button', { name: 'Create', exact: true }).click()
  const [request, response] = await Promise.all([createdRequest, createdResponse])
  const responseBody = await response.text()
  expect(response.ok(), responseBody).toBeTruthy()
  const { did } = JSON.parse(responseBody) as { did: string }
  expect(did).toBeTruthy()
  const document = request.postDataJSON().template_data as {
    'dcs:policies': {
      'odrl:permission'?: {
        'odrl:constraint'?: {
          'odrl:leftOperand': { '@id': string }
          'odrl:rightOperand': { '@id': string }[]
        }[]
      }[]
    }
  }
  const purpose = document['dcs:policies']['odrl:permission']?.[0]?.['odrl:constraint']?.find(
    (constraint) => constraint['odrl:leftOperand']['@id'] === 'odrl:purpose',
  )
  expect(purpose?.['odrl:rightOperand']).toEqual([
    { '@id': 'https://w3id.org/facis/dcs/taxonomy/v1#service-purpose-operations' },
    { '@id': 'https://w3id.org/facis/dcs/taxonomy/v1#data-use-purpose-operations' },
  ])

  await loginAs('Template Manager')
  await page.reload()
  const token = await page.evaluate(() => localStorage.getItem('access_token'))
  const verification = await page.request.post('/api/template/verify', {
    data: { did },
    headers: { Authorization: `Bearer ${token}` },
  })
  const verificationBody = await verification.text()
  expect(verification.ok(), verificationBody).toBeTruthy()
  expect(JSON.parse(verificationBody) as { did: string; findings: string[] }).toEqual({ did, findings: [] })
})

test('a newly registered catalog is discovered across separate ontology graphs', async ({ page, loginAs }) => {
  await loginAs('Template Manager')
  await page.goto('/ui/semantic-hub')
  const token = await page.evaluate(() => localStorage.getItem('access_token'))
  const headers = { Authorization: `Bearer ${token}` }
  const documents = [
    {
      name: 'e2e-pets-constraint',
      content: `@prefix dcs: <https://w3id.org/facis/dcs/ontology/v1#> .
@prefix odrl: <http://www.w3.org/ns/odrl/2/> .
@prefix pets: <https://example.org/e2e-pets#> .
pets:constraint a dcs:ValueConstraint ;
  dcs:odrlLeftOperand odrl:product ;
  dcs:valueCatalog pets:catalog .`,
    },
    {
      name: 'e2e-pets-scheme',
      content: `@prefix skos: <http://www.w3.org/2004/02/skos/core#> .
@prefix pets: <https://example.org/e2e-pets#> .
pets:catalog a skos:ConceptScheme ; skos:prefLabel "Pets"@en .`,
    },
    {
      name: 'e2e-pets-concepts',
      content: `@prefix skos: <http://www.w3.org/2004/02/skos/core#> .
@prefix pets: <https://example.org/e2e-pets#> .
pets:dog a skos:Concept ; skos:inScheme pets:catalog ; skos:notation "DOG" ; skos:prefLabel "Dogs"@en .
pets:cat a skos:Concept ; skos:inScheme pets:catalog ; skos:notation "CAT" ; skos:prefLabel "Cats"@en .
pets:bird a skos:Concept ; skos:inScheme pets:catalog ; skos:notation "BIRD" ; skos:prefLabel "Birds"@en .`,
    },
  ]
  for (const document of documents) {
    const response = await page.request.post('/api/semantic/schema/register', {
      data: {
        ...document,
        kind: 'ontology',
        media_type: 'text/turtle',
        activate: true,
      },
      headers,
    })
    expect(response.ok(), await response.text()).toBeTruthy()
  }

  await loginAs('Template Creator')
  await page.goto('/ui/templates/new')
  await page.getByRole('button', { name: /Component/ }).click()
  await page.getByRole('tab', { name: /Clauses/ }).click()
  const editor = page.getByTestId('split-clause-editor')
  await editor.getByRole('button', { name: '+ constraint' }).click()
  const row = editor.locator('.flex.flex-wrap.items-center.gap-1').last()
  await row.locator('select').nth(0).selectOption('odrl:product')
  await row.locator('select').nth(1).selectOption('odrl:isAnyOf')
  const multiselect = row.getByTestId('constraint-value-multiselect')
  await multiselect.locator('summary').click()
  const pets = multiselect.getByRole('group', { name: 'Pets', exact: true })
  await expect(pets.getByRole('checkbox')).toHaveCount(3)
  for (const label of ['Dogs (DOG)', 'Cats (CAT)', 'Birds (BIRD)']) {
    await expect(pets.getByRole('checkbox', { name: label, exact: true })).toBeVisible()
  }
})
