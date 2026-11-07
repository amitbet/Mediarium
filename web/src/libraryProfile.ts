// The Library's quality profile filter and sort. A title that follows the
// default (profile 0) counts under the profile that default is today.

interface Titled {
  kind: 'movie' | 'tv'
  profileId: number
  title: string
}

interface Named {
  id: number
  name: string
}

// effectiveProfileId is the profile a title really uses.
export function effectiveProfileId(i: Pick<Titled, 'kind' | 'profileId'>, defaultMovie: number, defaultTv: number): number {
  return i.profileId || (i.kind === 'movie' ? defaultMovie : defaultTv)
}

// profileFilterOptions lists the profiles that at least one of the titles uses,
// with how many titles use each, in the order the profiles are given.
export function profileFilterOptions(items: Pick<Titled, 'kind' | 'profileId'>[], profiles: Named[], defaultMovie: number, defaultTv: number): { id: number; name: string; count: number }[] {
  const counts = new Map<number, number>()
  for (const i of items) {
    const id = effectiveProfileId(i, defaultMovie, defaultTv)
    counts.set(id, (counts.get(id) ?? 0) + 1)
  }
  return profiles.filter((p) => counts.has(p.id)).map((p) => ({ id: p.id, name: p.name, count: counts.get(p.id) ?? 0 }))
}

// matchesProfile is the filter itself; '' lets everything through.
export function matchesProfile(i: Pick<Titled, 'kind' | 'profileId'>, wanted: string, defaultMovie: number, defaultTv: number): boolean {
  return !wanted || String(effectiveProfileId(i, defaultMovie, defaultTv)) === wanted
}

// profileName is the name shown in the table; '' when the profile is gone.
export function profileName(i: Pick<Titled, 'kind' | 'profileId'>, profiles: Named[], defaultMovie: number, defaultTv: number): string {
  const id = effectiveProfileId(i, defaultMovie, defaultTv)
  return profiles.find((p) => p.id === id)?.name ?? ''
}

// compareByProfile sorts by profile name (A to Z, titles with no known profile
// last), then by the title order given.
export function compareByProfile<T extends Pick<Titled, 'kind' | 'profileId'>>(a: T, b: T, profiles: Named[], defaultMovie: number, defaultTv: number, byTitle: (a: T, b: T) => number): number {
  const na = profileName(a, profiles, defaultMovie, defaultTv)
  const nb = profileName(b, profiles, defaultMovie, defaultTv)
  if (na !== nb) {
    if (!na) return 1
    if (!nb) return -1
    return na.localeCompare(nb, undefined, { numeric: true, sensitivity: 'base' })
  }
  return byTitle(a, b)
}
