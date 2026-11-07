import assert from 'node:assert/strict'
import { test } from 'node:test'
import { coverThumb, isMusicCover } from './coverThumb.ts'

test('music cover addresses get the thumbnail size', () => {
  assert.equal(coverThumb('/api/music/artists/12/cover'), '/api/music/artists/12/cover?size=thumb')
  assert.equal(coverThumb('/api/music/albums/7/cover'), '/api/music/albums/7/cover?size=thumb')
  assert.equal(coverThumb('/api/music/covers/release-group/aaaaaaaa-0000-4000-8000-000000000001'), '/api/music/covers/release-group/aaaaaaaa-0000-4000-8000-000000000001?size=thumb')
  assert.equal(coverThumb('/mediarium/api/music/albums/7/cover'), '/mediarium/api/music/albums/7/cover?size=thumb')
})

test('anything else is left alone', () => {
  assert.equal(coverThumb(undefined), undefined)
  assert.equal(coverThumb(''), '')
  assert.equal(coverThumb('https://image.tmdb.org/t/p/w500/x.jpg'), 'https://image.tmdb.org/t/p/w500/x.jpg')
  assert.equal(coverThumb('/api/music/albums/7/cover?size=thumb'), '/api/music/albums/7/cover?size=thumb')
  assert.equal(coverThumb('/api/music/albums/x/cover'), '/api/music/albums/x/cover')
  assert.equal(isMusicCover('/api/books/3/cover'), false)
})
