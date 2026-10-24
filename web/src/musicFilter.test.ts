// Run with: npm test (uses node's built-in test runner, no extra packages).
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { hiddenSummary, sameFilter, toggleKey } from './musicFilter.ts'

test('nothing hidden says nothing', () => {
  assert.equal(hiddenSummary({}), '')
  assert.equal(hiddenSummary({ live: 0 }), '')
})

test('the hidden count names each reason', () => {
  assert.equal(hiddenSummary({ live: 2, bootleg: 1 }), '3 hidden: 2 live, 1 bootleg')
  assert.equal(hiddenSummary({ bootleg: 1 }), '1 hidden: 1 bootleg')
})

test('reasons come in a fixed order whatever the server sent first', () => {
  assert.equal(hiddenSummary({ bootleg: 1, remix: 4, single: 2, live: 3 }), '10 hidden: 2 single, 3 live, 4 remix, 1 bootleg')
})

test('a type MusicBrainz adds later is called other', () => {
  assert.equal(hiddenSummary({ other: 2, 'audio-drama': 1 }), '3 hidden: 1 audio drama, 2 other')
})

test('toggling a key keeps the list in order', () => {
  const order = ['album', 'ep', 'single']
  assert.deepEqual(toggleKey(['album', 'single'], 'ep', order), ['album', 'ep', 'single'])
  assert.deepEqual(toggleKey(['album', 'ep'], 'album', order), ['ep'])
})

test('two choices are the same whatever the order of their lists', () => {
  const a = { types: ['album', 'ep'], hideSecondary: ['live', 'remix'], hideBootlegs: false }
  assert.ok(sameFilter(a, { types: ['ep', 'album'], hideSecondary: ['remix', 'live'], hideBootlegs: false }))
  assert.ok(!sameFilter(a, { ...a, hideBootlegs: true }))
  assert.ok(!sameFilter(a, { ...a, types: ['album'] }))
})
