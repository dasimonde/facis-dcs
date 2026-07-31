import { expect, test } from './dcs-test'

/**
 * HSM key inventory (ADR-28 addendum A2.1): the Sys. Administrator's read-only
 * view of the instance's five HSM key labels with their active version from
 * pki_active_key_version — including the new dcs-ecdh key-agreement (CEK wrap)
 * key. Rotation stays an operational procedure; this surface makes the key
 * material and its versions inspectable.
 */

const EXPECTED_LABELS = ['dcs-did', 'dcs-vc', 'dcs-oid4vp-jar', 'dcs-c2pa', 'dcs-ecdh']

test('the key inventory lists all five HSM keys with active versions', async ({ page, loginAs }) => {
  await loginAs('Sys. Administrator')
  await page.goto('/ui/admin/hsm-keys')

  await expect(page.getByTestId('key-inventory')).toBeVisible()
  await expect(page.getByTestId('key-inventory-row')).toHaveCount(EXPECTED_LABELS.length)

  for (const label of EXPECTED_LABELS) {
    const row = page.getByTestId('key-inventory-row').filter({ hasText: label })
    await expect(row, `inventory row for ${label}`).toBeVisible()
    // Every key carries its active version badge (v1 when never rotated).
    await expect(page.getByTestId(`key-version-${label}`)).toHaveText(/^v\d+$/)
  }
})
