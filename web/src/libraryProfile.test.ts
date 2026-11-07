import assert from 'node:assert/strict'
import { test } from 'node:test'
import { compareByProfile, effectiveProfileId, matchesProfile, profileFilterOptions, profileName } from './libraryProfile.ts'

const profiles = [
  { id: 1, name: 'HD-1080p' },
  { id: 2, name: 'Any' },
  { id: 3, name: 'Ultra-HD' },
]
const items = [
  { kind: 'movie' as const, profileId: 0, title: 'A' }, // default movie profile (1)
  { kind: 'movie' as const, profileId: 3, title: 'B' },
  { kind: 'movie' as const, profileId: 1, title: 'C' },
  { kind: 'tv' as const, profileId: 0, title: 'D' }, // default tv profile (2)
]

test('a title on the default counts under the profile the default is', () => {
  assert.equal(effectiveProfileId(items[0], 1, 2), 1)
  assert.equal(effectiveProfileId(items[3], 1, 2), 2)
  assert.equal(effectiveProfileId(items[1], 1, 2), 3)
})

test('options list only used profiles with their counts', () => {
  assert.deepEqual(profileFilterOptions(items, profiles, 1, 2), [
    { id: 1, name: 'HD-1080p', count: 2 },
    { id: 2, name: 'Any', count: 1 },
    { id: 3, name: 'Ultra-HD', count: 1 },
  ])
  assert.deepEqual(profileFilterOptions(items.slice(1, 2), profiles, 1, 2), [{ id: 3, name: 'Ultra-HD', count: 1 }])
})

test('the filter matches the effective profile, and empty lets everything through', () => {
  assert.equal(matchesProfile(items[0], '1', 1, 2), true)
  assert.equal(matchesProfile(items[0], '3', 1, 2), false)
  assert.equal(matchesProfile(items[3], '2', 1, 2), true)
  assert.equal(matchesProfile(items[3], '', 1, 2), true)
})

test('sorting by profile goes by name, then title, and unknown profiles last', () => {
  const byTitle = (a: { title: string }, b: { title: string }) => a.title.localeCompare(b.title)
  const withGone = [...items, { kind: 'movie' as const, profileId: 99, title: 'E' }]
  const sorted = [...withGone].sort((a, b) => compareByProfile(a, b, profiles, 1, 2, byTitle)).map((i) => i.title)
  assert.deepEqual(sorted, ['D', 'A', 'C', 'B', 'E'])
  assert.equal(profileName(withGone[4], profiles, 1, 2), '')
})
