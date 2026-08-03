import { execFileSync } from 'node:child_process'
import fs from 'node:fs'
import { homedir, tmpdir } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { applySession, type DcsRole, expect, mintSession } from './dcs-test'
import {
  E2E_API_BASE,
  E2E_API_BASE_B,
  E2E_DSS_URL,
  E2E_FRONTEND_B_ORIGIN,
  E2E_ISSUER_BASE_URL,
} from '../playwright.config'
import { formatNumberInput } from '../src/modules/template-repository/utils/number-format'
import type { Browser, BrowserContext, Page, Response } from '@playwright/test'

const here = path.dirname(fileURLToPath(import.meta.url))
const repoRoot = path.resolve(here, '../../..')
const python = process.env.E2E_BDD_PYTHON ?? path.join(homedir(), '.dcs-bdd-venv', 'bin', 'python3')

/**
 * Where the vertical persists every hop's PDF and its embedded JSON-LD for human
 * supervision — sibling of the e2e dir, outside Playwright's test-results output
 * (which it wipes at run start), and uploaded whole by CI as vertical-pdf-artifacts.
 */
const artifactDir = path.resolve(here, '../vertical-artifacts')

/**
 * A single DCS instance the two-instance vertical drives from its own UI: its
 * browser context/page bound to that DCS's frontend origin, its API base, and a
 * per-navigation session minter. Hydra rotates refresh tokens single-use, so
 * each top-level navigation re-mints a fresh role session for that instance.
 */
export interface Instance {
  readonly page: Page
  readonly context: BrowserContext
  readonly origin: string
  readonly apiBase: string
  gotoAs(role: DcsRole, url: string): Promise<void>
}

function makeInstance(page: Page, context: BrowserContext, origin: string, apiBase: string): Instance {
  return {
    page,
    context,
    origin,
    apiBase,
    async gotoAs(role, url) {
      await applySession(context, page, origin, mintSession(role, apiBase))
      await page.goto(url)
      // Two instances mean two browser contexts, and Chromium throttles timers
      // in pages it considers hidden. The signing ceremony dialog advances on a
      // 2.5s setInterval poll, so a backgrounded instance stops progressing:
      // the wallet leg verifies server-side while the viewer never notices and
      // never fetches the to-be-signed document. Keep the instance we are
      // driving in the foreground.
      await page.bringToFront()
    },
  }
}

/** Wraps the test's own fixture page/context as instance A (the originator). */
export function instanceA(page: Page, context: BrowserContext, origin: string): Instance {
  return makeInstance(page, context, origin, E2E_API_BASE)
}

/** Opens a second browser context/page for instance B (the counterparty), on
 *  B's own frontend origin and API base — the DCS-to-DCS peer. */
export async function openInstanceB(browser: Browser): Promise<Instance> {
  const context = await browser.newContext({ baseURL: E2E_FRONTEND_B_ORIGIN })
  const page = await context.newPage()
  return makeInstance(page, context, E2E_FRONTEND_B_ORIGIN, E2E_API_BASE_B)
}

/** What one run of the Secure Contract Viewer's signing ceremony produced: the
 *  ceremony it started, the signature field it bound, and the /signature/prepare
 *  response the viewer received — asserted by the caller, because whether that
 *  response is a document or a refusal is the whole subject of some of them. */
interface PreparedCeremony {
  ceremonyId: string
  signField: string
  prepared: Response
}

/**
 * The DID's final segment: unique per resource and untouched by the router's
 * param encoding, so a list row can be picked out by its own View link's href
 * without depending on how "did:web:..." is escaped into a URL.
 */
function didTail(did: string): string {
  return did.split(':').pop()!
}

/**
 * Follows one resource's own View link in whatever list is on screen.
 *
 * Arriving this way is the whole point of the helpers below. A hop that
 * navigates straight to a URL reaches its page even when nothing in the product
 * links there — which is how a negotiate view no list could reach passed every
 * run of this vertical while a human clicking through could not get to it at
 * all. Each stage keeps at least one hop that has to be FOUND.
 */
async function followViewLinkFor(inst: Instance, did: string, where: string): Promise<void> {
  const row = inst.page.locator('.list-row').filter({ has: inst.page.locator(`a[href*="${didTail(did)}"]`) })
  await expect(row, `${where} on ${inst.origin} shows no row for ${did}`).toHaveCount(1, { timeout: 30_000 })
  await row.getByRole('link', { name: 'View', exact: true }).click()
}

/**
 * Opens a contract from the Contracts list as a human would: searches the list
 * for it by DID (the list's default search filter) and clicks the row's own View
 * action.
 *
 * Where that lands is the product's decision, not this helper's:
 * ContractListItem.resolveViewRouteName sends a contract to its task view when
 * the acting instance holds a task for it, and to the read-only view otherwise.
 * Callers assert the destination they expect, so a task that was never minted
 * shows up as a wrong landing page rather than passing silently.
 */
export async function openContractFromList(inst: Instance, role: DcsRole, contractDid: string): Promise<void> {
  await inst.gotoAs(role, '/ui/contracts')
  const search = inst.page.getByRole('combobox', { name: 'Search contracts' })
  await expect(search).toBeVisible({ timeout: 30_000 })
  // The list is paginated and sorted oldest-first, so the contract under test is
  // not reliably on the page a fresh visit shows; searching is how a human finds
  // it. Armed before the fill and awaited before the row lookup: the list filters
  // itself from the search response, and reading rows while it is still in flight
  // reports "not in the list" for a contract that is.
  const searched = inst.page.waitForResponse((r) => r.url().includes('/contract/search'), { timeout: 30_000 })
  await search.fill(contractDid)
  await search.press('Enter')
  await searched
  await followViewLinkFor(inst, contractDid, 'the contract list')
}

/**
 * Opens a contract from one of the task tabs by clicking the task's own row.
 * The tab is the discoverable route into work a party owes an answer on, so a
 * tab that never grew a row — the federated contract's Negotiations tab, before
 * accepting an offer minted anything — fails here.
 */
export async function openContractFromTaskTab(
  inst: Instance,
  role: DcsRole,
  tab: 'negotiations' | 'reviews' | 'approvals',
  contractDid: string,
): Promise<void> {
  await inst.gotoAs(role, `/ui/tasks/${tab}`)
  await followViewLinkFor(inst, contractDid, `the ${tab} task tab`)
}

/**
 * Drives one signing ceremony on an instance through the real Secure Contract
 * Viewer (ADR-12): open the contract from the signing list, verify it, run the
 * wallet PID+PoA ceremony (the wallet leg arrives over OpenID4VP direct_post
 * against this instance's API base), and wait for the viewer's own
 * /signature/prepare response.
 *
 * Stops there deliberately. Preparation is where the DCS decides whether this
 * contract may be signed at all, so the two callers below share every step up
 * to that answer and differ only in what they assert about it.
 */
async function runSigningCeremonyOn(inst: Instance, contractDid: string, signatory: string): Promise<PreparedCeremony> {
  await inst.gotoAs('Contract Signer', '/ui/signing')
  const row = inst.page.getByRole('row').filter({ hasText: contractDid })
  await expect(row).toBeVisible()
  await row.getByRole('link', { name: /Open/ }).click()
  await expect(inst.page).toHaveURL(/\/signing\/.+/)

  // The badge follows the VERDICT, not the call completing
  // (SecureContractViewerView.verify), so an absent badge is a failed integrity
  // check rather than a slow one, and step 3 stays closed behind it. Capture the
  // verdict the viewer read so the failure names the mismatch and its findings
  // instead of reporting a missing element for fifteen seconds.
  const verified = inst.page.waitForResponse((r) => r.url().includes('/signature/verify'), { timeout: 60_000 })
  await inst.page.getByRole('button', { name: 'Verify', exact: true }).click()
  const verifyResponse = await verified
  await expect(
    inst.page.getByText('Verified', { exact: true }),
    `the integrity check of ${contractDid} on ${inst.origin} did not pass, so the ceremony stays closed: HTTP ${verifyResponse.status()} ${await verifyResponse.text().catch(() => '')}`,
  ).toBeVisible()

  // Match ANY ceremony-start response, then assert: an r.ok() filter turns a
  // refusal into "no response at all", which has cost several runs already.
  const ceremonyStarted = inst.page.waitForResponse(
    (r) => r.url().includes('/signature/request') && r.request().method() === 'POST',
    { timeout: 30_000 },
  )
  // Take the to-be-signed PDF from the app's OWN prepare response rather than
  // the browser download event. The ceremony still runs entirely through the UI
  // — this only changes how the bytes are observed. The download event proved
  // unreliable here: /signature/prepare answered 200 with the full PDF and the
  // app called its download helper, yet no download ever fired. Reading the
  // response the app actually received is both faithful and deterministic.
  // Armed before the click because the document is only prepared once the wallet
  // leg completes, further down, after complete_signing_webhook.py runs.
  // Match ANY prepare response, not only an ok one: filtering on r.ok() made a
  // rejected prepare (422) indistinguishable from no prepare at all, so the
  // failure reported a missing response instead of the refusal it actually got.
  const preparedResponse = inst.page.waitForResponse((r) => r.url().includes('/signature/prepare'), {
    timeout: 180_000,
  })
  // What the VIEWER itself saw, so a stall reports whether its poll ran at all
  // and what it got, rather than only that no prepare arrived.
  const viewerCalls: string[] = []
  inst.page.on('response', (r) => {
    if (/\/signature\/(request|prepare)/.test(r.url()))
      viewerCalls.push(`${r.status()} ${r.request().method()} ${r.url().split('/api')[1] ?? r.url()}`)
  })
  const viewerErrors: string[] = []
  inst.page.on('console', (m) => {
    if (m.type() === 'error') viewerErrors.push(m.text().slice(0, 200))
  })
  inst.page.on('pageerror', (e) => viewerErrors.push(`pageerror: ${e.message.slice(0, 200)}`))

  await inst.page.getByRole('button', { name: /download document to sign/ }).click()
  const ceremonyResponse = await ceremonyStarted
  expect(
    ceremonyResponse.ok(),
    `start signing ceremony on ${inst.origin}: HTTP ${ceremonyResponse.status()} ${await ceremonyResponse.text().catch(() => '')}`,
  ).toBeTruthy()
  const ceremony = (await ceremonyResponse.json()) as { ceremony_id: string; wallet_uri: string }
  expect(ceremony.ceremony_id).toBeTruthy()
  expect(ceremony.wallet_uri).toBeTruthy()

  const ceremonyStart = ceremonyResponse.request().postDataJSON() as { field_name?: string }
  const signField = ceremonyStart.field_name?.trim() ?? ''
  expect(signField, 'ceremony start must bind a signature field_name').toBeTruthy()

  execFileSync(python, [path.join(here, 'complete_signing_webhook.py'), ceremony.wallet_uri], {
    cwd: repoRoot,
    // E2E_SIGNATORY must match what's passed to sign_prepared_pdf.py below —
    // the DCS's cert-subject to PID name-match gate (ADR-20) checks the two
    // against each other.
    env: {
      ...process.env,
      ISSUER_BASE_URL: E2E_ISSUER_BASE_URL,
      BDD_DCS_BASE_URL: inst.apiBase,
      E2E_SIGNATORY: signatory,
    },
    stdio: 'pipe',
  })

  const prepared = await preparedResponse.catch((error: unknown) => {
    const message = error instanceof Error ? error.message : String(error)
    throw new Error(
      `${message}\nviewer signature calls:\n  ${viewerCalls.join('\n  ') || '(none)'}\nviewer console errors:\n  ${viewerErrors.join('\n  ') || '(none)'}`,
    )
  })
  return { ceremonyId: ceremony.ceremony_id, signField, prepared }
}

/**
 * Signs an APPROVED contract on a given instance through that instance's Secure
 * Contract Viewer, exactly as a real signer would (ADR-12): run the ceremony
 * above, download the to-be-signed PDF, sign it externally with the test
 * wallet's key via the DSS SCA, upload it, and confirm SIGNED. The signature
 * field is the signing party's own DCS DID slot; the wallet discovers it from
 * the PDF.
 */
export async function signOnInstance(inst: Instance, contractDid: string, signatory: string): Promise<void> {
  const { ceremonyId, signField, prepared } = await runSigningCeremonyOn(inst, contractDid, signatory)

  const preparedPath = path.join(tmpdir(), `prepared-${ceremonyId}.pdf`)
  expect(
    prepared.ok(),
    `prepare the to-be-signed document on ${inst.origin}: HTTP ${prepared.status()} ${await prepared.text().catch(() => '')}`,
  ).toBeTruthy()
  // /signature/prepare answers a JSON envelope carrying the PDF base64-encoded
  // (the viewer decodes it into the blob it hands the signatory), so decode it
  // the same way rather than treating the body as raw PDF bytes.
  const preparedEnvelope = (await prepared.json()) as { document: string }
  const preparedBytes = Buffer.from(preparedEnvelope.document, 'base64')
  expect(preparedBytes.subarray(0, 5).toString('latin1'), 'prepared document is a PDF').toBe('%PDF-')
  fs.writeFileSync(preparedPath, preparedBytes)
  const signedPath = path.join(tmpdir(), `signed-${ceremonyId}.pdf`)
  execFileSync(python, [path.join(here, 'sign_prepared_pdf.py'), preparedPath, signedPath], {
    cwd: repoRoot,
    env: {
      ...process.env,
      DSS_URL: E2E_DSS_URL,
      E2E_SIGNATORY: signatory,
      E2E_SIGN_FIELD: signField,
    },
    stdio: 'pipe',
  })

  // Assert the submit itself, with its body: the viewer swallows a failed submit
  // into an on-page message, so waiting only for the SIGNED badge reports a
  // missing element rather than why the DCS refused the signature.
  const submitted = inst.page.waitForResponse((r) => r.url().includes('/signature/submit'), { timeout: 120_000 })
  await inst.page.locator('input[type="file"]').setInputFiles(signedPath)
  const submitResponse = await submitted
  expect(
    submitResponse.ok(),
    `submit signature on ${inst.origin}: HTTP ${submitResponse.status()} ${await submitResponse.text().catch(() => '')}`,
  ).toBeTruthy()
  await expect(inst.page.getByText('SIGNED', { exact: true })).toBeVisible({ timeout: 60_000 })
}

/**
 * Stage 7 mutual-milestone gate — a signer whose OWN instance has finished its
 * workflow still cannot sign while the counterparty has not settled this
 * version.
 *
 * Distinct from assertNotYetSignable, which covers the local state gate before
 * approval (ADR-2 allows EventSign only from APPROVED) and is satisfied by the
 * contract simply not being offered. Here the instance is APPROVED and the
 * contract IS offered to its signer: the state machine is content, the signatory
 * presents their PID, and the refusal comes from the one thing local state
 * cannot supply — locally-held, verified evidence that the OTHER party agreed to
 * the document about to be signed (a settlement artifact the peer signs and
 * ships over the DCS-to-DCS channel). Intrinsic state is local (ADR-13), so an
 * instance reaching APPROVED says nothing at all about its counterparty, and a
 * signature binds the moment it is made — refusing to deploy afterwards would
 * not undo it.
 *
 * The refusal is asserted twice over: as the typed API code the frontend
 * dispatches on, and as what the signer is actually told — a signer looking at
 * a dead button with no explanation is the failure mode the code exists to
 * prevent.
 */
export async function assertSigningRefusedUntilCounterpartySettles(
  inst: Instance,
  contractDid: string,
  signatory: string,
): Promise<void> {
  const { prepared } = await runSigningCeremonyOn(inst, contractDid, signatory)

  const body = await prepared.text().catch(() => '')
  expect(
    prepared.ok(),
    `signing ${contractDid} on ${inst.origin} was allowed while the counterparty had not settled: HTTP ${prepared.status()} ${body}`,
  ).toBeFalsy()
  // By code, never by matching the message: bad_request also carries "you may
  // not sign this contract", which is a different answer to the signer.
  let refusal: { name?: string }
  try {
    refusal = JSON.parse(body) as { name?: string }
  } catch {
    throw new Error(`signing refusal on ${inst.origin} is not a typed error envelope: ${body}`)
  }
  expect(refusal.name, `signing refusal on ${inst.origin} must name the counterparty settlement, got ${body}`).toBe(
    'counterparty_not_settled',
  )

  await expect(inst.page.getByText(/Waiting for the counterparty to settle this version/)).toBeVisible({
    timeout: 30_000,
  })
}

/**
 * Establishes a role session on the instance and returns the Authorization
 * header its raw page.request calls need. applySession injects the token into
 * localStorage, but only the app's axios interceptor turns that into a bearer —
 * a raw page.request forwards cookies but omits the header, so JWT-scoped
 * endpoints 401. The navigation also refreshes the role's single-use token.
 */
export async function apiAuthHeaders(
  inst: Instance,
  role: DcsRole,
  landing: string,
): Promise<{ Authorization: string }> {
  await inst.gotoAs(role, landing)
  const token = await inst.page.evaluate(() => window.localStorage.getItem('access_token'))
  expect(token, `no access token for ${role} on ${inst.origin}`).toBeTruthy()
  return { Authorization: `Bearer ${token}` }
}

/**
 * Reads the contract's current optimistic-lock token (updated_at) from the
 * instance's own authenticated retrieve-by-id — the value state-transition POSTs
 * (offer/deploy/…) must echo. Fails loudly with the response shape if absent.
 */
export async function contractUpdatedAt(
  inst: Instance,
  contractDid: string,
  auth: { Authorization: string },
): Promise<string> {
  const resp = await inst.page.request.get(`${inst.apiBase}/contract/retrieve/${encodeURIComponent(contractDid)}`, {
    headers: auth,
  })
  expect(
    resp.ok(),
    `retrieve ${contractDid} on ${inst.origin}: HTTP ${resp.status()} ${await resp.text()}`,
  ).toBeTruthy()
  const body = (await resp.json()) as { updated_at?: string }
  expect(body.updated_at, `retrieve ${contractDid} on ${inst.origin} returned no updated_at`).toBeTruthy()
  return body.updated_at!
}

/**
 * Independently verifies the contract's exported PDF is a real, conformant
 * artifact — PDF/A-3a (veraPDF) + a valid C2PA manifest (c2patool/c2pa-rs) —
 * exporting it through the instance's own Contract Viewer and shelling out to
 * e2e/verify_artifact.py (the same external validators pdf-core runs). The
 * optional lifecycle is the SRS C2PA banner (draft during negotiation, active
 * once signed) — NOT the extrinsic negotiation phase.
 */
export async function verifyArtifact(
  inst: Instance,
  contractDid: string,
  opts: { lifecycle?: string; save?: string } = {},
): Promise<void> {
  const pdfPath = await exportContractPdf(inst, contractDid)
  const args = [path.join(here, 'verify_artifact.py'), pdfPath]
  if (opts.lifecycle) args.push('--lifecycle', opts.lifecycle)
  execFileSync(python, args, {
    cwd: repoRoot,
    stdio: 'pipe',
    timeout: 60_000,
    env: { ...process.env, PYTHONWARNINGS: 'ignore' },
  })
  if (opts.save) persistArtifact(pdfPath, opts.save)
}

/**
 * Exports the contract's PDF through the instance's own Contract Viewer and
 * returns the local path to the bytes.
 *
 * The Export PDF button is still clicked, and the export request it issues is
 * asserted — that is the real UI coverage. The bytes themselves are then read
 * back over the same authenticated endpoint rather than through the browser's
 * download event: capturing an artifact is a read, and the download event
 * proved an unreliable signal under two-instance CI load (the server answered
 * 200 with the full PDF and no error surfaced, yet no download ever fired).
 * Asserting the request keeps a genuinely broken button failing the suite.
 */
async function exportContractPdf(inst: Instance, contractDid: string): Promise<string> {
  await inst.gotoAs('Contract Manager', `/ui/contracts/view/${contractDid}`)
  const exportUrl = `${inst.apiBase}/pdf/export/contract/${encodeURIComponent(contractDid)}`

  const exported = inst.page.waitForResponse((r) => r.url().includes(`/pdf/export/contract/${contractDid}`) && r.ok(), {
    timeout: 120_000,
  })
  await inst.page.getByRole('button', { name: 'Export PDF' }).click()
  await exported

  const token = await inst.page.evaluate(() => window.localStorage.getItem('access_token'))
  const resp = await inst.page.request.get(exportUrl, {
    headers: { Authorization: `Bearer ${token}` },
    timeout: 120_000,
  })
  expect(resp.ok(), `export contract PDF on ${inst.origin}: HTTP ${resp.status()}`).toBeTruthy()
  const bytes = await resp.body()
  expect(bytes.subarray(0, 5).toString('latin1'), 'exported bytes are a PDF').toBe('%PDF-')

  // Save under a .pdf name: veraPDF (run by verify_artifact.py) refuses to
  // process a file without a .pdf extension.
  const out = path.join(tmpdir(), `export-${contractDid}-${Date.now()}.pdf`)
  fs.writeFileSync(out, bytes)
  return out
}

/**
 * Persists a hop's exported PDF and its embedded JSON-LD payload into the
 * vertical-artifacts dir (uploaded by CI for human supervision): `{label}.pdf`
 * beside `{label}.jsonld`. The JSON-LD is extracted from the very bytes we
 * saved, so the machine-readable payload always matches the human-readable PDF.
 */
function persistArtifact(pdfPath: string, label: string): void {
  fs.mkdirSync(artifactDir, { recursive: true })
  const outPdf = path.join(artifactDir, `${label}.pdf`)
  fs.copyFileSync(pdfPath, outPdf)
  execFileSync(
    python,
    [
      path.join(here, 'verify_artifact.py'),
      outPdf,
      '--extract-only',
      '--dump-jsonld',
      path.join(artifactDir, `${label}.jsonld`),
    ],
    { cwd: repoRoot, stdio: 'pipe', timeout: 60_000, env: { ...process.env, PYTHONWARNINGS: 'ignore' } },
  )
}

/** Saves a hop's PDF + embedded JSON-LD for the party's copy without running the
 *  heavyweight veraPDF/c2patool validators (those run at the verify hops). */
export async function saveArtifact(inst: Instance, contractDid: string, label: string): Promise<void> {
  persistArtifact(await exportContractPdf(inst, contractDid), label)
}

/** The C2PA manifest-history URL for a contract on an instance. The C2PA
 *  service is mounted on the API muxer (backend cmd/dcs/http.go), so it lives
 *  under DCS_API_PATH (/digital-contracting-service/api) like every other app
 *  endpoint — not at the service root (that is did.json, on the raw mux). */
function manifestHistoryUrl(inst: Instance, contractDid: string): string {
  return `${inst.apiBase}/c2pa/manifest/${encodeURIComponent(contractDid)}?history=true`
}

/**
 * Asserts the contract's C2PA manifest ingredient chain on this instance has
 * grown past prevCount (each PDF exchange adds one ingredient, so the
 * counterparty's provenance is chained rather than reset) and returns the new
 * length. Call on BOTH instances across a negotiation exchange.
 */
export async function assertManifestChainGrew(inst: Instance, contractDid: string, prevCount: number): Promise<number> {
  // The new PDF and its grown C2PA chain are produced by the event-driven
  // background regenerator AFTER the negotiate/sign call returns, and the peer's
  // copy replicates asynchronously over the PDF exchange. Until the regen lands
  // the export route reports "being regenerated" (backend exportcontract.go), so
  // poll the manifest history until the chain grows past prevCount, tolerating
  // the transient not-ready response.
  const deadline = Date.now() + 45_000
  let lastStatus = 0
  let lastLen = -1
  while (Date.now() < deadline) {
    const resp = await inst.page.request.get(manifestHistoryUrl(inst, contractDid))
    lastStatus = resp.status()
    if (resp.ok()) {
      const chain = (await resp.json()) as unknown[]
      if (Array.isArray(chain)) {
        lastLen = chain.length
        if (chain.length > prevCount) return chain.length
      }
    }
    await inst.page.waitForTimeout(1500)
  }
  expect(
    lastLen,
    `C2PA manifest chain on ${inst.origin} should grow past ${prevCount} within 45s (last HTTP ${lastStatus}, last length ${lastLen})`,
  ).toBeGreaterThan(prevCount)
  return lastLen
}

/** Current length of the contract's C2PA manifest chain on an instance (0 if
 *  none yet), for seeding assertManifestChainGrew. */
export async function manifestChainLength(inst: Instance, contractDid: string): Promise<number> {
  const resp = await inst.page.request.get(manifestHistoryUrl(inst, contractDid))
  if (!resp.ok()) return 0
  const chain = (await resp.json()) as unknown[]
  return Array.isArray(chain) ? chain.length : 0
}

/**
 * Polls the instance's own /contract/retrieve until the contract's state
 * matches expected (the peer-facing copy replicates asynchronously over the
 * PDF exchange, so allow the same window the peer-trust steps use).
 */
export async function assertReceivedInState(inst: Instance, contractDid: string, expected: string): Promise<void> {
  // Establish a Contract Manager session on this instance so the raw retrieve
  // carries the bearer the JWT-scoped endpoint requires (page.request forwards
  // cookies but not the Authorization header the app's axios interceptor adds).
  const auth = await apiAuthHeaders(inst, 'Contract Manager', `/ui/contracts/view/${contractDid}`)
  const deadline = Date.now() + 45_000
  let lastState = ''
  let lastStatus = 0
  let lastBody = ''
  while (Date.now() < deadline) {
    const resp = await inst.page.request.get(`${inst.apiBase}/contract/retrieve/${encodeURIComponent(contractDid)}`, {
      headers: auth,
    })
    lastStatus = resp.status()
    lastBody = await resp.text()
    if (resp.ok()) {
      lastState = String((JSON.parse(lastBody) as { state?: string }).state ?? '').toUpperCase()
      if (lastState === expected.toUpperCase()) return
    }
    await inst.page.waitForTimeout(1500)
  }
  // Peer replication failed: surface exactly what this instance sees so the CI
  // log disambiguates a never-created copy (ship/trust rejection at PostPdf) from
  // a slow or errored sync — the two look identical as an empty state otherwise.
  expect(
    lastState,
    `contract ${contractDid} on ${inst.origin} never reached ${expected} within 45s ` +
      `(last retrieve HTTP ${lastStatus}, state "${lastState}", body ${lastBody.slice(0, 400)})`,
  ).toBe(expected.toUpperCase())
}

/** Confirms the shared ConfirmationModal (comment/decision-note dialogs) on an
 *  instance's page. */
async function confirmModalOn(inst: Instance, buttonName: 'Submit' | 'Confirm'): Promise<void> {
  const dialog = inst.page.getByRole('dialog').filter({ hasText: 'Confirmation' })
  await expect(dialog).toBeVisible()
  await dialog.getByRole('button', { name: buttonName, exact: true }).click()
}

/** Waits until a template detail view finished loading (Global Name populated). */
async function waitForTemplateLoadedOn(inst: Instance, name: string): Promise<void> {
  await expect(inst.page.getByRole('group').filter({ hasText: 'Global Name' }).getByRole('textbox')).toHaveValue(name)
}

/**
 * Asserts a PDF/A can be exported for a document at the current lifecycle step
 * on an instance, using that instance's active bearer token and API base — the
 * same authenticated GET /pdf/export/{kind}/{did} the Export PDF button issues.
 */
async function assertPdfExportOn(
  inst: Instance,
  kind: 'template' | 'contract',
  did: string,
  step: string,
): Promise<void> {
  const token = await inst.page.evaluate(() => window.localStorage.getItem('access_token'))
  const resp = await inst.page.request.get(`${inst.apiBase}/pdf/export/${kind}/${encodeURIComponent(did)}`, {
    headers: { Authorization: `Bearer ${token}` },
    // The export blocks until the async regenerator catches up to the latest
    // change (server-side ceiling 60s); outwait it rather than hit Playwright's
    // 30s request default and mask the HTTP status this assert exists to read.
    timeout: 90_000,
  })
  expect(resp.ok(), `export ${kind} PDF at "${step}" on ${inst.origin}: HTTP ${resp.status()}`).toBeTruthy()
  const bytes = await resp.body()
  expect(bytes.subarray(0, 5).toString('latin1'), `PDF/A magic bytes at "${step}"`).toBe('%PDF-')
}

/** A non-trivial SHACL NodeShape TTL for the hub-publish stage: a payment
 *  clause asset type with a constrained monetary amount and currency. */
function paymentShapeTtl(name: string): string {
  return `@prefix sh: <http://www.w3.org/ns/shacl#> .
@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .
@prefix ex: <https://example.org/${name}#> .

ex:PaymentClauseShape
  a sh:NodeShape ;
  sh:targetClass ex:PaymentClause ;
  sh:property [
    sh:path ex:amount ;
    sh:datatype xsd:decimal ;
    sh:minInclusive 0 ;
    sh:minCount 1 ;
  ] ;
  sh:property [
    sh:path ex:currency ;
    sh:datatype xsd:string ;
    sh:in ( "EUR" "USD" ) ;
    sh:minCount 1 ;
  ] .
`
}

/**
 * Publishes a SHACL shapes-graph entry into the instance's Semantic Hub through
 * the dashboard UI (the Gaia-X case: an external shape enters a running
 * instance without a rebuild), then confirms it resolves through the hub's
 * public route and carries the expected shape.
 *
 * A domain vocabulary must be published on EVERY instance whose documents are
 * modelled against it: a document declares the library it was authored under in
 * its own sh:shapesGraph, and validation treats a declared graph the local hub
 * cannot resolve as a hard failure (backend validation/shaclengine.go
 * declaredShapes) — so a peer that never registered the library can neither
 * validate the received document nor render its data objects.
 */
export async function publishHubShapesOn(
  inst: Instance,
  name: string,
  ttl: string,
  expectedContent: string,
): Promise<void> {
  // Reached through the sidebar the role actually sees, so a section that is
  // navigable only by typing its URL fails here.
  await inst.gotoAs('Template Manager', '/ui/templates')
  await inst.page.getByRole('link', { name: 'Semantic Hub', exact: true }).click()
  await expect(inst.page).toHaveURL(/\/ui\/semantic-hub$/)
  await expect(inst.page.getByRole('heading', { name: 'Semantic Hub' })).toBeVisible()
  await inst.page.getByLabel('Entry name').fill(name)
  await inst.page.getByLabel('Entry kind').selectOption('shapes')
  await inst.page.getByLabel('Entry content').fill(ttl)
  await inst.page.getByRole('button', { name: 'Publish entry' }).click()
  await expect(inst.page.getByRole('heading', { name })).toBeVisible()
  await expect(inst.page.getByText('active').first()).toBeVisible()

  const resolved = await inst.page.request.get(`${inst.apiBase}/semantic/shapes/${name}`)
  expect(resolved.ok(), `published shape ${name} resolves on ${inst.origin}`).toBeTruthy()
  expect(await resolved.text()).toContain(expectedContent)
}

/**
 * Stage 1 — publishes a brand-new, non-trivial SHACL shapes-graph entry into
 * the instance's Semantic Hub through the dashboard UI (the Gaia-X case: an
 * external shape enters the running instance without a rebuild), then confirms
 * it resolves through the hub's public route. The vertical authors its own
 * vocabulary rather than assuming a seeded fixture.
 */
export async function publishShapeOnInstance(inst: Instance, name: string): Promise<void> {
  await publishHubShapesOn(inst, name, paymentShapeTtl(name), 'PaymentClauseShape')
}

/**
 * Stage 2 — builds a Component template with a semantic clause through the real
 * editor: a titled clause carrying human prose beside its machine-readable ODRL
 * meaning, bound to a SHACL-backed hub requirement field (Payment Amount), with
 * a permission bounded by that field, placed into the document outline. Returns
 * the created component's DID.
 */
export async function authorSemanticComponent(inst: Instance, name: string): Promise<string> {
  await inst.gotoAs('Template Creator', '/ui/templates/new')
  await inst.page.getByRole('button', { name: /Component/ }).click()
  await inst.page.getByRole('group').filter({ hasText: 'Global Name' }).getByRole('textbox').fill(name)
  await inst.page
    .getByRole('group')
    .filter({ hasText: 'Base Description' })
    .getByRole('textbox')
    .fill('Payment component authored by the two-instance vertical.')

  await inst.page.getByRole('tab', { name: /Clauses/ }).click()
  const editor = inst.page.getByTestId('split-clause-editor')
  await editor.getByPlaceholder('Clause title').fill('Payment terms')
  await editor.locator('select').first().selectOption({ label: 'Payment Amount' })
  await editor.locator('.clause-editor').first().click()
  await inst.page.keyboard.type('The provider invoices the agreed payment amount.')
  // Place an INLINE, fillable placeholder for Payment Amount by clicking its
  // building block in the "Available requirements" panel — RuleParamRow's click
  // fires insertPlaceholderFromPanel, which deterministically writes the
  // {{condition.param}} token into the clause. (Typing "{{" relies on a
  // contenteditable dropdown that does not fire under Playwright, so the
  // placeholder never landed and the contract carried no negotiable input.) Only
  // an inline placeholder renders an editable PreviewParamInput at contract time;
  // a field used solely as an ODRL constraint boundary renders nothing.
  //
  // The click MUST hit RuleParamRow (the leaf <li>, the row also showing
  // "required") — the enclosing condition <li> carries the same "Payment Amount"
  // text but has no click handler. Scope to the Available-requirements section and
  // exclude any <li> that itself contains an <li> (hasNot), leaving only the leaf
  // param row, so we hit neither the condition heading, the field <select>, nor
  // the ODRL constraint's "Payment Amount".
  const availableRequirements = editor.locator('section').filter({ hasText: 'Available requirements' })
  await availableRequirements
    .getByRole('listitem')
    .filter({ hasText: 'Payment Amount' })
    .filter({ hasNot: inst.page.getByRole('listitem') })
    .click()
  // Guard: the inline placeholder span must have landed in the clause editor
  // (ClauseTextEditor renders it as a span with data-parameter-name), else the
  // contract has no negotiable value and Stage 6 would fail silently later.
  await expect(editor.locator('[data-parameter-name]')).toHaveCount(1)

  const ruleSelect = (label: string) =>
    editor.locator('label.form-control').filter({ hasText: label }).locator('select')
  await ruleSelect('Rule').selectOption({ label: 'Permission — the assignee MAY' })
  await ruleSelect('Action').selectOption({ label: 'use' })
  await editor.getByRole('button', { name: '+ constraint' }).click()
  const constraint = editor.locator('.flex.flex-wrap.items-center.gap-1').last()
  await constraint.locator('select').nth(0).selectOption({ label: 'Payment Amount' })
  await constraint.locator('select').nth(1).selectOption({ label: 'less than or equal to' })
  // The bound must admit the amounts this vertical negotiates (20000 -> 10000 ->
  // 15000). Carried over from the single-instance component (which fills 250),
  // 500 made every negotiated value violate the contract's own ODRL rule, so the
  // reviewer's local semantic precheck withheld confirmation and the settle could
  // never complete.
  await constraint.locator('input[placeholder="value"]').fill('50000')
  // Denominate the boundary. A bare bound never reaches resolveConstraintUnit,
  // so every unit-related audit finding was unreachable from this vertical: a
  // declared unit was reported at warning severity, the workflow gate turns any
  // warning into REVIEW, and REVIEW refuses the offer — meaning no contract that
  // denominated a boundary could be offered at all, while this suite stayed
  // green. Stage 5 is the assertion: if the gate withholds the offer, B never
  // reaches OFFERED.
  await constraint.getByTestId('constraint-unit').fill('https://w3id.org/facis/dcs/taxonomy/v1#currency-EUR')

  await editor.getByRole('button', { name: 'Add clause', exact: true }).click()
  await expect(editor.getByPlaceholder('Clause title')).toHaveValue('')

  const modal = inst.page.getByRole('dialog')
  await inst.page.getByRole('button', { name: 'Place in document' }).first().click()
  await expect(modal.getByText('Selected clause')).toBeVisible()
  await modal.getByRole('button', { name: /Payment terms/ }).click()
  await expect(inst.page.getByRole('dialog')).toBeHidden()

  const created = inst.page.waitForResponse(
    (r) => r.url().includes('/template/create') && r.request().method() === 'POST' && r.ok(),
  )
  await inst.page.getByRole('button', { name: 'Create', exact: true }).click()
  const componentDid = ((await (await created).json()) as { did: string }).did
  expect(componentDid).toBeTruthy()
  await assertPdfExportOn(inst, 'template', componentDid, 'component DRAFT')
  return componentDid
}

/** DRAFT → SUBMITTED → REVIEWED → APPROVED for one template on an instance,
 *  via the real UI (submit, verify + reviewer recommendation, approval). */
export async function submitReviewApproveTemplateOn(inst: Instance, did: string, name: string): Promise<void> {
  await inst.gotoAs('Template Creator', `/ui/templates/view/${did}`)
  const submitted = inst.page.waitForResponse(
    (r) => r.url().includes('/template/submit') && r.request().method() === 'POST' && r.ok(),
  )
  await inst.page.getByRole('button', { name: 'Submit', exact: true }).click()
  await submitted
  await assertPdfExportOn(inst, 'template', did, `${name} SUBMITTED`)

  // Found in the Review Tasks tab rather than opened by URL: submitting is what
  // opens the review task, and the tab row is the reviewer's route to it.
  await inst.gotoAs('Template Reviewer', '/ui/tasks/reviews')
  await followViewLinkFor(inst, did, 'the reviews task tab')
  await expect(inst.page).toHaveURL(/\/ui\/templates\/review\//)
  await waitForTemplateLoadedOn(inst, name)
  const verified = inst.page.waitForResponse(
    (r) => r.url().includes('/template/verify') && r.request().method() === 'POST' && r.ok(),
  )
  const forwarded = inst.page.waitForResponse(
    (r) => r.url().includes('/template/submit') && r.request().method() === 'POST' && r.ok(),
  )
  await inst.page.getByRole('button', { name: 'Approve', exact: true }).click()
  await verified
  await inst.page.getByRole('dialog').getByRole('button', { name: 'Confirm approval', exact: true }).click()
  await forwarded

  // Likewise the approver: the row points at the approve view only while its
  // task is open and the template has been reviewed.
  await inst.gotoAs('Template Approver', '/ui/tasks/approvals')
  await followViewLinkFor(inst, did, 'the approvals task tab')
  await expect(inst.page).toHaveURL(/\/ui\/templates\/approve\//)
  await waitForTemplateLoadedOn(inst, name)
  const approved = inst.page.waitForResponse(
    (r) => r.url().includes('/template/approve') && r.request().method() === 'POST' && r.ok(),
  )
  await inst.page.getByRole('button', { name: 'Approve', exact: true }).click()
  await confirmModalOn(inst, 'Submit')
  await approved
  await assertPdfExportOn(inst, 'template', did, `${name} APPROVED`)
}

/**
 * Stage 3 — composes a Contract Template on an instance by inlining the approved
 * component's blocks, placeholders and policies into the document (Builder
 * outline, flatten-on-compose). Returns the created contract template's DID.
 */
export async function authorContractTemplate(inst: Instance, name: string, componentName: string): Promise<string> {
  await inst.gotoAs('Template Creator', '/ui/templates/new')
  await inst.page.getByRole('button', { name: /parent for other contracts/ }).click()
  await inst.page.getByRole('group').filter({ hasText: 'Global Name' }).getByRole('textbox').fill(name)
  await inst.page
    .getByRole('group')
    .filter({ hasText: 'Base Description' })
    .getByRole('textbox')
    .fill('Contract template composed by the two-instance vertical.')

  await inst.page.getByRole('tab', { name: /Builder/ }).click()
  await inst.page
    .getByRole('button', { name: /add.*block/i })
    .first()
    .click()
  const modal = inst.page.getByRole('dialog')
  await expect(modal.getByText('Components (inlined on add):')).toBeVisible()
  await modal.getByPlaceholder('Search components').fill(componentName)
  await modal.getByRole('button', { name: new RegExp(componentName) }).click()
  await expect(inst.page.getByRole('dialog')).toBeHidden()

  const created = inst.page.waitForResponse(
    (r) => r.url().includes('/template/create') && r.request().method() === 'POST' && r.ok(),
  )
  await inst.page.getByRole('button', { name: 'Create', exact: true }).click()
  const contractTemplateDid = ((await (await created).json()) as { did: string }).did
  expect(contractTemplateDid).toBeTruthy()
  return contractTemplateDid
}

/** Stage 3 tail — registers an approved contract template (publishes it to the
 *  Federated Catalogue) so contracts can be derived from it. */
export async function registerTemplateOn(inst: Instance, did: string, name: string): Promise<void> {
  await inst.gotoAs('Template Manager', `/ui/templates/view/${did}`)
  await waitForTemplateLoadedOn(inst, name)
  const registered = inst.page.waitForResponse(
    (r) => r.url().includes('/template/register') && r.request().method() === 'POST' && r.ok(),
  )
  await inst.page.getByRole('button', { name: 'Register', exact: true }).click()
  // Register goes through the shared ConfirmationModal ("Proceed with
  // registration?") before it actually submits — the click above only opens
  // it.
  await inst.page.getByRole('button', { name: 'Confirm', exact: true }).click()
  await registered
}

/**
 * Publishes a REGISTERED contract template to the Federated Catalogue — the
 * Template Manager's "Publish" action (TemplateManagerActions), which only
 * appears for a contract template in REGISTERED. Registering makes a template
 * usable locally; publishing is what puts it in the catalogue another DCS
 * browses.
 */
export async function publishTemplateOn(inst: Instance, did: string, name: string): Promise<void> {
  await inst.gotoAs('Template Manager', `/ui/templates/view/${did}`)
  await waitForTemplateLoadedOn(inst, name)
  const published = inst.page.waitForResponse(
    (r) => r.url().includes('/template/publish') && r.request().method() === 'POST',
    { timeout: 60_000 },
  )
  await inst.page.getByRole('button', { name: 'Publish', exact: true }).click()
  await inst.page.getByRole('button', { name: 'Confirm', exact: true }).click()
  const response = await published
  expect(
    response.ok(),
    `publish template on ${inst.origin}: HTTP ${response.status()} ${await response.text().catch(() => '')}`,
  ).toBeTruthy()
}

/**
 * Takes a catalogue-published template into this instance's own repository
 * through the real Template Catalogue UI: search the catalogue by name, open
 * the entry, read its Preview, then Register + Confirm. Returns the DID the
 * template got HERE — registering mints a local template (the view lands on
 * /ui/templates/edit/{did}), which is a different DID from the publisher's.
 */
export async function registerCatalogueTemplateOn(inst: Instance, name: string): Promise<string> {
  await inst.gotoAs('Template Manager', '/ui/catalogues/templates')

  // The search filter defaults to DID; switch it to Name through its popover.
  await inst.page.locator('#list-btn-search').click()
  await inst.page.locator('#list-popover-search').getByText('Name', { exact: true }).click()
  await inst.page.getByLabel('Search catalogue templates').fill(name)
  await inst.page.getByRole('button', { name: 'Search', exact: true }).click()

  const entry = inst.page.getByRole('listitem').filter({ hasText: name })
  await expect(entry, `catalogue entry ${name} on ${inst.origin}`).toHaveCount(1, { timeout: 60_000 })
  await entry.getByRole('link', { name: 'View' }).click()
  await expect(inst.page).toHaveURL(/\/catalogues\/templates\/view\/.+version=/)

  // The Preview tab renders the catalogue copy's own document — what a
  // manager reads before taking someone else's template into their repository.
  await inst.page.getByRole('tab', { name: 'Preview', exact: true }).click()

  const register = inst.page.getByRole('button', { name: 'Register', exact: true })
  await expect(register).toBeEnabled({ timeout: 60_000 })
  const registered = inst.page.waitForResponse(
    (r) => r.url().includes('/template/register') && r.request().method() === 'POST',
    { timeout: 60_000 },
  )
  await register.click()
  await inst.page.getByRole('button', { name: 'Confirm', exact: true }).click()
  const response = await registered
  expect(
    response.ok(),
    `register catalogue template on ${inst.origin}: HTTP ${response.status()} ${await response.text().catch(() => '')}`,
  ).toBeTruthy()

  await expect(inst.page).toHaveURL(/\/templates\/edit\/.+/, { timeout: 60_000 })
  const localDid = decodeURIComponent(new URL(inst.page.url()).pathname.split('/templates/edit/')[1] ?? '')
  expect(localDid, `local DID of the registered catalogue template on ${inst.origin}`).toBeTruthy()
  return localDid
}

/**
 * Fills a DRAFT contract's negotiable values through the real Contract Content
 * tab and saves them with "Update".
 *
 * Two kinds of input live there, and a contract carries both: the typed domain
 * objects of dcs:contractData render one control per negotiable leaf in the
 * data-objects editor (keyed by the leaf's property local name), while a field
 * placed inline in clause prose renders a PreviewParamInput labelled with the
 * field's own label. A data leaf constrained by sh:in renders a <select>, so
 * the control is driven by its tag rather than assumed to be a text input.
 */
export async function fillContractValuesOn(
  inst: Instance,
  contractDid: string,
  values: { dataLeaves?: Record<string, string>; inlineFields?: Record<string, string> },
): Promise<void> {
  await inst.gotoAs('Contract Creator', `/ui/contracts/edit/${contractDid}`)
  await inst.page.getByRole('tab', { name: 'Contract Content' }).click()

  for (const [property, value] of Object.entries(values.dataLeaves ?? {})) {
    const control = inst.page.getByTestId(`fill-${property}`)
    await expect(control, `negotiable leaf ${property} on ${inst.origin}`).toBeVisible({ timeout: 30_000 })
    if ((await control.evaluate((el) => el.tagName)) === 'SELECT') await control.selectOption(value)
    else await control.fill(value)
  }
  for (const [label, value] of Object.entries(values.inlineFields ?? {})) {
    const input = inst.page.getByRole('textbox', { name: label }).first()
    await expect(input, `inline field ${label} on ${inst.origin}`).toBeVisible({ timeout: 30_000 })
    await input.fill(value)
    await input.blur()
  }

  const updated = inst.page.waitForResponse(
    (r) => r.url().includes('/contract/update') && r.request().method() === 'PUT',
  )
  await inst.page.getByRole('button', { name: 'Update', exact: true }).click()
  const response = await updated
  expect(
    response.ok(),
    `contract update on ${inst.origin}: HTTP ${response.status()} ${await response.text().catch(() => '')}`,
  ).toBeTruthy()
}

/**
 * Asserts the DRAFT contract cannot be submitted while a filled value violates
 * the contract's own ODRL policy: the Contract Creator clicks Submit, the
 * editor's semantic verification (NewContractView verifySemanticValues) refuses
 * before any request leaves the browser, and the refusal names the violated
 * constraint in an alert. Asserting that NO /contract/submit was issued is what
 * makes this a refusal rather than a message.
 */
export async function expectSubmitRefusedOn(inst: Instance, contractDid: string, reason: RegExp): Promise<void> {
  await inst.gotoAs('Contract Creator', `/ui/contracts/edit/${contractDid}`)
  // Verification runs over the values the editor has LOADED, so clicking Submit
  // before the document arrived would refuse for "required but has no value"
  // instead of the policy violation this asserts.
  await inst.page.getByRole('tab', { name: 'Contract Content' }).click()
  await expect(inst.page.getByTestId('data-objects-editor')).toBeVisible({ timeout: 30_000 })
  const submitCalls: string[] = []
  inst.page.on('request', (request) => {
    if (request.url().includes('/contract/submit')) submitCalls.push(request.url())
  })
  await inst.page.getByRole('button', { name: 'Submit', exact: true }).click()
  await expect(inst.page.getByRole('alert').filter({ hasText: reason })).toBeVisible({ timeout: 30_000 })
  expect(submitCalls, `submit must not reach the DCS on ${inst.origin}`).toHaveLength(0)
}

/** The contract record as this instance holds it (the authenticated
 *  retrieve-by-id the Contract Manager's views read): its document plus the
 *  negotiation rounds this instance itself recorded. */
async function contractRecordOn(inst: Instance, contractDid: string): Promise<ContractRecord> {
  const auth = await apiAuthHeaders(inst, 'Contract Manager', `/ui/contracts/view/${contractDid}`)
  const resp = await inst.page.request.get(`${inst.apiBase}/contract/retrieve/${encodeURIComponent(contractDid)}`, {
    headers: auth,
  })
  expect(resp.ok(), `retrieve ${contractDid} on ${inst.origin}: HTTP ${resp.status()}`).toBeTruthy()
  const body = (await resp.json()) as ContractRecord
  expect(body.contract_data, `contract ${contractDid} on ${inst.origin} carries no document`).toBeTruthy()
  return body
}

interface ContractRecord {
  contract_data?: Record<string, unknown>
  negotiations?: Array<{ created_by?: string }>
}

/** The contract document as this instance holds it. */
export async function contractDocumentOn(inst: Instance, contractDid: string): Promise<Record<string, unknown>> {
  return (await contractRecordOn(inst, contractDid)).contract_data!
}

/** The counterparty's own did:web, resolved from its origin-root DID document
 *  (/.well-known/did.json) — the value A types into the R6 counterparty input. */
export async function resolveDidWeb(inst: Instance): Promise<string> {
  const resp = await inst.page.request.get(`${inst.origin}/.well-known/did.json`)
  expect(resp.ok(), `DID document for ${inst.origin}: HTTP ${resp.status()}`).toBeTruthy()
  const id = String(((await resp.json()) as { id?: string }).id ?? '')
  expect(id).toBeTruthy()
  return id
}

/**
 * Stage 4 — derives a contract from a registered template through the real UI,
 * naming the counterparty via the R6 ParticipantSelectionDialog (a single
 * counterparty did:web input). Returns the created contract's DID.
 */
export async function createContractViaUi(inst: Instance, templateName: string, counterparty: string): Promise<string> {
  // Entered from the Contracts list through its own New Contract action, so the
  // creator's route in is exercised rather than assumed.
  await inst.gotoAs('Contract Creator', '/ui/contracts')
  await inst.page.getByRole('link', { name: 'New Contract', exact: true }).click()
  await expect(inst.page).toHaveURL(/\/ui\/contracts\/new$/)
  const picker = inst.page.locator('select').first()
  const option = picker.locator('option', { hasText: templateName })
  await expect(option).toHaveCount(1)
  await picker.selectOption({ label: (await option.textContent())!.trim() })

  await inst.page.getByRole('button', { name: 'Create', exact: true }).click()
  const dialog = inst.page.getByRole('dialog').filter({ hasText: 'Contract Counterparty' })
  await expect(dialog).toBeVisible()
  await dialog.getByPlaceholder('did:web:...').fill(counterparty)
  const created = inst.page.waitForResponse(
    (r) => r.url().includes('/contract/create') && r.request().method() === 'POST',
  )
  await dialog.getByRole('button', { name: 'Apply', exact: true }).click()
  const resp = await created
  expect(resp.ok(), `contract create ${resp.status()}: ${await resp.text()}`).toBeTruthy()
  const contractDid = String(((await resp.json()) as { did?: string }).did ?? '')
  expect(contractDid).toBeTruthy()
  return contractDid
}

/**
 * Fills the contract's Payment Amount through the real edit UI and saves it via
 * "Update" — Contract Generation ends with a filled-out contract (SRS §2.2.2),
 * and command/offer.go's closedness gate rejects offering a draft whose
 * required placeholder is still unfilled, so the originator must propose its
 * opening amount before the draft may leave the instance.
 */
export async function fillContractAmountOn(inst: Instance, contractDid: string, value: string): Promise<void> {
  await inst.gotoAs('Contract Creator', `/ui/contracts/edit/${contractDid}`)
  await inst.page
    .getByRole('tab', { name: /content/i })
    .or(inst.page.getByText('Contract Content', { exact: true }))
    .first()
    .click()
  const amount = inst.page
    .getByRole('spinbutton', { name: /amount/i })
    .or(inst.page.getByRole('textbox', { name: /amount/i }))
    .first()
  await expect(amount).toBeVisible({ timeout: 30_000 })
  await amount.fill(value)
  await amount.blur()
  const updated = inst.page.waitForResponse(
    (r) => r.url().includes('/contract/update') && r.request().method() === 'PUT',
  )
  await inst.page.getByRole('button', { name: 'Update', exact: true }).click()
  const resp = await updated
  expect(resp.ok(), `contract update ${resp.status()}: ${await resp.text()}`).toBeTruthy()
}

/**
 * Makes a counter-offer through the SRS §3.1.1 "Save draft" leg: stages the
 * redline as the party's negotiation draft, proves it survives leaving the
 * view (restored value + Discard draft offered on return), then proposes it
 * via "Change Proposal". Until the propose, the staged redline is invisible
 * to the peer — the caller's assertManifestChainGrew after this call is what
 * proves the chain grew from the PROPOSE, the save itself ships nothing.
 */
export async function stagedCounterOffer(inst: Instance, contractDid: string, opts: { value: string }): Promise<void> {
  // Now that the party has engaged, the tab row IS the route: the task points at
  // the round, and following it must land on the negotiate view rather than the
  // read-only one.
  await openContractFromTaskTab(inst, 'Contract Negotiator', 'negotiations', contractDid)
  await expect(inst.page).toHaveURL(/\/ui\/contracts\/negotiate\//)
  await inst.page
    .getByRole('tab', { name: /content/i })
    .or(inst.page.getByText('Contract Content', { exact: true }))
    .first()
    .click()
  const amount = inst.page.getByRole('textbox', { name: 'Payment Amount' }).first()
  await expect(amount).toBeVisible({ timeout: 30_000 })
  await amount.fill(opts.value)
  const saved = inst.page.waitForResponse(
    (r) => r.url().includes('/contract/negotiation_draft') && r.request().method() === 'PUT',
  )
  await inst.page.getByRole('button', { name: 'Save draft', exact: true }).click()
  const saveResp = await saved
  expect(saveResp.ok(), `draft save ${saveResp.status()}: ${await saveResp.text()}`).toBeTruthy()

  // The staged draft survives navigation: a fresh visit restores the value
  // and offers Discard draft.
  await inst.gotoAs('Contract Manager', '/ui/contracts')
  await inst.gotoAs('Contract Manager', `/ui/contracts/negotiate/${contractDid}`)
  await inst.page
    .getByRole('tab', { name: /content/i })
    .or(inst.page.getByText('Contract Content', { exact: true }))
    .first()
    .click()
  const restored = inst.page.getByRole('textbox', { name: 'Payment Amount' }).first()
  await expect(restored).toBeVisible({ timeout: 30_000 })
  await expect(restored).toHaveValue(formatNumberInput(opts.value))
  await expect(inst.page.getByRole('button', { name: 'Discard draft', exact: true })).toBeVisible()

  const proposed = inst.page.waitForResponse(
    (r) => r.url().includes('/contract/negotiate') && r.request().method() === 'POST' && r.ok(),
    { timeout: 30_000 },
  )
  await inst.page.getByRole('button', { name: 'Change Proposal' }).click()
  await proposed
}

/**
 * Makes a non-trivial counter-offer on the instance's Negotiate view: edits a
 * requirement value in the contract editor (producing a change request) and
 * submits it, which regenerates the PDF and re-ships it to the counterparty.
 * NOTE: the editor field-drilling here is the coordination seam with the
 * backend R5 (counter-offer round-trip) — refine the selector during
 * integration once the negotiate → settle flow is wired end to end.
 */
export async function counterOffer(inst: Instance, contractDid: string, opts: { value: string }): Promise<void> {
  // The counterparty makes a counter-offer by proposing a redline through the
  // real Negotiate UI. Its received copy is OFFERED and it holds the Negotiator
  // role (not Creator), so it cannot /contract/submit (Creator-only) — instead
  // its "Change Proposal" (/contract/negotiate) opens negotiation directly
  // (Offered --EventNegotiate--> Negotiation; SRS DCS-IR-CWE-03/DCS-FR-CWE-18).
  // Both parties hold a task, so the originator has its own tab row to arrive
  // by: authoring the contract is its engagement with the first round, and a
  // re-ship carries that task to each new round rather than dropping it. Its
  // copy is still OFFERED (a peer's re-ship never moves this instance's own
  // intrinsic state), so the tab lists the contract on the strength of the
  // TASK's state — an entry that a filter keyed on the contract's state would
  // hide, which is how the tab used to look empty here.
  await openContractFromTaskTab(inst, 'Contract Creator', 'negotiations', contractDid)
  await expect(inst.page).toHaveURL(/\/ui\/contracts\/view\//)
  await inst.page.getByTestId('open-negotiation').click()
  await expect(inst.page).toHaveURL(/\/ui\/contracts\/negotiate\//)
  // The negotiable requirement-field value inputs live under the Contract Content
  // tab (NegotiateContractView renders them via TemplatePreview). Editing the
  // Payment Amount field THERE is what flips changedContractData, so the change
  // request carries the full contract_data the backend applies + re-ships — a
  // metadata-field edit would only set changedName and change nothing visible.
  await inst.page
    .getByRole('tab', { name: /content/i })
    .or(inst.page.getByText('Contract Content', { exact: true }))
    .first()
    .click()
  // PreviewParamInput renders the decimal field as <input type="text"
  // aria-label="Payment Amount"> (role textbox, not spinbutton): the reconstructed
  // param resolves its label from the seeded ontology field (dcst:...#field-
  // contract-payment-amount is a host-stable w3id IRI, so it matches on both
  // instances -> uiMetadata.label "Payment Amount"), never the parameterName.
  const amount = inst.page.getByRole('textbox', { name: 'Payment Amount' }).first()
  await expect(amount).toBeVisible({ timeout: 30_000 })
  await amount.fill(opts.value)
  const proposed = inst.page.waitForResponse(
    (r) => r.url().includes('/contract/negotiate') && r.request().method() === 'POST' && r.ok(),
    { timeout: 30_000 },
  )
  await inst.page.getByRole('button', { name: 'Change Proposal' }).click()
  await proposed
}

/**
 * Stage 5 — A transmits the DRAFT contract to its counterparty through the real
 * UI: the Contract Creator's "Offer to counterparty" action on the contract view
 * (DRAFT -> OFFERED). command/offer.go gates this on the ContractCreator role and
 * EventOffer, which the state machine allows only from DRAFT (SRS DCS-IR-CWE-01;
 * §1.2 offer→acceptance). The transition ships the PDF to the trusted peer.
 */
export async function offerToCounterparty(inst: Instance, contractDid: string): Promise<void> {
  // Found from the contract list rather than opened by URL: A's own route to
  // the contract it just authored is a row in that list.
  await openContractFromList(inst, 'Contract Creator', contractDid)
  await expect(inst.page).toHaveURL(/\/ui\/contracts\/view\//)
  const offered = inst.page.waitForResponse(
    (r) => r.url().includes('/contract/offer') && r.request().method() === 'POST' && r.ok(),
    { timeout: 30_000 },
  )
  await inst.page.getByRole('button', { name: 'Offer to counterparty' }).click()
  await offered
}

/**
 * The Responder takes an inbound offer into negotiation as it stands (SRS §4:
 * accept, negotiate or refuse), through the real "Accept offer" button.
 *
 * Receiving an offer mints nothing — a negotiation task records that a party
 * ENGAGED with the round, which is what submit's settlement gate reads. So this
 * is also what puts the contract in the Responder's Negotiations tab, asserted
 * here: no task, no row, which is how the tab stayed empty for every federated
 * contract.
 */
export async function acceptOfferOn(inst: Instance, contractDid: string): Promise<void> {
  // Symptom 1, before the accept: the offer has arrived and replicated, and the
  // Negotiations tab still knows nothing about it. Nothing was minted on
  // receipt, so there is no row to find.
  await inst.gotoAs('Contract Negotiator', '/ui/tasks/negotiations')
  const taskRow = inst.page.locator('.list-row').filter({
    has: inst.page.locator(`a[href*="${didTail(contractDid)}"]`),
  })
  // Absence only means something once the tab has actually rendered its answer:
  // an empty DOM satisfies toHaveCount(0) while the tasks are still loading.
  await expect(
    inst.page.locator('.list-row').or(inst.page.getByText('No negotiation tasks found.')).first(),
  ).toBeVisible({ timeout: 30_000 })
  await expect(taskRow, `${inst.origin} holds a negotiation task for an offer it has not accepted`).toHaveCount(0)

  // Symptom 2 — the Responder's route in. With no task there is no tab row, so
  // the contract list is the only surface the offer appears on and the contract
  // view's own action is the only way through to the negotiate view. Reaching it
  // by clicking is what makes a missing or wrongly-gated entry point fail here.
  await openContractFromList(inst, 'Contract Negotiator', contractDid)
  await expect(inst.page).toHaveURL(/\/ui\/contracts\/view\//)
  const intoNegotiation = inst.page.getByTestId('open-negotiation')
  await expect(intoNegotiation, `${inst.origin} offers no route from the received offer into negotiation`).toHaveText(
    'Review offer',
  )
  await intoNegotiation.click()
  await expect(inst.page).toHaveURL(/\/ui\/contracts\/negotiate\//)

  // Accepting as it stands: no redline, nothing edited.
  const accepted = inst.page.waitForResponse(
    (r) => r.url().includes('/contract/accept-offer') && r.request().method() === 'POST' && r.ok(),
    { timeout: 30_000 },
  )
  await inst.page.getByTestId('accept-offer').click()
  await accepted

  // Symptom 1, after: the accept minted the task, so the contract is now in the
  // Responder's Negotiations tab — matched by the row's own link, not counted,
  // so a row for some other contract cannot stand in for it.
  await inst.gotoAs('Contract Negotiator', '/ui/tasks/negotiations')
  await expect(
    taskRow,
    `accepting the offer put no negotiation task for ${contractDid} in ${inst.origin}'s Negotiations tab`,
  ).toHaveCount(1, { timeout: 30_000 })
}

/**
 * Stage 7 pre-settle gate — asserts a contract is not yet signable on an instance.
 * ADR-2 allows EventSign only from APPROVED, so before the contract is approved
 * the Secure Contract Viewer's signing list must not offer it. This is the real
 * UI gate a signer hits (there is no /signature/apply route to POST against).
 */
export async function assertNotYetSignable(inst: Instance, contractDid: string): Promise<void> {
  await inst.gotoAs('Contract Signer', '/ui/signing')
  await expect(inst.page.getByRole('heading', { name: /Signing/ })).toBeVisible()
  await expect(inst.page.getByRole('row').filter({ hasText: contractDid })).toHaveCount(0)
}

/**
 * Waits for a counter-offer the PEER proposed to reach this instance's own copy
 * of the contract, and returns the document it landed as.
 *
 * A redline crosses as the DOCUMENT, not as a change request: the proposing
 * instance applies it to its own contract_data and ships the re-rendered PDF,
 * and the receiver adopts that document verbatim (ADR-13 §1/§2 — the PDF is the
 * wire format, and "the counterparty receives it as a proposal"). Negotiation
 * rows and their decisions are local to the instance that recorded them and are
 * not replicated — the peer sync that used to carry them was deleted with the
 * single-writer-origin model. So the receiving side has no change request to
 * answer, and no Show/Accept entry in its Active negotiations; under ADR-13 §3
 * agreeing to the terms on the table is SETTLEMENT (a ship of the same version
 * stamped `agreed`), which the two-instance vertical covers.
 *
 * The ship is asynchronous and the views do not poll, so this reads the
 * authenticated retrieve until the redlined value is the one this instance
 * holds. It then asserts this instance lists no round at all, which pins that
 * boundary — so it serves only an instance that has proposed nothing itself.
 */
export async function awaitPeerRedlineOn(
  inst: Instance,
  contractDid: string,
  opts: { label: string; value: string },
): Promise<Record<string, unknown>> {
  let record: ContractRecord = {}
  let held = ''
  for (let attempt = 0; attempt < 12; attempt++) {
    record = await contractRecordOn(inst, contractDid)
    const fields = (record.contract_data?.['dcs:contractFields'] ?? []) as Array<Record<string, unknown>>
    held = JSON.stringify(fields.find((field) => field['dcs:label'] === opts.label)?.['dcs:value'] ?? null)
    if (held.includes(opts.value)) break
    await inst.page.waitForTimeout(5_000)
  }
  expect(held, `the peer's redline of ${opts.label} never reached ${inst.origin} for ${contractDid}`).toContain(
    opts.value,
  )
  // The boundary the document crossed alone: the receiver records no round of
  // its own for a counter it did not propose. Asserting it here is what makes a
  // later change to that rule surface as this stage failing rather than as a
  // responder view silently offering an Accept nothing ships.
  expect(
    record.negotiations ?? [],
    `${inst.origin} lists a change request for a counter-offer it did not propose`,
  ).toHaveLength(0)
  return record.contract_data!
}

/**
 * Opens an instance's negotiate view and waits for it to have loaded the
 * contract: Submit only renders once contract.state is known, and the counts
 * probed after it do NOT auto-wait — probing straight after navigation reported
 * "no open decisions" while the fetch was still in flight, silently skipping the
 * accept and leaving the round unresolvable.
 *
 * The navigation is repeated rather than waited on longer because the suite runs
 * against a Vite dev server: a page load that loses its module requests
 * mid-flight (net::ERR_NETWORK_CHANGED took out ~60 of them on the second
 * instance in run 30573745114) leaves an app that never mounts, and no wait
 * recovers a document that has already finished loading. A genuinely absent
 * Submit still fails, on the same assertion.
 */
async function openNegotiateView(inst: Instance, contractDid: string): Promise<void> {
  const submit = inst.page.getByRole('button', { name: 'Submit', exact: true })
  for (let attempt = 0; attempt < 2; attempt++) {
    await inst.gotoAs('Contract Creator', `/ui/contracts/negotiate/${contractDid}`)
    const mounted = await submit
      .waitFor({ state: 'visible', timeout: 30_000 })
      .then(() => true)
      .catch(() => false)
    if (mounted) return
  }
  await expect(submit, `negotiate view never mounted on ${inst.origin} for ${contractDid}`).toBeVisible({
    timeout: 30_000,
  })
}

/**
 * Accepts every outstanding change request on this instance (NegotiationList
 * Show → Accept → /contract/respond) until none remain.
 *
 * hasOpenDecisions counts EVERY undecided decision on the contract, including
 * the counterparty's, and that record replicates to both copies. So after the
 * final counter the offering side cannot submit until the RECEIVING side has
 * decided — settling is a mutual agreement, not a unilateral one. Reload between
 * rounds so the compare view "Show" opens (itself a Submit blocker) clears
 * before the next decision.
 */
export async function acceptOpenDecisionsOn(inst: Instance, contractDid: string): Promise<void> {
  for (let round = 0; round < 10; round++) {
    await openNegotiateView(inst, contractDid)
    const pending = await inst.page.getByRole('button', { name: 'Show' }).count()
    if (pending === 0) break

    // Walk every pending round rather than only the first: a change request this
    // instance authored itself stays pending forever (FR-CWE-07 refuses an accept
    // by its own author), so it must be stepped over to reach the peer's.
    let accepted = false
    for (let i = 0; i < pending && !accepted; i++) {
      await openNegotiateView(inst, contractDid)
      const showBtn = inst.page.getByRole('button', { name: 'Show' }).nth(i)
      if (!(await showBtn.isVisible().catch(() => false))) continue
      await showBtn.click()
      const responded = inst.page.waitForResponse(
        (r) => r.url().includes('/contract/respond') && r.request().method() === 'POST',
        { timeout: 30_000 },
      )
      await inst.page.getByRole('button', { name: 'Accept', exact: true }).click()
      await confirmModalOn(inst, 'Confirm')
      accepted = (await responded).ok()
    }
    if (!accepted) break
  }
}

/**
 * Stage 7 settle — drives an instance's contract from an open negotiation round
 * to APPROVED through the real UI, the SRS consolidation path (there is no
 * /contract/settle route; ACCEPTED is not a contract state). Accepts the
 * outstanding change request (NegotiationList Show → Accept → /contract/respond),
 * submits the merged round (NEGOTIATION → SUBMITTED), reviews it (SUBMITTED →
 * REVIEWED), and approves it (REVIEWED → APPROVED, EventApprove). Mirrors the
 * proven single-instance submit→review→approve sequence.
 */
export async function settleToApprovedOn(inst: Instance, contractDid: string): Promise<void> {
  await acceptOpenDecisionsOn(inst, contractDid)

  // Reload so the compare view that "Show" opened (which disables Submit) and
  // the now-resolved decision clear, then submit the merged round
  // (NEGOTIATION -> SUBMITTED) once Submit is enabled.
  await inst.gotoAs('Contract Creator', `/ui/contracts/negotiate/${contractDid}`)
  const submit = inst.page.getByRole('button', { name: 'Submit', exact: true })
  await expect(submit).toBeEnabled({ timeout: 30_000 })
  const submitted = inst.page.waitForResponse(
    (r) => r.url().includes('/contract/submit') && r.request().method() === 'POST' && r.ok(),
    { timeout: 30_000 },
  )
  await submit.click()
  await submitted

  // Review: SUBMITTED -> REVIEWED. The reviewer arrives the way the reviewer
  // finds work — the Review Tasks tab row, which points at the review view only
  // while the task is open and the contract is SUBMITTED.
  await openContractFromTaskTab(inst, 'Contract Reviewer', 'reviews', contractDid)
  await expect(inst.page).toHaveURL(/\/ui\/contracts\/review\//)
  const forwarded = inst.page.waitForResponse(
    (r) => r.url().includes('/contract/submit') && r.request().method() === 'POST' && r.ok(),
    { timeout: 30_000 },
  )
  await inst.page.getByRole('button', { name: 'Approve', exact: true }).click()
  await inst.page
    .getByRole('dialog', { name: /lokale semantische vorprüfung/i })
    .getByRole('button', { name: 'Confirm approval', exact: true })
    .click()
  await forwarded

  // Approve: REVIEWED -> APPROVED, found the same way in the Approval Tasks tab.
  await openContractFromTaskTab(inst, 'Contract Approver', 'approvals', contractDid)
  await expect(inst.page).toHaveURL(/\/ui\/contracts\/approve\//)
  const approved = inst.page.waitForResponse(
    (r) => r.url().includes('/contract/approve') && r.request().method() === 'POST' && r.ok(),
    { timeout: 30_000 },
  )
  await inst.page.getByRole('button', { name: 'Approve', exact: true }).click()
  await confirmModalOn(inst, 'Confirm')
  await approved
}

/**
 * Stage 9 — the Contract Manager deploys the fully-signed contract to the target
 * system through the real UI: the "Deploy" action in ContractManagerActions
 * (SIGNED -> ACTIVE, EventDeploy), gated on the Manager role and SIGNED state.
 */
export async function deployContract(inst: Instance, contractDid: string): Promise<void> {
  // The manager finds the signed contract in the list and opens it; a SIGNED
  // contract carries no open task, so the row leads to the contract view where
  // the Deploy action lives.
  await openContractFromList(inst, 'Contract Manager', contractDid)
  await expect(inst.page).toHaveURL(/\/ui\/contracts\/view\//)

  // A contract deploys to the target system it designates (ADR-25), so the
  // manager picks one first. The registry is seeded by the chart
  // (contractTargets in values.bdd.yml), so an entry is already there to pick.
  // Wait for the picker itself before deciding: an isVisible() poll on a page
  // that has not finished loading answers "no" and silently skips designation,
  // which then surfaces as a deploy refusal further down.
  await expect(inst.page.getByTestId('contract-target-picker')).toBeVisible({ timeout: 30_000 })
  if (
    await inst.page
      .getByTestId('contract-target-unset')
      .isVisible()
      .catch(() => false)
  ) {
    // Index 0 is the "none" placeholder.
    await inst.page.getByTestId('contract-target-select').selectOption({ index: 1 })
    const designated = inst.page.waitForResponse(
      (r) => r.url().includes('/contract/target/designate') && r.request().method() === 'POST',
      { timeout: 30_000 },
    )
    await inst.page.getByTestId('contract-target-save').click()
    const designateResponse = await designated
    expect(
      designateResponse.ok(),
      `designate target on ${inst.origin}: HTTP ${designateResponse.status()}`,
    ).toBeTruthy()
    await expect(inst.page.getByTestId('contract-target-name')).toBeVisible({ timeout: 15_000 })
  }

  // Match ANY deploy response, then assert: filtering on r.ok() made a refusal
  // indistinguishable from no request at all.
  const deployed = inst.page.waitForResponse(
    (r) => r.url().includes('/contract/deploy') && r.request().method() === 'POST',
    { timeout: 30_000 },
  )
  await inst.page.getByTestId('deploy-contract').click()
  const deployResponse = await deployed
  expect(
    deployResponse.ok(),
    `deploy contract on ${inst.origin}: HTTP ${deployResponse.status()} ${await deployResponse.text().catch(() => '')}`,
  ).toBeTruthy()
}
