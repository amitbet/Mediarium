import type { AddCollectionBody, SourcePref } from './api'

// The choices picked in the add window for a movie. The rest of its collection
// is added with exactly these.
export interface MovieAddChoices {
  profileId: number
  rootPath: string
  monitored: boolean
  sources: SourcePref
  searchNow: boolean
}

// The body for "add the whole collection" that matches the movie that was just
// added: same quality profile, folder, monitoring and search-now choice. The
// movie itself is already in the library, so the server leaves it alone.
export function collectionBodyFor(c: MovieAddChoices): AddCollectionBody {
  return {
    profileId: c.profileId || undefined,
    rootPath: c.rootPath || undefined,
    monitored: c.monitored,
    sources: c.sources,
    searchNow: c.searchNow && c.monitored,
  }
}

// Whether the tick box is offered: only for a movie, only for an account that
// can add collections, and only when the film belongs to one.
export function offersCollectionAdd(isMovie: boolean, admin: boolean, asks: boolean, collectionId: number | undefined): boolean {
  return isMovie && admin && !asks && !!collectionId
}
