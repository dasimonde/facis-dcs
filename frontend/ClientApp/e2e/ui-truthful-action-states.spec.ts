import { expect, type Page, type Route, test } from '@playwright/test'

function jwt(roles: string[]): string {
  const encode = (value: object) => Buffer.from(JSON.stringify(value)).toString('base64url')
  return `${encode({ alg: 'none', typ: 'JWT' })}.${encode({
    sub: 'did:web:ui-state.example.test',
    exp: Math.floor(Date.now() / 1000) + 3600,
    roles,
    ext: { iss: 'did:web:example.test', roles },
  })}.unsigned`
}

async function authenticate(page: Page, roles: string[]): Promise<void> {
  const accessToken = jwt(roles)
  await page.addInitScript(
    ([token]) => {
      localStorage.setItem('token_type', 'Bearer')
      localStorage.setItem('access_token', token)
    },
    [accessToken],
  )
  await page.route('**/auth/refresh', (route) => json(route, { token_type: 'Bearer', access_token: accessToken }))
  await page.route('**/semantic/schema/list', (route) => json(route, []))
  await page.route('**/semantic/ontology/dcs-odrl-profile', (route) => json(route, { content: '' }))
}

async function json(route: Route, body: unknown, status = 200): Promise<void> {
  await route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })
}

test('Non-compliance monitoring renders mutually exclusive idle, loading, error, clean, and filtered-empty states', async ({
  page,
}) => {
  await authenticate(page, ['Compliance Officer'])
  let monitor: Route | undefined
  await page.route('**/pac/monitor', (route) => {
    monitor = route
  })
  await page.goto('/ui/non-compliance')
  await expect(page.getByTestId('monitor-idle-state')).toBeVisible()
  await expect(page.getByTestId('monitor-empty-state')).toHaveCount(0)

  await page.getByTestId('run-monitoring-sweep').click()
  await expect(page.getByRole('status', { name: /running monitoring sweep/i })).toBeVisible()
  await json(monitor!, { message: 'monitor unavailable' }, 500)
  await expect(page.getByRole('alert')).toContainText(/monitor unavailable/i)
  await expect(page.getByTestId('monitor-empty-state')).toHaveCount(0)

  await page.unroute('**/pac/monitor')
  await page.route('**/pac/monitor', (route) => json(route, { checked_at: '2026-07-30T08:00:00Z', risks: [] }))
  await page.getByTestId('run-monitoring-sweep').click()
  await expect(page.getByTestId('monitor-empty-state')).toContainText(/completed.*no compliance risks/i)

  await page.unroute('**/pac/monitor')
  await page.route('**/pac/monitor', (route) =>
    json(route, {
      checked_at: '2026-07-30T08:01:00Z',
      risks: [
        {
          did: 'did:web:example.test:contract-1',
          risk_type: 'MISSING_APPROVAL',
          detail: 'Approval is missing',
          detected_at: '2026-07-30T08:01:00Z',
        },
      ],
    }),
  )
  await page.getByTestId('run-monitoring-sweep').click()
  await page.getByTestId('monitor-search').fill('not-present')
  await expect(page.getByTestId('monitor-empty-state')).toContainText(/match the current filter/i)
})

test('Compliance list/detail never claim empty before loading and export is gated by a successful current view', async ({
  page,
}) => {
  await authenticate(page, ['Contract Manager', 'Auditor'])
  let contractsRequest: Route | undefined
  await page.route('**/signature/retrieve', (route) => {
    contractsRequest = route
  })
  await page.goto('/ui/compliance')
  await expect(page.getByRole('status')).toContainText('Loading contracts')
  await expect(page.getByText(/No signed contracts|No contracts match/)).toHaveCount(0)
  await json(contractsRequest!, {
    contracts: [{ did: 'did:web:example.test:contract-1', name: 'Contract One', state: 'SIGNED' }],
    signing_tasks: [],
  })

  let viewRequest: Route | undefined
  await page.route('**/signature/view**', (route) => {
    viewRequest = route
  })
  await page.getByRole('button', { name: /Contract One/ }).click()
  await expect(page.getByRole('status')).toContainText('Loading signature data')
  await expect(page.getByText('No integrity findings recorded.')).toHaveCount(0)
  const exports = page.getByRole('button', { name: /Export (JSON|PDF)/ })
  for (let index = 0; index < (await exports.count()); index += 1) await expect(exports.nth(index)).toBeDisabled()

  await json(viewRequest!, { message: 'view unavailable' }, 500)
  await expect(page.getByRole('alert')).toContainText(/view unavailable/i)
  for (let index = 0; index < (await exports.count()); index += 1) await expect(exports.nth(index)).toBeDisabled()

  await page.unroute('**/signature/view**')
  let signatureStatus = 'ACTIVE'
  await page.route('**/signature/view**', (route) =>
    json(route, {
      contract_state: signatureStatus === 'ACTIVE' ? 'SIGNED' : 'REVOKED',
      integrity_findings: [],
      signatures: [
        {
          signer_did: 'did:web:example.test:signer-1',
          credential_type: 'AES',
          status: signatureStatus,
          signed_at: '2026-07-30T08:00:00Z',
          format: 'JAdES',
        },
      ],
    }),
  )
  await page.getByRole('button', { name: /Contract One/ }).click()
  for (let index = 0; index < (await exports.count()); index += 1) await expect(exports.nth(index)).toBeEnabled()

  let revokeRequests = 0
  let revokeRequest: Route | undefined
  await page.route('**/signature/revoke', (route) => {
    revokeRequests += 1
    revokeRequest = route
  })
  await page.getByRole('tab', { name: 'Revocation' }).click()
  const revokeButton = page.getByRole('button', { name: 'Revoke', exact: true })
  await revokeButton.click()
  const confirmation = page.getByRole('dialog', { name: 'Confirmation' })
  await confirmation.getByRole('button', { name: 'Cancel' }).click()
  expect(revokeRequests).toBe(0)
  await expect(revokeButton).toBeFocused()

  await revokeButton.click()
  await confirmation.getByPlaceholder('Reason for revocation').fill('  Signer credential compromised  ')
  await confirmation.getByRole('button', { name: 'Submit' }).click()
  await expect(page.getByRole('button', { name: 'Revoking…' })).toBeDisabled()
  for (let index = 0; index < (await exports.count()); index += 1) await expect(exports.nth(index)).toBeDisabled()
  expect(revokeRequests).toBe(1)
  expect(revokeRequest!.request().postDataJSON()).toMatchObject({ reason: 'Signer credential compromised' })
  signatureStatus = 'REVOKED'
  await json(revokeRequest!, {})
  await expect(page.getByText('REVOKED', { exact: true })).toBeVisible()
  expect(revokeRequests).toBe(1)
})

test('Template create trims required details, blocks the request, exposes inline errors, and focuses first invalid field', async ({
  page,
}) => {
  await authenticate(page, ['Template Creator'])
  let createRequests = 0
  await page.route('**/template/create', (route) => {
    createRequests += 1
    return json(route, {})
  })
  await page.goto('/ui/templates/new')
  await page.getByRole('button', { name: /^Contract/ }).click()
  await page.getByTestId('template-global-name').fill('   ')
  await page.getByTestId('template-base-description').fill('   ')
  await page.getByRole('button', { name: 'Create', exact: true }).click()

  await expect(page.getByTestId('template-global-name')).toBeFocused()
  await expect(page.getByTestId('template-global-name')).toHaveAttribute('aria-invalid', 'true')
  await expect(page.getByText('Global Name is required.')).toBeVisible()
  await expect(page.getByText('Base Description is required.')).toBeVisible()
  expect(createRequests).toBe(0)
})
