import { test } from 'node:test'
import assert from 'node:assert/strict'
import { failedForFolder, folderName, lostMarkerFolder } from './rootMarker.ts'

const lost = (p: string) =>
  `The library folder ${p} has lost its .mediarium-root file. That usually means its drive isn't connected and this is an empty folder on the machine's own disk, so nothing was put into it. Connect the drive and try again. If the folder is the right one, add an empty file named .mediarium-root to it.`

test('the folder is read from the lost-marker message', () => {
  const cases: [string | undefined, string | null][] = [
    [lost('/data/tv'), '/data/tv'],
    [lost('/mnt/My Shows/tv'), '/mnt/My Shows/tv'],
    [lost('D:\\Media\\TV'), 'D:\\Media\\TV'],
    ['The library folder /data/tv can\'t be found, so nothing was put into it.', null],
    ['import MobLand.S02E06.mkv: ' + lost('/data/tv'), '/data/tv'],
    ['', null],
    [undefined, null],
  ]
  for (const [msg, want] of cases) assert.equal(lostMarkerFolder(msg), want, String(msg).slice(0, 40))
})

test('only the failed downloads refused for that folder are picked to retry', () => {
  const items = [
    { id: 1, status: 'failed', error: lost('/data/tv') },
    { id: 2, status: 'failed', error: lost('/data/movies') },
    { id: 3, status: 'failed', error: 'Not enough space' },
    { id: 4, status: 'completed', error: lost('/data/tv') },
    { id: 5, status: 'failed', error: lost('/data/tv') },
  ]
  assert.deepEqual(failedForFolder(items, '/data/tv'), [1, 5])
  assert.deepEqual(failedForFolder(items, '/data/other'), [])
})

test('the folder name to type is the last part of the path', () => {
  const cases: [string, string][] = [
    ['/data/tv', 'tv'],
    ['/data/tv/', 'tv'],
    ['D:\\Media\\TV', 'TV'],
    ['/', '/'],
  ]
  for (const [p, want] of cases) assert.equal(folderName(p), want)
})
