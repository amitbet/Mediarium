// Run with: npm test (uses node's built-in test runner, no extra packages).
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { addName, emptyDraft, fileLine, mergeRules, previewLine, rulesOf, seasonShortcut, toDraft, toRule } from './customNumbering.ts'

test('a whole season renumbered', () => {
  const r = { librarySeason: 1, releaseSeason: 15 }
  assert.equal(previewLine(r), 'Library S01E01 = release S15E01')
  assert.equal(fileLine(r), 'File S15E01.mkv in the folder is found as library S01E01')
})

test('a continuing number shows the offset', () => {
  const r = { librarySeason: 2, releaseSeason: 1, offset: 12 }
  assert.equal(previewLine(r), 'Library S02E01 = release S01E13')
  assert.equal(fileLine(r), 'File S01E13.mkv in the folder is found as library S02E01')
})

test('a rule with a first episode talks about that one', () => {
  assert.equal(previewLine({ librarySeason: 1, releaseSeason: 15, from: 3, to: 8 }), 'Library S01E03 = release S15E03')
})

test('rows become rules, optional boxes may stay empty', () => {
  assert.deepEqual(toRule({ ...emptyDraft(), releaseSeason: '15' }), { librarySeason: 1, releaseSeason: 15 })
  assert.deepEqual(toRule({ librarySeason: '2', releaseSeason: '1', offset: '12', from: '', to: '' }), { librarySeason: 2, releaseSeason: 1, offset: 12 })
  assert.deepEqual(toRule({ librarySeason: '1', releaseSeason: '15', offset: '', from: '1', to: '8' }), { librarySeason: 1, releaseSeason: 15, from: 1, to: 8 })
})

test('unfinished or odd rows are not rules', () => {
  assert.equal(toRule(emptyDraft()), null)
  assert.equal(toRule({ ...emptyDraft(), releaseSeason: '1.5' }), null)
  assert.equal(toRule({ ...emptyDraft(), releaseSeason: '0' }), null)
  assert.equal(toRule({ ...emptyDraft(), releaseSeason: '3', from: '-1' }), null)
  const { rules, skipped } = rulesOf([{ ...emptyDraft(), releaseSeason: '15' }, emptyDraft()])
  assert.equal(rules.length, 1)
  assert.equal(skipped, 1)
})

test('a rule goes into a row and back', () => {
  const r = { librarySeason: 2, releaseSeason: 1, offset: -3, from: 4, to: 9 }
  assert.deepEqual(toRule(toDraft(r)), r)
})

test('the season shortcut', () => {
  assert.deepEqual(seasonShortcut(15, [1], false), [{ librarySeason: 1, releaseSeason: 15 }])
  assert.deepEqual(seasonShortcut(15, [1, 2, 3], false), [{ librarySeason: 1, releaseSeason: 15 }])
  assert.deepEqual(seasonShortcut(15, [3, 1, 2], true), [
    { librarySeason: 1, releaseSeason: 15 },
    { librarySeason: 2, releaseSeason: 16 },
    { librarySeason: 3, releaseSeason: 17 },
  ])
  assert.deepEqual(seasonShortcut(0, [1], false), [])
  assert.deepEqual(seasonShortcut(15, [], false), [{ librarySeason: 1, releaseSeason: 15 }])
})

test('pressing the shortcut again replaces the season, it does not repeat it', () => {
  const merged = mergeRules([{ librarySeason: 1, releaseSeason: 14 }, { librarySeason: 3, releaseSeason: 2 }], [{ librarySeason: 1, releaseSeason: 15 }])
  assert.deepEqual(merged, [
    { librarySeason: 1, releaseSeason: 15 },
    { librarySeason: 3, releaseSeason: 2 },
  ])
})

test('names are tidied and not repeated', () => {
  assert.deepEqual(addName([], '  The  Bachelor Ukraine '), ['The Bachelor Ukraine'])
  assert.deepEqual(addName(['The Bachelor Ukraine'], 'the bachelor ukraine'), ['The Bachelor Ukraine'])
  assert.deepEqual(addName(['A'], '   '), ['A'])
})
