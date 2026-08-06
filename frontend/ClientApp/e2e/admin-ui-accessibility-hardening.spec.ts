import { expect, type Locator, type Page, type Route, test } from '@playwright/test'

const target = {
  id: 'target-1',
  name: 'Archive gateway',
  url: `https://archive.example.test/${'long-segment/'.repeat(18)}`,
  description: 'Receives completed contracts.',
  enabled: true,
  oauth_client_id: 'target-client',
}

const identity = {
  id: 'identity-1',
  name: 'ERP integration',
  participant_did: 'did:web:erp.example.test',
  description: 'Creates contracts from the ERP.',
  roles: ['Sys. Contract Creator'],
  enabled: true,
  oauth_client_id: 'erp-client',
  secret_issued_at: '2026-07-29T08:00:00Z',
}

const key = {
  label: 'dcs-c2pa',
  purpose: `Content provenance ${'with a deliberately long purpose '.repeat(10)}`,
  active_version: 2,
  updated_at: '2026-07-29T08:00:00Z',
}

function jwt(): string {
  const encode = (value: object) => Buffer.from(JSON.stringify(value)).toString('base64url')
  const payload = {
    sub: 'did:web:admin.example.test',
    exp: Math.floor(Date.now() / 1000) + 3600,
    roles: ['Sys. Administrator'],
    ext: { iss: 'did:web:example.test', roles: ['Sys. Administrator'] },
  }
  return `${encode({ alg: 'none', typ: 'JWT' })}.${encode(payload)}.unsigned`
}

async function authenticate(page: Page): Promise<void> {
  const accessToken = jwt()
  await page.addInitScript(
    ([token]) => {
      localStorage.setItem('token_type', 'Bearer')
      localStorage.setItem('access_token', token)
    },
    [accessToken],
  )
}

async function json(route: Route, body: unknown, status = 200): Promise<void> {
  await route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })
}

async function mockAdminApis(page: Page): Promise<void> {
  await authenticate(page)
  const accessToken = jwt()
  await page.route('**/auth/refresh', (route) => json(route, { token_type: 'Bearer', access_token: accessToken }))
  await page.route('**/semantic/schema/list', (route) => json(route, []))
  await page.route('**/semantic/ontology/dcs-odrl-profile', (route) =>
    json(route, {
      content: `
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
      `,
    }),
  )
  await page.route('**/contract/targets', (route) => json(route, [target]))
  await page.route('**/machine-identities', (route) => json(route, { identities: [identity] }))
  await page.route('**/api/admin/hsm-keys', (route) => json(route, { keys: [key] }))
}

async function expectNoPageOverflow(page: Page): Promise<void> {
  const dimensions = await page.evaluate(() => ({
    body: document.body.scrollWidth,
    viewport: document.documentElement.clientWidth,
  }))
  expect(dimensions.body).toBeLessThanOrEqual(dimensions.viewport)
}

async function expectBinaryControl(page: Page, control: Locator): Promise<void> {
  await expect(control).toHaveAttribute('type', 'checkbox')
  await expect(control).toHaveClass(/\bcheckbox\b/)
  await expect(control).toHaveClass(/\bcheckbox-primary\b/)

  const initiallyChecked = await control.isChecked()
  await control.locator('xpath=ancestor::label[1]').click()
  await expect(control).toBeChecked({ checked: !initiallyChecked })
  await control.focus()
  await page.keyboard.press('Space')
  await expect(control).toBeChecked({ checked: initiallyChecked })
}

test.describe('admin UI accessibility hardening', () => {
  test.beforeEach(async ({ page }) => mockAdminApis(page))

  test('AC1 forms have vertical label spacing, full-width fields, and no page overflow at 375 and 1280', async ({
    page,
  }) => {
    for (const width of [375, 1280]) {
      await page.setViewportSize({ width, height: 800 })
      for (const fixture of [
        {
          path: '/ui/admin/targets',
          root: 'target-admin',
          inputs: ['target-name', 'target-url', 'target-description'],
        },
        {
          path: '/ui/admin/system-users',
          root: 'machine-identity-admin',
          inputs: ['identity-name', 'identity-participant-did', 'identity-description'],
        },
      ]) {
        await page.goto(fixture.path)
        await expect(page.getByTestId(fixture.root)).toBeVisible()
        for (const inputTestId of fixture.inputs) {
          const input = page.getByTestId(inputTestId)
          const label = input.locator('xpath=ancestor::label[1]')
          const labelText = label.locator('.label-text').first()
          const boxes = await Promise.all([label.boundingBox(), labelText.boundingBox(), input.boundingBox()])
          expect(boxes.every(Boolean)).toBeTruthy()
          expect(boxes[2]!.y - (boxes[1]!.y + boxes[1]!.height)).toBeGreaterThanOrEqual(4)
          expect(boxes[2]!.width).toBeGreaterThanOrEqual(boxes[0]!.width - 2)
        }
        await expectNoPageOverflow(page)
      }
    }
  })

  test('AC2 every binary admin control is a primary native checkbox operable by label and Space', async ({ page }) => {
    await page.goto('/ui/admin/targets')
    const acceptsDeployments = page.getByRole('checkbox', { name: 'Accepts deployments' })
    await expectBinaryControl(page, acceptsDeployments)

    await page.goto('/ui/admin/system-users')
    const roleControls = page.locator('fieldset').getByRole('checkbox')
    await expect(roleControls).toHaveCount(5)
    for (let index = 0; index < (await roleControls.count()); index += 1) {
      await expectBinaryControl(page, roleControls.nth(index))
    }

    await page.getByTestId('identity-edit').click()
    const enabled = page.getByRole('checkbox', { name: 'May call this deployment' })
    await expectBinaryControl(page, enabled)
  })

  test('AC3 each admin list distinguishes loading, empty, and data while wide tables scroll only locally', async ({
    page,
  }) => {
    const cases = [
      {
        path: '/ui/admin/targets',
        api: '**/contract/targets',
        empty: [],
        data: [target],
        row: 'target-row',
        emptyTestId: 'target-empty-state',
        errorTestId: 'target-admin-error',
      },
      {
        path: '/ui/admin/system-users',
        api: '**/machine-identities',
        empty: { identities: [] },
        data: { identities: [identity] },
        row: 'identity-row',
        emptyTestId: 'identity-empty-state',
        errorTestId: 'machine-identity-error',
      },
      {
        path: '/ui/admin/hsm-keys',
        api: '**/api/admin/hsm-keys',
        empty: { keys: [] },
        data: { keys: [key] },
        row: 'key-inventory-row',
        emptyTestId: 'key-inventory-empty-state',
        errorTestId: 'key-inventory-error',
      },
    ]

    for (const fixture of cases) {
      await page.unroute(fixture.api)
      let held: Route | undefined
      await page.route(fixture.api, (route) => {
        held = route
      })
      await page.goto(fixture.path, { waitUntil: 'domcontentloaded' })
      await expect(page.getByRole('status')).toHaveText(/Loading…/)
      expect(held).toBeDefined()
      await json(held!, fixture.empty)
      await expect(page.getByTestId(fixture.emptyTestId)).toHaveClass(/\balert\b/)

      await page.unroute(fixture.api)
      await page.route(fixture.api, (route) => json(route, { message: 'unavailable' }, 500))
      await page.reload()
      await expect(page.getByTestId(fixture.errorTestId)).toHaveAttribute('role', 'alert')

      await page.unroute(fixture.api)
      await page.route(fixture.api, (route) => json(route, fixture.data))
      await page.reload()
      const row = page.getByTestId(fixture.row)
      await expect(row).toHaveCount(1)
      const scroller = row.locator('xpath=ancestor::div[contains(@class,"overflow-x-auto")][1]')
      await expect(scroller).toHaveCount(1)
      await expectNoPageOverflow(page)
    }
  })

  test('AC4 Save is primary and its understandable pending state prevents duplicate requests', async ({ page }) => {
    const cases = [
      {
        path: '/ui/admin/targets',
        api: '**/contract/targets',
        list: [target],
        response: target,
        saveTestId: 'target-save',
        fill: async () => {
          await page.getByTestId('target-name').fill('New target')
          await page.getByTestId('target-url').fill('https://new.example.test/callback')
        },
        credentialDialog: false,
      },
      {
        path: '/ui/admin/system-users',
        api: '**/machine-identities',
        list: { identities: [identity] },
        response: {
          identity: { ...identity, name: 'New ERP' },
          credential: {
            client_id: 'erp-client',
            client_secret: 'shown-once-secret',
            token_url: 'https://dcs.example.test/oauth2/token',
          },
        },
        saveTestId: 'identity-save',
        fill: async () => {
          await page.getByTestId('identity-name').fill('New ERP')
          await page.getByTestId('identity-participant-did').fill('did:web:new-erp.example.test')
        },
        credentialDialog: true,
      },
    ]

    for (const fixture of cases) {
      let saveRequests = 0
      let finishSave = () => {}
      const release = new Promise<void>((resolve) => {
        finishSave = resolve
      })
      await page.unroute(fixture.api)
      await page.route(fixture.api, async (route) => {
        if (route.request().method() === 'POST') {
          saveRequests += 1
          await release
          return json(route, fixture.response)
        }
        return json(route, fixture.list)
      })
      await page.goto(fixture.path)
      await fixture.fill()
      const save = page.getByTestId(fixture.saveTestId)
      await expect(save).toHaveClass(/\bbtn-primary\b/)
      await save.dblclick()
      await expect(save).toBeDisabled()
      await expect(save).toHaveAccessibleName(/saving/i)
      await expect.poll(() => saveRequests).toBe(1)
      finishSave()
      if (fixture.credentialDialog) {
        await page.getByTestId('credential-done').click()
      }
      await expect(save).toBeEnabled()
    }
  })

  test('AC5 credential output is a named, described native dialog with focus containment and restoration', async ({
    page,
  }) => {
    await page.unroute('**/machine-identities')
    await page.route('**/machine-identities', async (route) => {
      if (route.request().method() === 'POST') {
        return json(route, {
          identity: { ...identity, name: 'New ERP' },
          credential: {
            client_id: 'erp-client',
            client_secret: 'shown-once-secret',
            token_url: 'https://dcs.example.test/oauth2/token',
          },
        })
      }
      return json(route, { identities: [identity] })
    })
    await page.goto('/ui/admin/system-users')
    await page.getByTestId('identity-name').fill('New ERP')
    await page.getByTestId('identity-participant-did').fill('did:web:new-erp.example.test')
    const trigger = page.getByTestId('identity-save')
    await trigger.click()

    const dialog = page.getByRole('dialog', { name: /credential for new erp/i })
    await expect(dialog).toBeVisible()
    await expect(dialog).toHaveAccessibleDescription(/shown once/i)
    await expect(dialog.getByTestId('credential-client-id')).toBeFocused()
    await page.keyboard.press('Shift+Tab')
    await expect(dialog.getByTestId('credential-done')).toBeFocused()
    await page.keyboard.press('Tab')
    await expect(dialog.getByTestId('credential-client-id')).toBeFocused()
    await page.keyboard.press('Escape')
    await expect(dialog).toBeHidden()
    await expect(trigger).toBeFocused()

    await page.getByTestId('identity-name').fill('New ERP')
    await page.getByTestId('identity-participant-did').fill('did:web:new-erp.example.test')
    await trigger.click()
    await expect(dialog).toBeVisible()
    await dialog.getByTestId('credential-done').click()
    await expect(dialog).toBeHidden()
    await expect(trigger).toBeFocused()

    await page.route('**/machine-identities/identity-1/credential', (route) =>
      json(route, {
        client_id: 'erp-client',
        client_secret: 'rotated-identity-secret',
        token_url: 'https://dcs.example.test/oauth2/token',
      }),
    )
    const rotateIdentity = page.getByTestId('identity-rotate')
    await rotateIdentity.click()
    const rotatedIdentityDialog = page.getByRole('dialog', { name: /new credential for erp integration/i })
    await expect(rotatedIdentityDialog).toBeVisible()
    await rotatedIdentityDialog.getByTestId('credential-done').click()
    await expect(rotatedIdentityDialog).toBeHidden()
    await expect(rotateIdentity).toBeFocused()

    await page.route('**/contract/targets/target-1/credential', (route) =>
      json(route, {
        client_id: 'target-client',
        client_secret: 'rotated-target-secret',
        token_url: 'https://dcs.example.test/oauth2/token',
      }),
    )
    await page.goto('/ui/admin/targets')
    const rotateTarget = page.getByTestId('target-issue-credential')
    await rotateTarget.click()
    const rotatedTargetDialog = page.getByRole('dialog', { name: /callback credential for archive gateway/i })
    await expect(rotatedTargetDialog).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(rotatedTargetDialog).toBeHidden()
    await expect(rotateTarget).toBeFocused()
  })

  test('AC6 removals require a named confirmation, cancel is inert, and pending prevents duplicates', async ({
    page,
  }) => {
    const cases = [
      {
        path: '/ui/admin/targets',
        // A target is deleted through the collection with the id in the body
        // (DELETE /contract/targets), not through a per-id path.
        api: '**/contract/targets',
        trigger: 'target-delete',
        name: /remove target system “archive gateway”/i,
      },
      {
        path: '/ui/admin/system-users',
        api: '**/machine-identities/identity-1',
        trigger: 'identity-delete',
        name: /remove system user “erp integration”/i,
      },
    ]

    for (const fixture of cases) {
      let requests = 0
      let releaseRequest = () => {}
      const release = new Promise<void>((resolve) => {
        releaseRequest = resolve
      })
      const holdDelete = async (route: Route) => {
        if (route.request().method() !== 'DELETE') return route.fallback()
        requests += 1
        await release
        return json(route, {})
      }
      await page.route(fixture.api, holdDelete)
      await page.goto(fixture.path)

      await page.getByTestId(fixture.trigger).click()
      const dialog = page.getByRole('dialog')
      await expect(dialog).toContainText(fixture.name)
      await dialog.getByRole('button', { name: 'Cancel' }).click()
      expect(requests).toBe(0)

      await page.getByTestId(fixture.trigger).click()
      await dialog.getByTestId('confirmation-confirm').dblclick()
      await expect(page.getByTestId(fixture.trigger)).toBeDisabled()
      await expect.poll(() => requests).toBe(1)
      releaseRequest()
      await expect(page.getByTestId(fixture.trigger)).toBeEnabled()
      await page.unroute(fixture.api, holdDelete)
    }
  })
})
