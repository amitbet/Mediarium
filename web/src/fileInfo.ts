import type { FileAudio, FileInfo, FileSubtitle, FileVideo, MediaSummary } from './api.ts'
import { locale, t } from './i18n.ts'

// What a video file holds (its codecs, picture size, audio and subtitle
// tracks), written the short way people read it: "HEVC 10-bit HDR10",
// "TrueHD 7.1 Atmos", "EN, DE audio". The server reads the files' headers; this
// only words the result and decides what the Library's file filters match.

// 2 channels are 2.0, 6 are 5.1, 8 are 7.1.
export function channelLayout(n: number | undefined): string {
  if (!n || n < 1) return ''
  return n >= 6 ? `${n - 1}.1` : `${n}.0`
}

export function videoText(v: FileVideo): string {
  const parts = [v.codec]
  if (v.bitDepth && v.bitDepth > 8) parts.push(t('{n}-bit', { n: String(v.bitDepth) }))
  if (v.hdr && v.hdr !== 'SDR') parts.push(v.hdr)
  return parts.join(' ')
}

export function audioText(a: FileAudio): string {
  const parts = [a.codec]
  const layout = channelLayout(a.channels)
  if (layout) parts.push(layout)
  if (a.atmos) parts.push('Atmos')
  return parts.join(' ')
}

// "de" as "DE": the short form on a chip.
export function langShort(code: string): string {
  return code.toUpperCase()
}

// "de" as "German" in the screen language, or the code itself where the browser doesn't know it.
export function langName(code: string): string {
  try {
    return new Intl.DisplayNames(locale() ?? 'en', { type: 'language' }).of(code) ?? code
  } catch {
    return code
  }
}

// The different languages of some tracks, in order, as "EN, DE".
export function langList(codes: (string | undefined)[]): string {
  return [...new Set(codes.filter((c): c is string => !!c))].map(langShort).join(', ')
}

export interface FileChip {
  key: string
  text: string
  title?: string // the longer description, shown when pointing at the chip
}

function audioTitle(tracks: FileAudio[]): string {
  return tracks.map((a) => `${a.lang ? langName(a.lang) : t('Unknown language')}: ${audioText(a)}${a.title ? ` (${a.title})` : ''}`).join('\n')
}

function subtitleTitle(tracks: FileSubtitle[]): string {
  return tracks.map((s) => `${s.lang ? langName(s.lang) : t('Unknown language')}: ${s.codec}${s.forced ? ` (${t('forced')})` : ''}`).join('\n')
}

// The chips for a file: video, picture size, the main sound track, the audio
// languages and the subtitle languages. A file that has not been read has none.
export function fileChips(info: FileInfo | null | undefined): FileChip[] {
  if (!info) return []
  const chips: FileChip[] = []
  if (info.video) {
    chips.push({ key: 'video', text: videoText(info.video), title: info.video.fps ? `${info.video.width ?? '?'}×${info.video.height ?? '?'} · ${info.video.fps} fps` : undefined })
    if (info.resolution) chips.push({ key: 'res', text: info.resolution, title: info.video.width && info.video.height ? `${info.video.width}×${info.video.height}` : undefined })
  }
  const audio = info.audio ?? []
  if (audio.length > 0) {
    const main = audio.find((a) => a.default) ?? audio[0]
    chips.push({ key: 'sound', text: audioText(main), title: audioTitle(audio) })
    const langs = langList(audio.map((a) => a.lang))
    if (langs) chips.push({ key: 'alangs', text: t('{langs} audio', { langs }), title: audioTitle(audio) })
  }
  const subs = (info.subtitles ?? []).filter((s) => !s.forced)
  const forced = (info.subtitles ?? []).filter((s) => s.forced)
  const slangs = langList(subs.map((s) => s.lang))
  if (slangs) chips.push({ key: 'subs', text: t('{langs} subs', { langs: slangs }), title: subtitleTitle(info.subtitles ?? []) })
  else if (forced.length > 0) chips.push({ key: 'forced', text: t('Forced subs only'), title: subtitleTitle(forced) })
  return chips
}

// ---- the Library's file filters ----

export interface FileFilters {
  vc: string // video codec
  hdr: string // "hdr" for any kind of HDR, or one kind
  res: string // resolution
  al: string // has audio in this language
  ac: string // audio codec
  sl: string // has subtitles in this language
  nal: string // has no audio in this language
  nsl: string // has no subtitles in this language (inside the video or as a file beside it)
}

export const NO_FILE_FILTERS: FileFilters = { vc: '', hdr: '', res: '', al: '', ac: '', sl: '', nal: '', nsl: '' }

export function fileFiltersActive(f: FileFilters): boolean {
  return Object.values(f).some((v) => v !== '')
}

// Whether a title's files satisfy every file filter. Titles whose files have
// not been read yet (no summary) match none of them.
export function matchesFileFilters(media: MediaSummary | undefined, f: FileFilters): boolean {
  if (!fileFiltersActive(f)) return true
  if (!media) return false
  if (f.vc && !(media.videoCodecs ?? []).includes(f.vc)) return false
  if (f.hdr) {
    const kinds = media.hdr ?? []
    if (f.hdr === 'hdr' ? !kinds.some((k) => k !== 'SDR') : !kinds.includes(f.hdr)) return false
  }
  if (f.res && !(media.resolutions ?? []).includes(f.res)) return false
  if (f.al && !(media.audioLangs ?? []).includes(f.al)) return false
  if (f.ac && !(media.audioCodecs ?? []).includes(f.ac)) return false
  if (f.sl && !(media.subLangs ?? []).includes(f.sl)) return false
  // "No audio in X" is only asked of titles whose files were read.
  if (f.nal && (media.audioLangs ?? []).includes(f.nal)) return false
  if (f.nsl && (media.subLangs ?? []).includes(f.nsl)) return false
  return true
}

export interface FileFilterOptions {
  videoCodecs: string[]
  hdr: string[]
  resolutions: string[]
  audioLangs: string[]
  audioCodecs: string[]
  subLangs: string[]
}

const RESOLUTION_ORDER = ['2160p', '1440p', '1080p', '720p', '576p', '480p']

// The values there are across the titles, for the filter lists.
export function fileFilterOptions(items: { media?: MediaSummary }[]): FileFilterOptions {
  const sets = { videoCodecs: new Set<string>(), hdr: new Set<string>(), resolutions: new Set<string>(), audioLangs: new Set<string>(), audioCodecs: new Set<string>(), subLangs: new Set<string>() }
  for (const i of items) {
    const m = i.media
    if (!m) continue
    for (const k of Object.keys(sets) as (keyof typeof sets)[]) for (const v of m[k] ?? []) sets[k].add(v)
  }
  const sorted = (s: Set<string>) => [...s].sort((a, b) => a.localeCompare(b))
  return {
    videoCodecs: sorted(sets.videoCodecs),
    hdr: sorted(sets.hdr).filter((k) => k !== 'SDR'),
    resolutions: [...sets.resolutions].sort((a, b) => RESOLUTION_ORDER.indexOf(a) - RESOLUTION_ORDER.indexOf(b)),
    audioLangs: sorted(sets.audioLangs),
    audioCodecs: sorted(sets.audioCodecs),
    subLangs: sorted(sets.subLangs),
  }
}
