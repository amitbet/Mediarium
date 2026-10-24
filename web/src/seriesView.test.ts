import assert from 'node:assert/strict'
import { test } from 'node:test'
import { largestFirst, libraryTotalText, sumBytes } from './diskTotals.ts'
import { monitoringSummary, readSeasonsOpen, rememberSeasonsOpen, seasonStartsOpen, type KeyValueStore, type MonitorEpisode } from './seriesView.ts'

const today = '2026-10-18'
// Season 1 and 2 have aired, season 3 has not (one episode has no date yet).
const ep = (season: number, airDate: string | undefined, monitored: boolean): MonitorEpisode => ({ season, airDate, monitored })
const show = (m: { s1: boolean; s2: boolean; s3: boolean }): MonitorEpisode[] => [
  ep(1, '2024-01-01', m.s1),
  ep(1, '2024-01-08', m.s1),
  ep(2, '2025-01-01', m.s2),
  ep(2, '2025-01-08', m.s2),
  ep(3, '2027-01-01', m.s3),
  ep(3, undefined, m.s3),
  ep(0, '2020-01-01', false), // a special: never counted
]

test('the monitoring line says what the show is looking for', () => {
  const cases: { name: string; on: boolean; eps: MonitorEpisode[]; kind: string; text: string }[] = [
    { name: 'all', on: true, eps: show({ s1: true, s2: true, s3: true }), kind: 'all', text: 'Monitoring: all episodes' },
    { name: 'nothing ticked but the show is on: new episodes', on: true, eps: show({ s1: false, s2: false, s3: false }), kind: 'new', text: 'Monitoring: new episodes only' },
    { name: 'only what has not aired', on: true, eps: show({ s1: false, s2: false, s3: true }), kind: 'future', text: "Monitoring: only episodes that haven't aired yet" },
    { name: 'one season', on: true, eps: show({ s1: false, s2: true, s3: false }), kind: 'seasons', text: 'Monitoring: season 2 only' },
    { name: 'two seasons', on: true, eps: show({ s1: true, s2: false, s3: true }), kind: 'seasons', text: 'Monitoring: seasons 1, 3 only' },
    { name: 'switched off, nothing ticked', on: false, eps: show({ s1: false, s2: false, s3: false }), kind: 'off', text: 'Not monitoring' },
    { name: 'switched off with episodes ticked', on: false, eps: show({ s1: true, s2: true, s3: true }), kind: 'off', text: 'Not monitoring: the show is switched off' },
    { name: 'a few episodes', on: true, eps: [ep(1, '2024-01-01', true), ep(1, '2024-01-08', false), ep(2, '2025-01-01', false)], kind: 'some', text: 'Monitoring: some episodes' },
  ]
  for (const c of cases) {
    const got = monitoringSummary(c.on, c.eps, today)
    assert.equal(got.kind, c.kind, c.name)
    assert.equal(got.text, c.text, c.name)
  }
})

test('an episode that airs today has not aired yet', () => {
  const eps = [ep(1, '2024-01-01', false), ep(2, today, true)]
  assert.equal(monitoringSummary(true, eps, today).kind, 'future')
})

test('the counts leave the specials out', () => {
  assert.equal(monitoringSummary(true, show({ s1: true, s2: false, s3: false }), today).detail, 'Episodes monitored: 2 of 6')
  assert.equal(monitoringSummary(true, [], today).detail, '')
  assert.equal(monitoringSummary(false, show({ s1: false, s2: false, s3: false }), today).detail, '')
  assert.equal(monitoringSummary(true, show({ s1: false, s2: false, s3: false }), today).detail, 'Episodes monitored: 0 of 6')
})

function memory(): KeyValueStore & { data: Map<string, string> } {
  const data = new Map<string, string>()
  return { data, getItem: (k) => data.get(k) ?? null, setItem: (k, v) => void data.set(k, v) }
}

test('seasons start the way they always did until a choice is made', () => {
  assert.equal(readSeasonsOpen(memory()), 'latest')
  assert.equal(seasonStartsOpen('latest', 4, 4), true)
  assert.equal(seasonStartsOpen('latest', 3, 4), false)
})

test('collapsed folds every season and all opens every one', () => {
  for (const season of [0, 1, 4]) {
    assert.equal(seasonStartsOpen('collapsed', season, 4), false)
    assert.equal(seasonStartsOpen('all', season, 4), true)
  }
})

test('the seasons choice is remembered in this browser, and junk or a blocked store gives the default', () => {
  const store = memory()
  rememberSeasonsOpen('collapsed', store)
  assert.equal(readSeasonsOpen(store), 'collapsed')
  store.data.set('mediarium-seasons-open', 'banana')
  assert.equal(readSeasonsOpen(store), 'latest')
  const blocked: KeyValueStore = {
    getItem() {
      throw new Error('blocked')
    },
    setItem() {
      throw new Error('blocked')
    },
  }
  rememberSeasonsOpen('all', blocked)
  assert.equal(readSeasonsOpen(blocked), 'latest')
  assert.equal(readSeasonsOpen(null), 'latest')
})

test('sizes add up and sort largest first, ties by title', () => {
  const items = [
    { title: 'B', sizeBytes: 5 },
    { title: 'A', sizeBytes: 5 },
    { title: 'C', sizeBytes: 9 },
    { title: 'D' },
  ]
  assert.equal(sumBytes(items), 19)
  const byTitle = (a: { title: string }, b: { title: string }) => a.title.localeCompare(b.title)
  assert.deepEqual([...items].sort((a, b) => largestFirst(a, b, byTitle)).map((i) => i.title), ['C', 'A', 'B', 'D'])
})

test('the library total names the whole library, or what the filter shows of it', () => {
  assert.equal(libraryTotalText(0, 0, false, 'tv'), '')
  assert.match(libraryTotalText(2 * 1024 ** 4, 2 * 1024 ** 4, false, 'tv'), /^Shows: 2\.0 TB$/)
  assert.match(libraryTotalText(2 * 1024 ** 4, 2 * 1024 ** 4, false, 'movie'), /^Movies: 2\.0 TB$/)
  assert.match(libraryTotalText(2 * 1024 ** 4, 1024 ** 4, true, 'tv'), /^Shown: 1\.0 TB of 2\.0 TB$/)
  assert.match(libraryTotalText(2 * 1024 ** 4, 2 * 1024 ** 4, true, 'tv'), /^Shows: /)
})
