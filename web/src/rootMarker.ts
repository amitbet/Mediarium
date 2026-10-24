// Reading a failed import that says a library folder has lost its marker file
// (.mediarium-root), so the row can offer to put it back. No React here.

const LOST = /The library folder (.+?) has lost its \.mediarium-root file\./

/** The library folder named by a "has lost its .mediarium-root file" message; null for any other message. */
export function lostMarkerFolder(error: string | undefined): string | null {
  const m = LOST.exec((error ?? '').trim())
  return m ? m[1] : null
}

/** The ids of the failed downloads that were refused because of this folder. */
export function failedForFolder(items: { id: number; status: string; error?: string }[], folder: string): number[] {
  return items.filter((q) => q.status === 'failed' && lostMarkerFolder(q.error) === folder).map((q) => q.id)
}

/** The last part of a folder path, to type as a confirmation. */
export function folderName(path: string): string {
  const parts = path.split(/[\\/]+/).filter(Boolean)
  return parts[parts.length - 1] ?? path
}
