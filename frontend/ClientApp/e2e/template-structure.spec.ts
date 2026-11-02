import { expect, test } from './dcs-test'
import {
  authorContractTemplateFrom,
  authorPaymentComponent,
  deriveLocalContract,
  gotoAs,
  registerContractTemplate,
  submitReviewApproveTemplate,
} from './lifecycle-helpers'

/**
 * Template levels and required templates against the real backend
 * (DCS-FR-TR-25, DCS-FR-TR-02, DCS-FR-TR-23): the template details editor sets
 * the hierarchy level and selects the template a parent contract must come
 * from. A frame agreement template and a sub-agreement template that requires
 * it are composed, approved and registered through the UI. A contract from the
 * sub-agreement template accepts a parent contract from the frame agreement
 * template, and the backend refuses a parent that is a sub-agreement.
 */

test('a sub-agreement template requires a frame agreement, and its contracts follow the parent rule', async ({
  page,
  loginAs,
}) => {
  // DCS-FR-TR-25 // DCS-FR-TR-02 // DCS-FR-TR-23
  test.setTimeout(480_000)
  page.setDefaultTimeout(15_000)
  const unique = Date.now()
  const componentName = `TR25 Component ${unique}`
  const frameName = `TR25 Frame ${unique}`
  const subName = `TR25 Sub ${unique}`

  const componentDid = await test.step('author the payment component', () =>
    authorPaymentComponent(page, loginAs, componentName, true))
  await submitReviewApproveTemplate(page, loginAs, componentDid, componentName)

  const frameDid = await test.step('compose the frame agreement template', () =>
    authorContractTemplateFrom(page, loginAs, frameName, componentName, { level: 'FRAME_AGREEMENT' }))
  await submitReviewApproveTemplate(page, loginAs, frameDid, frameName)
  await test.step('register the frame agreement template', () =>
    registerContractTemplate(page, loginAs, frameDid, frameName))

  const subDid = await test.step('compose the sub-agreement template that requires the frame agreement', () =>
    authorContractTemplateFrom(page, loginAs, subName, componentName, { level: 'SUB_AGREEMENT', requires: frameName }))
  await submitReviewApproveTemplate(page, loginAs, subDid, subName)
  await test.step('register the sub-agreement template', () => registerContractTemplate(page, loginAs, subDid, subName))

  await test.step('the registered sub-agreement template shows its level and required template', async () => {
    await gotoAs(page, loginAs, 'Template Manager', `/ui/templates/view/${subDid}`)
    await expect(page.getByTestId('template-level')).toHaveValue('SUB_AGREEMENT')
    await expect(page.getByTestId('template-level')).toBeDisabled()
    await expect(page.getByTestId('template-required-templates').getByRole('listitem')).toHaveText([frameName])
  })

  const frameContractDid = await test.step('derive a contract from the frame agreement template', () =>
    deriveLocalContract(page, loginAs, frameName))

  const subContractDid =
    await test.step('derive a contract from the sub-agreement template under the frame agreement contract', async () => {
      const linked = page.waitForResponse(
        (r) => r.url().includes('/contract/update') && r.request().method() === 'PUT',
        { timeout: 60_000 },
      )
      const did = await deriveLocalContract(page, loginAs, subName, frameName)
      const resp = await linked
      expect(resp.ok(), `parent link ${resp.status()}: ${await resp.text()}`).toBeTruthy()
      return did
    })

  await test.step('the sub-agreement contract is part of the frame agreement contract', async () => {
    await gotoAs(page, loginAs, 'Contract Manager', `/ui/contracts/view/${subContractDid}`)
    // The link names the parent, or shows its DID when the parent is not in the loaded contract list.
    const parentLink = page.getByRole('heading', { name: 'Part of Contract' }).locator('xpath=..').getByRole('link')
    const escapedDid = frameContractDid.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
    await expect(parentLink).toHaveText(new RegExp(`${frameName}|${escapedDid}`))
  })

  await test.step('a sub-agreement contract is refused as the parent of a sub-agreement contract', async () => {
    const linked = page.waitForResponse((r) => r.url().includes('/contract/update') && r.request().method() === 'PUT', {
      timeout: 60_000,
    })
    // The first sub-agreement contract is the only DRAFT that carries the sub-agreement template's name.
    await deriveLocalContract(page, loginAs, subName, subName)
    const resp = await linked
    expect(resp.status(), `parent link of a sub-agreement to a sub-agreement: ${await resp.text()}`).toBe(400)
    expect(await resp.text()).toContain('must be a frame agreement')
  })
})
