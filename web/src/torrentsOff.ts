import { st } from './i18n.ts'

// What happens when a torrent release is picked while torrents are switched
// off (Settings > Downloading > Torrents): the server refuses with a stable
// code, and the page offers the one thing that fixes it.

// The code that comes with the refusal (the words themselves are translated).
export const TORRENTS_OFF_CODE = 'torrents_off'

// How the refusal begins in English, for a failure that was stored as text
// (a download that failed in Activity) and so carries no code.
const TORRENTS_OFF_TEXT = 'Torrents are switched off.'

// True for an error from the server that says torrents are switched off.
export function isTorrentsOff(e: unknown): boolean {
  if (typeof e !== 'object' || e === null) return false
  return (e as { code?: unknown }).code === TORRENTS_OFF_CODE
}

// True for the stored text of a download that failed for that reason.
export function isTorrentsOffText(raw: string | null | undefined): boolean {
  if (!raw) return false
  const text = raw.trim()
  return text.startsWith(TORRENTS_OFF_TEXT) || text.startsWith(st(TORRENTS_OFF_TEXT))
}

// Whether any of the releases is a torrent.
export function hasTorrentRelease(results: readonly { protocol?: string }[]): boolean {
  return results.some((r) => r.protocol === 'torrent')
}

// Whether a release list should warn that its torrents can't be downloaded:
// torrents are off and the list has at least one torrent.
export function showTorrentsOffHint(torrentsOn: boolean, results: readonly { protocol?: string }[]): boolean {
  return !torrentsOn && hasTorrentRelease(results)
}

// What to save to switch torrents on. When the downloader choice is "Usenet
// only", it goes back to "both", the same as turning torrents on in Settings.
export function switchOnChanges(defaultSources: string | undefined): { torrentEnabled: true; defaultSources?: 'both' } {
  return defaultSources === 'usenet' ? { torrentEnabled: true, defaultSources: 'both' } : { torrentEnabled: true }
}
