import { describe, expect, it } from 'vitest'
import { verifyMerkleInclusion } from './merkle-inclusion'

// Vectors from backend/internal/base/merkle.go: five leaves (MerkleLeafHash of
// "entry-a" … "entry-e"), so the last leaf is promoted on the first level.
const root = '48ba578022b004753b4466952e75c45806df915fcdd13b06acba3ebce2598647'
const proofs = [
  {
    leaf: 'e0cedcff5e5d4853f6ffbacdc62e14ededae5efbf4cdc863498acbbc56c6840c',
    siblings: [
      'c120f32004f3093a2e166eb15d1e4caccff04fd819cfb5459c93a5a862bb0a7f',
      '8e4fce1d7ffa926b4bd93f7804858c3d5da98db62cbe9d64fee18122dc2386ac',
      '0fd6aa6f31ae10f035920ee19bf4ba406757e3219706d7b8fac65cc6eb6ab919',
    ],
  },
  {
    leaf: 'c120f32004f3093a2e166eb15d1e4caccff04fd819cfb5459c93a5a862bb0a7f',
    siblings: [
      'e0cedcff5e5d4853f6ffbacdc62e14ededae5efbf4cdc863498acbbc56c6840c',
      '8e4fce1d7ffa926b4bd93f7804858c3d5da98db62cbe9d64fee18122dc2386ac',
      '0fd6aa6f31ae10f035920ee19bf4ba406757e3219706d7b8fac65cc6eb6ab919',
    ],
  },
  {
    leaf: '256dda541299683bac8b3d9095a93bfa73643792f9511e55dc18038244ac9609',
    siblings: [
      'c29dc076f9e48a2641a4e55ddd1aac8c03b9bc9b5d1f980d8fd2ad441892b8bd',
      '7ec35e98284cce509343200fe4fc2458611d10946f64b261bf90bacc1f58f0c3',
      '0fd6aa6f31ae10f035920ee19bf4ba406757e3219706d7b8fac65cc6eb6ab919',
    ],
  },
  {
    leaf: 'c29dc076f9e48a2641a4e55ddd1aac8c03b9bc9b5d1f980d8fd2ad441892b8bd',
    siblings: [
      '256dda541299683bac8b3d9095a93bfa73643792f9511e55dc18038244ac9609',
      '7ec35e98284cce509343200fe4fc2458611d10946f64b261bf90bacc1f58f0c3',
      '0fd6aa6f31ae10f035920ee19bf4ba406757e3219706d7b8fac65cc6eb6ab919',
    ],
  },
  {
    leaf: '0fd6aa6f31ae10f035920ee19bf4ba406757e3219706d7b8fac65cc6eb6ab919',
    siblings: ['4ba812d5a68fbe28906e6995fc01d2114990ac5eba53598dfd910ff01695db54'],
  },
]

describe('verifyMerkleInclusion', () => {
  it.each(proofs.map((proof, index) => [index, proof] as const))('accepts the proof of leaf %i', async (index, p) => {
    expect(await verifyMerkleInclusion(p.leaf, p.siblings, index, 5, root)).toBe(true)
  })

  it('refuses a proof for another position, root or leaf count', async () => {
    const p = proofs[2]!
    expect(await verifyMerkleInclusion(p.leaf, p.siblings, 3, 5, root)).toBe(false)
    expect(await verifyMerkleInclusion(p.leaf, p.siblings, 2, 5, proofs[0]!.leaf)).toBe(false)
    expect(await verifyMerkleInclusion(p.leaf, p.siblings, 2, 4, root)).toBe(false)
    expect(await verifyMerkleInclusion(p.leaf, p.siblings.slice(1), 2, 5, root)).toBe(false)
    expect(await verifyMerkleInclusion(p.leaf, [...p.siblings, p.leaf], 2, 5, root)).toBe(false)
    expect(await verifyMerkleInclusion('zz', p.siblings, 2, 5, root)).toBe(false)
  })

  it('accepts a single leaf as its own root', async () => {
    expect(await verifyMerkleInclusion(proofs[0]!.leaf, [], 0, 1, proofs[0]!.leaf)).toBe(true)
  })
})
