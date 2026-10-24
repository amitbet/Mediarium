// What a release search says about each indexer it had to do with: the
// summary line above the list, the table behind it and the indexer filter.
import { plural, st, t } from './i18n.ts'

export type SearchedStatus = 'found' | 'empty' | 'failed' | 'skipped'

export interface Searched {
  indexerId: number
  name: string
  kind: string // 'torrent' or 'usenet'
  status: SearchedStatus
  count: number // releases of this indexer in the list
  ms: number // how long its search took
  // The short why behind a failed or skipped line, and the same in a sentence.
  code?: string
  reason?: string
}

// summaryLine is the one short line: "Searched 8 indexers: 2 had releases, 5 had
// nothing for this title, 1 did not answer." Parts that are zero are left out.
export function summaryLine(searched: Searched[]): string {
  const n = (s: SearchedStatus) => searched.filter((x) => x.status === s).length
  const parts: string[] = []
  if (n('found')) parts.push(t('{n} had releases', { n: n('found') }))
  if (n('empty')) parts.push(t('{n} had nothing for this title', { n: n('empty') }))
  if (n('failed')) parts.push(t('{n} did not answer', { n: n('failed') }))
  if (n('skipped')) parts.push(t('{n} skipped', { n: n('skipped') }))
  const head = plural(searched.length, 'Searched {n} indexer', 'Searched {n} indexers')
  return t('{head}: {parts}.', { head, parts: parts.join(', ') })
}

// why puts a failed or skipped line's code in a few plain words.
export function why(s: Searched): string {
  switch (s.code) {
    case 'timeout':
      return t('timed out')
    case 'daily_limit':
      return t('daily limit used up')
    case 'rate_limited':
      return t('too many requests')
    case 'auth':
      return t('key or login refused')
    case 'unreachable':
      return t('could not be reached')
    case 'vpn':
      return t('waiting for the VPN')
    case 'paused':
      return t('paused after repeated failures')
    case 'disabled':
      return t('turned off')
    case 'no_category':
      return t('does not list this kind of release')
    default:
      return s.status === 'failed' ? t('returned an error') : s.reason ? st(s.reason) : t('not asked')
  }
}

// resultText is the Result column: "12 releases", "Nothing found", "Did not
// answer: timed out", "Skipped: turned off".
export function resultText(s: Searched): string {
  switch (s.status) {
    case 'found':
      return plural(s.count, '{n} release', '{n} releases')
    case 'empty':
      return t('Nothing found')
    case 'failed':
      return t('Did not answer: {why}', { why: why(s) })
    default:
      return t('Skipped: {why}', { why: why(s) })
  }
}

// seconds writes how long a search took: "0.4 s", "12 s". A search that was
// not asked has no time.
export function seconds(ms: number): string {
  if (ms <= 0) return ''
  const s = ms / 1000
  return t('{n} s', { n: s < 10 ? s.toFixed(1) : String(Math.round(s)) })
}

export interface IndexerChoice {
  value: string
  label: string
  count: number
  failed: boolean
}

// indexerChoices lists the indexers the filter offers: every one that was
// asked (those with nothing too, those that did not answer marked), and the
// ones named by a release even if the search did not report them. Counts come
// from the releases in the list, so they are what the filter will show.
export function indexerChoices(releases: { indexerName: string }[], searched?: Searched[]): IndexerChoice[] {
  const counts = new Map<string, number>()
  for (const r of releases) counts.set(r.indexerName, (counts.get(r.indexerName) ?? 0) + 1)
  const out = new Map<string, IndexerChoice>()
  for (const s of searched ?? []) {
    if (s.status === 'skipped') continue
    out.set(s.name, { value: s.name, label: s.name, count: counts.get(s.name) ?? 0, failed: s.status === 'failed' })
  }
  for (const [name, count] of counts) if (!out.has(name)) out.set(name, { value: name, label: name, count, failed: false })
  return [...out.values()].sort((a, b) => a.label.localeCompare(b.label))
}
