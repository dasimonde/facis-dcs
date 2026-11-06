import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, expect, it } from 'vitest'
import TemplatePreview from '@template-repository/components/builder-editor/preview/TemplatePreview.vue'
import ContractAuditList from './ContractAuditList.vue'
import type { DcsBlock, DcsLayoutNode } from '@/models/dcs-jsonld'
import type { ContractAuditResponse } from '@/models/responses/contract-response'

beforeEach(() => setActivePinia(createPinia()))

// DCS-FR-CWE-25 role attribution and DCS-FR-CWE-15 approver comments: each
// review decision shows the role the actor held and the comment given.
it('shows the actor role and the approver comment of a decision', () => {
  const audits = [
    {
      id: 1,
      event_type: 'APPROVE_CONTRACT',
      component: 'CONTRACT_WORKFLOW_ENGINE',
      created_at: '2026-10-30T12:00:00Z',
      event_data: {
        did: 'did:web:c1',
        approved_by: 'Org A',
        user_roles: ['Contract Approver'],
        decision_notes: ['clause 4 checked'],
        occurred_at: '2026-10-30T12:00:00Z',
      },
    },
  ] as unknown as ContractAuditResponse

  const wrapper = mount(ContractAuditList, { props: { audits } })

  expect(wrapper.get('[data-testid="audit-roles"]').text()).toBe('Role: Contract Approver')
  expect(wrapper.get('[data-testid="audit-comments"]').text()).toContain('clause 4 checked')
})

// DCS-FR-CWE-25 highlighting: the blocks a reviewer annotates are highlighted
// in the contract preview.
it('highlights the blocks a reviewer marked', () => {
  const layout = [
    { '@id': 'root', 'dcs:isRoot': true, 'dcs:children': { '@list': [{ '@id': 'b1' }, { '@id': 'b2' }] } },
    { '@id': 'b1', 'dcs:children': { '@list': [] } },
    { '@id': 'b2', 'dcs:children': { '@list': [] } },
  ] as unknown as DcsLayoutNode[]
  const blocks = [
    { '@id': 'b1', '@type': 'dcs:TextBlock', 'dcs:text': 'Marked text' },
    { '@id': 'b2', '@type': 'dcs:TextBlock', 'dcs:text': 'Other text' },
  ] as unknown as DcsBlock[]

  const wrapper = mount(TemplatePreview, {
    props: { layout, blocks, semanticConditions: [], highlightedBlockIds: ['b1'], disabled: true },
  })

  const highlighted = wrapper.findAll('.review-highlight')
  expect(highlighted).toHaveLength(1)
  expect(highlighted[0]!.text()).toContain('Marked text')
  expect(highlighted[0]!.text()).not.toContain('Other text')
})
