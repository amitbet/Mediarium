import assert from 'node:assert/strict'
import { test } from 'node:test'
import type { FileInfo, MediaSummary } from './api.ts'
import { channelLayout, fileChips, fileFilterOptions, matchesFileFilters, NO_FILE_FILTERS, audioText, videoText } from './fileInfo.ts'

test('channel counts are written the usual way', () => {
  const cases: [number | undefined, string][] = [
    [undefined, ''],
    [0, ''],
    [1, '1.0'],
    [2, '2.0'],
    [6, '5.1'],
    [8, '7.1'],
  ]
  for (const [n, want] of cases) assert.equal(channelLayout(n), want, String(n))
})

test('video and audio text', () => {
  assert.equal(videoText({ codec: 'H.264', bitDepth: 8, hdr: 'SDR' }), 'H.264')
  assert.equal(videoText({ codec: 'HEVC', bitDepth: 10, hdr: 'HDR10' }), 'HEVC 10-bit HDR10')
  assert.equal(videoText({ codec: 'HEVC', bitDepth: 10, hdr: 'Dolby Vision' }), 'HEVC 10-bit Dolby Vision')
  assert.equal(videoText({ codec: 'AV1' }), 'AV1')
  assert.equal(audioText({ codec: 'TrueHD', channels: 8, atmos: true }), 'TrueHD 7.1 Atmos')
  assert.equal(audioText({ codec: 'AAC', channels: 2 }), 'AAC 2.0')
  assert.equal(audioText({ codec: 'FLAC' }), 'FLAC')
})

test('the chips of a file', () => {
  const info: FileInfo = {
    container: 'matroska',
    resolution: '2160p',
    video: { codec: 'HEVC', width: 3840, height: 2160, bitDepth: 10, hdr: 'HDR10', fps: 23.976 },
    audio: [
      { codec: 'AC-3', channels: 6, lang: 'de' },
      { codec: 'TrueHD', channels: 8, lang: 'en', default: true, atmos: true },
    ],
    subtitles: [
      { codec: 'PGS', lang: 'en' },
      { codec: 'PGS', lang: 'nl' },
      { codec: 'PGS', lang: 'de', forced: true },
    ],
  }
  assert.deepEqual(
    fileChips(info).map((c) => c.text),
    ['HEVC 10-bit HDR10', '2160p', 'TrueHD 7.1 Atmos', 'DE, EN audio', 'EN, NL subs'],
  )
  assert.deepEqual(fileChips(null), [])
  assert.deepEqual(fileChips(undefined), [])
  // no video track, no subtitles: only what is there
  assert.deepEqual(
    fileChips({ container: 'mp4', audio: [{ codec: 'AAC', channels: 2 }] }).map((c) => c.text),
    ['AAC 2.0'],
  )
  // only forced subtitles
  assert.deepEqual(
    fileChips({ container: 'mp4', subtitles: [{ codec: 'SRT', lang: 'en', forced: true }] }).map((c) => c.text),
    ['Forced subs only'],
  )
})

const nightHarbour: MediaSummary = { videoCodecs: ['HEVC'], hdr: ['HDR10'], resolutions: ['2160p'], audioCodecs: ['TrueHD', 'AC-3', 'Atmos'], audioLangs: ['en', 'de'], subLangs: ['nl'] }
const tidePool: MediaSummary = { videoCodecs: ['H.264'], hdr: ['SDR'], resolutions: ['1080p'], audioCodecs: ['AAC'], audioLangs: ['en'], subLangs: [] }
const show: MediaSummary = { videoCodecs: ['H.264', 'HEVC'], hdr: ['SDR', 'HDR10'], resolutions: ['1080p', '2160p'], audioCodecs: ['AAC'], audioLangs: ['ja'] }

test('file filters', () => {
  const none = NO_FILE_FILTERS
  const cases: { name: string; media: MediaSummary | undefined; filters: Partial<typeof none>; want: boolean }[] = [
    { name: 'no filter matches a title that was not read', media: undefined, filters: {}, want: true },
    { name: 'a filter does not match a title that was not read', media: undefined, filters: { vc: 'HEVC' }, want: false },
    { name: 'video codec', media: nightHarbour, filters: { vc: 'HEVC' }, want: true },
    { name: 'other video codec', media: tidePool, filters: { vc: 'HEVC' }, want: false },
    { name: 'any HDR', media: nightHarbour, filters: { hdr: 'hdr' }, want: true },
    { name: 'any HDR, not for SDR', media: tidePool, filters: { hdr: 'hdr' }, want: false },
    { name: 'SDR', media: tidePool, filters: { hdr: 'SDR' }, want: true },
    { name: 'a show with both kinds matches either', media: show, filters: { hdr: 'SDR' }, want: true },
    { name: 'one kind of HDR', media: nightHarbour, filters: { hdr: 'Dolby Vision' }, want: false },
    { name: 'resolution', media: nightHarbour, filters: { res: '2160p' }, want: true },
    { name: 'audio in a language', media: nightHarbour, filters: { al: 'de' }, want: true },
    { name: 'no audio in that language', media: tidePool, filters: { al: 'de' }, want: false },
    { name: 'audio codec', media: nightHarbour, filters: { ac: 'Atmos' }, want: true },
    { name: 'subtitles in a language', media: nightHarbour, filters: { sl: 'nl' }, want: true },
    { name: 'no subtitles', media: tidePool, filters: { sl: 'nl' }, want: false },
    { name: 'missing a language: it has it', media: nightHarbour, filters: { nal: 'de' }, want: false },
    { name: 'missing a language: it lacks it', media: tidePool, filters: { nal: 'de' }, want: true },
    { name: 'missing a language: not read', media: undefined, filters: { nal: 'de' }, want: false },
    { name: 'missing subtitles: it has them', media: nightHarbour, filters: { nsl: 'nl' }, want: false },
    { name: 'missing subtitles: it lacks them', media: tidePool, filters: { nsl: 'nl' }, want: true },
    { name: 'missing subtitles: a show with none listed', media: show, filters: { nsl: 'nl' }, want: true },
    { name: 'missing subtitles: not read', media: undefined, filters: { nsl: 'nl' }, want: false },
    { name: 'filters add up', media: nightHarbour, filters: { vc: 'HEVC', al: 'de', sl: 'nl', res: '2160p' }, want: true },
    { name: 'filters add up, one fails', media: nightHarbour, filters: { vc: 'HEVC', al: 'de', sl: 'fr' }, want: false },
  ]
  for (const c of cases) assert.equal(matchesFileFilters(c.media, { ...none, ...c.filters }), c.want, c.name)
})

test('the values to choose from', () => {
  const o = fileFilterOptions([{ media: nightHarbour }, { media: tidePool }, { media: show }, {}])
  assert.deepEqual(o.videoCodecs, ['H.264', 'HEVC'])
  assert.deepEqual(o.hdr, ['HDR10'])
  assert.deepEqual(o.resolutions, ['2160p', '1080p'])
  assert.deepEqual(o.audioLangs, ['de', 'en', 'ja'])
  assert.deepEqual(o.subLangs, ['nl'])
  assert.deepEqual(fileFilterOptions([]).videoCodecs, [])
})
