// Run with: npm test (uses node's built-in test runner, no extra packages).
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { albumRescanNote, type AlbumRescanReport } from './albumRescan.ts'

const base: AlbumRescanReport = { status: 'downloaded', quality: 'FLAC', folder: true, files: 12, tracks: 12, matched: 12, linked: 0, dropped: 0, extra: 0, complete: true, missingTitles: [], monitored: true }

test('a complete album says so, with its quality, and offers no search', () => {
  const n = albumRescanNote(base)
  assert.equal(n.good, true)
  assert.equal(n.offerSearch, false)
  assert.match(n.lines[0], /Every track has a file\. Quality: FLAC\./)
})

test('an incomplete monitored album lists what is missing and offers the search', () => {
  const n = albumRescanNote({ ...base, matched: 9, complete: false, missingTitles: ['A', 'B'], linked: 1 })
  assert.equal(n.offerSearch, true)
  assert.equal(n.good, false)
  const text = n.lines.join(' ')
  assert.match(text, /9 of 12/)
  assert.match(text, /still incomplete/)
  assert.match(text, /Missing: A, B and 1 more/)
  assert.match(text, /1 track linked/)
})

test('an incomplete album that is not monitored says nothing will be searched for', () => {
  const n = albumRescanNote({ ...base, matched: 5, complete: false, missingTitles: ['A'], monitored: false })
  assert.equal(n.offerSearch, false)
  assert.match(n.lines.join(' '), /isn't monitored/)
})

test('an empty folder says the album is wanted again, a deleted file is named', () => {
  const n = albumRescanNote({ ...base, status: 'missing', folder: false, files: 0, matched: 0, complete: false, deleted: 'CD1/01 - Song.flac' })
  assert.match(n.lines[0], /Deleted 01 - Song\.flac\./)
  assert.match(n.lines[1], /wanted again/)
  assert.equal(n.offerSearch, false)
})
