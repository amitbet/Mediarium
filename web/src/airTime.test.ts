// Run with: npm test (uses node's built-in test runner, no extra packages).
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { dayInZone, entryDay, entryTime, groupByDay, isZone, timeInZone } from './airTime.ts'

const ep = (releaseDate: string, airAt?: string) => ({ kind: 'episode', releaseDate, airAt })

test('a Pacific 9pm show lands on the next morning for a viewer in the UK', () => {
  // Monday 15 Jan 2024, 21:00 in Los Angeles (UTC-8) is Tuesday 05:00 UTC.
  const show = ep('2024-01-15', '2024-01-16T05:00:00Z')
  assert.equal(entryDay(show, 'America/Los_Angeles'), '2024-01-15')
  assert.equal(entryTime(show, 'America/Los_Angeles'), '21:00')
  assert.equal(entryDay(show, 'Europe/London'), '2024-01-16')
  assert.equal(entryTime(show, 'Europe/London'), '05:00')
  // One hour further east: still Tuesday, an hour later.
  assert.equal(entryDay(show, 'Europe/Paris'), '2024-01-16')
  assert.equal(entryTime(show, 'Europe/Paris'), '06:00')
})

test('in summer the same Pacific 9pm is 04:00 UTC and 05:00 in London', () => {
  // Monday 17 June 2024, 21:00 PDT (UTC-7) is Tuesday 04:00 UTC, 05:00 BST.
  const show = ep('2024-06-17', '2024-06-18T04:00:00Z')
  assert.equal(entryDay(show, 'America/Los_Angeles'), '2024-06-17')
  assert.equal(entryDay(show, 'Europe/London'), '2024-06-18')
  assert.equal(entryTime(show, 'Europe/London'), '05:00')
  assert.equal(entryTime(show, 'UTC'), '04:00')
})

test('23:30 UTC stays on Monday in London in winter and moves to Tuesday in Paris', () => {
  const show = ep('2024-01-15', '2024-01-15T23:30:00Z')
  assert.equal(entryDay(show, 'Europe/London'), '2024-01-15')
  assert.equal(entryTime(show, 'Europe/London'), '23:30')
  assert.equal(entryDay(show, 'Europe/Paris'), '2024-01-16')
  assert.equal(entryTime(show, 'Europe/Paris'), '00:30')
})

test('23:30 UTC in summer is already Tuesday in London (BST)', () => {
  const show = ep('2024-06-17', '2024-06-17T23:30:00Z')
  assert.equal(entryDay(show, 'Europe/London'), '2024-06-18')
  assert.equal(entryTime(show, 'Europe/London'), '00:30')
})

test('the night the clocks go forward in the UK', () => {
  // 31 Mar 2024: 01:00 UTC is when London jumps to BST, so 01:30 UTC is 02:30.
  assert.equal(timeInZone('2024-03-31T00:30:00Z', 'Europe/London'), '00:30')
  assert.equal(timeInZone('2024-03-31T01:30:00Z', 'Europe/London'), '02:30')
})

test('an episode with no known time stays on its date, whatever the zone', () => {
  const show = ep('2024-01-15')
  for (const zone of ['Pacific/Auckland', 'UTC', 'America/Los_Angeles']) {
    assert.equal(entryDay(show, zone), '2024-01-15')
    assert.equal(entryTime(show, zone), '')
  }
})

test('only episodes are moved by an air time', () => {
  const movie = { kind: 'movie', releaseDate: '2024-01-15', airAt: '2024-01-16T05:00:00Z' }
  assert.equal(entryDay(movie, 'Europe/London'), '2024-01-15')
  assert.equal(entryTime(movie, 'Europe/London'), '')
})

test('an unreadable time falls back to the date', () => {
  assert.equal(entryDay(ep('2024-01-15', 'tonight'), 'Europe/London'), '2024-01-15')
  assert.equal(timeInZone('tonight', 'UTC'), '')
})

test('the viewer changing zone moves the day but never the stored moment', () => {
  const show = ep('2024-01-15', '2024-01-16T05:00:00Z')
  const before = JSON.stringify(show)
  const days = ['America/Los_Angeles', 'Europe/London', 'Asia/Tokyo'].map((z) => entryDay(show, z))
  assert.deepEqual(days, ['2024-01-15', '2024-01-16', '2024-01-16'])
  assert.equal(JSON.stringify(show), before)
  assert.equal(new Date(show.airAt!).getTime(), Date.UTC(2024, 0, 16, 5))
})

test('two shows on Monday evening in their own countries split up for a UK viewer', () => {
  const uk = { ...ep('2024-01-15', '2024-01-15T21:00:00Z'), id: 1 }
  const us = { ...ep('2024-01-15', '2024-01-16T05:00:00Z'), id: 2 }
  const dateOnly = { ...ep('2024-01-15'), id: 3 }
  const london = groupByDay([us, dateOnly, uk], 'Europe/London')
  assert.deepEqual([...london.keys()].sort(), ['2024-01-15', '2024-01-16'])
  // No time first, then by time.
  assert.deepEqual(london.get('2024-01-15')?.map((e) => e.id), [3, 1])
  assert.deepEqual(london.get('2024-01-16')?.map((e) => e.id), [2])
  const la = groupByDay([us, dateOnly, uk], 'America/Los_Angeles')
  assert.deepEqual([...la.keys()], ['2024-01-15'])
  assert.deepEqual(la.get('2024-01-15')?.map((e) => e.id), [3, 1, 2])
})

test('dayInZone follows the wall calendar of the zone', () => {
  const t = Date.UTC(2024, 11, 31, 23, 30)
  assert.equal(dayInZone(t, 'UTC'), '2024-12-31')
  assert.equal(dayInZone(t, 'Pacific/Auckland'), '2025-01-01')
  assert.equal(dayInZone(t, 'America/Los_Angeles'), '2024-12-31')
})

test('isZone accepts real zones only', () => {
  assert.equal(isZone('Europe/London'), true)
  assert.equal(isZone('UTC'), true)
  assert.equal(isZone('Mars/Olympus'), false)
})
