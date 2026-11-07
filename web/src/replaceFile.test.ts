// Run with: npm test (uses node's built-in test runner, no extra packages).
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { keptDataText, keptFileLine } from './keptFile.ts'
import { baseName, describe, episodeFilesToReplace, movieFileToReplace, replaceQuestion } from './replaceFile.ts'

const eps = [
  { season: 1, episode: 1, status: 'downloaded', filePath: '/tv/Show/Season 01/Show - S01E01.mkv', quality: 'HDTV-720p', sizeBytes: 1_000_000_000 },
  { season: 1, episode: 2, status: 'missing' },
  { season: 1, episode: 3, status: 'downloaded', filePath: 'C:\\tv\\Show\\Season 01\\Show - S01E03.mkv', quality: 'WEBDL-1080p', sizeBytes: 2_000_000_000 },
  { season: 2, episode: 1, status: 'downloaded', filePath: '/tv/Show/Season 02/Show - S02E01.mkv' },
  { season: 1, episode: 4, status: 'downloading', filePath: '/tv/Show/Season 01/partial.mkv' },
]

test('the name of a file from either kind of path', () => {
  assert.equal(baseName('/movies/A (2000)/A (2000).mkv'), 'A (2000).mkv')
  assert.equal(baseName('C:\\movies\\A\\A.mkv'), 'A.mkv')
  assert.equal(baseName('A.mkv'), 'A.mkv')
})

test('a movie has a file to replace only when it is downloaded', () => {
  assert.deepEqual(movieFileToReplace({ status: 'downloaded', filePath: '/m/A (2000).mkv', quality: 'WEBDL-1080p', sizeBytes: 5 }), {
    name: 'A (2000).mkv',
    quality: 'WEBDL-1080p',
    sizeBytes: 5,
  })
  assert.equal(movieFileToReplace({ status: 'missing' }), null)
  assert.equal(movieFileToReplace({ status: 'downloading', filePath: '/m/A.mkv' }), null)
  assert.equal(movieFileToReplace({ status: 'downloaded' }), null)
})

test('one episode: only its own file', () => {
  const got = episodeFilesToReplace(eps, { season: 1, episodes: [3] }, { season: 1, episode: 3 })
  assert.deepEqual(
    got.map((f) => f.name),
    ['Show - S01E03.mkv'],
  )
  assert.deepEqual(episodeFilesToReplace(eps, { season: 1, episodes: [2] }, { season: 1, episode: 2 }), [])
})

test('a season pack: every episode of the season that has a file', () => {
  const got = episodeFilesToReplace(eps, { season: 1 }, { season: 1 })
  assert.deepEqual(
    got.map((f) => f.name),
    ['Show - S01E01.mkv', 'Show - S01E03.mkv'],
  )
})

test('a multi-episode release: the episodes it names', () => {
  const got = episodeFilesToReplace(eps, { season: 1, episodes: [1, 2] }, { season: 1, episode: 1 })
  assert.deepEqual(
    got.map((f) => f.name),
    ['Show - S01E01.mkv'],
  )
})

test('a release that names no season is taken for what the search was for', () => {
  assert.deepEqual(
    episodeFilesToReplace(eps, {}, { season: 1, episode: 1 }).map((f) => f.name),
    ['Show - S01E01.mkv'],
  )
  assert.equal(episodeFilesToReplace(eps, {}, { season: 1 }).length, 2)
  assert.deepEqual(episodeFilesToReplace(eps, {}, null), [])
})

test('files and releases are described with what is known of them', () => {
  assert.equal(describe('A.mkv', 'WEBDL-1080p', 1_500_000_000), 'A.mkv (WEBDL-1080p, 1.4 GB)')
  assert.equal(describe('A.mkv', undefined, 0), 'A.mkv')
  assert.equal(describe('A.mkv', '720p'), 'A.mkv (720p)')
})

test('the question for one file names the old file and the release', () => {
  const q = replaceQuestion([{ name: 'A (2000).mkv', quality: 'HDTV-720p', sizeBytes: 1_000_000_000 }], {
    title: 'A.2000.1080p.WEB-DL-GRP',
    resolution: '1080p',
    source: 'WEB-DL',
    sizeBytes: 4_000_000_000,
  })
  assert.equal(q.title, 'Replace the file you have?')
  assert.equal(q.lines[0], 'This will replace A (2000).mkv (HDTV-720p, 953.7 MB) with A.2000.1080p.WEB-DL-GRP (1080p WEB-DL, 3.7 GB).')
  assert.match(q.lines[1], /recycle bin/)
})

test('the question for a pack counts the episodes that have files', () => {
  const q = replaceQuestion(
    [
      { name: 'a.mkv', sizeBytes: 1 },
      { name: 'b.mkv', sizeBytes: 1 },
      { name: 'c.mkv', sizeBytes: 1 },
    ],
    { title: 'Show.S01.1080p-GRP' },
  )
  assert.equal(q.title, 'Replace the files you have?')
  assert.equal(q.lines[0], 'This will replace the files of 3 episodes that already have one with the files in Show.S01.1080p-GRP.')
  assert.match(q.lines.join(' '), /no file yet are filled in as usual/)
})

test('a kept download says what became of its files', () => {
  assert.equal(keptDataText('deleted'), 'Its downloaded files were deleted.')
  assert.match(keptDataText('seeding'), /still seeding/)
  assert.match(keptDataText('client'), /download client/)
  assert.equal(keptDataText(''), 'Its downloaded files were deleted.')
  assert.equal(keptFileLine('/movies/A (2000)/A (2000).mkv'), 'Kept: A (2000).mkv')
})
