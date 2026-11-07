import { formatBytes } from './format.ts'
import { plural, t } from './i18n.ts'

// A release picked by hand for a title that already has a file would be turned
// away by the import (the title is set to leave its file alone, or the release
// is not better). So the page asks first, and when the person agrees the grab
// carries `replace: true`. These are the pieces of that question.

// A file the title has now: its name, quality and size.
export interface ReplaceFile {
  name: string
  quality?: string
  sizeBytes?: number
}

// The release that was picked: its name and what the list shows of it.
export interface PickedRelease {
  title: string
  resolution?: string
  source?: string
  sizeBytes?: number
}

// The file name of a path, from either kind of slash.
export function baseName(path: string): string {
  const parts = path.split(/[\\/]/).filter(Boolean)
  return parts.length > 0 ? parts[parts.length - 1] : path
}

interface MovieLike {
  status: string
  filePath?: string
  quality?: string
  sizeBytes?: number
}

// The file a movie has, or null when it has none (nothing to replace).
export function movieFileToReplace(m: MovieLike): ReplaceFile | null {
  if (m.status !== 'downloaded' || !m.filePath) return null
  return { name: baseName(m.filePath), quality: m.quality, sizeBytes: m.sizeBytes }
}

interface EpisodeLike {
  season: number
  episode: number
  status: string
  filePath?: string
  quality?: string
  sizeBytes?: number
}

// What a release covers, as the search list knows it: the season and episodes
// its name says. No episodes with a season means a whole-season pack.
export interface Covered {
  season?: number
  episodes?: number[]
}

// The files of the episodes a picked release covers that already have one.
// A release that names no season is taken for what the search was for: one
// episode, or the whole season.
export function episodeFilesToReplace(eps: EpisodeLike[], release: Covered, target: { season: number; episode?: number } | null): ReplaceFile[] {
  const season = release.season ?? target?.season
  if (season === undefined) return []
  let wanted: number[] | null = null // null = every episode of the season
  if (release.season !== undefined && release.episodes && release.episodes.length > 0) wanted = release.episodes
  else if (release.season === undefined && target?.episode) wanted = [target.episode]
  const out: ReplaceFile[] = []
  for (const e of eps) {
    if (e.season !== season || e.status !== 'downloaded' || !e.filePath) continue
    if (wanted && !wanted.includes(e.episode)) continue
    out.push({ name: baseName(e.filePath), quality: e.quality, sizeBytes: e.sizeBytes })
  }
  return out
}

// "Name.mkv (WEBDL-1080p, 4.2 GB)": a file or release with what is known of it.
export function describe(name: string, quality?: string, sizeBytes?: number): string {
  const bits = [quality, sizeBytes && sizeBytes > 0 ? formatBytes(sizeBytes) : ''].filter(Boolean)
  return bits.length > 0 ? `${name} (${bits.join(', ')})` : name
}

export function releaseQuality(r: PickedRelease): string {
  return [r.resolution, r.source].filter(Boolean).join(' ')
}

// The words of the question. `title` heads the dialog, `lines` are its paragraphs.
export function replaceQuestion(files: ReplaceFile[], release: PickedRelease): { title: string; lines: string[] } {
  const picked = describe(release.title, releaseQuality(release), release.sizeBytes)
  if (files.length <= 1) {
    const f = files[0]
    return {
      title: t('Replace the file you have?'),
      lines: [
        t('This will replace {old} with {release}.', { old: f ? describe(f.name, f.quality, f.sizeBytes) : '', release: picked }),
        t('The old file goes to the recycle bin. If the recycle bin is off, it is deleted.'),
      ],
    }
  }
  return {
    title: t('Replace the files you have?'),
    lines: [
      plural(
        files.length,
        'This will replace the file of {n} episode that already has one with the files in {release}.',
        'This will replace the files of {n} episodes that already have one with the files in {release}.',
        { release: picked },
      ),
      t('Episodes that have no file yet are filled in as usual.'),
      t('The old files go to the recycle bin. If the recycle bin is off, they are deleted.'),
    ],
  }
}
