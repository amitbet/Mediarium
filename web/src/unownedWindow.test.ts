import assert from 'node:assert/strict'
import { test } from 'node:test'
import { unownedWindow } from './unownedWindow.ts'

const page = (from: number, n = 20) => Array.from({ length: n }, (_, i) => ({ tmdbId: from + i }))
const pages = (count: number) => new Map(Array.from({ length: count }, (_, i) => [i + 1, page(i * 20 + 1)] as const))
const ids = (w: { items: { tmdbId: number }[] }) => w.items.map((m) => m.tmdbId)

test('nothing owned: the first window comes straight from the first page', () => {
  const w = unownedWindow(pages(3), () => false, 0, 5, 40, 30)
  assert.deepEqual(ids(w), [1, 2, 3, 4, 5])
  assert.deepEqual(w.need, [])
  assert.equal(w.total, undefined)
})

test('owned titles are skipped and the window is filled from later pages', () => {
  // Titles 1 to 25 are owned: the first 5 free ones are 26 to 30, on page 2.
  const w = unownedWindow(pages(2), (id) => id <= 25, 0, 5, 3, 30)
  assert.deepEqual(ids(w), [26, 27, 28, 29, 30])
  assert.deepEqual(w.need, [])
})

test('asks for the next pages when the fetched ones do not hold enough', () => {
  const w = unownedWindow(pages(1), (id) => id <= 18, 0, 6, 10, 30)
  assert.deepEqual(ids(w), [19, 20])
  assert.deepEqual(w.need, [2, 3])
})

test('never asks past the last page or the scan cap', () => {
  assert.deepEqual(unownedWindow(pages(1), () => true, 0, 6, 2, 30).need, [2])
  assert.deepEqual(unownedWindow(new Map([[1, page(1)], [2, page(21)]]), () => true, 0, 6, 100, 3).need, [3])
})

test('the end of the list gives the total without the owned titles', () => {
  const w = unownedWindow(pages(2), (id) => id % 2 === 0, 0, 30, 2, 30)
  assert.equal(w.total, 20)
  assert.equal(w.items.length, 20)
  assert.deepEqual(w.need, [])
})

test('a list that is all owned ends empty instead of reading on forever', () => {
  const w = unownedWindow(pages(30), () => true, 0, 6, undefined, 30)
  assert.equal(w.total, 0)
  assert.deepEqual(w.items, [])
  assert.deepEqual(w.need, [])
})

test('a second page of the window starts where the first one stopped', () => {
  const owned = (id: number) => id % 2 === 1
  const first = unownedWindow(pages(3), owned, 0, 4, 3, 30)
  const second = unownedWindow(pages(3), owned, 4, 8, 3, 30)
  assert.deepEqual(ids(first), [2, 4, 6, 8])
  assert.deepEqual(ids(second), [10, 12, 14, 16])
})

test('a title that TMDB repeats on two pages is listed once', () => {
  const m = new Map([[1, [{ tmdbId: 1 }, { tmdbId: 2 }]], [2, [{ tmdbId: 2 }, { tmdbId: 3 }]]])
  assert.deepEqual(ids(unownedWindow(m, () => false, 0, 10, 2, 30)), [1, 2, 3])
})
