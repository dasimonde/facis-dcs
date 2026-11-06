import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuthStore } from '@/stores/auth-store'
import NonComplianceInvestigationView from './NonComplianceInvestigationView.vue'
import type { UserRole } from '@/types/user-role'

/**
 * DCS-FR-PACM-03: the Compliance Officer decides the REVIEW runs of the
 * workflow gates. DCS-IR-PACM-04, DCS-FR-PACM-05: incidents link to a contract
 * or a template, and the cases are listed, looked up and exported.
 */

const pac = vi.hoisted(() => ({
  monitor: vi.fn(),
  reportIncident: vi.fn(),
  exportCaseFile: vi.fn(),
  cases: vi.fn(),
  workflowGateRuns: vi.fn(),
  decideWorkflowGateReview: vi.fn(),
}))
const download = vi.hoisted(() => ({ downloadBlob: vi.fn() }))
vi.mock('@/services/pac-non-compliance-service', () => ({ pacNonComplianceService: pac }))
vi.mock('@/utils/download-blob', () => download)
vi.mock('@/router/router', () => ({ ROUTES: { AUDIT: { LIST: 'audit' } } }))

const run = {
  run_id: '11111111-1111-1111-1111-111111111111',
  contract_did: 'did:web:example.com:contract:1',
  contract_version: 2,
  gate: 'deployment',
  status: 'REVIEW',
  findings: ['regulatory.gdpr.pid-in-document: the document carries personal data'],
  created_at: '2026-10-30T10:00:00Z',
}
const caseItem = {
  case_id: 'case-1',
  did: 'did:web:example.com:template:1',
  linked_to: 'template',
  reported_by: 'Org A',
  reported_at: '2026-10-30T11:00:00Z',
  findings: 1,
}

async function mountView(roles: UserRole[]) {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().user = { issuer: 'did:web:example.com', holder: 'user', roles }
  const wrapper = mount(NonComplianceInvestigationView, {
    global: {
      plugins: [pinia],
      mocks: { $route: { meta: { name: 'Non-Compliance Investigation' } } },
      stubs: { RouterLink: { props: ['to'], template: '<a :data-to="JSON.stringify(to)"><slot /></a>' } },
    },
  })
  await flushPromises()
  return wrapper
}

describe('non-compliance investigation', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    pac.cases.mockResolvedValue([caseItem])
    pac.workflowGateRuns.mockResolvedValue([run])
    pac.decideWorkflowGateReview.mockResolvedValue(undefined)
    pac.exportCaseFile.mockResolvedValue(new Blob(['{}']))
  })

  it('lets the Compliance Officer decide a REVIEW run with a justification', async () => {
    const wrapper = await mountView(['COMPLIANCE_OFFICER'])
    const item = wrapper.get(`[data-testid="gate-run-${run.run_id}"]`)
    expect(item.text()).toContain(run.contract_did)
    expect(item.text()).toContain('the document carries personal data')
    const approve = wrapper.get(`[data-testid="gate-approve-${run.run_id}"]`)
    await wrapper.get(`[data-testid="gate-justification-${run.run_id}"]`).setValue('   ')
    expect(approve.attributes('disabled')).toBeDefined()

    pac.workflowGateRuns.mockResolvedValue([])
    await wrapper.get(`[data-testid="gate-justification-${run.run_id}"]`).setValue('personal data is masked')
    await approve.trigger('click')
    await flushPromises()
    expect(pac.decideWorkflowGateReview).toHaveBeenCalledWith(run.run_id, 'approve', 'personal data is masked')
    expect(pac.workflowGateRuns).toHaveBeenCalledTimes(2)
    expect(wrapper.find(`[data-testid="gate-run-${run.run_id}"]`).exists()).toBe(false)
  })

  // DCS-FR-PACM-04: a sweep names the denied actors only with a justification.
  it('sends the monitoring justification only when one is given', async () => {
    pac.monitor.mockResolvedValue({ checked_at: '2026-10-30T10:00:00Z', risks: [] })
    const wrapper = await mountView(['COMPLIANCE_OFFICER'])
    await wrapper.get('[data-testid="run-monitoring-sweep"]').trigger('click')
    await flushPromises()
    expect(pac.monitor).toHaveBeenLastCalledWith(undefined)

    await wrapper.get('[data-testid="monitor-justification"]').setValue('denied access review')
    await wrapper.get('[data-testid="run-monitoring-sweep"]').trigger('click')
    await flushPromises()
    expect(pac.monitor).toHaveBeenLastCalledWith('denied access review')
  })

  it('links an incident to a template', async () => {
    pac.reportIncident.mockResolvedValue('case-2')
    const wrapper = await mountView(['COMPLIANCE_OFFICER'])
    await wrapper.get('[data-testid="incident-linked-to"]').setValue('template')
    await wrapper.get('[data-testid="incident-did"]').setValue('did:web:example.com:template:1')
    await wrapper.get('[data-testid="incident-risk-type"]').setValue('OUTDATED_CLAUSE')
    await wrapper.get('[data-testid="incident-detail"]').setValue('clause refers to a repealed law')
    await wrapper.get('[data-testid="incident-form"]').trigger('submit')
    await flushPromises()
    expect(pac.reportIncident).toHaveBeenCalledWith({
      template_did: 'did:web:example.com:template:1',
      findings: [{ risk_type: 'OUTDATED_CLAUSE', detail: 'clause refers to a repealed law' }],
    })
    expect((wrapper.get('[data-testid="case-lookup"]').element as HTMLInputElement).value).toBe('case-2')
    expect(pac.cases).toHaveBeenCalledTimes(2)
  })

  it('links a risk to the action history of its contract', async () => {
    pac.monitor.mockResolvedValue({
      checked_at: '2026-10-30T12:00:00Z',
      risks: [{ did: run.contract_did, risk_type: 'MISSING_APPROVAL', detail: 'x', detected_at: '2026-10-30' }],
    })
    const wrapper = await mountView(['COMPLIANCE_OFFICER'])
    await wrapper.get('[data-testid="run-monitoring-sweep"]').trigger('click')
    await flushPromises()
    const link = wrapper.get('[data-testid="monitor-risk-did"] a')
    expect(JSON.parse(link.attributes('data-to')!)).toEqual({ name: 'audit', query: { did: run.contract_did } })
  })

  it('shows the Auditor the cases only and exports a case file with a justification', async () => {
    const wrapper = await mountView(['AUDITOR'])
    expect(wrapper.find('[data-testid="run-monitoring-sweep"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="incident-form"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="gate-reviews"]').exists()).toBe(false)
    expect(pac.workflowGateRuns).not.toHaveBeenCalled()
    expect(wrapper.get(`[data-testid="case-${caseItem.case_id}"]`).text()).toContain(caseItem.did)

    await wrapper.get('[data-testid="case-justification"]').setValue('regulator request 42')
    await wrapper.get(`[data-testid="case-export-${caseItem.case_id}"]`).trigger('click')
    await flushPromises()
    expect(pac.exportCaseFile).toHaveBeenCalledWith('case-1', 'regulator request 42')
    expect(download.downloadBlob).toHaveBeenCalledWith(expect.any(Blob), 'case-case-1.json')

    await wrapper.get('[data-testid="case-lookup"]').setValue('case-9')
    await wrapper.get('[data-testid="case-export"]').trigger('submit')
    await flushPromises()
    expect(pac.exportCaseFile).toHaveBeenLastCalledWith('case-9', 'regulator request 42')
  })
})
