import { expect, test } from './dcs-test'
import { buildDraftContractFixture, type DraftContractFixture } from './lifecycle-helpers'

/**
 * The storytelling layer: the frontpage narrates the product's stages per
 * role, and workflow views carry a lifecycle banner that says where the
 * object is, what happens next, and who acts.
 */

test('frontpage shows the template creator their stages but not the semantic hub', async ({ page, loginAs }) => {
  await loginAs('Template Creator')
  await page.goto('/ui/frontpage')

  await expect(page.getByRole('heading', { name: 'Digital Contracting Service' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Templates', exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Semantic Hub' })).not.toBeVisible()
})

test('frontpage shows the semantic hub stage to the template manager', async ({ page, loginAs }) => {
  await loginAs('Template Manager')
  await page.goto('/ui/frontpage')

  await expect(page.getByRole('heading', { name: 'Semantic Hub' })).toBeVisible()
})

test.describe('lifecycle narration on an authored draft', () => {
  let fixture: DraftContractFixture

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(180_000)
    fixture = await buildDraftContractFixture(browser)
  })

  test('contract edit view narrates the draft stage of the lifecycle', async ({ page, loginAs }) => {
    await loginAs('Contract Creator')
    await page.goto(`/ui/contracts/edit/${fixture.contractDid}`)

    await expect(page.getByText('This contract is a draft')).toBeVisible()
    await expect(page.getByText('then submit for review')).toBeVisible()
  })
})
