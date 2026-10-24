import assert from 'node:assert/strict'
import { test } from 'node:test'
import {
  allowedFromRows,
  cutoffRow,
  cutoffText,
  defaultGroupName,
  defaultRows,
  fixedCutoff,
  groupRows,
  isBuiltInOrder,
  moveRow,
  orderCounts,
  orderFromRows,
  orderProblem,
  qualitiesLine,
  renameGroup,
  rowsFromProfile,
  shiftRow,
  splitOut,
  toggleRow,
  trashLayout,
  ungroupRow,
  webAndTvTogether,
} from './qualityOrder.ts'

// The API lists the qualities worst first.
const ladder = [
  'Unknown', 'CAM/TeleSync', 'SDTV', 'DVD', 'WEBDL-480p', 'HDTV-720p', 'WEBDL-720p', 'Bluray-720p', 'HDTV-1080p',
  'WEBDL-1080p', 'Bluray-1080p', 'Remux-1080p', 'HDTV-2160p', 'WEBDL-2160p', 'Bluray-2160p', 'Remux-2160p',
]
const keys = (rows: { key: string }[]) => rows.map((r) => r.key)
const words = { noName: 'no name', tooLong: (n: string) => `long ${n}`, twice: (n: string) => `twice ${n}`, none: 'none' }

test('the built-in order lists accepted qualities first, best first', () => {
  const rows = defaultRows(ladder, ['HDTV-1080p', 'Bluray-1080p', 'WEBDL-1080p'])
  assert.deepEqual(keys(rows).slice(0, 3), ['Bluray-1080p', 'WEBDL-1080p', 'HDTV-1080p'])
  assert.equal(rows.length, ladder.length)
  assert.deepEqual(rows.map((r) => r.allowed).slice(0, 4), [true, true, true, false])
  assert.ok(isBuiltInOrder(ladder, rows))
})

test('a profile without its own order gets the built-in rows', () => {
  assert.deepEqual(rowsFromProfile(ladder, ['WEBDL-720p'], undefined), defaultRows(ladder, ['WEBDL-720p']))
  assert.deepEqual(rowsFromProfile(ladder, ['WEBDL-720p'], []), defaultRows(ladder, ['WEBDL-720p']))
})

test('a profile order becomes rows, the rest listed after it as left out', () => {
  const rows = rowsFromProfile(ladder, [], [
    { tiers: ['Bluray-1080p'] },
    { name: 'Web or TV', tiers: ['WEBDL-1080p', 'HDTV-1080p'] },
  ])
  assert.equal(rows[0].key, 'Bluray-1080p')
  assert.equal(rows[1].name, 'Web or TV')
  assert.deepEqual(rows[1].tiers, ['WEBDL-1080p', 'HDTV-1080p'])
  assert.ok(rows.slice(0, 2).every((r) => r.allowed))
  assert.ok(rows.slice(2).every((r) => !r.allowed))
  assert.equal(rows.flatMap((r) => r.tiers).length, ladder.length)
})

test('what goes to the server is the accepted rows in order, and it round-trips', () => {
  const order = [
    { tiers: ['Bluray-1080p'] },
    { name: 'Web or TV', tiers: ['WEBDL-1080p', 'HDTV-1080p'] },
    { tiers: ['WEBDL-720p'] },
  ]
  const rows = rowsFromProfile(ladder, [], order)
  assert.deepEqual(orderFromRows(rows), order)
  assert.deepEqual(allowedFromRows(ladder, rows), ['HDTV-720p'].slice(1).concat(['WEBDL-720p', 'HDTV-1080p', 'WEBDL-1080p', 'Bluray-1080p']))
})

test('moving rows', () => {
  const rows = defaultRows(ladder, ['HDTV-1080p', 'Bluray-1080p', 'WEBDL-1080p'])
  assert.deepEqual(keys(moveRow(rows, 'HDTV-1080p', 0)).slice(0, 3), ['HDTV-1080p', 'Bluray-1080p', 'WEBDL-1080p'])
  assert.deepEqual(keys(shiftRow(rows, 'Bluray-1080p', 1)).slice(0, 3), ['WEBDL-1080p', 'Bluray-1080p', 'HDTV-1080p'])
  // At the top or bottom of the accepted rows nothing leaves them.
  assert.deepEqual(keys(shiftRow(rows, 'Bluray-1080p', -1)), keys(rows))
  assert.deepEqual(keys(shiftRow(rows, 'HDTV-1080p', 1)).slice(0, 3), ['Bluray-1080p', 'WEBDL-1080p', 'HDTV-1080p'])
  assert.deepEqual(keys(moveRow(rows, 'nothing', 0)), keys(rows))
})

test('leaving out and accepting a row moves it between the two parts', () => {
  const rows = defaultRows(ladder, ['HDTV-1080p', 'Bluray-1080p', 'WEBDL-1080p'])
  const off = toggleRow(rows, 'Bluray-1080p')
  assert.deepEqual(keys(off).slice(0, 3), ['WEBDL-1080p', 'HDTV-1080p', 'Bluray-1080p'])
  assert.equal(off[2].allowed, false)
  const on = toggleRow(off, 'DVD')
  assert.deepEqual(keys(on).slice(0, 4), ['WEBDL-1080p', 'HDTV-1080p', 'DVD', 'Bluray-1080p'])
  assert.equal(on[2].allowed, true)
})

test('grouping puts the rows at the top-most place and sorts the qualities best first', () => {
  const rows = defaultRows(ladder, ['HDTV-1080p', 'Bluray-1080p', 'WEBDL-1080p', 'WEBDL-720p'])
  const g = groupRows(ladder, rows, ['HDTV-1080p', 'WEBDL-1080p'], 'Web or TV')
  assert.ok(g)
  assert.deepEqual(keys(g.rows).slice(0, 3), ['Bluray-1080p', g.key, 'WEBDL-720p'])
  assert.deepEqual(g.rows[1].tiers, ['WEBDL-1080p', 'HDTV-1080p'])
  assert.equal(g.rows[1].name, 'Web or TV')
  assert.equal(g.rows.length, rows.length - 1)
})

test('grouping needs two accepted rows', () => {
  const rows = defaultRows(ladder, ['HDTV-1080p', 'Bluray-1080p'])
  assert.equal(groupRows(ladder, rows, ['HDTV-1080p'], 'x'), null)
  assert.equal(groupRows(ladder, rows, ['HDTV-1080p', 'DVD'], 'x'), null)
})

test('grouping groups, and ungrouping', () => {
  let rows = defaultRows(ladder, ['HDTV-1080p', 'Bluray-1080p', 'WEBDL-1080p', 'WEBDL-720p'])
  const a = groupRows(ladder, rows, ['HDTV-1080p', 'WEBDL-1080p'], 'Web or TV')!
  const b = groupRows(ladder, a.rows, [a.key, 'WEBDL-720p'], 'Everything web')!
  assert.deepEqual(b.rows[1].tiers, ['WEBDL-1080p', 'HDTV-1080p', 'WEBDL-720p'])
  rows = ungroupRow(b.rows, b.key)
  assert.deepEqual(keys(rows).slice(0, 4), ['Bluray-1080p', 'WEBDL-1080p', 'HDTV-1080p', 'WEBDL-720p'])
  assert.ok(isBuiltInOrder(ladder, rows))
})

test('taking one quality out of a group', () => {
  const rows = defaultRows(ladder, ['HDTV-1080p', 'Bluray-1080p', 'WEBDL-1080p'])
  const g = groupRows(ladder, rows, ['HDTV-1080p', 'WEBDL-1080p'], 'Web or TV')!
  const out = splitOut(g.rows, g.key, 'HDTV-1080p')
  assert.deepEqual(keys(out).slice(0, 3), ['Bluray-1080p', g.key, 'HDTV-1080p'])
  assert.deepEqual(out[1].tiers, ['WEBDL-1080p'])
  const last = splitOut(out, g.key, 'WEBDL-1080p')
  assert.deepEqual(keys(last).slice(0, 3), ['Bluray-1080p', 'WEBDL-1080p', 'HDTV-1080p'])
})

test('default group names', () => {
  const label = (n: number) => `Group ${n}`
  const rows = defaultRows(ladder, ['HDTV-1080p', 'WEBDL-1080p'])
  assert.equal(defaultGroupName(rows, ['WEBDL-1080p', 'HDTV-1080p'], label), 'WEBDL-1080p + HDTV-1080p')
  assert.equal(defaultGroupName(rows, ['Remux-2160p', 'Bluray-2160p', 'WEBDL-2160p', 'HDTV-2160p'], label), 'Group 1')
  const taken = renameGroup(rows, 'HDTV-1080p', 'Group 1')
  assert.equal(defaultGroupName(taken, ['Remux-2160p', 'Bluray-2160p', 'WEBDL-2160p', 'HDTV-2160p'], label), 'Group 2')
})

test('problems with the order', () => {
  const rows = defaultRows(ladder, ['HDTV-1080p', 'Bluray-1080p', 'WEBDL-1080p'])
  assert.equal(orderProblem(rows, words), null)
  assert.equal(orderProblem(defaultRows(ladder, []), words), 'none')
  const g = groupRows(ladder, rows, ['HDTV-1080p', 'WEBDL-1080p'], 'Web')!
  assert.equal(orderProblem(g.rows, words), null)
  assert.equal(orderProblem(renameGroup(g.rows, g.key, '  '), words), 'no name')
  assert.match(orderProblem(renameGroup(g.rows, g.key, 'x'.repeat(41)), words) ?? '', /^long/)
  const two = groupRows(ladder, defaultRows(ladder, ['HDTV-1080p', 'WEBDL-1080p', 'HDTV-720p', 'WEBDL-720p']), ['HDTV-720p', 'WEBDL-720p'], 'web')!
  const both = groupRows(ladder, two.rows, ['HDTV-1080p', 'WEBDL-1080p'], 'WEB')!
  assert.equal(orderProblem(both.rows, words), 'twice web')
})

test('the cutoff follows the rows', () => {
  const rows = defaultRows(ladder, ['HDTV-1080p', 'Bluray-1080p', 'WEBDL-1080p'])
  const g = groupRows(ladder, rows, ['HDTV-1080p', 'WEBDL-1080p'], 'Web or TV')!
  assert.equal(fixedCutoff(g.rows, 'HDTV-1080p'), 'HDTV-1080p')
  assert.equal(cutoffRow(g.rows, 'HDTV-1080p')?.name, 'Web or TV')
  const off = toggleRow(g.rows, g.key)
  assert.equal(fixedCutoff(off, 'HDTV-1080p'), 'Bluray-1080p')
  assert.equal(fixedCutoff(defaultRows(ladder, []), 'x'), '')
})

test('counts for the summary', () => {
  const rows = defaultRows(ladder, ['HDTV-1080p', 'Bluray-1080p', 'WEBDL-1080p'])
  assert.deepEqual(orderCounts(rows), { qualities: 3, groups: 0 })
  const g = groupRows(ladder, rows, ['HDTV-1080p', 'WEBDL-1080p'], 'Web or TV')!
  assert.deepEqual(orderCounts(g.rows), { qualities: 3, groups: 1 })
})

test('the TRaSH layout names the web qualities and keeps the built-in order', () => {
  const rows = trashLayout(ladder, defaultRows(ladder, ['HDTV-1080p', 'Bluray-1080p', 'WEBDL-1080p', 'WEBDL-2160p']))
  assert.deepEqual(rows.filter((r) => r.name).map((r) => r.name), ['WEB 2160p', 'WEB 1080p'])
  assert.deepEqual(orderFromRows(rows).map((i) => i.tiers[0]), ['WEBDL-2160p', 'Bluray-1080p', 'WEBDL-1080p', 'HDTV-1080p'])
})

test('web and TV together groups each resolution that has both', () => {
  const rows = webAndTvTogether(ladder, defaultRows(ladder, ['HDTV-1080p', 'Bluray-1080p', 'WEBDL-1080p', 'WEBDL-720p', 'HDTV-2160p']), (res) => `Web and TV ${res}`)
  const order = orderFromRows(rows)
  assert.deepEqual(order.map((i) => i.name ?? i.tiers[0]), ['HDTV-2160p', 'Bluray-1080p', 'Web and TV 1080p', 'WEBDL-720p'])
  assert.deepEqual(order[2].tiers, ['WEBDL-1080p', 'HDTV-1080p'])
})

test('a profile as one line', () => {
  const tl = (t: string) => t
  assert.equal(qualitiesLine(['A', 'B'], undefined, tl), 'A, B')
  assert.equal(qualitiesLine([], [{ tiers: ['Bluray-1080p'] }, { name: 'Web', tiers: ['WEBDL-1080p', 'HDTV-1080p'] }], tl), 'Bluray-1080p > Web (WEBDL-1080p, HDTV-1080p)')
  assert.equal(cutoffText('HDTV-1080p', [{ tiers: ['Bluray-1080p'] }, { name: 'Web', tiers: ['WEBDL-1080p', 'HDTV-1080p'] }]), 'Web')
  assert.equal(cutoffText('Bluray-1080p', undefined), 'Bluray-1080p')
})
