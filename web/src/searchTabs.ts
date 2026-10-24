import type { BookFound, MusicArtistResult } from './api'
import { t } from './i18n.ts'

// The tabs of the search results page. "Releases" is its own page; these
// share /search and are told apart by the tab in the address.
export type ResultsTab = 'titles' | 'books' | 'music'

// resultsTab is the tab to show: the one in the address when it can be shown,
// else the first one that is switched on (titles, books, then music).
export function resultsTab(param: string | null, titlesOn: boolean, booksOn: boolean, musicOn = false): ResultsTab {
  if (param === 'books' && booksOn) return 'books'
  if (param === 'music' && musicOn) return 'music'
  if (param === 'titles' && titlesOn) return 'titles'
  if (titlesOn) return 'titles'
  if (booksOn) return 'books'
  if (musicOn) return 'music'
  return 'titles'
}

// resultsLink is the address of a results tab for a search term. The titles
// are the default tab, so they have no tab in the address.
export function resultsLink(tab: ResultsTab, q: string): string {
  const p = new URLSearchParams()
  const term = q.trim()
  if (term) p.set('q', term)
  if (tab !== 'titles') p.set('tab', tab)
  const s = p.toString()
  return s ? `/search?${s}` : '/search'
}

// resultsParams is the query part of the address for the term typed on a tab.
export function resultsParams(tab: ResultsTab, q: string): Record<string, string> {
  const term = q.trim()
  const out: Record<string, string> = {}
  if (term) out.q = term
  if (tab !== 'titles') out.tab = tab
  return out
}

// booksTabLabel is the label of the Books tab: with the number of books found
// once the search has answered.
export function booksTabLabel(count: number | null): string {
  return count === null ? t('Books') : t('Books ({n})', { n: count })
}

// musicTabLabel is the label of the Music tab: with the number of artists
// found once the search has answered.
export function musicTabLabel(count: number | null): string {
  return count === null ? t('Music') : t('Music ({n})', { n: count })
}

// splitArtists puts the artists already in the library first, like the titles.
// An artist added from a card on the page counts as in the library too.
export function splitArtists(list: MusicArtistResult[], added: Record<string, number> = {}): { owned: MusicArtistResult[]; fresh: MusicArtistResult[] } {
  const marked = list.map((a) => (added[a.mbid] ? { ...a, artistId: added[a.mbid] } : a))
  return { owned: marked.filter((a) => a.artistId > 0), fresh: marked.filter((a) => !(a.artistId > 0)) }
}

// splitBooks puts the books already in the library first, like the titles.
export function splitBooks(list: BookFound[], added: Record<string, number> = {}): { owned: BookFound[]; fresh: BookFound[] } {
  const marked = list.map((b) => (added[b.key] ? { ...b, libraryId: added[b.key] } : b))
  return { owned: marked.filter((b) => !!b.libraryId), fresh: marked.filter((b) => !b.libraryId) }
}
