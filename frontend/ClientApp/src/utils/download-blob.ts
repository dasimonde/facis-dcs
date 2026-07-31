/** How long a triggered download's object URL is kept alive after the click. */
const REVOKE_DELAY_MS = 60_000

/**
 * Triggers a browser download for `blob` under `filename`.
 *
 * The anchor is attached to the document and the object URL outlives the click:
 * the browser starts the download asynchronously, so revoking the URL on the
 * same tick can abort a large file before it ever begins, producing no download
 * and no error at all.
 */
export function downloadBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  anchor.style.display = 'none'
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
  setTimeout(() => URL.revokeObjectURL(url), REVOKE_DELAY_MS)
}
