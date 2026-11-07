import assert from 'node:assert/strict'
import { test } from 'node:test'
import { collectionBodyFor, offersCollectionAdd } from './collectionAdd.ts'

test('the collection is added with the same choices as the movie', () => {
  assert.deepEqual(collectionBodyFor({ profileId: 4, rootPath: '/movies2', monitored: true, sources: 'usenet', searchNow: true }), {
    profileId: 4,
    rootPath: '/movies2',
    monitored: true,
    sources: 'usenet',
    searchNow: true,
  })
})

test('default profile and main folder are left out, like for the movie', () => {
  const b = collectionBodyFor({ profileId: 0, rootPath: '', monitored: true, sources: '', searchNow: false })
  assert.equal(b.profileId, undefined)
  assert.equal(b.rootPath, undefined)
  assert.equal(b.searchNow, false)
})

test('an unmonitored movie never starts a search for the others', () => {
  assert.equal(collectionBodyFor({ profileId: 1, rootPath: '', monitored: false, sources: '', searchNow: true }).searchNow, false)
})

test('the tick box needs a movie in a collection and an administrator', () => {
  assert.equal(offersCollectionAdd(true, true, false, 10), true)
  assert.equal(offersCollectionAdd(false, true, false, 10), false)
  assert.equal(offersCollectionAdd(true, false, false, 10), false)
  assert.equal(offersCollectionAdd(true, true, true, 10), false)
  assert.equal(offersCollectionAdd(true, true, false, undefined), false)
})
