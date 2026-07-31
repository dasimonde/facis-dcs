import { expect, test } from './dcs-test'
import {
  acceptPeerProposalOn,
  assertReceivedInState,
  authorContractTemplate,
  contractDocumentOn,
  counterOffer,
  createContractViaUi,
  expectSubmitRefusedOn,
  fillContractValuesOn,
  instanceA,
  offerToCounterparty,
  openInstanceB,
  publishHubShapesOn,
  publishTemplateOn,
  registerCatalogueTemplateOn,
  registerTemplateOn,
  resolveDidWeb,
  submitReviewApproveTemplateOn,
} from './multi-dcs-helpers'
import { E2E_FRONTEND_ORIGIN } from '../playwright.config'

/**
 * An SLA is not hard-coded anywhere in the DCS: the service-level vocabulary
 * enters instance A at runtime as a SHACL library in its Semantic Hub, a
 * Template Creator clicks two of its classes into a reusable template as typed
 * domain objects whose leaves are negotiated at contract time, and the clause
 * that governs them carries the human prose beside its machine-readable ODRL
 * meaning — a permission over a nested constraint tree, a duty bounded by a
 * negotiated field, and the duty's consequence. The template then crosses the
 * Federated Catalogue to instance B, which registers it, derives a contract
 * from it, and finds A's availability floor enforced against its own fill.
 *
 * What this spec exists to prove, and nothing more (the two-instance vertical
 * already covers signing, provenance chains, deployment and audit for a
 * federated contract):
 *   1. a domain vocabulary imported into a running instance drives authoring,
 *   2. the ODRL a builder can author reaches the document intact,
 *   3. a template published by A becomes B's own through the catalogue,
 *   4. the policy A authored refuses B's non-conforming value — and survives a
 *      negotiation round, which moves values and never policy structure.
 *
 * SRS traceability: DCS-FR-TR-03 (Semantic Hub schema storage), DCS-FR-TR-25 /
 * ADR-23 (typed contract data from registered shape libraries), DCS-IR-TR-01
 * (template builder), DCS-IR-SI-01 / DCS-IR-TR-07 (catalogue integration and
 * template registration), DCS-FR-CWE-16 (contract creation), DCS-FR-CWE-18 /
 * DCS-IR-CWE-03 (negotiation and redlines), ADR-11 (ODRL evaluation), ADR-13
 * (PDF-exchange federation).
 */

// One long federated flow: a retry re-runs the whole thing to the same failure
// and doubles the suite's wall-clock, so it opts out of the CI retry the way
// the two-instance vertical does.
test.describe.configure({ retries: 0 })

const stamp = Date.now()
const SLA_VOCAB = `https://example.org/sla/${stamp}#`
const SLA_CLASS = `${SLA_VOCAB}SLA`
const SERVICE_CLASS = `${SLA_VOCAB}ManagedService`

/**
 * The SLA domain library: two NodeShapes whose target classes become
 * authorable objects and whose property shapes become their fields.
 *
 * Every property is a literal with an sh:datatype (a leaf without one renders
 * no input at all) and none carries sh:minCount: a template leaves its
 * negotiable leaves unfilled, and an unfilled field reference is dropped from
 * the validated data graph, so a minimum cardinality would make the template
 * itself unconformant. Constraints that DO belong to the vocabulary — the
 * availability format, the two enumerations — are expressed instead.
 */
const SLA_SHAPES_TTL = `@prefix sh:   <http://www.w3.org/ns/shacl#> .
@prefix xsd:  <http://www.w3.org/2001/XMLSchema#> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
@prefix sla:  <${SLA_VOCAB}> .

sla:SLAShape a sh:NodeShape ;
  rdfs:label "SLA" ;
  sh:targetClass sla:SLA ;
  sh:property [ sh:path sla:committedAvailability ; sh:name "Committed availability" ;
                sh:datatype xsd:string ; sh:pattern "^[0-9]{1,3}([.][0-9]+)?$" ; sh:maxCount 1 ] ;
  sh:property [ sh:path sla:serviceCredit ; sh:name "Service credit" ; sh:datatype xsd:string ; sh:maxCount 1 ] ;
  sh:property [ sh:path sla:responseTime ; sh:name "Response time" ; sh:datatype xsd:string ; sh:maxCount 1 ] ;
  sh:property [ sh:path sla:escalationThreshold ; sh:name "Escalation threshold" ;
                sh:datatype xsd:string ; sh:maxCount 1 ] ;
  sh:property [ sh:path sla:supportWindow ; sh:name "Support window" ;
                sh:datatype xsd:string ; sh:in ( "24x7" "business-hours" ) ; sh:maxCount 1 ] ;
  sh:property [ sh:path sla:reportingCadence ; sh:name "Reporting cadence" ;
                sh:datatype xsd:string ; sh:maxCount 1 ] ;
  sh:property [ sh:path sla:agreementReference ; sh:name "Agreement reference" ;
                sh:datatype xsd:string ; sh:maxCount 1 ] .

sla:ManagedServiceShape a sh:NodeShape ;
  rdfs:label "Managed Service" ;
  sh:targetClass sla:ManagedService ;
  sh:property [ sh:path sla:serviceName ; sh:name "Service name" ; sh:datatype xsd:string ; sh:maxCount 1 ] ;
  sh:property [ sh:path sla:serviceRegion ; sh:name "Service region" ; sh:datatype xsd:string ; sh:maxCount 1 ] ;
  sh:property [ sh:path sla:serviceTier ; sh:name "Service tier" ;
                sh:datatype xsd:string ; sh:in ( "gold" "silver" "bronze" ) ; sh:maxCount 1 ] ;
  sh:property [ sh:path sla:dataCentre ; sh:name "Data centre" ; sh:datatype xsd:string ; sh:maxCount 1 ] ;
  sh:property [ sh:path sla:maintenanceWindow ; sh:name "Maintenance window" ;
                sh:datatype xsd:string ; sh:maxCount 1 ] .
`

/** The twelve shape-derived leaves B agrees to, keyed by property local name
 *  (the data-objects editor keys its fill controls that way). */
const SLA_FILLS: Record<string, string> = {
  committedAvailability: '99.9',
  serviceCredit: '10',
  responseTime: '15',
  escalationThreshold: '30',
  supportWindow: '24x7',
  reportingCadence: 'monthly',
  agreementReference: `SLA-${stamp}`,
  serviceName: 'Managed Analytics Platform',
  serviceRegion: 'EU',
  serviceTier: 'gold',
  dataCentre: 'Frankfurt',
  maintenanceWindow: 'Sun 02:00-04:00',
}

/** The availability floor the permission declares, and the value that breaks
 *  it — the whole exercise turns on B being refused this one. */
const AVAILABILITY_FLOOR = '99.5'
const BREAKING_AVAILABILITY = '94.0'

/** Field display labels the clause editor derives (an asset's properties are
 *  qualified by their instance name, so two objects stay tellable apart). */
const label = {
  availability: 'SLA · Committed availability',
  responseTime: 'SLA · Response time',
  escalation: 'SLA · Escalation threshold',
  region: 'Managed Service · Service region',
  tier: 'Managed Service · Service tier',
}

interface Reference {
  '@id': string
}
type Operand = Reference | { '@value': string; '@type': string }
interface Constraint {
  '@type': string
  'odrl:leftOperand'?: Reference
  'odrl:operator'?: Reference
  'odrl:rightOperand'?: Operand
  'odrl:or'?: { '@list': Constraint[] }
}
interface Duty {
  '@type': string
  'odrl:action': Reference | Reference[]
  'odrl:constraint'?: Constraint[]
  'odrl:consequence'?: Duty[]
}
interface Rule {
  '@id': string
  '@type': string
  'odrl:action': Reference | Reference[]
  'odrl:target'?: Reference
  'odrl:constraint'?: Constraint[]
  'odrl:duty'?: Duty[]
}
interface PolicySet {
  '@type'?: string
  'odrl:permission'?: Rule[]
  'odrl:prohibition'?: Rule[]
  'odrl:obligation'?: Rule[]
}
interface ContractDocument {
  'dcs:policies'?: PolicySet | Rule[]
  'dcs:contractFields'?: { '@id': string; 'dcs:label': string; 'dcs:value'?: unknown }[]
  'dcs:contractData'?: Record<string, unknown>[]
  'sh:shapesGraph'?: unknown
}

function rulesOf(document: ContractDocument): Rule[] {
  const policies = document['dcs:policies']
  if (!policies) return []
  if (Array.isArray(policies)) return policies
  return [
    ...(policies['odrl:obligation'] ?? []),
    ...(policies['odrl:permission'] ?? []),
    ...(policies['odrl:prohibition'] ?? []),
  ]
}

/** Every atomic constraint of a rule and of its duties, flattened through the
 *  logical nodes — the tree is what must survive federation, at any depth. */
function atomicConstraints(nodes: Constraint[] = []): Constraint[] {
  return nodes.flatMap((node) =>
    node['@type'] === 'odrl:LogicalConstraint' ? atomicConstraints(node['odrl:or']?.['@list'] ?? []) : [node],
  )
}

function dutiesOf(rules: Rule[]): Duty[] {
  return rules.flatMap((rule) => rule['odrl:duty'] ?? [])
}

/** The ODRL shape a copy of the contract carries: what must be identical on
 *  both instances, and what a negotiation round must leave untouched. */
function policyShapeOf(document: ContractDocument) {
  const rules = rulesOf(document)
  const duties = dutiesOf(rules)
  const fieldIds = new Set((document['dcs:contractFields'] ?? []).map((field) => field['@id']))
  const dutyConstraints = duties.flatMap((duty) => atomicConstraints(duty['odrl:constraint']))
  return {
    ruleCount: rules.length,
    ruleTypes: rules.map((rule) => rule['@type']).sort(),
    dutyActions: duties.map((duty) => (Array.isArray(duty['odrl:action']) ? '' : duty['odrl:action']['@id'])).sort(),
    consequenceActions: duties
      .flatMap((duty) => duty['odrl:consequence'] ?? [])
      .map((consequence) => (Array.isArray(consequence['odrl:action']) ? '' : consequence['odrl:action']['@id']))
      .sort(),
    // A right operand that is a reference to a declared field IS the negotiated
    // boundary; counting them proves the reference still resolves after the hop.
    negotiatedBoundaries: [
      ...atomicConstraints(rules.flatMap((rule) => rule['odrl:constraint'] ?? [])),
      ...dutyConstraints,
    ]
      .map((constraint) => constraint['odrl:rightOperand'])
      .filter((operand): operand is Reference => !!operand && '@id' in operand && fieldIds.has(operand['@id'])).length,
    // The permission's constraint tree keeps its nested logical node.
    logicalNodes: rules
      .flatMap((rule) => rule['odrl:constraint'] ?? [])
      .filter((node) => node['@type'] === 'odrl:LogicalConstraint').length,
  }
}

let bInstance: Awaited<ReturnType<typeof openInstanceB>> | undefined
test.afterEach(async () => {
  await bInstance?.context.close().catch(() => undefined)
  bInstance = undefined
})

test('an SLA authored from a hub shape crosses the catalogue and enforces on the consumer', async ({
  page,
  context,
  browser,
}) => {
  // Two instances, three template lifecycles and a negotiation round; the
  // suite-wide 90s budget covers a single view, not a federated flow.
  test.setTimeout(720_000)
  const a = instanceA(page, context, E2E_FRONTEND_ORIGIN)
  const b = await openInstanceB(browser)
  bInstance = b

  const shapeName = `e2e-sla-shapes-${stamp}`
  const componentName = `SLA Clause ${stamp}`
  const templateName = `SLA Template ${stamp}`
  let componentDid = ''
  let templateDid = ''
  let localTemplateDid = ''
  let contractDid = ''

  // ---- Stage 1 [DCS-FR-TR-03]: the SLA vocabulary enters BOTH running
  // instances as a hub shapes library. Both, because a document declares the
  // library it was authored under and is validated against exactly what it
  // declares — and because B renders its typed data objects from the library
  // it holds locally.
  await test.step('Stage 1 [DCS-FR-TR-03]: the SLA domain shape is published to both hubs', async () => {
    await publishHubShapesOn(a, shapeName, SLA_SHAPES_TTL, 'SLAShape')
    await publishHubShapesOn(b, shapeName, SLA_SHAPES_TTL, 'SLAShape')
  })

  // ---- Stage 2 [DCS-IR-TR-01, DCS-FR-TR-25, ADR-23, ADR-11]: A authors the
  // SLA clause — two objects of the imported vocabulary declared as the
  // clause's subject matter (13 negotiable fields with the inline payment
  // amount), prose beside ODRL, and a policy with a nested constraint tree, a
  // duty bounded by a negotiated field, and the duty's consequence.
  await test.step('Stage 2 [DCS-IR-TR-01, ADR-23, ADR-11]: A authors the SLA clause and its ODRL', async () => {
    await a.gotoAs('Template Creator', '/ui/templates/new')
    await a.page.getByRole('button', { name: /Component/ }).click()
    await a.page.getByRole('group').filter({ hasText: 'Global Name' }).getByRole('textbox').fill(componentName)
    await a.page
      .getByRole('group')
      .filter({ hasText: 'Base Description' })
      .getByRole('textbox')
      .fill('Service levels for a managed service, authored from the imported SLA vocabulary.')
    await a.page.getByRole('tab', { name: /Clauses/ }).click()

    const editor = a.page.getByTestId('split-clause-editor')
    await editor.getByPlaceholder('Clause title').fill('Service levels')

    // The objects picker offers everything the hub holds. A shape registered
    // after app start becomes pickable on the editor's next mount, so wait for
    // the class rather than assuming it is already there. Options are chosen by
    // VALUE (the class IRI): the label is derived from the class local name and
    // a previous run's library would offer the same one.
    const objectPicker = editor.locator('select').first()
    await expect(objectPicker.locator(`option[value="asset:${SLA_CLASS}"]`)).toBeAttached({ timeout: 60_000 })
    await objectPicker.selectOption(`asset:${SLA_CLASS}`)
    await objectPicker.selectOption(`asset:${SERVICE_CLASS}`)
    // The negotiated fee is a flat hub data field rather than a property of
    // either object: it is the one value the parties redline in the negotiate
    // view, which renders inline clause placeholders (see the gap noted on the
    // ping-pong stage).
    await objectPicker.selectOption({ label: 'Payment Amount' })

    await editor.locator('.clause-editor').first().click()
    await a.page.keyboard.type(
      'The provider operates the managed service at the committed availability, ' +
        'reports on the agreed cadence, and invoices the agreed amount.',
    )
    // Only an inline placeholder renders an editable input at contract time;
    // the shape-derived leaves get their inputs from the data-objects editor,
    // so just the fee is placed in the prose. The click must hit the leaf param
    // row in "Available requirements" (the enclosing condition row carries the
    // same text but no handler), hence the hasNot filter.
    await editor
      .locator('section')
      .filter({ hasText: 'Available requirements' })
      .getByRole('listitem')
      .filter({ hasText: 'Payment Amount' })
      .filter({ hasNot: a.page.getByRole('listitem') })
      .click()
    await expect(editor.locator('[data-parameter-name]')).toHaveCount(1)

    const ruleSelect = (name: string) =>
      editor.locator('label.form-control').filter({ hasText: name }).locator('select')
    await ruleSelect('Rule').selectOption({ label: 'Permission — the assignee MAY' })
    await ruleSelect('Action').selectOption({ label: 'use' })
    // The permission targets the declared service object, not the contract —
    // an ODRL rule is about a thing (ADR-23).
    await ruleSelect('Toward').selectOption({ label: 'Managed Service' })

    // A constraint row: left operand, operator, boundary source, then either a
    // fixed literal or the negotiated field the boundary reads from.
    const addConstraint = async (
      scope: ReturnType<typeof editor.locator>,
      operand: string,
      operator: string,
      boundary: { fixed: string } | { field: string },
    ) => {
      await scope.getByRole('button', { name: '+ constraint' }).click()
      const row = scope.locator('.flex.flex-wrap.items-center.gap-1').last()
      await row.locator('select').nth(0).selectOption({ label: operand })
      await row.locator('select').nth(1).selectOption({ label: operator })
      if ('fixed' in boundary) await row.locator('input[placeholder="value"]').fill(boundary.fixed)
      // The boundary select renders a field as the “<label>” (curly quotes).
      else
        await row
          .locator('select')
          .nth(2)
          .selectOption({ label: `the “${boundary.field}”` })
    }

    // The floor A publishes and is itself later held to.
    await addConstraint(editor, label.availability, 'greater than or equal to', { fixed: AVAILABILITY_FLOOR })
    // …AND one of two service properties, a nested logical node inside the
    // top-level conjunction (ODRL IM §2.6).
    await editor.getByRole('button', { name: '+ group' }).click()
    const group = editor.locator('.border-dashed').last()
    await addConstraint(group, label.region, 'equal to', { fixed: 'EU' })
    await addConstraint(group, label.tier, 'equal to', { fixed: 'gold' })
    await group.locator('select[title="How this group combines"]').selectOption('or')

    // A duty the assignee must fulfil to exercise the permission, bounded by a
    // NEGOTIATED field rather than a literal — the boundary is agreed at
    // contract time — and a consequence duty for when it is not fulfilled.
    await editor.getByRole('button', { name: '+ duty' }).click()
    const duty = editor.getByTestId('odrl-duty').last()
    await duty.locator('select').first().selectOption({ label: 'inform' })
    await addConstraint(duty, label.responseTime, 'less than or equal to', { field: label.escalation })
    await duty.getByRole('button', { name: '+ consequence' }).click()
    const consequence = duty.getByTestId('odrl-consequence').last()
    await consequence.locator('select').first().selectOption({ label: 'compensate' })

    await editor.getByRole('button', { name: 'Add clause', exact: true }).click()
    await expect(editor.getByPlaceholder('Clause title')).toHaveValue('')

    // The clause goes into the document outline.
    //
    // ONE clause carries the policy here, deliberately. A second clause with a
    // second rule cannot be authored in the same sitting: the rule builder's
    // draft is seeded once and never reset when a clause is saved, so the next
    // clause starts holding the previous one's constraint tree and duties —
    // pointing at fields that clause no longer declares. Deep ODRL is proven by
    // this rule's tree instead of by a second rule.
    const modal = a.page.getByRole('dialog')
    await a.page.getByRole('button', { name: 'Place in document' }).first().click()
    await expect(modal.getByText('Selected clause')).toBeVisible()
    await modal.getByRole('button', { name: /Service levels/ }).click()
    await expect(a.page.getByRole('dialog')).toBeHidden()

    const authored = a.page.waitForRequest((r) => r.url().includes('/template/create') && r.method() === 'POST')
    const created = a.page.waitForResponse(
      (r) => r.url().includes('/template/create') && r.request().method() === 'POST',
    )
    await a.page.getByRole('button', { name: 'Create', exact: true }).click()
    const response = await created
    expect(response.ok(), `component create: ${response.status()} ${await response.text()}`).toBeTruthy()
    componentDid = ((await response.json()) as { did: string }).did

    // What the builder actually produced, read off the request it sent.
    const authoredDocument = (await authored).postDataJSON().template_data as ContractDocument
    expect(authoredDocument['dcs:contractFields']?.length, 'thirteen negotiable fields declared').toBe(13)
    const objectTypes = (authoredDocument['dcs:contractData'] ?? []).map((object) => object['@type'])
    expect(objectTypes, 'both imported classes are in the data graph').toEqual(
      expect.arrayContaining([SLA_CLASS, SERVICE_CLASS]),
    )
    const shape = policyShapeOf(authoredDocument)
    expect(shape.ruleCount, 'the clause carries exactly its own rule').toBe(1)
    expect(shape.ruleTypes).toEqual(['odrl:Permission'])
    expect(shape.logicalNodes, 'the nested or-group survived composition').toBe(1)
    expect(shape.dutyActions, 'the duty the permission carries').toEqual(['odrl:inform'])
    expect(shape.consequenceActions, 'the consequence of an unmet duty').toEqual(['odrl:compensate'])
    expect(shape.negotiatedBoundaries, "the duty's boundary is a declared field").toBe(1)
  })

  // ---- Stage 3 [DCS-IR-SI-01, DCS-IR-TR-07]: A takes the clause through its
  // four-role lifecycle, composes the contract template that inlines it, takes
  // that through the same lifecycle, registers it and PUBLISHES it to the
  // Federated Catalogue.
  await test.step('Stage 3 [DCS-IR-SI-01, DCS-IR-TR-07]: A approves, registers and publishes the template', async () => {
    await submitReviewApproveTemplateOn(a, componentDid, componentName)
    templateDid = await authorContractTemplate(a, templateName, componentName)
    await submitReviewApproveTemplateOn(a, templateDid, templateName)
    await registerTemplateOn(a, templateDid, templateName)
    await publishTemplateOn(a, templateDid, templateName)
  })

  // ---- Stage 4-5 [DCS-IR-SI-01]: B finds A's template in the catalogue,
  // registers it into its own repository, and runs its OWN four-role lifecycle
  // over it — a catalogue template is a proposal, not an instruction.
  await test.step('Stage 4-5 [DCS-IR-SI-01]: B takes the template from the catalogue and approves it', async () => {
    localTemplateDid = await registerCatalogueTemplateOn(b, templateName)
    expect(localTemplateDid, 'B registers the catalogue template under its own DID').not.toBe(templateDid)
    await submitReviewApproveTemplateOn(b, localTemplateDid, templateName)
    await registerTemplateOn(b, localTemplateDid, templateName)
  })

  // ---- Stage 6 [DCS-FR-CWE-16, ADR-13]: B — the consumer — derives the
  // contract, names A as counterparty, and agrees all thirteen values through
  // the real inputs.
  await test.step('Stage 6 [DCS-FR-CWE-16]: B derives a contract and fills every negotiable value', async () => {
    const aDidWeb = await resolveDidWeb(a)
    contractDid = await createContractViaUi(b, templateName, aDidWeb)
    await fillContractValuesOn(b, contractDid, {
      dataLeaves: { ...SLA_FILLS, committedAvailability: BREAKING_AVAILABILITY },
      inlineFields: { 'Payment Amount': '20000' },
    })
  })

  // ---- Stage 7 [ADR-11]: the demonstration the exercise exists for. B's fill
  // of 94.0 breaks the availability floor A declared in the template, and B's
  // own UI refuses the submit and names the constraint — the policy travelled
  // with the template through the catalogue and is enforced by the consumer's
  // instance, against nothing but the document.
  await test.step('Stage 7 [ADR-11]: B is refused for breaking the floor A published', async () => {
    await expectSubmitRefusedOn(b, contractDid, /violates an ODRL obligation.*99\.5/)
    await fillContractValuesOn(b, contractDid, {
      dataLeaves: { committedAvailability: SLA_FILLS.committedAvailability },
    })
  })

  // ---- Stage 8 [ADR-13, DCS-NFR-BR-08]: B offers; A replicates the contract
  // into its own OFFERED copy, carrying the same policy tree and pinned to the
  // same shapes.
  await test.step('Stage 8 [ADR-13]: B offers; A receives the identical policy and shape pin', async () => {
    await offerToCounterparty(b, contractDid)
    await assertReceivedInState(a, contractDid, 'OFFERED')

    const onB = (await contractDocumentOn(b, contractDid)) as ContractDocument
    const onA = (await contractDocumentOn(a, contractDid)) as ContractDocument
    expect(policyShapeOf(onA), "A's copy carries B's policy tree unchanged").toEqual(policyShapeOf(onB))
    expect(policyShapeOf(onA).consequenceActions, 'the consequence crossed the wire').toEqual(['odrl:compensate'])
    // Both sides are pinned to the same shapes graphs, the SLA library among
    // them — the document names the vocabulary it must be read under.
    expect(onA['sh:shapesGraph'], 'both copies pin the same shapes graphs').toEqual(onB['sh:shapesGraph'])
    expect(JSON.stringify(onA['sh:shapesGraph']), 'the imported SLA library is pinned').toContain(shapeName)
  })

  // ---- Stage 9 [DCS-FR-CWE-18, DCS-IR-CWE-03]: one negotiation round. A
  // redlines the fee on its received copy and ships it back; B accepts. What
  // must hold afterwards is that negotiation moved a VALUE and left the policy
  // structure exactly as authored.
  //
  // The redline is the inline fee rather than a service level: the negotiate
  // view renders only the clause preview, so a value that lives on a
  // dcs:contractData object has no input there (see the reported gap) — and
  // the fee reuses the vertical's own counter-offer helper unchanged.
  await test.step('Stage 9 [DCS-FR-CWE-18]: A counter-offers, B accepts, the policy is untouched', async () => {
    const before = policyShapeOf(await contractDocumentOn(a, contractDid))
    await counterOffer(a, contractDid, { value: '18500' })
    // B answers the inbound redline while its own copy is still OFFERED — a
    // received proposal does not move the peer's intrinsic state.
    await acceptPeerProposalOn(b, contractDid)

    const onA = (await contractDocumentOn(a, contractDid)) as ContractDocument
    const onB = (await contractDocumentOn(b, contractDid)) as ContractDocument
    expect(policyShapeOf(onA), 'the round moved a value, not the policy').toEqual(before)
    expect(policyShapeOf(onB), 'and both parties still hold the same policy').toEqual(before)
    const fee = (onB['dcs:contractFields'] ?? []).find((field) => field['dcs:label'] === 'Payment Amount')
    expect(JSON.stringify(fee?.['dcs:value']), 'the agreed fee is the redlined one').toContain('18500')
  })
})
