import { expect, test } from './dcs-test'
import { deriveLocalContract, gotoAs, registerContractTemplate, submitReviewApproveTemplate } from './lifecycle-helpers'

/**
 * Semantic data objects, clicked into existence (ADR-23 / DCS-FR-TR-25):
 * a Template Creator registers an external SHACL library in the Semantic
 * Hub, clicks a LegalPerson into a contract template's data graph, fixes
 * its registration number, nests a linked Address, and marks the address's
 * country negotiable. The derived contract presents the agreed graph and
 * takes the country fill — which lands as a typed literal on the declared
 * field while the object graph keeps referencing it by @id.
 */

const LEGAL_VOCAB = 'https://example.org/legal#'

const LEGAL_SHAPES_TTL = `
@prefix sh:  <http://www.w3.org/ns/shacl#> .
@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .
@prefix ex:  <${LEGAL_VOCAB}> .

ex:LegalPersonShape a sh:NodeShape ;
    sh:targetClass ex:LegalPerson ;
    sh:property [ sh:path ex:registrationNumber ; sh:datatype xsd:string ; sh:minCount 1 ; sh:maxCount 1 ] ;
    sh:property [ sh:path ex:legalForm ; sh:in ( "GmbH" "AG" "SE" ) ; sh:maxCount 1 ] ;
    sh:property [ sh:path ex:website ; sh:nodeKind sh:IRI ; sh:maxCount 1 ] ;
    sh:property [ sh:path ex:legalAddress ; sh:class ex:Address ; sh:minCount 1 ; sh:maxCount 1 ] .

ex:AddressShape a sh:NodeShape ;
    sh:targetClass ex:Address ;
    sh:property [ sh:path ex:countryName ; sh:datatype xsd:string ; sh:minCount 1 ; sh:maxCount 1 ] .

ex:MarkerShape a sh:NodeShape ;
    sh:targetClass ex:Marker .
`

interface AuthoredDocument {
  'dcs:contractFields': {
    '@id': string
    'dcs:label': string
    'dcs:datatype': string
    'dcs:required': boolean
    'dcs:value'?: unknown
  }[]
  'dcs:contractData': Record<string, unknown>[]
}

function objectOfType(doc: AuthoredDocument, classIri: string): Record<string, unknown> | undefined {
  return doc['dcs:contractData'].find((entry) => entry['@type'] === classIri)
}

test('a LegalPerson data object is clicked into a template and filled in the contract', async ({ page, loginAs }) => {
  test.setTimeout(420_000)
  const stamp = Date.now()
  const templateName = `Legal Data Template ${stamp}`

  await test.step('an external SHACL library is registered in the Semantic Hub', async () => {
    await loginAs('Template Manager')
    await page.goto('/ui/semantic-hub')
    const token = await page.evaluate(() => localStorage.getItem('access_token'))
    const registered = await page.request.post('/api/semantic/schema/register', {
      data: {
        name: 'e2e-legal-shapes',
        kind: 'shapes',
        media_type: 'text/turtle',
        content: LEGAL_SHAPES_TTL,
        activate: true,
      },
      headers: { Authorization: `Bearer ${token}` },
    })
    expect(registered.ok(), await registered.text()).toBeTruthy()
  })

  let templateDid = ''
  await test.step('a LegalPerson with a nested Address is clicked into a contract template', async () => {
    await gotoAs(page, loginAs, 'Template Creator', '/ui/templates/new')
    await page.getByRole('button', { name: /parent for other contracts/ }).click()
    await page.getByRole('group').filter({ hasText: 'Global Name' }).getByRole('textbox').fill(templateName)
    await page
      .getByRole('group')
      .filter({ hasText: 'Base Description' })
      .getByRole('textbox')
      .fill('Fixture for shape-driven data-object authoring.')

    await page.getByRole('tab', { name: 'Data', exact: true }).click()
    const editor = page.getByTestId('data-objects-editor')
    const classPicker = editor.getByTestId('data-object-class')
    // A marker shape (targetClass, zero property shapes) stays pickable — other
    // shapes reference such classes via sh:node.
    await expect(classPicker.locator(`option[value="${LEGAL_VOCAB}Marker"]`)).toBeAttached()
    await classPicker.selectOption(`${LEGAL_VOCAB}LegalPerson`)
    await editor.getByTestId('add-data-object').click()

    const person = editor.getByTestId('data-object-LegalPerson')
    await expect(person).toBeVisible()
    await person.getByTestId('literal-registrationNumber').fill('HRB 4711')
    // sh:in renders a select; sh:nodeKind sh:IRI takes an external IRI.
    await person.getByTestId('literal-legalForm').selectOption('GmbH')
    await person.getByTestId('iri-website').fill('https://musterfirma.example.org/')
    await person.getByTestId('add-nested-legalAddress').click()

    const address = person.getByTestId('data-object-Address')
    await expect(address).toBeVisible()
    await address.getByTestId('toggle-negotiable-countryName').check()
    await expect(address.getByTestId('negotiable-countryName')).toBeVisible()

    const created = page.waitForRequest((r) => r.url().includes('/template/create') && r.method() === 'POST')
    const response = page.waitForResponse(
      (r) => r.url().includes('/template/create') && r.request().method() === 'POST',
    )
    await page.getByRole('button', { name: 'Create', exact: true }).click()
    const createResp = await response
    expect(createResp.ok(), await createResp.text()).toBeTruthy()
    templateDid = ((await createResp.json()) as { did: string }).did

    const doc = (await created).postDataJSON().template_data as AuthoredDocument
    const personNode = objectOfType(doc, `${LEGAL_VOCAB}LegalPerson`)
    const addressNode = objectOfType(doc, `${LEGAL_VOCAB}Address`)
    expect(personNode, 'the LegalPerson is in the document graph').toBeTruthy()
    expect(addressNode, 'the nested Address is in the document graph').toBeTruthy()
    // xsd:string emits RDF 1.1's simple-literal form so external validators
    // term-match it against plain sh:in members.
    expect(personNode![`${LEGAL_VOCAB}registrationNumber`], 'the fixed string literal is bare').toBe('HRB 4711')
    expect(personNode![`${LEGAL_VOCAB}legalForm`], 'the sh:in choice is a bare string').toBe('GmbH')
    expect(personNode![`${LEGAL_VOCAB}website`], 'the IRI leaf names the external resource').toEqual({
      '@id': 'https://musterfirma.example.org/',
    })
    expect(personNode![`${LEGAL_VOCAB}legalAddress`], 'the parent references its nested object by @id').toEqual({
      '@id': addressNode!['@id'],
    })
    const countryRef = addressNode![`${LEGAL_VOCAB}countryName`] as { '@id': string }
    const field = doc['dcs:contractFields'].find((entry) => entry['@id'] === countryRef['@id'])
    expect(field, 'the negotiable leaf declared a contract field the address references').toBeTruthy()
    expect(field!['dcs:datatype']).toBe('xsd:string')
    expect(field!['dcs:required']).toBe(true)
  })

  await test.step('the template reaches REGISTERED', async () => {
    await submitReviewApproveTemplate(page, loginAs, templateDid, templateName)
    await registerContractTemplate(page, loginAs, templateDid, templateName)
  })

  let contractDid = ''
  await test.step('the derived contract takes the country fill on the negotiable leaf', async () => {
    contractDid = await deriveLocalContract(page, loginAs, templateName)
    await gotoAs(page, loginAs, 'Contract Creator', `/ui/contracts/edit/${contractDid}`)
    await page.getByRole('tab', { name: 'Contract Content' }).click()

    const editor = page.getByTestId('data-objects-editor')
    await expect(editor.getByTestId('data-object-LegalPerson')).toBeVisible()
    await editor.getByTestId('fill-countryName').fill('DE')

    const updated = page.waitForRequest((r) => r.url().includes('/contract/update') && r.method() === 'PUT')
    const response = page.waitForResponse((r) => r.url().includes('/contract/update') && r.request().method() === 'PUT')
    await page.getByRole('button', { name: 'Update', exact: true }).click()
    const updateResp = await response
    expect(updateResp.ok(), await updateResp.text()).toBeTruthy()

    const doc = (await updated).postDataJSON().contract_data as AuthoredDocument
    const addressNode = objectOfType(doc, `${LEGAL_VOCAB}Address`)
    expect(addressNode, 'the contract carries the agreed Address object').toBeTruthy()
    const countryRef = addressNode![`${LEGAL_VOCAB}countryName`] as { '@id': string }
    const field = doc['dcs:contractFields'].find((entry) => entry['@id'] === countryRef['@id'])
    expect(field, 'the address still references the declared field by @id').toBeTruthy()
    expect(field!['dcs:value'], 'the fill is a typed literal of the declared datatype').toEqual({
      '@value': 'DE',
      '@type': 'xsd:string',
    })
  })
})
