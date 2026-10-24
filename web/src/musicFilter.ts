import { t } from './i18n.ts'

// What the music pages list: the release types, the secondary types to hide
// and whether bootleg-only releases are hidden. The server owns the rules
// (internal/music/filter.go); this is the wording and the small helpers the
// settings card and the artist page share.

export interface MusicFilter {
  types: string[]
  hideSecondary: string[]
  hideBootlegs: boolean
}

// The order the reasons are listed in: types first, then secondary types in
// the order of the settings card, bootlegs last.
export const REASON_ORDER = ['album', 'ep', 'single', 'live', 'remix', 'compilation', 'dj-mix', 'soundtrack', 'demo', 'mixtape', 'spokenword', 'interview', 'audiobook', 'audio-drama', 'other', 'bootleg']

// The release types, with the plural names the cards use.
export function typeName(key: string): string {
  switch (key) {
    case 'album':
      return t('Albums')
    case 'ep':
      return t('EPs')
    case 'single':
      return t('Singles')
    default:
      return key
  }
}

// The name of a secondary type on the checkboxes.
export function secondaryName(key: string): string {
  switch (key) {
    case 'live':
      return t('Live')
    case 'remix':
      return t('Remix')
    case 'compilation':
      return t('Compilation')
    case 'dj-mix':
      return t('DJ-mix')
    case 'soundtrack':
      return t('Soundtrack')
    case 'demo':
      return t('Demo')
    case 'mixtape':
      return t('Mixtape/Street')
    case 'spokenword':
      return t('Spokenword')
    case 'interview':
      return t('Interview')
    case 'audiobook':
      return t('Audiobook')
    case 'audio-drama':
      return t('Audio drama')
    default:
      return t('Other')
  }
}

// What a hidden release is called in "3 hidden: 2 live, 1 bootleg": one
// short word that reads after a number.
export function reasonWord(key: string): string {
  switch (key) {
    case 'album':
      return t('album')
    case 'ep':
      return t('EP')
    case 'single':
      return t('single')
    case 'live':
      return t('live')
    case 'remix':
      return t('remix')
    case 'compilation':
      return t('compilation')
    case 'dj-mix':
      return t('DJ-mix')
    case 'soundtrack':
      return t('soundtrack')
    case 'demo':
      return t('demo')
    case 'mixtape':
      return t('mixtape')
    case 'spokenword':
      return t('spoken word')
    case 'interview':
      return t('interview')
    case 'audiobook':
      return t('audiobook')
    case 'audio-drama':
      return t('audio drama')
    case 'bootleg':
      return t('bootleg')
    default:
      return t('other')
  }
}

// "3 hidden: 2 live, 1 bootleg", from the count per reason the server sends.
// Empty when nothing is hidden.
export function hiddenSummary(counts: Record<string, number>): string {
  const keys = Object.keys(counts).filter((k) => counts[k] > 0)
  if (keys.length === 0) return ''
  const rank = (k: string) => {
    const i = REASON_ORDER.indexOf(k)
    return i < 0 ? REASON_ORDER.length : i
  }
  keys.sort((a, b) => rank(a) - rank(b))
  const total = keys.reduce((n, k) => n + counts[k], 0)
  const list = keys.map((k) => `${counts[k]} ${reasonWord(k)}`).join(', ')
  return t('{n} hidden: {list}', { n: total, list })
}

function sameSet(a: string[], b: string[]): boolean {
  return a.length === b.length && a.every((x) => b.includes(x))
}

// Whether two choices are the same.
export function sameFilter(a: MusicFilter, b: MusicFilter): boolean {
  return a.hideBootlegs === b.hideBootlegs && sameSet(a.types, b.types) && sameSet(a.hideSecondary, b.hideSecondary)
}

// Adds or removes one key from a list, keeping the order of `order`.
export function toggleKey(list: string[], key: string, order: string[]): string[] {
  const next = list.includes(key) ? list.filter((k) => k !== key) : [...list, key]
  return order.filter((k) => next.includes(k))
}
