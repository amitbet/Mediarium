import { formatBytes } from './format.ts'
import { t } from './i18n.ts'

// Sizes on disk for the Library header and the show page.

interface Sized {
  sizeBytes?: number
}

// sumBytes adds up what the titles (or episodes) take on disk.
export function sumBytes(items: Sized[]): number {
  return items.reduce((n, i) => n + (i.sizeBytes ?? 0), 0)
}

// largestFirst orders by size on disk, biggest first; titles that take the
// same room fall back to the title order given.
export function largestFirst<T extends Sized>(a: T, b: T, byTitle: (a: T, b: T) => number): number {
  return (b.sizeBytes ?? 0) - (a.sizeBytes ?? 0) || byTitle(a, b)
}

// libraryTotalText is the line in the Library header: what all the movies (or
// all the shows) take, and when a filter narrows the list, what the titles
// shown take. Empty when nothing has a size yet.
export function libraryTotalText(total: number, shown: number, filtered: boolean, kind: 'movie' | 'tv'): string {
  if (total <= 0) return ''
  if (filtered && shown !== total) return t('Shown: {shown} of {size}', { shown: formatBytes(shown), size: formatBytes(total) })
  return kind === 'movie' ? t('Movies: {size}', { size: formatBytes(total) }) : t('Shows: {size}', { size: formatBytes(total) })
}
