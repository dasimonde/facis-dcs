import { describe, expect, it } from 'vitest'
import {
  draftToAnnotations,
  filterAnnotatableBlocks,
  incompleteAnnotationCount,
  listAnnotatableBlocks,
  orderedBlockIdsFromLayout,
  resolveAnnotationContentLabel,
} from './review-block-annotations'
import type { DcsBlock, DcsLayoutNode } from '@/models/dcs-jsonld'

describe('listAnnotatableBlocks', () => {
  it('lists only clauses with content preview (not title); skips section/text for now', () => {
    const blocks = [
      { '@id': 'urn:b#s1', '@type': 'dcs:Section', 'dcs:title': 'Terms' },
      { '@id': 'urn:b#t1', '@type': 'dcs:TextBlock', 'dcs:text': 'Hello world' },
      {
        '@id': 'urn:b#c1',
        '@type': 'dcs:Clause',
        'dcs:title': 'Clause 01',
        'dcs:content': { '@list': ['Pay ', { '@id': 'urn:field#fee' }, ' monthly.'] },
      },
    ] as DcsBlock[]
    expect(listAnnotatableBlocks(blocks)).toEqual([
      {
        blockId: 'urn:b#c1',
        label: 'Pay […] monthly.',
        type: 'Clause',
        preview: 'Pay […] monthly.',
      },
    ])
  })

  it('orders clauses by document layout (content order)', () => {
    const blocks = [
      { '@id': 'urn:b#c2', '@type': 'dcs:Clause', 'dcs:content': { '@list': ['Second'] } },
      { '@id': 'urn:b#s1', '@type': 'dcs:Section', 'dcs:title': 'First' },
      { '@id': 'urn:b#c1', '@type': 'dcs:Clause', 'dcs:content': { '@list': ['First clause'] } },
      { '@id': 'urn:b#t1', '@type': 'dcs:TextBlock', 'dcs:text': 'Text' },
    ] as DcsBlock[]
    const layout = [
      {
        '@id': 'urn:b#root',
        'dcs:isRoot': true,
        'dcs:children': { '@list': [{ '@id': 'urn:b#s1' }] },
      },
      {
        '@id': 'urn:b#s1',
        'dcs:children': {
          '@list': [{ '@id': 'urn:b#c1' }, { '@id': 'urn:b#t1' }, { '@id': 'urn:b#c2' }],
        },
      },
      { '@id': 'urn:b#c1', 'dcs:children': { '@list': [] } },
      { '@id': 'urn:b#t1', 'dcs:children': { '@list': [] } },
      { '@id': 'urn:b#c2', 'dcs:children': { '@list': [] } },
    ] as DcsLayoutNode[]
    expect(listAnnotatableBlocks(blocks, layout).map((b) => b.blockId)).toEqual(['urn:b#c1', 'urn:b#c2'])
  })
})

describe('orderedBlockIdsFromLayout', () => {
  it('walks depth-first from root children', () => {
    const layout = [
      {
        '@id': 'root',
        'dcs:isRoot': true,
        'dcs:children': { '@list': [{ '@id': 'a' }, { '@id': 'b' }] },
      },
      { '@id': 'a', 'dcs:children': { '@list': [{ '@id': 'a1' }] } },
      { '@id': 'a1', 'dcs:children': { '@list': [] } },
      { '@id': 'b', 'dcs:children': { '@list': [] } },
    ] as DcsLayoutNode[]
    expect(orderedBlockIdsFromLayout(layout)).toEqual(['a', 'a1', 'b'])
  })
})

describe('filterAnnotatableBlocks', () => {
  it('matches label, type, or preview', () => {
    const blocks = [
      { blockId: '1', label: 'Payment terms', type: 'Section', preview: '' },
      { blockId: '2', label: 'Pay […] monthly.', type: 'Clause', preview: 'Pay […] monthly.' },
    ]
    expect(filterAnnotatableBlocks(blocks, 'pay').map((b) => b.blockId)).toEqual(['1', '2'])
    expect(filterAnnotatableBlocks(blocks, 'clause').map((b) => b.blockId)).toEqual(['2'])
  })
})

describe('draftToAnnotations', () => {
  it('drops empty comments', () => {
    expect(
      draftToAnnotations({
        'urn:b#c1': '  confirm amount  ',
        'urn:b#c2': '   ',
      }),
    ).toEqual([{ block_id: 'urn:b#c1', comment: 'confirm amount' }])
  })
})

describe('incompleteAnnotationCount', () => {
  it('counts rows with no text', () => {
    expect(
      incompleteAnnotationCount({
        'urn:b#c1': 'ok',
        'urn:b#c2': '',
        'urn:b#c3': '  ',
      }),
    ).toBe(2)
  })
})

describe('resolveAnnotationContentLabel', () => {
  it('resolves text blocks for audit and never returns a raw @id', () => {
    const blocks = [{ '@id': 'urn:b#t1', '@type': 'dcs:TextBlock', 'dcs:text': 'Here is text.' }] as DcsBlock[]
    expect(resolveAnnotationContentLabel('urn:b#t1', blocks)).toBe('Here is text.')
    expect(resolveAnnotationContentLabel('urn:missing', blocks)).toBe('Content')
  })
})
