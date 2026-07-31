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
  return (await response.json()) as DIDDocument
}
