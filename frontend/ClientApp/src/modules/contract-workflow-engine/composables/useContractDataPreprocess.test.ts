import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import { flattenPolicySet, useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { useContractDataPreprocess } from './useContractDataPreprocess'

describe('contract layout preprocessing', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it.each(['array', '@list'])('reads %s layout without rewriting document data', (format) => {
    const document = useDcsDraftStore().templateDocument
    const layout = document['dcs:documentStructure']['dcs:layout']['@list']
    const input = {
      ...document,
      'dcs:documentStructure': {
        ...document['dcs:documentStructure'],
        'dcs:layout': format === 'array' ? layout : { '@list': layout },
      },
    }
    const original = JSON.stringify(input)
    const result = useContractDataPreprocess().preprocessContractData(input)
    expect(result?.layout).toBe(layout)
    expect(result?.blocks).toBe(document['dcs:documentStructure']['dcs:blocks']['@list'])
    expect(result?.contractFields).toBe(document['dcs:contractFields'])
    expect(result?.contractData).toBe(document['dcs:contractData'])
    expect(result?.policies).toEqual(flattenPolicySet(document['dcs:policies']))
    expect(JSON.stringify(input)).toBe(original)
  })

  it.each([undefined, null, {}, { '@list': null }, { '@list': 'invalid' }])(
    'rejects missing or invalid layout %j',
    (layout) => {
      const document = useDcsDraftStore().templateDocument
      expect(
        useContractDataPreprocess().preprocessContractData({
          ...document,
          'dcs:documentStructure': { ...document['dcs:documentStructure'], 'dcs:layout': layout },
        }),
      ).toBeNull()
    },
  )
})
