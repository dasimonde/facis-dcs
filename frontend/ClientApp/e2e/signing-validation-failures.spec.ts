import { test } from './dcs-test'
import { buildApprovedContract, signApprovedContractViaViewerWithMismatchedCert } from './lifecycle-helpers'

/**
 * Signing validation-failure view (ADR-20 item 11): when the ceremony
 * callback rejects a submitted signature, the Secure Contract Viewer must
 * render the SPECIFIC, typed reason (backend/design/signature_management.go's
 * Error() results, mapped by signingErrorMessage() in
 * signature-management-service.ts) — never a generic "something went wrong."
 *
 * Drives a real rejection through the actual signing ceremony (start ->
 * PID/PoA presentation -> prepare -> external sign with a certificate that
 * deliberately does not name the ceremony's verified PID -> upload), so this
 * proves the DCS rejects it AND that the UI surfaces why, not just that it
 * failed. The happy path and the level-aware compliance viewer are covered by
 * full-vertical.spec.ts and signature-compliance-viewer.spec.ts respectively.
 */

test('the signing view renders the specific rejection reason for a cert/PID mismatch', async ({ page, loginAs }) => {
  // DCS-FR-CWE-26 // DCS-FR-SM-23
  test.setTimeout(90_000)
  page.setDefaultTimeout(15_000)

  const contractDid = await buildApprovedContract(page, loginAs)

  await test.step('a signature from a certificate naming someone other than the verified PID is rejected, with the specific reason shown', async () => {
    await signApprovedContractViaViewerWithMismatchedCert(page, loginAs, contractDid)
  })
})
