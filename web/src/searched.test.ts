// Run with: npm test (uses node's built-in test runner, no extra packages).
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { indexerChoices, resultText, seconds, summaryLine, why, type Searched } from './searched.ts'

const line = (name: string, status: Searched['status'], extra: Partial<Searched> = {}): Searched => ({
  indexerId: 1,
  name,
  kind: 'torrent',
  status,
  count: 0,
  ms: 0,
  ...extra,
})

test('the summary line counts each kind of answer and leaves out the empty ones', () => {
  const eight = [
    line('A', 'found', { count: 3 }),
    line('B', 'found', { count: 9 }),
    line('C', 'empty'),
    line('D', 'empty'),
    line('E', 'empty'),
    line('F', 'empty'),
    line('G', 'empty'),
    line('H', 'failed', { code: 'daily_limit' }),
  ]
  assert.equal(summaryLine(eight), 'Searched 8 indexers: 2 had releases, 5 had nothing for this title, 1 did not answer.')
})

test('the summary line uses the singular for one indexer and mentions skipped ones', () => {
  assert.equal(summaryLine([line('A', 'empty')]), 'Searched 1 indexer: 1 had nothing for this title.')
  assert.equal(
    summaryLine([line('A', 'found', { count: 1 }), line('B', 'skipped', { code: 'no_category' })]),
    'Searched 2 indexers: 1 had releases, 1 skipped.',
  )
})

test('each result says what happened in plain words', () => {
  assert.equal(resultText(line('A', 'found', { count: 12 })), '12 releases')
  assert.equal(resultText(line('A', 'found', { count: 1 })), '1 release')
  assert.equal(resultText(line('A', 'empty')), 'Nothing found')
  assert.equal(resultText(line('A', 'failed', { code: 'timeout' })), 'Did not answer: timed out')
  assert.equal(resultText(line('A', 'failed', { code: 'daily_limit' })), 'Did not answer: daily limit used up')
  assert.equal(resultText(line('A', 'failed', { code: 'error' })), 'Did not answer: returned an error')
  assert.equal(resultText(line('A', 'skipped', { code: 'no_category' })), 'Skipped: does not list this kind of release')
  assert.equal(resultText(line('A', 'skipped', { code: 'disabled' })), 'Skipped: turned off')
  assert.equal(resultText(line('A', 'skipped', { code: 'paused' })), 'Skipped: paused after repeated failures')
})

test('a failure without a known code still gets a reason', () => {
  assert.equal(why(line('A', 'failed')), 'returned an error')
  assert.equal(why(line('A', 'skipped', { reason: 'Because.' })), 'Because.')
})

test('the time is in seconds, and empty when the indexer was not asked', () => {
  assert.equal(seconds(0), '')
  assert.equal(seconds(420), '0.4 s')
  assert.equal(seconds(1500), '1.5 s')
  assert.equal(seconds(12400), '12 s')
})

test('the filter lists every indexer that was asked, with counts', () => {
  const releases = [{ indexerName: 'Lime' }, { indexerName: 'Lime' }, { indexerName: 'Extra' }]
  const searched = [line('YTS', 'empty'), line('Lime', 'found', { count: 2 }), line('Slug', 'failed'), line('Extra', 'found', { count: 1 }), line('Off', 'skipped')]
  assert.deepEqual(
    indexerChoices(releases, searched).map((c) => [c.value, c.count, c.failed]),
    [
      ['Extra', 1, false],
      ['Lime', 2, false],
      ['Slug', 0, true],
      ['YTS', 0, false],
    ],
  )
})

test('without the search details the filter lists the indexers of the releases', () => {
  const releases = [{ indexerName: 'B' }, { indexerName: 'A' }, { indexerName: 'B' }]
  assert.deepEqual(
    indexerChoices(releases).map((c) => [c.value, c.count]),
    [
      ['A', 1],
      ['B', 2],
    ],
  )
})
