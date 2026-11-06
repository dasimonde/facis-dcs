interface DIDDocument {
  id: string
}

/**
 * The instance's own did:web document lives at the ORIGIN root
 * (/.well-known/did.json) — that is where did:web:host resolves — NOT under
 * the API base path. Fetched origin-relative so it resolves both in the dev
 * server (proxied to the backend) and in a deployed instance (served by the
 * ingress at the origin root), rather than through the API-prefixed client,
 * which appends the API base and 404s.
 */
export async function getLocalDIDFile(): Promise<DIDDocument> {
  const response = await fetch('/.well-known/did.json', { headers: { Accept: 'application/json' } })
  if (!response.ok) {
    throw new Error(`local DID document unavailable: HTTP ${response.status}`)
  }
  const doc = (await response.json()) as DIDDocument
  if (!doc?.id) {
    throw new Error('local DID document has no id')
  }
  return doc
}

/**
 * Retries getLocalDIDFile a few times. Negotiate/View used to swallow a single
 * failed fetch into '', which left Accept/Awaiting badges dead for the whole
 * page lifetime (decision rows are keyed by this did:web).
 */
export async function resolveLocalInstanceDid(attempts = 3): Promise<string> {
  let lastError: unknown
  for (let attempt = 0; attempt < attempts; attempt++) {
    try {
      return (await getLocalDIDFile()).id
    } catch (err) {
      lastError = err
      if (attempt + 1 < attempts) {
        await new Promise((resolve) => setTimeout(resolve, 250 * (attempt + 1)))
      }
    }
  }
  throw lastError instanceof Error ? lastError : new Error(`local DID document unavailable after ${attempts} attempts`)
}
