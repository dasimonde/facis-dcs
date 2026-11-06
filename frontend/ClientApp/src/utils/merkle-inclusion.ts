// Verifies an audit checkpoint inclusion proof in the browser (ADR-16). It is
// the port of VerifyMerkleInclusion in backend/internal/base/merkle.go: RFC 6962
// node hashes (prefix 0x01), and a level with an odd number of nodes promotes
// its last node without a sibling, so the leaf count of the checkpoint is
// necessary to walk the proof.

function fromHex(value: string): Uint8Array | null {
  if (!/^(?:[0-9a-fA-F]{2})*$/.test(value)) return null
  return Uint8Array.from(value.match(/../g) ?? [], (pair) => parseInt(pair, 16))
}

async function node(left: Uint8Array, right: Uint8Array): Promise<Uint8Array> {
  const data = new Uint8Array(1 + left.length + right.length)
  data[0] = 0x01
  data.set(left, 1)
  data.set(right, 1 + left.length)
  return new Uint8Array(await crypto.subtle.digest('SHA-256', data))
}

export async function verifyMerkleInclusion(
  leafHash: string,
  proof: string[],
  index: number,
  leafCount: number,
  root: string,
): Promise<boolean> {
  let current = fromHex(leafHash)
  if (!current) return false
  let consumed = 0
  for (let size = leafCount; size > 1; size = Math.floor((size + 1) / 2)) {
    if (index !== size - 1 || size % 2 === 0) {
      const sibling = consumed < proof.length ? fromHex(proof[consumed]!) : null
      if (!sibling) return false
      consumed++
      current = index % 2 === 0 ? await node(current, sibling) : await node(sibling, current)
    }
    index = Math.floor(index / 2)
  }
  const want = fromHex(root)
  const got = current
  return consumed === proof.length && !!want && want.length === got.length && want.every((b, i) => b === got[i])
}
