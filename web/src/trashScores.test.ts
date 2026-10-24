import assert from 'node:assert/strict'
import { test } from 'node:test'
import { importScoreAction, profileForApp, scorePreview, signed } from './trashScores.ts'

test('a format that is not added yet gets the guide score, a 0 included', () => {
  const scores = new Map<number, number>()
  assert.deepEqual(scorePreview({ defaultScore: 1700, importedId: 0 }, scores), { willGet: 1700, current: null, flag: null })
  assert.deepEqual(scorePreview({ defaultScore: 0, importedId: 0 }, scores), { willGet: 0, current: null, flag: null })
})

test('no profile chosen means nothing is scored', () => {
  assert.deepEqual(scorePreview({ defaultScore: 500, importedId: 4 }, null), { willGet: null, current: 0, flag: null })
})

test('an added format scored 0 although the guides score it is flagged', () => {
  const none = new Map<number, number>()
  assert.equal(scorePreview({ defaultScore: 500, importedId: 4 }, none).flag, 'zero')
  assert.equal(scorePreview({ defaultScore: -10000, importedId: 4 }, new Map([[4, 0]])).flag, 'zero')
  // A format the guides don't score is fine at 0.
  assert.equal(scorePreview({ defaultScore: 0, importedId: 4 }, none).flag, null)
})

test('an added format with another score than the guides is marked as different, the same score is not', () => {
  assert.deepEqual(scorePreview({ defaultScore: 500, importedId: 4 }, new Map([[4, 120]])), { willGet: 500, current: 120, flag: 'differs' })
  assert.equal(scorePreview({ defaultScore: 500, importedId: 4 }, new Map([[4, 500]])).flag, null)
})

test('scores are written with a plus and shows can have their own default profile', () => {
  assert.equal(signed(5), '+5')
  assert.equal(signed(-5), '-5')
  assert.equal(signed(0), '0')
  assert.equal(profileForApp('radarr', 1, 2), 1)
  assert.equal(profileForApp('sonarr', 1, 2), 2)
  assert.equal(profileForApp('sonarr', 1, 0), 1)
})

test('a format the profile has is skipped unless scores are to be replaced', () => {
  const f = { defaultScore: 500, importedId: 7 }
  const has = new Map([[7, 100]])
  assert.equal(importScoreAction(f, has, false), 'skip')
  assert.equal(importScoreAction(f, has, true), 'replace')
  assert.equal(importScoreAction(f, new Map(), false), 'add')
  assert.equal(importScoreAction({ defaultScore: 0, importedId: 7 }, new Map([[7, 0]]), false), 'skip')
  assert.equal(importScoreAction({ defaultScore: 500, importedId: 0 }, has, false), 'add')
  assert.equal(importScoreAction(f, null, true), 'none')
})
