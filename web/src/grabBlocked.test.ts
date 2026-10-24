// Run with: npm test
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { blockingDetail, blockingOf } from './grabBlocked.ts'

const refused = (blocking: unknown, status = 409) => Object.assign(new Error('Already downloading: X. Cancel it first to pick another.'), { status, blocking })

test('a refused grab names the download in the way', () => {
  const b = blockingOf(refused({ id: 7, title: 'Show.S01.1080p', progress: 42.4, sizeBytes: 3 * 1024 ** 3, paused: false }))
  assert.deepEqual(b, { id: 7, title: 'Show.S01.1080p', progress: 42.4, sizeBytes: 3 * 1024 ** 3, paused: false })
})

test('anything else is not a download in the way', () => {
  assert.equal(blockingOf(null), null)
  assert.equal(blockingOf(new Error('nope')), null)
  assert.equal(blockingOf(refused(undefined)), null)
  assert.equal(blockingOf(refused({ id: 0, title: 'x' })), null)
  assert.equal(blockingOf(refused({ id: 3 })), null)
  assert.equal(blockingOf(refused({ id: 3, title: 'x' }, 400)), null)
})

test('odd numbers are tidied', () => {
  const b = blockingOf(refused({ id: 3, title: 'x', progress: 250, sizeBytes: -5 }))
  assert.equal(b?.progress, 100)
  assert.equal(b?.sizeBytes, 0)
  assert.equal(b?.paused, false)
})

test('the progress line says how far it has come', () => {
  assert.match(blockingDetail({ id: 1, title: 'x', progress: 42.4, sizeBytes: 3 * 1024 ** 3, paused: false }), /^42% of 3[.,]0 GB done$/)
  assert.equal(blockingDetail({ id: 1, title: 'x', progress: 0, sizeBytes: 0, paused: false }), '0% done')
  assert.equal(blockingDetail({ id: 1, title: 'x', progress: 10, sizeBytes: 0, paused: true }), 'Paused at 10%')
  assert.match(blockingDetail({ id: 1, title: 'x', progress: 10, sizeBytes: 1024 ** 2, paused: true }), /^Paused at 10% of 1[.,]0 MB$/)
})
