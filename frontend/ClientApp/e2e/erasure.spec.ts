import { expect, test } from './dcs-test'
import {
  acceptOfferOn,
  acceptOpenDecisionsOn,
  apiAuthHeaders,
  assertManifestChainGrew,
  assertReceivedInState,
  authorContractTemplate,
  authorSemanticComponent,
  counterOffer,
  createContractViaUi,
  fillContractAmountOn,
  type Instance,
  instanceA,
  offerToCounterparty,
  openInstanceB,
  publishShapeOnInstance,
  registerTemplateOn,
  resolveDidWeb,
  settleToApprovedOn,
  signOnInstance,
  stagedCounterOffer,
  submitReviewApproveTemplateOn,
} from './multi-dcs-helpers'
import { E2E_FRONTEND_ORIGIN } from '../playwright.config'

/**
 * GDPR erasure as key shredding, driven through the real UI on both federation
 * instances (ADR-28; DCS-NFR-COMP-03, DCS-NFR-SEC-13, DCS-IR-CSA-03).
 *
 * A drives a contract to SIGNED with B as counterparty (the ship carries the
 * wrapped CEK, the signing apply archives the contract), then A's Archive
 * Manager deletes the archive entry through the confirmation dialog — the
 * justification feeds shred_reason and the explicit irreversibility checkbox
 * must be ticked. Asserted afterwards: the archive entry disappears, the
 * AuditView erasure panel reports "Keys destroyed" with the peer confirmed on
 * A and the same end state on B (its API base is dcs-b.localhost), KEY_SHREDDED
 * audit events exist on both instances (HTTP read — reads may bypass the UI,
 * actions never do), the export attempt surfaces the defined "Content erased"
 * message instead of a crash, and the contract list still serves the metadata
 * (erasure destroys content keys, not Postgres-side metadata).
 */

test.describe.configure({ retries: 0 })

let bInstance: Awaited<ReturnType<typeof openInstanceB>> | undefined
test.afterEach(async () => {
  await bInstance?.context.close().catch(() => undefined)
  bInstance = undefined
})

/** Polls POST /pac/audit on an instance until a KEY_SHREDDED entry for the
 *  contract shows up (the outbox anchors asynchronously). */
async function expectKeyShredded(inst: Instance, contractDid: string): Promise<void> {
  const auth = await apiAuthHeaders(inst, 'Auditor', '/ui/audit')
  await expect
    .poll(
      async () => {
        const resp = await inst.page.request.post(`${inst.apiBase}/pac/audit`, {
          headers: auth,
          data: {
            scope: 'CONTRACT_STORAGE_ARCHIVE',
            did: contractDid,
            justification: 'Erasure E2E — KEY_SHREDDED evidence',
          },
        })
        if (!resp.ok()) return `HTTP ${resp.status()}`
        // POST /pac/audit returns one executor-run envelope, not a list of
        // per-scope trails; the DCS-procured entries live under `timeline`.
        const run = (await resp.json()) as { timeline?: { did?: string; event_type?: string }[] }
        const types = (run.timeline ?? []).filter((e) => e.did === contractDid)
        return types.some((e) => e.event_type === 'KEY_SHREDDED')
          ? 'found'
          : JSON.stringify(types.map((e) => e.event_type))
      },
      { timeout: 120_000, intervals: [3_000] },
    )
    .toBe('found')
}

/** Opens the AuditView pre-parameterized on the contract's archive scope and
 *  waits for the erasure panel to report the expected badge, reloading while
 *  the asynchronous shred/peer handshake settles. */
async function expectErasureBadge(inst: Instance, contractDid: string, badge: string): Promise<void> {
  await inst.gotoAs('Archive Manager', `/ui/audit?scope=archive&did=${encodeURIComponent(contractDid)}`)
  await expect(async () => {
    await inst.page.reload()
    await expect(inst.page.getByTestId('erasure-status')).toBeVisible({ timeout: 5_000 })
    await expect(inst.page.getByTestId('erasure-status-badge')).toHaveText(badge, { timeout: 5_000 })
  }).toPass({ timeout: 120_000 })
}

/** Export attempt on a shredded contract: the button must yield the defined
 *  "Content erased" message — a clean end state, not a crash. */
async function expectErasedExportMessage(inst: Instance, contractDid: string, contractName: string): Promise<void> {
  await inst.gotoAs('Contract Manager', `/ui/contracts/view/${contractDid}`)
  const exportAnswered = inst.page.waitForResponse((r) => r.url().includes(`/pdf/export/contract/`), {
    timeout: 120_000,
  })
  await inst.page.getByRole('button', { name: 'Export PDF' }).click()
  await exportAnswered
  await expect(inst.page.getByText('Content erased. Encryption keys destroyed.')).toBeVisible({ timeout: 30_000 })
  // The view survives the refusal: the contract's metadata keeps rendering.
  // It identifies the contract by name — the only DID on this view is the
  // template's — so the name is what proves the Postgres-side record survived
  // the shred.
  await expect(inst.page.getByRole('textbox', { name: 'Global Name' })).toHaveValue(contractName)
}

test('archive deletion shreds the encryption keys on both instances', async ({ page, context, browser }) => {
  test.setTimeout(750_000)
  const a = instanceA(page, context, E2E_FRONTEND_ORIGIN)
  const b = await openInstanceB(browser)
  bInstance = b

  const unique = Date.now()
  const shapeName = `e2e-erasure-shape-${unique}`
  const componentName = `Erasure Component ${unique}`
  const templateName = `Erasure Contract ${unique}`

  let contractDid = ''
  let bDidWeb = ''

  await test.step('setup: A drives a contract to SIGNED with B as counterparty (archived, CEK shipped)', async () => {
    await publishShapeOnInstance(a, shapeName)
    const componentDid = await authorSemanticComponent(a, componentName)
    await submitReviewApproveTemplateOn(a, componentDid, componentName)
    const templateDid = await authorContractTemplate(a, templateName, componentName)
    await submitReviewApproveTemplateOn(a, templateDid, templateName)
    await registerTemplateOn(a, templateDid, templateName)

    bDidWeb = await resolveDidWeb(b)
    contractDid = await createContractViaUi(a, templateName, bDidWeb)
    await fillContractAmountOn(a, contractDid, '20000')
    await offerToCounterparty(a, contractDid)
    await assertReceivedInState(b, contractDid, 'OFFERED')

    // A full negotiation round, as in full-vertical-2dcs: settling is mutual,
    // so neither side can consolidate straight from OFFERED. Each redline value
    // must differ from the last, or the editor stays undirty and the staged
    // draft never saves. Each redline also ships a new PDF, so the chain-growth
    // polls double as the replication barrier between the two sides.
    let aChain = await assertManifestChainGrew(a, contractDid, 0)
    let bChain = await assertManifestChainGrew(b, contractDid, 0)

    // B enters the round before it can redline it: receiving the offer queues
    // nothing, so B holds no negotiation task and its Negotiations tab — the
    // route stagedCounterOffer takes into the negotiate view — has no row yet.
    // Accepting the offer is what mints it.
    await acceptOfferOn(b, contractDid)

    await stagedCounterOffer(b, contractDid, { value: '10000' })
    bChain = await assertManifestChainGrew(b, contractDid, bChain)
    aChain = await assertManifestChainGrew(a, contractDid, aChain)

    // A answers with its own redline rather than accepting B's: Submit renders
    // for a party that has acted in the round, not for one that has only
    // received a proposal.
    await counterOffer(a, contractDid, { value: '15000' })
    await assertManifestChainGrew(a, contractDid, aChain)
    await assertManifestChainGrew(b, contractDid, bChain)

    // A moved last, so the outstanding decision is B's to settle (FR-CWE-07
    // refuses an accept by the record's own author).
    await acceptOpenDecisionsOn(b, contractDid)

    await settleToApprovedOn(a, contractDid)
    await settleToApprovedOn(b, contractDid)
    await signOnInstance(a, contractDid, 'Erasure Signatory')
    await assertReceivedInState(a, contractDid, 'SIGNED')
  })

  await test.step('A: Archive Manager deletes the entry through the irreversibility dialog [DCS-IR-CSA-03]', async () => {
    await a.gotoAs('Archive Manager', '/ui/archive/dashboard')
    // The archive entry is written asynchronously at SIGNED — reload until the
    // dashboard lists it.
    await expect(async () => {
      await a.page.reload()
      await expect(a.page.getByTestId('archive-entries')).toBeVisible({ timeout: 5_000 })
      await expect(a.page.getByTestId(`archive-entry-${contractDid}`)).toBeVisible({ timeout: 5_000 })
    }).toPass({ timeout: 120_000 })

    await expect(a.page.getByTestId(`erasure-badge-${contractDid}`)).toHaveText(/Keys live/)

    await a.page.getByTestId(`archive-delete-${contractDid}`).click()
    const dialog = a.page.getByRole('dialog').filter({ hasText: 'Confirmation' })
    await expect(dialog).toBeVisible()
    // The dialog names the irreversible consequence and refuses to submit
    // until it is acknowledged and a justification is given.
    await expect(dialog.getByText('Encryption keys will be destroyed on both instances (irreversible)')).toBeVisible()
    await expect(a.page.getByTestId('confirmation-confirm')).toBeDisabled()
    await dialog.locator('textarea[id^="v-"]').fill('GDPR Art. 17 erasure request (E2E)')
    await a.page.getByTestId('confirmation-acknowledgement-checkbox').check()
    const deleted = a.page.waitForResponse(
      (r) => r.url().includes('/archive/delete') && r.request().method() === 'DELETE' && r.ok(),
    )
    await a.page.getByTestId('confirmation-confirm').click()
    await deleted
    await expect(a.page.getByText('Archive entry deleted. Encryption keys destroyed.')).toBeVisible()

    // The soft-deleted entry leaves the dashboard list; the authoritative
    // status surface from here on is the AuditView erasure panel.
    await expect(a.page.getByTestId(`archive-entry-${contractDid}`)).toHaveCount(0)
  })

  await test.step('A: erasure panel reports Keys destroyed with the peer confirmed', async () => {
    await expectErasureBadge(a, contractDid, 'Keys destroyed')
    await expect(async () => {
      await a.gotoAs('Archive Manager', `/ui/audit?scope=archive&did=${encodeURIComponent(contractDid)}`)
      // The peer table sits inside the collapsed "Erasure details" disclosure:
      // its rows are in the DOM but hidden until the summary is opened.
      await a.page.getByText('Erasure details', { exact: true }).click()
      const peerRow = a.page.getByTestId(`erasure-peer-${bDidWeb}`)
      await expect(peerRow).toBeVisible({ timeout: 5_000 })
      await expect(peerRow).toContainText('confirmed', { timeout: 5_000 })
    }).toPass({ timeout: 120_000 })
  })

  await test.step('B: the same end state on the counterparty', async () => {
    await expectErasureBadge(b, contractDid, 'Keys destroyed')
  })

  await test.step('KEY_SHREDDED audit events exist on BOTH instances [DCS-NFR-SEC-13]', async () => {
    await expectKeyShredded(a, contractDid)
    await expectKeyShredded(b, contractDid)
  })

  await test.step('export attempts surface "Content erased" on both instances — no crash', async () => {
    await expectErasedExportMessage(a, contractDid, templateName)
    await expectErasedExportMessage(b, contractDid, templateName)
  })

  await test.step('the contract list still serves the metadata (erasure ≠ row deletion)', async () => {
    await a.gotoAs('Contract Manager', '/ui/contracts')
    // The list keys rows by DID but renders a Name column, so the name is the
    // surviving metadata that is actually on screen.
    await expect(a.page.getByText(templateName).first()).toBeVisible({ timeout: 30_000 })
  })
})
