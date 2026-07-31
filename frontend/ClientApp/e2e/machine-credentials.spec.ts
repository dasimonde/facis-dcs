import { expect, test } from './dcs-test'
import { gotoAs } from './lifecycle-helpers'

/**
 * Machine credentials are issued through the UI and shown once (ADR-27).
 *
 * This is the part that used to live in a values file: a system user's secret
 * was configuration, so adding an integrator or rotating a credential meant a
 * redeploy. The test drives the administration screens by clicking them, then
 * checks the issued credential against the real token endpoint — a secret that
 * cannot be exchanged for a token would be a convincing-looking string and
 * nothing more.
 */
test('an administrator issues, uses and rotates a system user credential', async ({ page, loginAs }) => {
  test.setTimeout(5 * 60_000)

  const identityName = `E2E Integration ${Date.now()}`
  let clientId = ''
  let firstSecret = ''

  await test.step('the administrator registers a system user', async () => {
    await gotoAs(page, loginAs, 'Sys. Administrator', '/ui/admin/system-users')
    await expect(page.getByTestId('machine-identity-admin')).toBeVisible()

    await page.getByTestId('identity-name').fill(identityName)
    await page.getByTestId('identity-participant-did').fill('did:web:dcs-a.localhost%3A18080')
    await page.getByTestId('identity-description').fill('Registered by the machine credential e2e.')
    await page.getByTestId('identity-role-Sys. Contract Creator').check()

    const registered = page.waitForResponse(
      (r) => r.url().includes('/machine-identities') && r.request().method() === 'POST',
    )
    await page.getByTestId('identity-save').click()
    const response = await registered
    expect(response.ok(), `register identity ${response.status()}: ${await response.text()}`).toBeTruthy()
  })

  await test.step('the secret is shown once, with the warning that it will not be shown again', async () => {
    await expect(page.getByTestId('credential-dialog')).toBeVisible()
    await expect(page.getByTestId('credential-once-warning')).toContainText('shown once')

    clientId = (await page.getByTestId('credential-client-id').inputValue()).trim()
    firstSecret = (await page.getByTestId('credential-secret').inputValue()).trim()
    expect(clientId).not.toEqual('')
    expect(firstSecret.length).toBeGreaterThan(20)

    await page.getByTestId('credential-done').click()
    await expect(page.getByTestId('credential-dialog')).toBeHidden()
  })

  await test.step('the secret is not recoverable from the screen afterwards', async () => {
    // The row exists and names the client, but nothing anywhere shows the
    // secret again: DCS never stored it.
    const row = page.getByTestId('identity-row').filter({ hasText: identityName })
    await expect(row).toHaveCount(1)
    await expect(row).toContainText(clientId)
    await expect(page.locator('body')).not.toContainText(firstSecret)
  })

  await test.step('the issued credential really authenticates', async () => {
    const res = await page.request.post('/oauth2/token', {
      form: { grant_type: 'client_credentials', client_id: clientId, client_secret: firstSecret },
    })
    expect(res.ok(), `token ${res.status()}: ${await res.text()}`).toBeTruthy()
    expect((await res.json()).access_token).toBeTruthy()
  })

  await test.step('rotating issues a new secret and stops the old one working', async () => {
    const row = page.getByTestId('identity-row').filter({ hasText: identityName })
    const rotated = page.waitForResponse((r) => r.url().includes('/credential') && r.request().method() === 'POST')
    await row.getByTestId('identity-rotate').click()
    const response = await rotated
    expect(response.ok(), `rotate ${response.status()}`).toBeTruthy()

    await expect(page.getByTestId('credential-dialog')).toBeVisible()
    const secondSecret = (await page.getByTestId('credential-secret').inputValue()).trim()
    expect(secondSecret).not.toEqual(firstSecret)
    await page.getByTestId('credential-done').click()

    const stale = await page.request.post('/oauth2/token', {
      form: { grant_type: 'client_credentials', client_id: clientId, client_secret: firstSecret },
    })
    expect(stale.ok(), 'the rotated-away secret still authenticated').toBeFalsy()

    const fresh = await page.request.post('/oauth2/token', {
      form: { grant_type: 'client_credentials', client_id: clientId, client_secret: secondSecret },
    })
    expect(fresh.ok(), `new secret ${fresh.status()}`).toBeTruthy()
  })

  await test.step('disabling it refuses the credential at once', async () => {
    const row = page.getByTestId('identity-row').filter({ hasText: identityName })
    await row.getByTestId('identity-edit').click()
    await page.getByTestId('identity-enabled').uncheck()

    const saved = page.waitForResponse((r) => r.url().includes('/machine-identities') && r.request().method() === 'PUT')
    await page.getByTestId('identity-save').click()
    expect((await saved).ok()).toBeTruthy()

    await expect(
      page.getByTestId('identity-row').filter({ hasText: identityName }).getByTestId('identity-row-disabled'),
    ).toBeVisible()
  })
})

/**
 * A contract target proves WHICH target it is. Under one shared secret a
 * callback proved only that some target was calling, so any registered target
 * could acknowledge another's deployment.
 */
test('an administrator issues a callback credential to a target system', async ({ page, loginAs }) => {
  test.setTimeout(5 * 60_000)

  const targetName = `E2E Credentialed Target ${Date.now()}`

  await gotoAs(page, loginAs, 'Sys. Administrator', '/ui/admin/targets')
  await expect(page.getByTestId('target-admin')).toBeVisible()

  await page.getByTestId('target-name').fill(targetName)
  await page.getByTestId('target-url').fill('http://dcs-orce:1880/contract-target/deploy')
  await page.getByTestId('target-save').click()

  const row = page.getByTestId('target-row').filter({ hasText: targetName })
  await expect(row).toHaveCount(1)
  // A target starts with no credential, so it cannot acknowledge anything yet.
  await expect(row.getByTestId('target-row-no-credential')).toBeVisible()

  const issued = page.waitForResponse((r) => r.url().includes('/credential') && r.request().method() === 'POST')
  await row.getByTestId('target-issue-credential').click()
  expect((await issued).ok()).toBeTruthy()

  await expect(page.getByTestId('credential-dialog')).toBeVisible()
  const clientId = (await page.getByTestId('credential-client-id').inputValue()).trim()
  const secret = (await page.getByTestId('credential-secret').inputValue()).trim()
  await page.getByTestId('credential-done').click()

  const res = await page.request.post('/oauth2/token', {
    form: { grant_type: 'client_credentials', client_id: clientId, client_secret: secret },
  })
  expect(res.ok(), `target token ${res.status()}: ${await res.text()}`).toBeTruthy()

  await expect(
    page.getByTestId('target-row').filter({ hasText: targetName }).getByTestId('target-row-credentialed'),
  ).toBeVisible()
})
