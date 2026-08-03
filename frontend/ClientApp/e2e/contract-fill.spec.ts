import { expect, test } from './dcs-test'
import { buildDraftContractFixture, type DraftContractFixture } from './lifecycle-helpers'

/**
 * The contract fill flow's semantic boundary, through the real edit UI: a
 * value typed into a placeholder input is carried inline on the typed
 * dcs:ContractField it fills (dcs:value on the field an ODRL constraint
 * names as its odrl:leftOperand), with the editor-internal (blockId,
 * placeholder @id) tuple never leaking as a separate values array, and the
 * unsigned contract's policy set staying an odrl:Offer.
 *
 * The fixture is authored by clicking a Payment component (with an inline
 * fillable placeholder) through the real template lifecycle and deriving a
 * DRAFT contract from it — so it always carries whatever shape the current
 * authoring emits.
 */

interface Placeholder {
  '@id'?: string
  'dcs:label'?: string
  'dcs:value'?: unknown
}

let fixture: DraftContractFixture

test.beforeAll(async ({ browser }) => {
  test.setTimeout(180_000)
  fixture = await buildDraftContractFixture(browser)
})

test('filling a placeholder writes the value inline on the placeholder of an odrl:Offer', async ({ page, loginAs }) => {
  await loginAs('Contract Creator')
  await page.goto(`/ui/contracts/edit/${fixture.contractDid}`)

  // The fill inputs live under the Contract Content tab; the authored fixture
  // carries one placeholder for the Payment Amount value, and the input is
  // labeled with the placeholder's dcs:label (a decimal renders a spinbutton).
  await page
    .getByRole('tab', { name: /content/i })
    .or(page.getByText('Contract Content', { exact: true }))
    .first()
    .click()
  const input = page
    .getByRole('spinbutton', { name: /amount/i })
    .or(page.getByRole('textbox', { name: /amount/i }))
    .first()
  await expect(input).toBeVisible()
  await input.fill('250')

  const updateRequest = page.waitForRequest((r) => r.url().includes('/contract/update') && r.method() === 'PUT')
  await page.getByRole('button', { name: 'Update', exact: true }).click()
  const payload = (await updateRequest).postDataJSON() as {
    contract_data: {
      'dcs:contractFields'?: Placeholder[]
      'dcs:policies': { '@type': string }
    }
  }

  const placeholders = payload.contract_data['dcs:contractFields'] ?? []
  const amount = placeholders.find((ph) => /amount/i.test(ph['dcs:label'] ?? ''))
  expect(amount, 'the payment amount placeholder is in the document').toBeTruthy()
  // The value lives inline on the field, not in a separate values array —
  // serialized as a typed literal carrying the field's declared datatype,
  // with the exact lexical token the user agreed to.
  expect(amount!['dcs:value'], 'the filled value is carried inline as a typed literal').toEqual({
    '@value': '250',
    '@type': 'xsd:decimal',
  })

  expect(payload.contract_data['dcs:policies']['@type'], 'unsigned contracts stay an odrl:Offer').toBe('odrl:Offer')
})
