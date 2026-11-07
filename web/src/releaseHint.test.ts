// Run with: npm test (uses node's built-in test runner, no extra packages).
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { bookReleaseHint, releaseHint, skippedHint } from './releaseHint.ts'

const down = [{ name: 'Example NZB Site', message: 'Could not reach Example NZB Site.' }]

test('no indexer at all', () => {
  const h = releaseHint(0, [], 0)
  assert.equal(h?.text, 'No indexer is set up yet. Add one under')
  assert.equal(h?.link, true)
  assert.equal(h?.empty, true)
})

test('an indexer that did not answer is named, not blamed on missing setup', () => {
  const h = releaseHint(1, down, 0)
  assert.equal(h?.text, 'The indexer did not answer: Example NZB Site. Check it under')
  assert.equal(h?.link, true)
  assert.doesNotMatch(h?.text ?? '', /set up/)
  const two = releaseHint(2, [...down, { name: 'B', message: '' }], 0)
  assert.equal(two?.text, 'The indexers did not answer: Example NZB Site, B. Check them under')
})

test('some answered, none had the title', () => {
  const h = releaseHint(3, down, 0)
  assert.equal(h?.text, 'No matching releases. 1 indexer did not answer: Example NZB Site. Check it under')
})

test('everything answered and nothing matched', () => {
  const h = releaseHint(2, [], 0)
  assert.match(h?.text ?? '', /^No matching releases right now/)
  assert.equal(h?.link, false)
})

test('releases came back but none fits the title: says so and names some', () => {
  const h = releaseHint(2, [], 0, ['Some.Film.2006.1080p.BluRay-GRP', 'Some.Film.2023.2160p.WEB-GRP'])
  assert.equal(h?.empty, true)
  assert.match(h?.text ?? '', /^Your indexers answered with releases, but none of them fits this title and year/)
  assert.match(h?.text ?? '', /Some\.Film\.2006\.1080p\.BluRay-GRP; Some\.Film\.2023\.2160p\.WEB-GRP$/)
  // An indexer that did not answer is still the first thing to say.
  assert.match(releaseHint(2, down, 0, ['X'])?.text ?? '', /^No matching releases\. 1 indexer did not answer/)
  // A list with releases in it needs no such note.
  assert.equal(releaseHint(2, [], 5, ['X']), null)
})

test('releases found: only a note when someone did not answer', () => {
  assert.equal(releaseHint(2, [], 5), null)
  const h = releaseHint(2, down, 5)
  assert.equal(h?.empty, false)
  assert.equal(h?.text, '1 indexer did not answer: Example NZB Site. The list may be missing some releases.')
})

test('indexers left out of a book search are named', () => {
  assert.equal(skippedHint([]), null)
  assert.equal(skippedHint(['Movie Site']), "1 indexer that doesn't do books or audiobooks was skipped: Movie Site.")
  assert.equal(skippedHint(['A', 'B', 'C']), "3 indexers that don't do books or audiobooks were skipped: A, B, C.")
})

test('a book search with every indexer left out does not say none is set up', () => {
  const h = bookReleaseHint(0, [], 0, ['Movie Site'])
  assert.equal(h?.text, 'None of your indexers lists books or audiobooks. Check them under')
  assert.equal(h?.link, true)
  assert.equal(h?.empty, true)
  // Nothing skipped and nothing asked is still a missing setup.
  assert.equal(bookReleaseHint(0, [], 0, [])?.text, 'No indexer is set up yet. Add one under')
  // Otherwise it reads as before.
  assert.equal(bookReleaseHint(2, [], 5, ['X']), null)
  assert.match(bookReleaseHint(2, [], 0, ['X'])?.text ?? '', /^No matching releases right now/)
})
