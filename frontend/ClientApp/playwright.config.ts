import { defineConfig, devices } from '@playwright/test'

// The BDD venv's requests/urllib3 pairing emits a RequestsDependencyWarning on
// every import, flooding the e2e output. Silence Python warnings once here — the
// config is loaded by every worker, and every python subprocess spreads
// process.env — instead of per-execFileSync call.
process.env.PYTHONWARNINGS ??= 'ignore'

/**
 * E2E suite against a running DCS instance (default: the BDD kind cluster's
 * instance A). The dev server proxies /api to E2E_DCS_API_TARGET with the
 * instance's API base path, and each test mints its role tokens through the
 * instance's real OID4VP headless login (reusing the BDD suite's
 * AuthService), so specs drive the UI exactly as an authenticated user.
 * Fixtures are authored by clicking through the real UI (no raw-HTTP seed);
 * a spec that needs one builds it in a beforeAll (see lifecycle-helpers).
 *
 * Requirements: the target DCS instance is up (make -C tests/bdd kind_up)
 * and the BDD venv exists (~/.dcs-bdd-venv, created by the BDD Makefile).
 */

const FRONTEND_PORT = Number(process.env.E2E_FRONTEND_PORT ?? 5199)

/**
 * Instance A's public origin is "localhost" (values.bdd.yml): Hydra's
 * consent/callback legs and the status-list service live there, and the
 * OID4VP login flow's state cookie is host-scoped — so the API base must be
 * the localhost origin (dcs-a.localhost is an ADDITIONAL host used by the
 * DCS-to-DCS peer suite, not a self-contained login origin).
 */
export const E2E_API_BASE = process.env.E2E_DCS_API_BASE ?? 'http://localhost:18080/digital-contracting-service/api'

/**
 * The status-list service the minted credentials embed. In the BDD kind
 * stack this is reachable ONLY at the "localhost" public origin — in-cluster
 * the DCS resolves it through the statusListLocalhostProxy, host-side
 * through the Traefik port-forward — so it is NOT derived from the API
 * origin (dcs-a.localhost has no /statuslist route).
 */
export const E2E_STATUSLIST_URL = process.env.E2E_STATUSLIST_URL ?? 'http://localhost:18080/statuslist'

// The EU DSS the test wallet drives as its external SCA to sign the prepared
// contract PDF with the signatory's own key (ADR-12). The DCS holds no key.
export const E2E_DSS_URL = process.env.E2E_DSS_URL ?? 'http://localhost:18099'

/**
 * Instance B (dcs2) for the DCS-to-DCS scenarios: its own public origin is
 * dcs-b.localhost (values.bdd.yml), served in the browser by a second vite
 * dev server so the two instances' UIs run at distinct origins exactly as
 * two organizations would. The full-vertical peer-negotiation test drives
 * B's real UI through this origin.
 */
export const E2E_FRONTEND_ORIGIN = `http://localhost:${FRONTEND_PORT}`
const FRONTEND_B_PORT = Number(process.env.E2E_FRONTEND_B_PORT ?? 5198)
export const E2E_FRONTEND_B_ORIGIN = `http://localhost:${FRONTEND_B_PORT}`
export const E2E_API_BASE_B =
  process.env.E2E_DCS_API_BASE_B ?? 'http://dcs-b.localhost:18080/digital-contracting-service/api'

const apiTarget = new URL(E2E_API_BASE)
const apiTargetB = new URL(E2E_API_BASE_B)

export default defineConfig({
  testDir: './e2e',
  // Generous per-test budget: tests run fully parallel against one shared
  // kind-cluster backend, so an individual test can slow down under load
  // (the hub register-version flow renders ~100KB schema documents).
  timeout: 90_000,
  expect: { timeout: 15_000 },
  // Every test mints its own OID4VP session, and a file that needs a fixture
  // authors its own in a beforeAll, so tests within a file are as independent
  // as tests across files — run them all in parallel.
  fullyParallel: true,
  // One shared kind-cluster backend and one vite dev server serve every
  // worker — beyond ~6 local workers, page loads start starving instead of
  // parallelizing. CI keeps Playwright's own core-based default.
  workers: process.env.CI ? undefined : 6,
  retries: process.env.CI ? 1 : 0,
  reporter: [['list'], ['html', { open: 'never' }]],
  use: {
    baseURL: `http://localhost:${FRONTEND_PORT}`,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: [
    {
      command: 'npx vite --port ' + FRONTEND_PORT,
      url: `http://localhost:${FRONTEND_PORT}`,
      reuseExistingServer: !process.env.CI,
      env: {
        DCS_FRONTEND_PORT: String(FRONTEND_PORT),
        DCS_API_TARGET: apiTarget.origin,
        DCS_API_TARGET_PATH: apiTarget.pathname,
      },
    },
    {
      command: 'npx vite --port ' + FRONTEND_B_PORT,
      url: E2E_FRONTEND_B_ORIGIN,
      reuseExistingServer: !process.env.CI,
      env: {
        DCS_FRONTEND_PORT: String(FRONTEND_B_PORT),
        DCS_API_TARGET: apiTargetB.origin,
        DCS_API_TARGET_PATH: apiTargetB.pathname,
      },
    },
  ],
})
