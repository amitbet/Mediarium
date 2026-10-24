import assert from 'node:assert/strict'
import { test } from 'node:test'
import { activeExtraFilters, browseTitle, COMMON_LANGUAGES, languageName, languageOptions } from './discoverFilters.ts'

test('language names come from Intl in the reader\'s language', () => {
  assert.equal(languageName('ko', 'en'), 'Korean')
  assert.equal(languageName('ko', 'es'), 'coreano')
  assert.equal(languageName('cn', 'en'), 'Cantonese')
  assert.equal(languageName('zh', 'en'), 'Chinese')
})

test('a language Intl cannot name falls back to its code', () => {
  assert.equal(languageName('q1', 'en'), 'Q1')
})

test('the common languages come first, then the rest by name', () => {
  const o = languageOptions('Any language', 'en')
  assert.deepEqual(o[0], { value: '', label: 'Any language' })
  assert.deepEqual(o.slice(1, 1 + COMMON_LANGUAGES.length).map((x) => x.value), [...COMMON_LANGUAGES])
  const rest = o.slice(1 + COMMON_LANGUAGES.length).map((x) => x.label)
  assert.ok(rest.length > 30)
  assert.deepEqual(rest, [...rest].sort((a, b) => a.localeCompare(b, 'en')))
  assert.equal(new Set(o.map((x) => x.value)).size, o.length, 'no code is listed twice')
})

test('counting the extra filters that are on', () => {
  assert.equal(activeExtraFilters({ language: '', minRating: '', runtime: '' }), 0)
  assert.equal(activeExtraFilters({ language: 'ko', minRating: '7', runtime: '' }), 2)
  assert.equal(activeExtraFilters({ language: 'ko', minRating: '7', runtime: 'over120' }), 3)
})

test('the heading says what is filtered', () => {
  const cases: [string, Parameters<typeof browseTitle>[1], string | undefined, string][] = [
    ['Shows', {}, undefined, 'Shows'],
    ['Shows', { language: 'ko', minRating: '7' }, 'Korean', 'Shows in Korean, rated 7+'],
    ['Drama movies', { language: 'ja', runtime: 'under90' }, 'Japanese', 'Drama movies in Japanese, under 90 min'],
    ['Shows', { language: 'ko', minRating: '7', yearFrom: '2020', yearTo: '2026' }, 'Korean', 'Shows in Korean from 2020 to 2026, rated 7+'],
    ['Movies', { minRating: '8', runtime: '90to120', yearFrom: '1990' }, undefined, 'Movies from 1990, rated 8+, 90 to 120 min'],
    ['Movies', { runtime: 'over120', yearTo: '1999' }, undefined, 'Movies up to 1999, over 120 min'],
    ['Movies', { language: 'ko' }, undefined, 'Movies'],
  ]
  for (const [what, f, lang, want] of cases) assert.equal(browseTitle(what, f, lang), want)
})
