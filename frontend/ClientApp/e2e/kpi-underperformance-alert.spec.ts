import { expect, test } from './dcs-test'
import { buildApprovedContract, gotoAs, signApprovedContractViaViewer } from './lifecycle-helpers'
import type { APIRequestContext } from '@playwright/test'

/**
 * A breached KPI must reach the compliance officer's screen (DCS-FR-CWE-31:
 * "Alerts MUST be raised for underperformance or missed targets").
 *
 * The backend path is already covered by a BDD scenario. What that scenario
 * cannot show is that an officer can actually get to the alert: this drives the
 * whole chain from the officer's and manager's seats, clicking the real
 * controls, and ends on the rendered alert badge rather than on a JSON field.
 *
 * Deployment happens twice here, which is the SRS's own arrangement rather than
 * a quirk of the fixture: DCS-FR-SM-12 automatically triggers deployment when
 * signing completes, and UC-05's stimulus is separately "a Contract Manager
 * submits a signed contract for deployment". So the contract reaches ACTIVE on
 * its own, and the manager then re-dispatches it by hand — the recovery an
 * operator needs when a target was unreachable, lost its state, or was replaced.
 * The state machine treats that as an idempotent ACTIVE -> ACTIVE re-dispatch.
 *
 * Where it deploys to is chosen through the UI as well: an administrator
 * registers a target system, and the manager points the contract at it before
 * signing (ADR-25). Nothing is seeded behind the UI's back.
 *
 * The one step that is not a click is the KPI report: the reporter is the
 * contract *target system*, an external party posting over the deployment
 * callback channel with its own credential, quoting the correlation id the
 * manager's deployment returned. No DCS user performs it and no control exists
 * to click.
 *
 * Every action a DCS *user* performs — author, submit, review, approve, sign,
 * deploy, sweep — goes through the real controls as the role that owns it.
 *
 * The threshold is the one buildApprovedContract already authors through the
 * clause editor: an ODRL constraint binding the Payment Amount contract field
 * with "less than or equal to" 500. The target system is what executes and
 * observes the contract, so it is the target that concludes 900 breaches that
 * rule and says so — verdict plus the rule's @id, quoted back from the
 * odrl:policy the deployment envelope handed it (ADR-33). The DCS records the
 * verdict, attributes it to that rule, and alerts on it; it derives none of it.
 * Nothing here is seeded behind the UI's back.
 */

// The target authenticates its callback as its own registered client (ADR-27),
// so the suite obtains a token the same way a real target system would.
const TARGET_CLIENT_ID = process.env.E2E_TARGET_CLIENT_ID ?? 'dcs-orce-target'
const TARGET_CLIENT_SECRET = process.env.E2E_TARGET_CLIENT_SECRET ?? 'dcs-orce-target-secret'

const targetAccessToken = async (request: APIRequestContext) => {
  const res = await request.post('/oauth2/token', {
    form: {
      grant_type: 'client_credentials',
      client_id: TARGET_CLIENT_ID,
      client_secret: TARGET_CLIENT_SECRET,
    },
  })
  expect(res.ok(), `target token ${res.status()}: ${await res.text()}`).toBeTruthy()
  return (await res.json()).access_token as string
}

/** The shipped ORCE contract-target flow the kind stack runs (values.bdd.yml).
 *  The flow holds ONE credential, issued to the seeded registry entry below, and
 *  a callback is accepted only from the target its deployment went to (ADR-27).
 *  So the contract must be deployed to that entry: a target registered here
 *  would have no credential, and its acknowledgement would be refused —
 *  correctly, but the deployment would never reach ACTIVE. Registering a target
 *  through the UI is covered by machine-credentials.spec.ts. */
const SEEDED_TARGET_NAME = process.env.E2E_CONTRACT_TARGET_NAME ?? 'BDD Contract Target'

type JsonLdNode = Record<string, unknown>

const ODRL_RULE_BUCKETS = ['odrl:permission', 'odrl:prohibition', 'odrl:obligation']

/** The @id of the rule in the deployed odrl:policy whose constraint names
 *  `fieldIri` as its odrl:leftOperand. A verdict names one term of the signed
 *  contract, and this is how the target picks the term its measurement is
 *  about — the @id travelled to it verbatim in the deployment envelope. */
const ruleConstrainingField = (policy: JsonLdNode, fieldIri: string): string => {
  const constrains = (node: unknown): boolean => {
    if (Array.isArray(node)) return node.some(constrains)
    if (node !== null && typeof node === 'object') {
      const left = (node as JsonLdNode)['odrl:leftOperand'] as JsonLdNode | undefined
      if (left?.['@id'] === fieldIri) return true
      return Object.values(node as JsonLdNode).some(constrains)
    }
    return false
  }

  for (const bucket of ODRL_RULE_BUCKETS) {
    const rules = policy[bucket]
    const nodes = (Array.isArray(rules) ? rules : rules ? [rules] : []) as JsonLdNode[]
    for (const rule of nodes) {
      if (!constrains(rule['odrl:constraint'])) continue
      const ruleId = rule['@id']
      expect(typeof ruleId, 'the deployed rule carries the @id a verdict has to name').toBe('string')
      return ruleId as string
    }
  }
  throw new Error(`no deployed ODRL rule constrains ${fieldIri}: ${JSON.stringify(policy)}`)
}

test('@DCS-FR-CWE-31 @DCS-IR-PACM-03 a breached KPI raises an underperformance alert the compliance officer can see', async ({
  page,
  loginAs,
}) => {
  // The fixture drives template authoring, negotiation, review, approval and
  // signing through the UI; each leg is slow and this test owns all of them.
  test.setTimeout(20 * 60_000)

  const targetName = SEEDED_TARGET_NAME

  await test.step('the target system the ORCE flow answers for is registered and credentialed', async () => {
    await gotoAs(page, loginAs, 'Sys. Administrator', '/ui/admin/targets')
    await expect(page.getByTestId('target-admin')).toBeVisible()

    // It verifies the payload hash and acknowledges over the callback channel,
    // which is what drives SIGNED -> ACTIVE.
    const row = page.getByTestId('target-row').filter({ hasText: targetName })
    await expect(row, `the seeded target ${targetName} is registered`).toHaveCount(1)
    await expect(
      row.getByTestId('target-row-credentialed'),
      'it holds the credential the flow authenticates with',
    ).toBeVisible()
  })

  const contractDid = await test.step('author and approve a contract whose ODRL policy bounds a numeric field', () =>
    buildApprovedContract(page, loginAs))

  await test.step('the contract manager points it at that target system', async () => {
    // Before signing, deliberately: signing completion deploys the contract
    // automatically with nobody present to choose a destination, so a contract
    // that reaches it without one does not deploy at all (ADR-25).
    await gotoAs(page, loginAs, 'Contract Manager', `/ui/contracts/view/${contractDid}`)
    await expect(page.getByTestId('contract-target-unset'), 'no target is set yet').toBeVisible()

    await page.getByTestId('contract-target-select').selectOption({ label: targetName })
    const designated = page.waitForResponse(
      (r) => r.url().includes('/contract/target/designate') && r.request().method() === 'POST',
    )
    await page.getByTestId('contract-target-save').click()
    const response = await designated
    expect(response.ok(), `designate ${response.status()}`).toBeTruthy()

    await expect(page.getByTestId('contract-target-name')).toContainText(targetName)
  })

  await test.step('sign it', () => signApprovedContractViaViewer(page, loginAs, contractDid))

  const boundFieldIri =
    await test.step('signing auto-deploys it and the target acknowledges, taking the contract ACTIVE', async () => {
      // Nothing is simulated: the auto-deploy subscriber dispatched to the
      // shipped ORCE contract-target flow, whose callback drives SIGNED ->
      // ACTIVE. Both the state and the identifier a KPI reports against are
      // read off the screen — the contract renders its machine-readable side
      // beside the document (UC-04), which is where the node IRIs live.
      await expect
        .poll(
          async () => {
            await gotoAs(page, loginAs, 'Contract Manager', `/ui/contracts/view/${contractDid}`)
            return (
              await page
                .getByTestId('contract-state-badge')
                .innerText()
                .catch(() => '')
            )
              .trim()
              .toUpperCase()
          },
          {
            message: 'the automatic deployment is acknowledged, driving SIGNED -> ACTIVE',
            timeout: 180_000,
            intervals: [5_000],
          },
        )
        .toBe('ACTIVE')

      // The machine-readable side sits beside the document it renders, so the
      // Content tab has to be open before it can be expanded.
      await page
        .getByRole('tab', { name: /content/i })
        .or(page.getByText('Contract Content', { exact: true }))
        .first()
        .click()
      await page.getByTestId('machine-readable-toggle').click()
      const identifiers = page.getByTestId('machine-readable-field-id')
      await expect(
        identifiers.first(),
        'the machine-readable view names the field a KPI can report against',
      ).toBeVisible()
      const iri = (await identifiers.first().innerText()).trim()
      expect(iri, 'a node IRI, not a label').toContain('#')
      return iri
    })

  const deployment = await test.step('the contract manager re-dispatches it to the target', async () => {
    // UC-05's stimulus is a Contract Manager submitting a contract for
    // deployment, and the state machine treats DEPLOY from ACTIVE as an
    // idempotent re-dispatch — the operator's recovery when a target has to
    // receive the contract again. Clicking it is what a manager would do, and
    // the response carries both the correlation id the target's callback must
    // quote and the rules it is handed to enforce.
    await gotoAs(page, loginAs, 'Contract Manager', `/ui/contracts/view/${contractDid}`)
    const deploy = page.getByTestId('deploy-contract')
    await expect(deploy, 'a manager can re-dispatch an active contract to its target').toBeVisible({
      timeout: 30_000,
    })
    await expect(deploy).toHaveText('Redeploy')

    // The click reloads the page on success (router.go(0)), which discards the
    // response body before it can be read off a waitForResponse handle. Route
    // the request instead: route.fetch() performs the real call and hands back
    // the real response, which is then passed through unchanged — the body is
    // captured on the way past rather than substituted.
    let captured: { status: number; text: string } | undefined
    await page.route('**/contract/deploy', async (route) => {
      const response = await route.fetch()
      const text = await response.text()
      captured = { status: response.status(), text }
      await route.fulfill({ response, body: text })
    })

    await deploy.click()
    await expect.poll(() => captured !== undefined, { message: 'the deployment request completes' }).toBe(true)
    await page.unroute('**/contract/deploy')

    expect(captured!.status, `redeploy ${captured!.status}: ${captured!.text}`).toBe(200)
    const body = JSON.parse(captured!.text) as {
      correlation_id?: string
      target_name?: string
      payload?: { 'odrl:policy'?: JsonLdNode }
    }
    expect(body.correlation_id, 'the deployment names the correlation id its callback quotes').toBeTruthy()
    expect(body.target_name, 'the deployment names the target system it went to').toBeTruthy()
    const policy = body.payload?.['odrl:policy']
    expect(policy, 'the envelope hands the target the rules it is to enforce').toBeTruthy()
    return {
      correlationId: body.correlation_id!,
      ruleId: ruleConstrainingField(policy!, boundFieldIri),
    }
  })

  await test.step('the target concludes the reported value breaches a rule and says which', async () => {
    // 900 against "Payment Amount <= 500" — the constraint authored in the
    // clause editor by the fixture. The classification is the target's, and it
    // names the rule it concluded about so the recorded row is traceable to
    // that term of the contract (ADR-33).
    const res = await page.request.post('/api/contract/deployment/callback', {
      headers: { Authorization: `Bearer ${await targetAccessToken(page.request)}` },
      data: {
        did: contractDid,
        correlation_id: deployment.correlationId,
        kpi: {
          metric: 'payment_amount',
          value: '900',
          verdict: 'violated',
          rule: deployment.ruleId,
        },
      },
    })
    expect(res.ok(), `KPI callback ${res.status()}: ${await res.text()}`).toBeTruthy()
  })

  await test.step('the manager sees the verdict recorded against the rule it names', async () => {
    await gotoAs(page, loginAs, 'Contract Manager', `/ui/contracts/view/${contractDid}`)
    const row = page.getByTestId('contract-kpi-row').filter({ hasText: 'payment_amount' })
    await expect(row, 'the reported KPI reaches the contract detail').toHaveCount(1, { timeout: 30_000 })
    await expect(row.getByTestId('contract-kpi-verdict'), "the target's own verdict is what is shown").toHaveText(
      'Violation',
    )
    await expect(
      row.getByTestId('contract-kpi-rule'),
      'the verdict is traceable to the exact rule inside the contract',
    ).toHaveText(deployment.ruleId)
  })

  await test.step('the compliance officer sweeps and is alerted', async () => {
    await gotoAs(page, loginAs, 'Compliance Officer', '/ui/non-compliance')

    const swept = page.waitForResponse((r) => r.url().includes('/pac/monitor') && r.ok())
    await page.getByTestId('run-monitoring-sweep').click()
    await swept

    // The sweep is system-wide and other suites drive contracts concurrently,
    // so narrow to this contract rather than assume it is the only finding.
    await page.getByTestId('monitor-search').fill(contractDid)

    const row = page.getByTestId('monitor-risk-row').filter({ hasText: contractDid })
    await expect(row, 'the breached contract is flagged').toHaveCount(1, { timeout: 30_000 })
    await expect(row.getByTestId('monitor-risk-type')).toContainText('CONTRACT_UNDERPERFORMANCE')
    await expect(
      row.getByTestId('monitor-risk-underperformance-badge'),
      'underperformance is highlighted as an alert, not listed as another grey row',
    ).toBeVisible()
    // The officer must be able to act on it: the detail names the observed
    // value and the rule the target concluded it breached, so the alert points
    // at a term of the contract rather than at "something is wrong".
    await expect(row.getByTestId('monitor-risk-detail')).toContainText('900')
    await expect(row.getByTestId('monitor-risk-detail')).toContainText(deployment.ruleId)
  })
})
