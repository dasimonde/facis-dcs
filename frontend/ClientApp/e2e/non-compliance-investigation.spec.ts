import { expect, test } from './dcs-test'
import {
  buildContractPendingApproval,
  buildDraftContractFixture,
  type DraftContractFixture,
  gotoAs,
} from './lifecycle-helpers'

/**
 * Non-Compliance Investigation view (DCS-IR-PACM-03/-04, UC-08).
 *
 * ROUTE CONTRACT (does not exist yet — this is what the implementer builds):
 *   path: /non-compliance   (served under the SPA's /ui base, i.e. /ui/non-compliance)
 *   guarded to role COMPLIANCE_OFFICER only (see frontend/ClientApp/src/router/router.ts's
 *   role-guard convention: an authenticated user without the role in
 *   route.meta.roles is bounced to the FRONT_PAGE route, i.e. /ui/frontpage —
 *   the existing /compliance "Signature Compliance Viewer" route is a
 *   DIFFERENT view (validation/revocation of signed contracts) and must NOT
 *   be reused for this one).
 *
 * DATA-TESTID CONTRACT (selectors the implementer must render verbatim):
 *   run-monitoring-sweep    — button; triggers GET /pac/monitor
 *   monitor-search          — text input; client-side filter of the rendered
 *                             risk rows by contract DID substring (see the
 *                             design note on AC2 below for why this exists)
 *   monitor-risk-row        — one per risk in the (filtered) risks list
 *     monitor-risk-did          — nested: the affected contract DID
 *     monitor-risk-type         — nested: risk_type
 *     monitor-risk-detail       — nested: detail text
 *     monitor-risk-detected-at  — nested: detected_at timestamp
 *   monitor-empty-state     — shown instead of a blank area when the
 *                             (filtered) risks list is empty
 *   incident-form           — the incident-report form container
 *     incident-contract-did     — input: contract DID the finding is linked to
 *     incident-risk-type        — input: machine-readable risk type
 *     incident-detail            — textarea: human-readable finding detail
 *     incident-submit            — button; triggers POST /pac/report
 *   incident-success        — success confirmation shown after a 200 response
 *
 * DESIGN NOTE on AC2 ("no risks" empty state): GET /pac/monitor sweeps ALL
 * open approval tasks system-wide (querymonitor.go has no tenant/holder
 * scoping) — against the shared BDD/e2e backend, with other suites
 * concurrently driving contracts through review, a genuinely empty global
 * result cannot be guaranteed. Rather than mock the response (this suite
 * never mocks — every other e2e spec drives the real backend), AC2 is
 * exercised through the SAME "zero rows" render branch via monitor-search:
 * filtering to a contract DID that provably has no flagged risk collapses
 * the rendered list to zero exactly as a genuinely clean sweep would.
 */

test('@REQ-non-compliance-investigation-ui-AC1 @DCS-IR-PACM-03 @UC-08-02 compliance officer triggers a monitoring sweep and sees a detected risk', async ({
  // DCS-FR-UC-06-1
  page,
  loginAs,
}) => {
  test.setTimeout(60_000)

  const contractDid = await buildContractPendingApproval(page, loginAs)

  await gotoAs(page, loginAs, 'Compliance Officer', '/ui/non-compliance')

  const swept = page.waitForResponse(
    (r) => r.url().includes('/pac/monitor') && r.request().method() === 'GET' && r.ok(),
  )
  await page.getByTestId('run-monitoring-sweep').click()
  await swept

  // Scope down to the fixture built above — the sweep is global (see the
  // file-level design note), so other contracts may also be flagged.
  await page.getByTestId('monitor-search').fill(contractDid)

  const row = page.getByTestId('monitor-risk-row').filter({ hasText: contractDid })
  await expect(row).toBeVisible()
  await expect(row.getByTestId('monitor-risk-did')).toHaveText(contractDid)
  await expect(row.getByTestId('monitor-risk-type')).toHaveText('MISSING_APPROVAL')
  await expect(row.getByTestId('monitor-risk-detail')).not.toHaveText('')
  await expect(row.getByTestId('monitor-risk-detected-at')).toHaveText(/\d{4}-\d{2}-\d{2}/)
})

test('@REQ-non-compliance-investigation-ui-AC2 @DCS-IR-PACM-03 monitor view shows an explicit empty state instead of a blank area', async ({
  page,
  loginAs,
}) => {
  await gotoAs(page, loginAs, 'Compliance Officer', '/ui/non-compliance')

  const swept = page.waitForResponse(
    (r) => r.url().includes('/pac/monitor') && r.request().method() === 'GET' && r.ok(),
  )
  await page.getByTestId('run-monitoring-sweep').click()
  await swept

  // See the file-level design note: filter to a DID guaranteed to carry no
  // flagged risk, forcing the identical "zero rows" branch a clean global
  // sweep would render.
  await page.getByTestId('monitor-search').fill(`did:example:non-compliance-investigation-ui-empty-${Date.now()}`)

  await expect(page.getByTestId('monitor-empty-state')).toBeVisible()
  await expect(page.getByTestId('monitor-risk-row')).toHaveCount(0)
})

test('@REQ-non-compliance-investigation-ui-AC3 @DCS-IR-PACM-03 @DCS-IR-PACM-04 the view is reachable only for the Compliance Officer scope', async ({
  page,
  loginAs,
}) => {
  await test.step('Compliance Officer reaches the view', async () => {
    await gotoAs(page, loginAs, 'Compliance Officer', '/ui/non-compliance')
    await expect(page).toHaveURL(/\/ui\/non-compliance/)
    await expect(page.getByTestId('run-monitoring-sweep')).toBeVisible()
  })

  await test.step('a role outside the Compliance Officer scope is guard-redirected away', async () => {
    await gotoAs(page, loginAs, 'Contract Manager', '/ui/non-compliance')
    await expect(page).not.toHaveURL(/\/ui\/non-compliance/)
    await expect(page.getByTestId('run-monitoring-sweep')).not.toBeVisible()
  })
})

test.describe('non-compliance incident report submission', () => {
  let fixture: DraftContractFixture

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(180_000)
    fixture = await buildDraftContractFixture(browser)
  })

  test('@REQ-non-compliance-investigation-ui-AC4 @DCS-IR-PACM-04 @UC-08-02 compliance officer submits an incident report and sees a success confirmation', async ({
    page,
    loginAs,
  }) => {
    await gotoAs(page, loginAs, 'Compliance Officer', '/ui/non-compliance')

    await page.getByTestId('incident-contract-did').fill(fixture.contractDid)
    await page.getByTestId('incident-risk-type').fill('MISSING_APPROVAL')
    await page.getByTestId('incident-detail').fill('Non-compliance investigation UI e2e finding.')

    const submitted = page.waitForResponse(
      (r) => r.url().includes('/pac/report') && r.request().method() === 'POST' && r.ok(),
    )
    await page.getByTestId('incident-submit').click()
    await submitted

    await expect(page.getByTestId('incident-success')).toBeVisible()
  })

  test('@REQ-non-compliance-investigation-ui-AC5 @DCS-IR-PACM-04 the submitted report links the finding typed to a contract DID', async ({
    // DCS-FR-PACM-05
    page,
    loginAs,
  }) => {
    await gotoAs(page, loginAs, 'Compliance Officer', '/ui/non-compliance')

    await page.getByTestId('incident-contract-did').fill(fixture.contractDid)
    await page.getByTestId('incident-risk-type').fill('MISSING_APPROVAL')
    await page.getByTestId('incident-detail').fill('Non-compliance investigation UI e2e typed-link finding.')

    const submitted = page.waitForResponse(
      (r) => r.url().includes('/pac/report') && r.request().method() === 'POST' && r.ok(),
    )
    await page.getByTestId('incident-submit').click()
    const resp = await submitted

    // Contract with the implementer's Goa payload (DCS-IR-PACM-04): the
    // report body must carry the typed contract_did/findings link the
    // backend behave scenario (features/08_audit_compliance/
    // process_audit_and_compliance.feature) asserts gets persisted.
    const payload = resp.request().postDataJSON() as {
      contract_did?: string
      findings?: { risk_type?: string; detail?: string }[]
    }
    expect(payload.contract_did).toBe(fixture.contractDid)
    expect(payload.findings?.length).toBeGreaterThan(0)
    expect(payload.findings?.[0]?.risk_type).toBe('MISSING_APPROVAL')
  })
})
