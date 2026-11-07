import { test } from 'node:test'
import assert from 'node:assert/strict'
import { anyMoving, fileDetailsPending, libraryTick, ownedPollMs, pollEvery } from './pollPlan.ts'

test('only queued, downloading and importing downloads count as moving', () => {
  assert.equal(anyMoving([]), false)
  assert.equal(anyMoving([{ status: 'completed' }, { status: 'failed' }, { status: 'paused' }, { status: 'stopped' }]), false)
  for (const status of ['queued', 'downloading', 'importing']) assert.equal(anyMoving([{ status: 'failed' }, { status }]), true)
})

test('the fast clock runs only while something moves', () => {
  assert.equal(pollEvery(true, 3000, 10000), 3000)
  assert.equal(pollEvery(false, 3000, 10000), 10000)
})

test('the Library reads only the list of the tab that is shown', () => {
  assert.deepEqual(libraryTick('movie'), { movies: true, shows: false, artists: false, books: false })
  assert.deepEqual(libraryTick('tv'), { movies: false, shows: true, artists: false, books: false })
  assert.deepEqual(libraryTick('music'), { movies: false, shows: false, artists: true, books: false })
  assert.deepEqual(libraryTick('ebook'), { movies: false, shows: false, artists: false, books: true })
  assert.deepEqual(libraryTick('audiobook'), { movies: false, shows: false, artists: false, books: true })
})

test('file details stop being asked for once every file is read', () => {
  assert.equal(fileDetailsPending(null), true)
  assert.equal(fileDetailsPending({ done: 3, total: 10 }), true)
  assert.equal(fileDetailsPending({ done: 10, total: 10 }), false)
  assert.equal(fileDetailsPending({ done: 0, total: 0 }), false)
})

test('Discover asks every 30 seconds, every 15 while a download moves', () => {
  assert.equal(ownedPollMs(undefined), 30000)
  assert.equal(ownedPollMs([{ status: 'failed' }]), 30000)
  assert.equal(ownedPollMs([{ status: 'downloading' }]), 15000)
})
