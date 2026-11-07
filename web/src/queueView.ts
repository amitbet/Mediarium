// How the Activity page arranges and describes the download queue. Kept apart
// from the page so the order and the wording can be tested. The server sorts
// the list the same way (sortQueueItems in Go); this repeats it so the page
// stays in order even when it is shown a list in another order.
import type { QueueItem, QueueUnpack } from './api'
import { formatBytes } from './format.ts'
import { t } from './i18n.ts'

type Status = QueueItem['status']

// Downloading and post-processing come first, then waiting, then paused: the
// order rows appear in the "In progress" section.
const RANK: Record<Status, number> = {
  downloading: 0,
  importing: 0,
  queued: 1,
  paused: 2,
  failed: 3,
  conflict: 4,
  stopped: 5,
  completed: 6,
}

export const RUNNING: ReadonlySet<Status> = new Set<Status>(['queued', 'downloading', 'importing'])

const time = (s?: string) => {
  const t = s ? Date.parse(s) : NaN
  return Number.isNaN(t) ? 0 : t
}

// In-progress rows are ordered by when they started (first started on top), the
// ones waiting in line in the order they will start, and the rest newest first. Nothing here depends on progress, so a row never moves
// while it downloads.
export function sortQueue(items: readonly QueueItem[]): QueueItem[] {
  return [...items].sort((a, b) => {
    const ra = RANK[a.status] ?? 7
    const rb = RANK[b.status] ?? 7
    if (ra !== rb) return ra - rb
    if (a.status === 'queued' && b.status === 'queued') return (a.queuePosition ?? Infinity) - (b.queuePosition ?? Infinity) || a.id - b.id
    if (ra <= 2) return time(a.addedAt) - time(b.addedAt) || a.id - b.id
    const ta = time(a.completedAt) || time(a.addedAt)
    const tb = time(b.completedAt) || time(b.addedAt)
    return tb - ta || b.id - a.id
  })
}

export interface QueueSections {
  inProgress: QueueItem[] // downloading and importing, then waiting, then paused
  failed: QueueItem[]
  decision: QueueItem[] // a file already exists and someone has to choose
  stopped: QueueItem[]
  finished: QueueItem[] // completed: shown under History
}

export function groupQueue(items: readonly QueueItem[]): QueueSections {
  const sorted = sortQueue(items)
  const only = (...s: Status[]) => sorted.filter((q) => s.includes(q.status))
  return {
    inProgress: only('downloading', 'importing', 'queued', 'paused'),
    failed: only('failed'),
    decision: only('conflict'),
    stopped: only('stopped'),
    finished: only('completed'),
  }
}

// What the status badge says.
export function statusLabel(q: Pick<QueueItem, 'status' | 'interrupted' | 'pending' | 'unpack'> & { waiting?: boolean }): string {
  if (q.pending === 'pausing') return t('Pausing…')
  if (q.pending === 'stopping') return t('Stopping…')
  switch (q.status) {
    case 'queued':
      return t('Waiting in line')
    case 'downloading':
      return t('Downloading')
    case 'importing':
      if (q.waiting) return t('Waiting for MusicBrainz')
      return q.unpack ? t('Unpacking') : t('Importing')
    case 'paused':
      return q.interrupted ? t('Partly downloaded, stopped') : t('Paused')
    case 'stopped':
      return t('Stopped')
    case 'failed':
      return t('Failed')
    case 'conflict':
      return t('Needs a decision')
    case 'completed':
      return t('Done')
  }
}

// The line under a download that is being unpacked: how much is done, in plain
// words. The total is the size of the archive's volumes, so it is the same
// whatever the files inside compress to.
export function unpackLine(u: QueueUnpack): string {
  const done = formatBytes(u.doneBytes)
  if (u.totalBytes <= 0) return t('Unpacking: {done} so far', { done })
  const total = formatBytes(u.totalBytes)
  if (u.count > 1) return t('Unpacking archive {n} of {count}: {done} of {total}', { n: u.index, count: u.count, done, total })
  return t('Unpacking: {done} of {total}', { done, total })
}

// Where a download waiting in line stands: "Next in line", "3rd in line",
// "11th in line", "21st in line". Each English ending is its own phrase, so
// other languages can write the place their own way.
export function linePlace(position?: number): string {
  if (!position || position < 1) return t('Waiting in line')
  if (position === 1) return t('Next in line')
  const n = String(position)
  const last = position % 10
  const teen = position % 100 >= 11 && position % 100 <= 13
  if (!teen && last === 1) return t('{n}st in line', { n })
  if (!teen && last === 2) return t('{n}nd in line', { n })
  if (!teen && last === 3) return t('{n}rd in line', { n })
  return t('{n}th in line', { n })
}

// The line under a waiting, paused or stopped download that says what happened
// and what can be done.
export function stateNote(q: Pick<QueueItem, 'status' | 'interrupted' | 'keptFiles' | 'progressPct' | 'queuePosition'>): string {
  if (q.status === 'queued') return q.queuePosition ? linePlace(q.queuePosition) : ''
  if (q.status === 'paused') {
    if (q.interrupted) {
      return q.keptFiles
        ? t('Mediarium was restarted while this was downloading. Press Resume to carry on from what is already saved.')
        : t('Mediarium was restarted before this started downloading. Press Resume to start it.')
    }
    return q.keptFiles || q.progressPct > 0 ? t('Paused. What is downloaded so far is kept. Press Resume to carry on.') : t('Paused. Press Resume to start it.')
  }
  if (q.status === 'stopped') {
    return q.keptFiles ? t('The part that was downloaded is still on disk.') : ''
  }
  return ''
}

// Which of the two buttons at the top of the queue can be used.
export function bulkState(items: readonly QueueItem[]): { canPause: boolean; canResume: boolean } {
  return {
    canPause: items.some((q) => RUNNING.has(q.status) && !q.pending),
    canResume: items.some((q) => q.status === 'paused'),
  }
}

// Titles that only look like they are waiting for a release because a
// download for them is already in the list (running, paused or waiting for a
// decision) are left out of "Waiting for a release".
export function stillWaiting<T extends { movieId?: number; albumId?: number }>(waiting: readonly T[], items: readonly QueueItem[]): T[] {
  const open = items.filter((q) => q.status === 'queued' || q.status === 'downloading' || q.status === 'importing' || q.status === 'paused' || q.status === 'conflict')
  const movies = new Set(open.filter((q) => q.movieId && !q.seriesId && !q.albumId).map((q) => q.movieId))
  const albums = new Set(open.filter((q) => q.albumId).map((q) => q.albumId))
  return waiting.filter((w) => !(w.movieId && movies.has(w.movieId)) && !(w.albumId && albums.has(w.albumId)))
}
