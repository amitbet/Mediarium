import { useCallback, useEffect, useRef, useState } from 'react'
import { api, type OwnedAnswer } from './api'
import { movieState, seriesState, type ItemStateInput } from './components/state'
import { ownedPollMs } from './pollPlan'
import { useLive } from './useLive'

export interface Owned {
  movies: Map<number, ItemStateInput>
  shows: Map<number, ItemStateInput>
  reload: () => void
}

// Which TMDB titles are already in the library, and their state (wanted,
// downloading, downloaded), kept live so Discover cards update on their own.
// It reads one small answer (GET /api/library/owned: ids, status and the
// downloads of those titles) instead of the whole movie list, show list and
// queue. It asks every 30 seconds, every 15 while a download moves, and not
// at all while the tab is hidden.
export function useOwned(): Owned {
  const [movies, setMovies] = useState<Map<number, ItemStateInput>>(new Map())
  const [shows, setShows] = useState<Map<number, ItemStateInput>>(new Map())
  const [ms, setMs] = useState(ownedPollMs(undefined))

  // The same answer keeps the same Map, so the cards on Discover are not redrawn every poll for nothing.
  // The server answers "nothing changed" with the very same object; then there is nothing to rebuild.
  const seen = useRef<OwnedAnswer | undefined>(undefined)
  const reload = useCallback(() => {
    const same = (next: Map<number, ItemStateInput>) => (prev: Map<number, ItemStateInput>) => (JSON.stringify([...prev]) === JSON.stringify([...next]) ? prev : next)
    api.libraryOwned().then((a) => {
      if (seen.current === a) return
      seen.current = a
      setMs(ownedPollMs(a.queue))
      setMovies(same(new Map(a.movies.map((x) => [x.tmdbId, { id: x.id, state: movieState(x, a.queue) }]))))
      setShows(same(new Map(a.shows.map((x) => [x.tmdbId, { id: x.id, state: seriesState(x, a.queue) }]))))
    }).catch(() => undefined)
  }, [])

  useEffect(reload, [reload])
  useLive(reload, ms)
  return { movies, shows, reload }
}
