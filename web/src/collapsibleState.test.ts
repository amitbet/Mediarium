import assert from 'node:assert/strict'
import { test } from 'node:test'
import { readOpen, rememberOpen, type KeyValueStore } from './collapsibleState.ts'

function memory(): KeyValueStore & { data: Map<string, string> } {
  const data = new Map<string, string>()
  return { data, getItem: (k) => data.get(k) ?? null, setItem: (k, v) => void data.set(k, v) }
}

const broken: KeyValueStore = {
  getItem() {
    throw new Error('blocked')
  },
  setItem() {
    throw new Error('blocked')
  },
}

test('a section nobody has touched uses its default', () => {
  const store = memory()
  assert.equal(readOpen('scores', false, store), false)
  assert.equal(readOpen('scores', true, store), true)
})

test('the choice is remembered per section', () => {
  const store = memory()
  rememberOpen('scores', true, store)
  rememberOpen('sizes', false, store)
  assert.equal(readOpen('scores', false, store), true)
  assert.equal(readOpen('sizes', true, store), false)
  assert.equal(readOpen('music', true, store), true)
})

test('something else in storage is ignored', () => {
  const store = memory()
  store.data.set('mediarium-open:scores', 'banana')
  assert.equal(readOpen('scores', true, store), true)
})

test('storage that refuses never throws', () => {
  assert.equal(readOpen('scores', true, broken), true)
  assert.doesNotThrow(() => rememberOpen('scores', true, broken))
  assert.equal(readOpen('scores', false, null), false)
  assert.doesNotThrow(() => rememberOpen('scores', true, null))
})
