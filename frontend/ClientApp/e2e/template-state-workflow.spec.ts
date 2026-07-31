import { expect, test } from './dcs-test'

test('template state stays workflow-controlled in the builder', async ({ page, loginAs }) => {
  const did = 'did:web:example.com:workflow-controlled-state'
  const name = `Workflow-controlled state ${Date.now()}`
  let description = 'State changes only through workflow actions.'
  let templateData: unknown

  await loginAs('Template Manager')
  await page.route('**/template/create', async (route) => {
    const request = route.request().postDataJSON()
    templateData = request.template_data
    await route.fulfill({ json: { did } })
  })
  await page.route('**/template/retrieve**', async (route) => {
    if (!new URL(route.request().url()).pathname.endsWith(`/template/retrieve/${did}`)) {
      await route.fulfill({ json: { contract_templates: [], review_tasks: [], approval_tasks: [] } })
      return
    }
    await route.fulfill({
      json: {
        did,
        version: 1,
        state: 'DRAFT',
        template_type: 'COMPONENT',
        name,
        description,
        created_by: 'template-manager',
        created_at: '2026-07-24T08:00:00Z',
        updated_at: '2026-07-24T08:00:00Z',
        template_data: templateData,
      },
    })
  })
  await page.route('**/template/update_manage', async (route) => {
    const request = route.request().postDataJSON()
    description = request.description
    templateData = request.template_data
    await route.fulfill({ json: { did } })
  })
  await page.route('**/template/submit', async (route) => route.fulfill({ json: { did } }))

  await page.goto('/ui/templates/new')
  await page.getByRole('button', { name: /Component/ }).click()

  await expect(page.getByText('Template State', { exact: true })).toHaveCount(0)
  await expect(page.getByRole('group').filter({ hasText: 'Contract Type' })).toBeVisible()
  await page.getByRole('group').filter({ hasText: 'Global Name' }).getByRole('textbox').fill(name)
  await page.getByRole('group').filter({ hasText: 'Base Description' }).getByRole('textbox').fill(description)
  await page.getByRole('tab', { name: /Builder/ }).click()
  await page.getByRole('button', { name: 'Add block', exact: true }).click()
  await page.getByRole('dialog').getByRole('button', { name: 'Text', exact: true }).click()
  await page.getByPlaceholder('Text content').fill('Workflow-controlled template content.')
  await page.getByRole('button', { name: 'Confirm', exact: true }).click()

  const createRequest = page.waitForRequest(
    (request) => request.url().includes('/template/create') && request.method() === 'POST',
  )
  const createResponse = page.waitForResponse(
    (response) => response.url().includes('/template/create') && response.request().method() === 'POST',
  )
  await page.getByRole('button', { name: 'Create', exact: true }).click()
  const [request, response] = await Promise.all([createRequest, createResponse])
  expect(response.ok()).toBeTruthy()
  expect(request.postDataJSON()).not.toHaveProperty('state')

  await page.goto(`/ui/templates/view/${did}`)
  await expect(page.getByRole('group').filter({ hasText: 'Global Name' }).getByRole('textbox')).toHaveValue(name)
  await expect(page.locator('.card-title .badge-secondary').filter({ hasText: /^DRAFT$/ })).toBeVisible()
  await expect(page.getByText('This template is a draft', { exact: true })).toBeVisible()
  await expect(page.getByText('Template State', { exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Submit', exact: true })).toBeVisible()

  await page.goto(`/ui/templates/edit/${did}`)
  const descriptionField = page.getByRole('group').filter({ hasText: 'Base Description' }).getByRole('textbox')
  await expect(descriptionField).toHaveValue('State changes only through workflow actions.')
  await descriptionField.fill('Updated without changing workflow state.')
  const updateRequest = page.waitForRequest(
    (update) => update.url().includes('/template/update_manage') && update.method() === 'POST',
  )
  const updateResponse = page.waitForResponse(
    (update) => update.url().includes('/template/update_manage') && update.request().method() === 'POST',
  )
  await page.getByRole('button', { name: 'Update', exact: true }).click()
  expect((await updateRequest).postDataJSON()).not.toHaveProperty('state')
  expect((await updateResponse).ok()).toBeTruthy()

  await page.goto(`/ui/templates/view/${did}`)
  await expect(page.locator('.card-title .badge-secondary').filter({ hasText: /^DRAFT$/ })).toBeVisible()
  await expect(page.getByRole('group').filter({ hasText: 'Base Description' }).getByRole('textbox')).toHaveValue(
    'Updated without changing workflow state.',
  )
  const submitRequest = page.waitForRequest(
    (submit) => submit.url().includes('/template/submit') && submit.method() === 'POST',
  )
  await page.getByRole('button', { name: 'Submit', exact: true }).click()
  expect((await submitRequest).postDataJSON()).not.toHaveProperty('state')

  await page.goto('/ui/templates/new')
  await page.getByRole('button', { name: /parent for other contracts/ }).click()
  await expect(page.getByText('Template State', { exact: true })).toHaveCount(0)
  await expect(page.getByRole('group').filter({ hasText: 'Global Name' })).toBeVisible()
  await expect(page.getByRole('group').filter({ hasText: 'Base Description' })).toBeVisible()
})
