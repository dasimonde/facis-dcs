import { expect, type Page, type Route, test } from '@playwright/test'

const odrlProfileFixture = `
  @prefix dcs: <https://w3id.org/facis/dcs/ontology/v1#> .
  @prefix odrl: <http://www.w3.org/ns/odrl/2/> .
  @prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
  <https://w3id.org/facis/dcs/ontology/v1/odrl-profile>
    a odrl:Profile ;
    dcs:defaultConstraintAction dcs:provideCompliantValue .
  odrl:eq
    a odrl:Operator ;
    rdfs:label "Must equal" ;
    dcs:appliesToParameterType "string" .
`

const contractDid = 'did:web:example.test:contracts:semantic-pin-carry-over'
const pinnedBundle = {
  '@context': [
    'https://dcs.example.test/semantic/context/facis-dcs?version=7',
    { dcs: 'https://w3id.org/facis/dcs/ontology/v1#' },
  ],
  'sh:shapesGraph': [
    { '@id': 'https://dcs.example.test/semantic/shapes/facis-dcs?version=11' },
    { '@id': 'https://dcs.example.test/semantic/shapes/payment-library?version=3' },
  ],
  'dcs:effectiveShapes': [
    {
      '@id': 'https://dcs.example.test/semantic/shapes/facis-dcs?version=11',
      'dcs:digest': 'sha256:canonical',
    },
    {
      '@id': 'https://dcs.example.test/semantic/shapes/payment-library?version=3',
      'dcs:digest': 'sha256:payment',
    },
  ],
  'dcterms:conformsTo': {
    '@id': 'https://dcs.example.test/semantic/profile/facis-dcs?version=5',
  },
}

function jwt(role: string): string {
  const encode = (value: object) => Buffer.from(JSON.stringify(value)).toString('base64url')
  return `${encode({ alg: 'none', typ: 'JWT' })}.${encode({
    sub: 'did:web:creator.example.test',
    exp: Math.floor(Date.now() / 1000) + 3600,
    roles: [role],
    ext: { iss: 'did:web:example.test', roles: [role] },
  })}.unsigned`
}

async function json(route: Route, body: unknown, status = 200): Promise<void> {
  await route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })
}

async function authenticate(page: Page): Promise<void> {
  const accessToken = jwt('Contract Creator')
  await page.addInitScript(
    ([token]) => {
      localStorage.setItem('token_type', 'Bearer')
      localStorage.setItem('access_token', token)
    },
    [accessToken],
  )
  await page.route('**/auth/refresh', (route) => json(route, { token_type: 'Bearer', access_token: accessToken }))
  await page.route('**/semantic/schema/list', (route) => json(route, []))
  await page.route('**/semantic/ontology/dcs-odrl-profile', (route) => json(route, { content: odrlProfileFixture }))
}

test('Retrieve-to-update carries the immutable semantic bundle without modification', async ({ page }) => {
  await authenticate(page)
  const retrievedDocument = {
    '@id': contractDid,
    '@type': 'dcs:Contract',
    ...pinnedBundle,
    'dcs:metadata': {
      '@type': 'dcs:ContractMetadata',
      'dcs:title': 'Pinned draft',
      'dcs:description': 'A draft whose server-owned semantic bundle must survive editing.',
    },
    'dcs:documentStructure': {
      '@type': 'dcs:DocumentStructure',
      'dcs:blocks': { '@list': [] },
      'dcs:layout': { '@list': [] },
    },
    'dcs:contractData': [],
    'dcs:contractFields': [],
    'dcs:policies': {
      '@id': `${contractDid}#policy`,
      '@type': 'odrl:Offer',
      'odrl:profile': { '@id': 'https://w3id.org/facis/dcs/ontology/v1/odrl-profile' },
    },
  }
  const retrievedContract = {
    did: contractDid,
    state: 'DRAFT',
    name: 'Pinned draft',
    description: 'A draft whose server-owned semantic bundle must survive editing.',
    created_by: 'contract-creator',
    created_at: '2026-07-31T08:00:00Z',
    updated_at: '2026-07-31T08:00:00Z',
    contract_data: retrievedDocument,
  }

  await page.route('**/contract/retrieve**', (route) => {
    const path = new URL(route.request().url()).pathname
    if (path.endsWith('/contract/retrieve')) {
      return json(route, {
        contracts: [retrievedContract],
        review_tasks: [],
        approval_tasks: [],
        negotiation_tasks: [],
      })
    }
    return json(route, retrievedContract)
  })
  await page.route('**/contract/update', (route) =>
    json(route, { did: contractDid, updated_at: '2026-07-31T08:01:00Z' }),
  )

  await page.goto(`/ui/contracts/edit/${encodeURIComponent(contractDid)}`)
  await expect(page.getByTestId('contract-global-name')).toHaveValue('Pinned draft')

  const updateRequest = page.waitForRequest(
    (request) => request.url().includes('/contract/update') && request.method() === 'PUT',
  )
  await page.getByRole('button', { name: 'Update', exact: true }).click()
  const payload = (await updateRequest).postDataJSON() as { contract_data: Record<string, unknown> }

  for (const field of ['@context', 'sh:shapesGraph', 'dcs:effectiveShapes', 'dcterms:conformsTo'] as const) {
    expect(payload.contract_data[field], `${field} is unchanged between retrieve and update`).toEqual(
      retrievedDocument[field],
    )
  }
})
