import assert from 'node:assert/strict'
import { test } from 'node:test'
import { hasTorrentRelease, isTorrentsOff, isTorrentsOffText, showTorrentsOffHint, switchOnChanges } from './torrentsOff.ts'

test('only the stable code marks a refusal because torrents are off', () => {
  assert.equal(isTorrentsOff({ status: 409, code: 'torrents_off' }), true)
  assert.equal(isTorrentsOff({ status: 409, code: 'other' }), false)
  assert.equal(isTorrentsOff({ status: 409, message: 'Torrents are switched off.' }), false)
  assert.equal(isTorrentsOff(null), false)
  assert.equal(isTorrentsOff('torrents_off'), false)
})

test('a stored failure is recognised by how it begins', () => {
  assert.equal(isTorrentsOffText('Torrents are switched off. Switch them on in Settings > Downloading > Torrents to download torrent releases.'), true)
  assert.equal(isTorrentsOffText('  Torrents are switched off.'), true)
  assert.equal(isTorrentsOffText('The download failed'), false)
  assert.equal(isTorrentsOffText(''), false)
  assert.equal(isTorrentsOffText(undefined), false)
})

test('the hint shows only when torrents are off and the list has a torrent', () => {
  const mixed = [{ protocol: 'usenet' }, { protocol: 'torrent' }]
  const usenet = [{ protocol: 'usenet' }]
  assert.equal(hasTorrentRelease(mixed), true)
  assert.equal(hasTorrentRelease(usenet), false)
  assert.equal(hasTorrentRelease([]), false)
  assert.equal(showTorrentsOffHint(false, mixed), true)
  assert.equal(showTorrentsOffHint(false, usenet), false)
  assert.equal(showTorrentsOffHint(true, mixed), false)
  assert.equal(showTorrentsOffHint(false, []), false)
})

test('switching torrents on also leaves "Usenet only"', () => {
  assert.deepEqual(switchOnChanges('usenet'), { torrentEnabled: true, defaultSources: 'both' })
  assert.deepEqual(switchOnChanges('torrent'), { torrentEnabled: true })
  assert.deepEqual(switchOnChanges('both'), { torrentEnabled: true })
  assert.deepEqual(switchOnChanges(undefined), { torrentEnabled: true })
})
