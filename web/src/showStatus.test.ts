import assert from 'node:assert/strict'
import { test } from 'node:test'
import { isFinishedShow, isRunningShow, showRibbon } from './showStatus.ts'

test('ended and canceled shows are finished, the others are not', () => {
  const cases: [string | undefined, boolean, boolean][] = [
    ['ended', true, false],
    ['canceled', true, false],
    ['continuing', false, true],
    ['upcoming', false, true],
    ['', false, false],
    [undefined, false, false],
  ]
  for (const [status, finished, running] of cases) {
    assert.equal(isFinishedShow(status), finished, `finished: ${status}`)
    assert.equal(isRunningShow(status), running, `running: ${status}`)
  }
})

test('only a finished show gets a corner ribbon', () => {
  assert.equal(showRibbon('ended')?.tone, 'ended')
  assert.equal(showRibbon('canceled')?.tone, 'ended')
  assert.notEqual(showRibbon('ended')?.label, '')
  assert.equal(showRibbon('continuing'), undefined)
  assert.equal(showRibbon(''), undefined)
  assert.equal(showRibbon(undefined), undefined)
})
