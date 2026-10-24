import assert from 'node:assert/strict'
import { test } from 'node:test'
import { applyTrashSizes, maxBoxText } from './trashSizes.ts'

const rows = [
  { quality: 'WEBDL-1080p', minText: '4', maxText: '130' },
  { quality: 'Remux-1080p', minText: '35', maxText: '' },
  { quality: 'SDTV', minText: '2', maxText: '100' },
]

test('TRaSH sizes replace the numbers of the qualities they list and report the changes', () => {
  const r = applyTrashSizes(rows, [
    { quality: 'WEBDL-1080p', min: 12.5, max: 0 },
    { quality: 'Remux-1080p', min: 35, max: 0 },
  ])
  assert.deepEqual(r.rows[0], { quality: 'WEBDL-1080p', minText: '12.5', maxText: '' })
  assert.deepEqual(r.changes, [{ quality: 'WEBDL-1080p', fromMin: 4, fromMax: 130, toMin: 12.5, toMax: 0 }])
  // A quality TRaSH gives no size for stays as it is, and is named.
  assert.deepEqual(r.rows[2], rows[2])
  assert.deepEqual(r.untouched, ['SDTV'])
})

test('the same sizes again change nothing', () => {
  const r = applyTrashSizes(rows, [{ quality: 'WEBDL-1080p', min: 4, max: 130 }])
  assert.deepEqual(r.changes, [])
})

test('a half-typed or empty box counts as 0 and the input rows are not modified', () => {
  const draft = [{ quality: 'DVD', minText: '', maxText: 'abc' }]
  const r = applyTrashSizes(draft, [{ quality: 'DVD', min: 5, max: 90 }])
  assert.deepEqual(r.changes, [{ quality: 'DVD', fromMin: 0, fromMax: 0, toMin: 5, toMax: 90 }])
  assert.deepEqual(draft, [{ quality: 'DVD', minText: '', maxText: 'abc' }])
})

test('no limit is an empty box', () => {
  assert.equal(maxBoxText(0), '')
  assert.equal(maxBoxText(155), '155')
})
