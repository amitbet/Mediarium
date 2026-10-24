// Run with: npm test
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { changeCount, profileChangeText, profileChangesText, type ProfileScoreChange } from './formatChanges.ts'

const change = (o: Partial<ProfileScoreChange>): ProfileScoreChange => ({ profileId: 1, name: '1080p', added: 0, alreadyThere: 0, replaced: 0, removed: 0, ...o })

test('each outcome is said plainly', () => {
  const cases: [Partial<ProfileScoreChange>, string][] = [
    [{ added: 3 }, '1080p: 3 added.'],
    [{ added: 3, alreadyThere: 2 }, '1080p: 3 added, Already in the profile (skipped): 2.'],
    [{ alreadyThere: 2 }, '1080p: Already in the profile (skipped): 2.'],
    [{ replaced: 1, alreadyThere: 1 }, '1080p: Already in the profile (skipped): 1, Scores replaced: 1.'],
    [{ removed: 4 }, '1080p: Taken out: 4.'],
    [{}, '1080p: nothing to change.'],
  ]
  for (const [c, want] of cases) assert.equal(profileChangeText(change(c)), want, JSON.stringify(c))
})

test('several profiles are joined', () => {
  assert.equal(profileChangesText([change({ added: 1 }), change({ name: '720p', alreadyThere: 1 })]), '1080p: 1 added. 720p: Already in the profile (skipped): 1.')
})

test('only real changes are counted', () => {
  assert.equal(changeCount(change({ alreadyThere: 5 })), 0)
  assert.equal(changeCount(change({ added: 1, replaced: 2, removed: 3 })), 6)
})
