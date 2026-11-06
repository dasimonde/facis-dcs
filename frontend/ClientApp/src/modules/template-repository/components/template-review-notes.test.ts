import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import TemplateReviewNotes from './TemplateReviewNotes.vue'
import type { TemplateReviewNote } from '@/models/contract-template/contract-template'

/**
 * DCS-IR-TR-04, DCS-IR-TR-05: the creator sees why the template came back and
 * the approver sees the review comments.
 */

const notes: TemplateReviewNote[] = [
  {
    kind: 'RETURNED_TO_DRAFT',
    text: ['Clause 3 is missing.'],
    by: 'did:web:example.com:reviewer',
    created_at: '2026-07-01T09:00:00Z',
  },
  { kind: 'REJECTED', text: ['Not compliant.'], created_at: '2026-07-02T09:00:00Z' },
]

describe('TemplateReviewNotes', () => {
  it('shows each note with its kind, text, author and time', () => {
    const wrapper = mount(TemplateReviewNotes, { props: { notes } })

    const items = wrapper.findAll('[data-testid="template-review-note"]')
    expect(items).toHaveLength(2)
    expect(items[0]!.text()).toContain('Returned to draft')
    expect(items[0]!.text()).toContain('Clause 3 is missing.')
    expect(items[0]!.text()).toContain('did:web:example.com:reviewer')
    expect(items[0]!.text()).toContain(new Date('2026-07-01T09:00:00Z').toLocaleString())
    expect(items[1]!.text()).toContain('Rejected')
    expect(items[1]!.text()).toContain('Not compliant.')
  })

  it('shows nothing without notes', () => {
    expect(
      mount(TemplateReviewNotes, { props: { notes: [] } })
        .find('section')
        .exists(),
    ).toBe(false)
    expect(mount(TemplateReviewNotes).find('section').exists()).toBe(false)
  })
})
