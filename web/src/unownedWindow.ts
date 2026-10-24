// "Hide what I already have" on a paged Discover list. The hidden titles leave
// holes in the pages TMDB serves, so the grid has to read further ahead until
// it has enough titles you do not own, or the list ends.

export interface UnownedWindow<T> {
  // The titles to show, start to end of the list without the owned ones.
  items: T[]
  // Pages to ask for next, or none when everything needed is here.
  need: number[]
  // How many titles the whole list has once the owned ones are left out.
  // Known only when the end of the list has been reached.
  total?: number
}

// pages are the fetched TMDB pages (1-based, perFetch titles each, in order).
// lastPage is the last page the list has, once known. At most scanCap pages are
// read, so a library that holds everything cannot make Discover read forever.
export function unownedWindow<T extends { tmdbId: number }>(
  pages: Map<number, T[]>,
  isOwned: (tmdbId: number) => boolean,
  start: number,
  end: number,
  lastPage: number | undefined,
  scanCap: number,
  ahead = 2,
): UnownedWindow<T> {
  const seen = new Set<number>()
  const kept: T[] = []
  // One title past the window is read too, so the caller knows whether a next page exists.
  for (let p = 1; kept.length <= end; p++) {
    if ((lastPage !== undefined && p > lastPage) || p > scanCap) return { items: kept.slice(start, end), need: [], total: kept.length }
    const page = pages.get(p)
    if (!page) {
      const last = Math.min(p + ahead - 1, lastPage ?? Infinity, scanCap)
      return { items: kept.slice(start, end), need: Array.from({ length: last - p + 1 }, (_, i) => p + i) }
    }
    for (const m of page) {
      if (seen.has(m.tmdbId)) continue
      seen.add(m.tmdbId)
      if (!isOwned(m.tmdbId)) kept.push(m)
    }
  }
  return { items: kept.slice(start, end), need: [] }
}
