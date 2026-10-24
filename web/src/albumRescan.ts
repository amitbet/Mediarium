import { plural, t } from './i18n.ts'

// What a rescan of an album's folder (or the deletion of one of its files)
// left the album as, as sentences for the album card.
export interface AlbumRescanReport {
  status: 'missing' | 'downloading' | 'downloaded'
  quality?: string
  folder: boolean // the album still has audio files in a folder
  files: number
  tracks: number
  matched: number
  linked: number
  dropped: number
  extra: number
  complete: boolean
  missingTitles: string[]
  monitored: boolean
  deleted?: string
  trashed?: boolean
}

export interface AlbumRescanNote {
  lines: string[]
  // The album is monitored and still lacks tracks: offer "Search for missing tracks".
  offerSearch: boolean
  // Nothing is lacking: shown calmly, with no warning colour.
  good: boolean
}

export function albumRescanNote(r: AlbumRescanReport): AlbumRescanNote {
  const lines: string[] = []
  if (r.deleted) lines.push(t('Deleted {file}.', { file: r.deleted.split('/').pop() ?? r.deleted }))
  if (!r.folder && r.matched === 0) {
    lines.push(r.monitored ? t('No audio files are left in the album folder, so the album is wanted again.') : t("No audio files are left in the album folder. The album isn't monitored, so Mediarium won't look for it."))
    return { lines, offerSearch: false, good: false }
  }
  const quality = r.quality ? ' ' + t('Quality: {quality}.', { quality: r.quality }) : ''
  if (r.complete) lines.push(t('Every track has a file.') + quality)
  else lines.push(t('Tracks with a file: {n} of {total}.', { n: r.matched, total: r.tracks }) + quality)
  if (r.linked > 0) lines.push(plural(r.linked, '{n} track linked to a file.', '{n} tracks linked to files.'))
  if (r.dropped > 0) lines.push(plural(r.dropped, '{n} file is gone, so its track is empty.', '{n} files are gone, so their tracks are empty.'))
  if (r.extra > 0) lines.push(plural(r.extra, '{n} file fits no track.', '{n} files fit no track.'))
  if (!r.complete) {
    lines.push(r.monitored ? t('The album is still incomplete.') : t("The album is still incomplete. It isn't monitored, so Mediarium won't look for the rest."))
    if (r.missingTitles.length > 0) {
      const more = r.tracks - r.matched - r.missingTitles.length
      const names = r.missingTitles.join(', ') + (more > 0 ? ' ' + t('and {n} more', { n: more }) : '')
      lines.push(t('Missing: {titles}', { titles: names }))
    }
  }
  return { lines, offerSearch: !r.complete && r.monitored, good: r.complete }
}
