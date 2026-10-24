// The address the naming preview is read from. keep asks for the name a file
// keeps under "Keep the original file names" (the folder is named as usual).
export function namingPreviewPath(preset: string, format?: string, kind: 'movie' | 'tv' = 'movie', keep = false, folder?: string): string {
  const params = new URLSearchParams({ preset, kind })
  if (format) params.set('format', format)
  if (keep) params.set('keep', '1')
  if (folder) params.set('folder', folder)
  return `/settings/naming-preview?${params.toString()}`
}
