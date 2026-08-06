import { type DcsRole, expect, test } from './dcs-test'
import type { Page } from '@playwright/test'

type LoginAs = (role: DcsRole) => Promise<void>

async function gotoAs(page: Page, loginAs: LoginAs, role: DcsRole, url: string): Promise<void> {
  await loginAs(role)
  await page.goto(url)
}

async function fillRequiredTemplateDetails(page: Page, name: string): Promise<void> {
  await page.getByTestId('template-global-name').fill(name)
  await page.getByTestId('template-base-description').fill('Playwright clause-editor fixture.')
}

async function selectBilateralRoles(editor: ReturnType<Page['getByTestId']>): Promise<void> {
  const roleSelect = (label: string) =>
    editor.locator('label.form-control').filter({ hasText: label }).locator('select')
  const assigner = roleSelect('Granted by')
  const assignee = roleSelect('Applies to')
  const values = await assigner
    .locator('option:not([disabled])')
    .evaluateAll((options) => options.map((option) => (option as HTMLOptionElement).value).filter(Boolean))
  expect(values.length, 'the role catalog exposes a bilateral choice').toBeGreaterThanOrEqual(2)
  const [assignerRole, assigneeRole] = values
  if (!assignerRole || !assigneeRole) throw new Error('the role catalog does not expose two roles')
  await assigner.selectOption(assignerRole)
  await assignee.selectOption(assigneeRole)
}

interface Ref {
  '@id': string
}
interface Constraint {
  'odrl:leftOperand': Ref
  'odrl:operator': Ref
  'odrl:rightOperand'?: Ref | { '@value': string; '@type': string }
}
interface Rule {
  '@type': string
  'odrl:action': Ref
  'odrl:target': Ref
  'odrl:constraint'?: Constraint[]
}
interface TemplateData {
  'dcs:policies': { 'odrl:permission'?: Rule[] }
  'dcs:contractFields': { '@id': string }[]
  'dcs:documentStructure': { 'dcs:blocks': { '@list': { 'dcs:title'?: string }[] } }
}

// The SRS Appendix C policy is not hard-coded — the DCS produces a reusable
// access-grant *template* whose spatial and temporal boundaries are negotiated
// fields (the permitted country, the access deadline). Filling those fields at
// contract negotiation with "DE" and the deadline yields exactly the Appendix C
// TempoSpatialAccess agreement. The clause carries the human prose beside its
// machine-readable ODRL meaning, terms picked and never typed.
test('an exhaustive access-grant template lets the Appendix C policy be negotiated', async ({ page, loginAs }) => {
  page.setDefaultTimeout(15_000)
  const clauseTitle = `API access grant ${Date.now()}`

  await gotoAs(page, loginAs, 'Template Creator', '/ui/templates/new')
  await page.getByRole('button', { name: /Component/ }).click()
  await fillRequiredTemplateDetails(page, `FV Access ${Date.now()}`)
  await page.getByRole('tab', { name: /Clauses/ }).click()

  const editor = page.getByTestId('split-clause-editor')
  await editor.getByPlaceholder('Clause title').fill(clauseTitle)

  // Hub objects: the two negotiated boundaries, plus the accessed asset that
  // the permission targets (Appendix C's ShowtimesAPI). Boundaries are picked
  // first so their rightSource option indices (1, 2) stay stable.
  const fieldPicker = editor.locator('select').first()
  await fieldPicker.selectOption({ label: 'Company Country' })
  await fieldPicker.selectOption({ label: 'Validity End Date' })
  await fieldPicker.selectOption({ label: 'Service Description' })

  await editor.locator('.clause-editor').first().click()
  await page.keyboard.type('The assignee may access the service within the agreed country until the agreed date.')

  const ruleSelect = (label: string) =>
    editor.locator('label.form-control').filter({ hasText: label }).locator('select')
  await ruleSelect('Rule').selectOption({ label: 'Permission: the assignee MAY' })
  await ruleSelect('Action').selectOption({ label: 'use' })
  // Party roles are supplied by the Semantic Hub catalog. Pick two distinct
  // enabled catalog entries without coupling this test to a particular role
  // vocabulary or its ordering labels.
  const assigner = ruleSelect('Granted by')
  const assignee = ruleSelect('Applies to')
  const partyRoleValues = await assigner
    .locator('option:not([disabled])')
    .evaluateAll((options) => options.map((option) => (option as HTMLOptionElement).value).filter(Boolean))
  expect(partyRoleValues.length, 'the role catalog exposes a bilateral choice').toBeGreaterThanOrEqual(2)
  const [assignerRole, assigneeRole] = partyRoleValues
  if (!assignerRole || !assigneeRole) throw new Error('the role catalog does not expose two roles')
  await assigner.selectOption(assignerRole)
  await assignee.selectOption(assigneeRole)
  // The target is the accessed asset — a declared object, not the contract.
  await ruleSelect('Toward').selectOption({ label: 'Service Description' })

  // Each context constraint's boundary is one of the negotiated fields
  // (option index 0 is "a fixed value"; 1 and 2 are the two picked fields).
  const addContext = async (operand: string, operator: string, boundaryIndex: number) => {
    await editor.getByRole('button', { name: '+ constraint' }).click()
    const row = editor.locator('.flex.flex-wrap.items-center.gap-1').last()
    await row.locator('select').nth(0).selectOption({ label: operand })
    await row.locator('select').nth(1).selectOption({ label: operator })
    await row.locator('select').nth(2).selectOption({ index: boundaryIndex })
  }
  await addContext('access region (spatial)', 'equal to', 1)
  await addContext('access time (dateTime)', 'less than or equal to', 2)

  await editor.getByRole('button', { name: 'Add clause', exact: true }).click()
  await expect(editor.getByPlaceholder('Clause title')).toHaveValue('')

  const created = page.waitForRequest((r) => r.url().includes('/template/create') && r.method() === 'POST')
  await page.getByRole('button', { name: 'Create', exact: true }).click()
  const doc = (await created).postDataJSON().template_data as TemplateData

  expect(
    doc['dcs:documentStructure']['dcs:blocks']['@list'].some((b) => b['dcs:title'] === clauseTitle),
    'prose block persisted',
  ).toBeTruthy()

  // The two boundaries are declared as fillable typed contract fields.
  const fieldIds = new Set(doc['dcs:contractFields'].map((field) => field['@id']))
  expect(fieldIds.size, 'two negotiated fields declared').toBeGreaterThanOrEqual(2)

  // The Permission's spatial + temporal constraints reference those fields as
  // their boundaries — resolved to the filled values at negotiation.
  const permission = (doc['dcs:policies']['odrl:permission'] ?? []).find(
    (r) => r['@type'] === 'odrl:Permission' && r['odrl:action']['@id'] === 'odrl:use',
  )
  expect(permission, 'a Permission to use').toBeTruthy()

  // The permission targets the accessed asset (a declared object), the way
  // Appendix C targets the ShowtimesAPI — not the contract itself.
  expect(fieldIds.has(permission!['odrl:target']['@id']), 'target is the declared asset object').toBeTruthy()

  const byOperand = new Map((permission!['odrl:constraint'] ?? []).map((c) => [c['odrl:leftOperand']['@id'], c]))
  const spatial = byOperand.get('odrl:spatial')
  const dateTime = byOperand.get('odrl:dateTime')
  expect(spatial?.['odrl:operator']['@id'], 'spatial eq').toBe('odrl:eq')
  expect(
    fieldIds.has((spatial?.['odrl:rightOperand'] as Ref)?.['@id']),
    'spatial boundary is a negotiated field',
  ).toBeTruthy()
  expect(dateTime?.['odrl:operator']['@id'], 'dateTime lteq').toBe('odrl:lteq')
  expect(
    fieldIds.has((dateTime?.['odrl:rightOperand'] as Ref)?.['@id']),
    'dateTime boundary is a negotiated field',
  ).toBeTruthy()
})

// The builder generates an ODRL LogicalConstraint (IM §2.6) when constraints
// are combined with anything other than ALL: two spatial constraints under
// "ANY may hold" emit a single odrl:or over both.
test('the builder emits a logical (or) constraint when constraints are combined with ANY', async ({
  page,
  loginAs,
}) => {
  page.setDefaultTimeout(15_000)
  await gotoAs(page, loginAs, 'Template Creator', '/ui/templates/new')
  await page.getByRole('button', { name: /Component/ }).click()
  await fillRequiredTemplateDetails(page, `FV Logical ${Date.now()}`)
  await page.getByRole('tab', { name: /Clauses/ }).click()

  const editor = page.getByTestId('split-clause-editor')
  await editor.getByPlaceholder('Clause title').fill(`Logical or ${Date.now()}`)
  await editor.locator('.clause-editor').first().click()
  await page.keyboard.type('The assignee may access from an allowed region.')

  const ruleSelect = (label: string) =>
    editor.locator('label.form-control').filter({ hasText: label }).locator('select')
  await ruleSelect('Rule').selectOption({ label: 'Permission: the assignee MAY' })
  await ruleSelect('Action').selectOption({ label: 'use' })
  await selectBilateralRoles(editor)

  const addSpatial = async (value: string) => {
    await editor.getByRole('button', { name: '+ constraint' }).click()
    const row = editor.locator('.flex.flex-wrap.items-center.gap-1').last()
    await row.locator('select').nth(0).selectOption({ label: 'access region (spatial)' })
    await row.locator('select').nth(1).selectOption({ label: 'equal to' })
    await row.locator('select').nth(3).selectOption({ label: value })
  }
  await addSpatial('Germany (DEU)')
  await addSpatial('Austria (AUT)')
  await editor.locator('select[title="How the constraints combine"]').selectOption('or')

  await editor.getByRole('button', { name: 'Add clause', exact: true }).click()
  const created = page.waitForRequest((r) => r.url().includes('/template/create') && r.method() === 'POST')
  await page.getByRole('button', { name: 'Create', exact: true }).click()
  const doc = (await created).postDataJSON().template_data as {
    'dcs:policies': { 'odrl:permission'?: { 'odrl:constraint'?: unknown[] }[] }
  }

  const constraints = (doc['dcs:policies']['odrl:permission'] ?? [])[0]?.['odrl:constraint'] ?? []
  expect(constraints.length, 'a single logical constraint node').toBe(1)
  const logical = constraints[0] as { '@type': string; 'odrl:or'?: { '@list': unknown[] } }
  expect(logical['@type']).toBe('odrl:LogicalConstraint')
  expect(logical['odrl:or']?.['@list']?.length, 'or over both spatial constraints').toBe(2)
})

// A Permission may carry nested duties (ODRL IM §2.5): obligations the assignee
// must fulfil to exercise it. The builder emits odrl:duty, each a Duty fragment
// with its own action and constraints.
test('a Permission can carry a nested duty the assignee must fulfil', async ({ page, loginAs }) => {
  page.setDefaultTimeout(15_000)
  await gotoAs(page, loginAs, 'Template Creator', '/ui/templates/new')
  await page.getByRole('button', { name: /Component/ }).click()
  await fillRequiredTemplateDetails(page, `FV Duty ${Date.now()}`)
  await page.getByRole('tab', { name: /Clauses/ }).click()

  const editor = page.getByTestId('split-clause-editor')
  await editor.getByPlaceholder('Clause title').fill(`Duty clause ${Date.now()}`)
  await editor.locator('.clause-editor').first().click()
  await page.keyboard.type('The assignee may use the asset, and must delete it in an allowed region afterwards.')

  const ruleSelect = (label: string) =>
    editor.locator('label.form-control').filter({ hasText: label }).locator('select')
  await ruleSelect('Rule').selectOption({ label: 'Permission: the assignee MAY' })
  await ruleSelect('Action').selectOption({ label: 'use' })
  await selectBilateralRoles(editor)

  // Attach a duty: the assignee MUST delete, bounded by two of the duty's own
  // constraints combined with ANY — a duty is as expressive as a rule.
  await editor.getByRole('button', { name: '+ duty' }).click()
  const duty = editor.getByTestId('odrl-duty').last()
  await duty.locator('select').first().selectOption({ label: 'delete' })
  const addDutyRegion = async (value: string) => {
    await duty.getByRole('button', { name: '+ constraint' }).click()
    const row = duty.locator('.flex.flex-wrap.items-center.gap-1').last()
    await row.locator('select').nth(0).selectOption({ label: 'access region (spatial)' })
    await row.locator('select').nth(1).selectOption({ label: 'equal to' })
    await row.locator('select').nth(3).selectOption({ label: value })
  }
  await addDutyRegion('Germany (DEU)')
  await addDutyRegion('Austria (AUT)')
  await duty.locator('select[title="How the duty\'s constraints combine"]').selectOption('or')

  await editor.getByRole('button', { name: 'Add clause', exact: true }).click()
  const created = page.waitForRequest((r) => r.url().includes('/template/create') && r.method() === 'POST')
  await page.getByRole('button', { name: 'Create', exact: true }).click()
  const doc = (await created).postDataJSON().template_data as {
    'dcs:policies': {
      'odrl:permission'?: {
        'dcs:prose': { '@id': string }
        'odrl:duty'?: {
          '@type': string
          'dcs:prose'?: { '@id': string }
          'odrl:action': { '@id': string }
          'odrl:constraint'?: { '@type': string; 'odrl:or'?: { '@list': unknown[] } }[]
        }[]
      }[]
    }
  }

  const permission = (doc['dcs:policies']['odrl:permission'] ?? [])[0]
  const duties = permission?.['odrl:duty'] ?? []
  expect(duties.length, 'one duty attached to the permission').toBe(1)
  expect(duties[0]?.['@type']).toBe('odrl:Duty')
  // A nested duty is an odrl:Duty like any other, so the hub shapes demand the
  // prose it is backed by: the clause it was authored in, the same block its
  // permission cites. Without it the template is refused at submit.
  expect(duties[0]?.['dcs:prose']?.['@id'], 'the duty is backed by the clause it was authored in').toBe(
    permission?.['dcs:prose']['@id'],
  )
  expect(duties[0]?.['odrl:action']['@id'], 'the duty action').toBe('odrl:delete')
  // The duty's two constraints combined into a single logical (or) node.
  const dutyConstraints = duties[0]?.['odrl:constraint'] ?? []
  expect(dutyConstraints.length, 'a single logical constraint node on the duty').toBe(1)
  expect(dutyConstraints[0]?.['@type']).toBe('odrl:LogicalConstraint')
  expect(dutyConstraints[0]?.['odrl:or']?.['@list']?.length, 'or over both duty constraints').toBe(2)
})

// The builder authors an arbitrarily deep constraint tree (ODRL IM §2.6): a
// top-level ALL conjunction holding one atomic constraint and a nested ANY
// group emits [Constraint, LogicalConstraint(or, [·, ·])].
test('the builder authors a nested constraint tree (and over an or-group)', async ({ page, loginAs }) => {
  page.setDefaultTimeout(15_000)
  await gotoAs(page, loginAs, 'Template Creator', '/ui/templates/new')
  await page.getByRole('button', { name: /Component/ }).click()
  await fillRequiredTemplateDetails(page, `FV Tree ${Date.now()}`)
  await page.getByRole('tab', { name: /Clauses/ }).click()

  const editor = page.getByTestId('split-clause-editor')
  await editor.getByPlaceholder('Clause title').fill(`Nested tree ${Date.now()}`)
  await editor.locator('.clause-editor').first().click()
  await page.keyboard.type('The assignee may use the asset within a purpose and one of two allowed regions.')

  const ruleSelect = (label: string) =>
    editor.locator('label.form-control').filter({ hasText: label }).locator('select')
  await ruleSelect('Rule').selectOption({ label: 'Permission: the assignee MAY' })
  await ruleSelect('Action').selectOption({ label: 'use' })
  await selectBilateralRoles(editor)

  // A top-level atomic constraint (purpose), then a nested group of two spatial
  // constraints combined with ANY — while the top level stays ALL.
  await editor.getByRole('button', { name: '+ constraint' }).click()
  const topRow = editor.locator('.flex.flex-wrap.items-center.gap-1').last()
  await topRow.locator('select').nth(0).selectOption({ label: 'purpose' })
  await topRow.locator('select').nth(1).selectOption({ label: 'equal to' })
  await topRow.locator('select').nth(3).selectOption({ label: 'Research (RESEARCH)' })

  await editor.getByRole('button', { name: '+ group' }).click()
  const group = editor.locator('.border-dashed').last()
  const addGroupRegion = async (value: string) => {
    await group.getByRole('button', { name: '+ constraint' }).click()
    const row = group.locator('.flex.flex-wrap.items-center.gap-1').last()
    await row.locator('select').nth(0).selectOption({ label: 'access region (spatial)' })
    await row.locator('select').nth(1).selectOption({ label: 'equal to' })
    await row.locator('select').nth(3).selectOption({ label: value })
  }
  await addGroupRegion('Germany (DEU)')
  await addGroupRegion('Austria (AUT)')
  await group.locator('select[title="How this group combines"]').selectOption('or')

  await editor.getByRole('button', { name: 'Add clause', exact: true }).click()
  const created = page.waitForRequest((r) => r.url().includes('/template/create') && r.method() === 'POST')
  await page.getByRole('button', { name: 'Create', exact: true }).click()
  const doc = (await created).postDataJSON().template_data as {
    'dcs:policies': {
      'odrl:permission'?: {
        'odrl:constraint'?: {
          '@type': string
          'odrl:leftOperand'?: { '@id': string }
          'odrl:or'?: { '@list': unknown[] }
        }[]
      }[]
    }
  }

  const constraints = (doc['dcs:policies']['odrl:permission'] ?? [])[0]?.['odrl:constraint'] ?? []
  expect(constraints.length, 'top-level conjunction of two nodes').toBe(2)
  const atomic = constraints.find((c) => c['@type'] === 'odrl:Constraint')
  const logical = constraints.find((c) => c['@type'] === 'odrl:LogicalConstraint')
  expect(atomic?.['odrl:leftOperand']?.['@id'], 'the top-level atomic is the purpose constraint').toBe('odrl:purpose')
  expect(logical, 'a nested logical constraint node').toBeTruthy()
  expect(logical?.['odrl:or']?.['@list']?.length, 'the nested or holds both regions').toBe(2)
})

// A duty carries a consequence (ODRL IM §2.5): a further duty triggered when
// the duty is not fulfilled. The builder emits odrl:consequence.
test('a duty can carry a consequence duty', async ({ page, loginAs }) => {
  page.setDefaultTimeout(15_000)
  await gotoAs(page, loginAs, 'Template Creator', '/ui/templates/new')
  await page.getByRole('button', { name: /Component/ }).click()
  await fillRequiredTemplateDetails(page, `FV Conseq ${Date.now()}`)
  await page.getByRole('tab', { name: /Clauses/ }).click()

  const editor = page.getByTestId('split-clause-editor')
  await editor.getByPlaceholder('Clause title').fill(`Consequence ${Date.now()}`)
  await editor.locator('.clause-editor').first().click()
  await page.keyboard.type('The assignee may use the asset and must delete it; failing that, must attribute.')

  const ruleSelect = (label: string) =>
    editor.locator('label.form-control').filter({ hasText: label }).locator('select')
  await ruleSelect('Rule').selectOption({ label: 'Permission: the assignee MAY' })
  await ruleSelect('Action').selectOption({ label: 'use' })
  await selectBilateralRoles(editor)

  await editor.getByRole('button', { name: '+ duty' }).click()
  const duty = editor.getByTestId('odrl-duty').last()
  await duty.locator('select').first().selectOption({ label: 'delete' })
  await duty.getByRole('button', { name: '+ consequence' }).click()
  const consequence = duty.getByTestId('odrl-consequence').last()
  await consequence.locator('select').first().selectOption({ label: 'display' })

  await editor.getByRole('button', { name: 'Add clause', exact: true }).click()
  const created = page.waitForRequest((r) => r.url().includes('/template/create') && r.method() === 'POST')
  await page.getByRole('button', { name: 'Create', exact: true }).click()
  const doc = (await created).postDataJSON().template_data as {
    'dcs:policies': {
      'odrl:permission'?: {
        'dcs:prose': { '@id': string }
        'odrl:duty'?: {
          'odrl:action': { '@id': string }
          'odrl:consequence'?: { '@type': string; 'dcs:prose'?: { '@id': string }; 'odrl:action': { '@id': string } }[]
        }[]
      }[]
    }
  }

  const permission = (doc['dcs:policies']['odrl:permission'] ?? [])[0]
  const duties = permission?.['odrl:duty'] ?? []
  expect(duties[0]?.['odrl:action']['@id'], 'the duty action').toBe('odrl:delete')
  const consequences = duties[0]?.['odrl:consequence'] ?? []
  expect(consequences.length, 'one consequence duty').toBe(1)
  expect(consequences[0]?.['@type']).toBe('odrl:Duty')
  expect(consequences[0]?.['odrl:action']['@id'], 'the consequence action').toBe('odrl:display')
  // A consequence is a Duty too, so it owes prose as well.
  expect(consequences[0]?.['dcs:prose']?.['@id'], 'the consequence is backed by the same clause').toBe(
    permission?.['dcs:prose']['@id'],
  )
})
