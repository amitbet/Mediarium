// Run with: npm test (uses node's built-in test runner, no extra packages).
import assert from 'node:assert/strict'
import { test } from 'node:test'
import type { SubtitleDiagnostic } from './api.ts'
import { setCatalogue } from './i18n.ts'
import { diagnosticLine } from './subtitleDiagnostics.ts'

test('each kind of lookup is explained in plain words', () => {
  setCatalogue('en', {})
  const cases: [SubtitleDiagnostic, string][] = [
    [{ source: 'SubDL', lookup: 'tmdb', value: '661', count: 0, found: 0 }, 'SubDL: by TMDB id 661: 0 results, the site answered normally'],
    [{ source: 'SubDL', lookup: 'imdb', value: 'tt0167404', count: 0, found: 2, note: 'wrong-language', detail: 'DE, FR' }, 'SubDL: by IMDb id tt0167404: 2 found, all dropped: wrong language (found: DE, FR)'],
    [{ source: 'SubDL', lookup: 'title', value: 'The Sixth Sense (1999)', count: 12, found: 14 }, 'SubDL: by title "The Sixth Sense (1999)": 12 results'],
    [{ source: 'SubDL', lookup: 'title', value: 'X', count: 1, found: 1 }, 'SubDL: by title "X": 1 result'],
    [{ source: 'SubDL', lookup: 'tmdb', value: '5', bare: true, count: 0, found: 0, note: 'no-match', detail: "can't find movie or tv" }, "SubDL: by TMDB id 5: with fewer filters: not found (can't find movie or tv)"],
    [{ source: 'OpenSubtitles', lookup: 'imdb', value: 'tt1', count: 0, found: 0, note: 'error', detail: 'unexpected status 500' }, 'OpenSubtitles: by IMDb id tt1: failed: unexpected status 500'],
    [{ source: 'SubDL', lookup: 'title', value: 'S', count: 0, found: 3, note: 'wrong-episode' }, 'SubDL: by title "S": 3 found, all dropped: not for this episode'],
    [{ source: 'SubDL', lookup: 'title', value: 'S', count: 0, found: 3, note: 'bad-link' }, 'SubDL: by title "S": 3 found, all dropped: no usable download link'],
    [{ source: 'SubDL', lookup: 'title', value: 'S', count: 0, found: 3 }, 'SubDL: by title "S": 3 found, none usable'],
  ]
  for (const [d, want] of cases) assert.equal(diagnosticLine(d), want)
})
