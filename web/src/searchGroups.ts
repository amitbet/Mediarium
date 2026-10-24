// How the search box popup is laid out: which of the three tabs is showing and
// how many settings matches are listed. The popup is mostly about titles, so
// the settings matches stay compact (three rows) and never push the titles
// down unless what was typed is a settings name, and a person who never
// wants them can pick the Media tab once and the choice is remembered.

export type SearchTab = 'all' | 'media' | 'settings'

// SETTINGS_CAP is how many settings matches the All tab lists before it
// offers "Show all".
export const SETTINGS_CAP = 3

// LEAD_SCORE is the settingsIndex score from which a match counts as what was
// typed (the same or starting with it); only such a match leads the list.
export const LEAD_SCORE = 85

const STORE_KEY = 'mediarium-search-tab'

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

export function isSearchTab(v: unknown): v is SearchTab {
  return v === 'all' || v === 'media' || v === 'settings'
}

// readSearchTab is the tab the person used last, or "all". Reading never throws:
// private windows and blocked site data can refuse.
export function readSearchTab(store: KeyValueStore | null = browserStore()): SearchTab {
  try {
    const v = store?.getItem(STORE_KEY)
    if (isSearchTab(v)) return v
  } catch {
    // Not readable: the default is used.
  }
  return 'all'
}

export function rememberSearchTab(tab: SearchTab, store: KeyValueStore | null = browserStore()): void {
  try {
    store?.setItem(STORE_KEY, tab)
  } catch {
    // Not remembered; the popup still works.
  }
}

// effectiveTab is the tab that applies to this search. Without any settings
// match there is nothing to switch between, so the popup is the plain list
// (and the tab bar is not drawn).
export function effectiveTab(tab: SearchTab, settingsHits: number): SearchTab {
  return settingsHits > 0 ? tab : 'all'
}

export interface SettingsPlan {
  media: boolean // the titles, artists and books are listed
  show: boolean // a Settings group is listed
  lead: boolean // ... and comes before the media rows
  shown: number // settings rows listed
  more: number // matches left out by the cap (0 = none): a "Show all" row follows
}

// planSettings decides what the popup lists. hits is the number of settings
// matches, topScore the score of the best one, tab the tab (as effectiveTab
// returns it) and expanded whether "Show all" was pressed.
export function planSettings(hits: number, topScore: number, tab: SearchTab, expanded: boolean): SettingsPlan {
  if (hits <= 0 || tab === 'media') return { media: true, show: false, lead: false, shown: 0, more: 0 }
  if (tab === 'settings') return { media: false, show: true, lead: true, shown: hits, more: 0 }
  const shown = expanded ? hits : Math.min(hits, SETTINGS_CAP)
  return { media: true, show: true, lead: topScore >= LEAD_SCORE, shown, more: hits - shown }
}
