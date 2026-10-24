import { t } from './i18n.ts'

// Helpers for a show's page: what is being monitored, in plain words, and
// which seasons start open.

export interface MonitorEpisode {
  season: number
  monitored: boolean
  airDate?: string
}

export type MonitoringKind = 'off' | 'all' | 'new' | 'future' | 'seasons' | 'some'

export interface MonitoringSummary {
  kind: MonitoringKind
  // The short answer ("Monitoring: all episodes").
  text: string
  // The counts ("12 of 40 episodes monitored"); empty when there is nothing to count.
  detail: string
  // Seasons that are monitored as a whole, for kind "seasons".
  seasons: number[]
}

// monitoringSummary says what a show is looking for. The show's own switch
// is "on" when it watches for new episodes or has any monitored episode; the
// episodes (specials left out, they are never looked for by default) say
// which ones. today is a YYYY-MM-DD date: an episode with no date or a date
// from today on has not aired yet.
export function monitoringSummary(showMonitored: boolean, episodes: MonitorEpisode[], today: string): MonitoringSummary {
  const real = episodes.filter((e) => e.season > 0)
  const total = real.length
  const monitored = real.filter((e) => e.monitored)
  const count = monitored.length
  const detail = total > 0 ? t('Episodes monitored: {monitored} of {n}', { monitored: count, n: total }) : ''
  const make = (kind: MonitoringKind, text: string, seasons: number[] = []): MonitoringSummary => ({ kind, text, detail, seasons })

  if (!showMonitored) {
    return {
      kind: 'off',
      text: count > 0 ? t('Not monitoring: the show is switched off') : t('Not monitoring'),
      detail: count > 0 ? detail : '',
      seasons: [],
    }
  }
  if (count === 0) return make('new', t('Monitoring: new episodes only'))
  if (count === total) return make('all', t('Monitoring: all episodes'))

  const unaired = (e: MonitorEpisode) => !e.airDate || e.airDate >= today
  if (real.every((e) => e.monitored === unaired(e))) return make('future', t("Monitoring: only episodes that haven't aired yet"))

  // Seasons that are monitored as a whole, with nothing monitored in the others.
  const bySeason = new Map<number, { all: number; on: number }>()
  for (const e of real) {
    const s = bySeason.get(e.season) ?? { all: 0, on: 0 }
    s.all++
    if (e.monitored) s.on++
    bySeason.set(e.season, s)
  }
  const whole = [...bySeason.entries()].filter(([, s]) => s.on > 0)
  if (whole.every(([, s]) => s.on === s.all)) {
    const seasons = whole.map(([n]) => n).sort((a, b) => a - b)
    if (seasons.length === 1) return make('seasons', t('Monitoring: season {n} only', { n: seasons[0] }), seasons)
    if (seasons.length <= 4) return make('seasons', t('Monitoring: seasons {list} only', { list: seasons.join(', ') }), seasons)
  }
  return make('some', t('Monitoring: some episodes'))
}

// How the seasons of a show's page start: all folded, only the one that needs
// attention (the latest with missing episodes, else the latest) open, or all open.
export type SeasonsOpen = 'collapsed' | 'latest' | 'all'

export const SEASONS_OPEN_DEFAULT: SeasonsOpen = 'latest'
const SEASONS_KEY = 'mediarium-seasons-open'

export interface KeyValueStore {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
}

function browserStore(): KeyValueStore | null {
  try {
    return typeof localStorage === 'undefined' ? null : localStorage
  } catch {
    return null
  }
}

export function isSeasonsOpen(v: unknown): v is SeasonsOpen {
  return v === 'collapsed' || v === 'latest' || v === 'all'
}

// readSeasonsOpen is this browser's choice, or the default (what the page did
// before there was a choice) when none was made or it can't be read.
export function readSeasonsOpen(store: KeyValueStore | null = browserStore()): SeasonsOpen {
  try {
    const v = store?.getItem(SEASONS_KEY)
    if (isSeasonsOpen(v)) return v
  } catch {
    // Not readable: the default is used.
  }
  return SEASONS_OPEN_DEFAULT
}

export function rememberSeasonsOpen(mode: SeasonsOpen, store: KeyValueStore | null = browserStore()): void {
  try {
    store?.setItem(SEASONS_KEY, mode)
  } catch {
    // Not remembered; the page still works.
  }
}

// seasonStartsOpen is whether a season is open before the person has opened or
// folded anything on this visit; attention is the season that needs it most.
export function seasonStartsOpen(mode: SeasonsOpen, season: number, attention: number | undefined): boolean {
  if (mode === 'all') return true
  if (mode === 'collapsed') return false
  return season === attention
}
