// Run with: npm test (uses node's built-in test runner, no extra packages).
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { afterClick, afterKey, type OpenGroup } from './navGroups.ts'

test('a click from another page goes to the group and opens it', () => {
  assert.equal(afterClick(null, 'library', false), 'library')
  assert.equal(afterClick(null, 'settings', false), 'settings')
})

test('opening one group closes the other', () => {
  assert.equal(afterClick('library', 'settings', false), 'settings')
  assert.equal(afterClick('settings', 'library', false), 'library')
  // Also from a page inside the group that was clicked.
  assert.equal(afterClick('library', 'settings', true), 'settings')
})

test('a click on the group you are in folds its sub menu up and down', () => {
  assert.equal(afterClick(null, 'library', true), 'library')
  assert.equal(afterClick('library', 'library', true), null)
  assert.equal(afterClick('settings', 'settings', true), null)
})

test('a click from another page opens a group that was already open', () => {
  // Open, then moved on to the Dashboard: clicking the group again goes back to it and keeps it open.
  assert.equal(afterClick('library', 'library', false), 'library')
})

test('arrow keys: Right opens, Left folds up, anything else is left alone', () => {
  const cases: [OpenGroup, 'library' | 'settings', string, OpenGroup | undefined][] = [
    [null, 'library', 'ArrowRight', 'library'],
    ['settings', 'library', 'ArrowRight', 'library'],
    ['library', 'library', 'ArrowRight', undefined],
    ['library', 'library', 'ArrowLeft', null],
    [null, 'library', 'ArrowLeft', undefined],
    ['settings', 'library', 'ArrowLeft', undefined],
    ['library', 'library', 'Enter', undefined],
    ['library', 'library', ' ', undefined],
  ]
  for (const [open, group, key, want] of cases) assert.equal(afterKey(open, group, key), want, `${open} ${group} ${key}`)
})
