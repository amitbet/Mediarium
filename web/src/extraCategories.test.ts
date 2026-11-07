// Run with: npm test (uses node's built-in test runner, no extra packages).
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { addToText, buildExtra, formatCategories, parseCategoryText, suggestionsFor, textsFrom } from './extraCategories.ts'

test('parseCategoryText reads numbers and refuses anything else', () => {
  const cases: [string, number[], boolean][] = [
    ['', [], false],
    ['   ', [], false],
    ['105000', [105000], false],
    ['105010, 105000', [105000, 105010], false],
    ['7120;7121 7122', [7120, 7121, 7122], false],
    ['105000,105000,', [105000], false],
    ['abc', [], true],
    ['7120, tv', [], true],
    ['12.5', [], true],
    ['0', [], true],
    ['-4', [], true],
    ['1000000', [], true],
  ]
  for (const [text, list, bad] of cases) {
    const r = parseCategoryText(text)
    assert.equal(r.error !== null, bad, text)
    assert.deepEqual(r.list, list, text)
  }
  const many = Array.from({ length: 31 }, (_, i) => 100000 + i).join(',')
  assert.notEqual(parseCategoryText(many).error, null)
})

test('textsFrom and buildExtra round-trip, and an empty box means nothing is sent for that kind', () => {
  const texts = textsFrom({ tv: [105000, 105010], books: [7120] })
  assert.equal(texts.tv, '105000, 105010')
  assert.equal(texts.movies, '')
  const { extra, errors } = buildExtra(texts)
  assert.deepEqual(extra, { tv: [105000, 105010], books: [7120] })
  assert.deepEqual(errors, {})
  assert.deepEqual(buildExtra(textsFrom(undefined)).extra, {})
  const bad = buildExtra({ ...texts, music: 'x1' })
  assert.deepEqual(Object.keys(bad.errors), ['music'])
  assert.equal(formatCategories(undefined), '')
})

test('suggestionsFor offers what fits the kind, the indexer own numbers first', () => {
  const caps = [
    { id: 2000, name: 'Movies' },
    { id: 2040, name: 'Movies/HD' },
    { id: 3030, name: 'Audio/Audiobook' },
    { id: 3040, name: 'Audio/Lossless' },
    { id: 5000, name: 'TV' },
    { id: 5040, name: 'TV/HD' },
    { id: 7000, name: 'Books' },
    { id: 7020, name: 'Books/Ebook' },
    { id: 7120, name: 'Books/German' },
    { id: 105000, name: 'TV Specials' },
    { id: 107000, name: 'Books German' },
    { id: 108000, name: 'Hörbuch' },
  ]
  const ids = (kind: Parameters<typeof suggestionsFor>[0], have: number[] = []) => suggestionsFor(kind, caps, have).map((c) => c.id)
  assert.deepEqual(ids('tv'), [105000, 5040])
  assert.deepEqual(ids('books'), [107000, 7120])
  assert.deepEqual(ids('movies'), [2040])
  assert.deepEqual(ids('music'), []) // 3040 is already searched; the audiobook category is not music
  assert.deepEqual(ids('audiobooks'), [108000]) // 3030 is already searched
  assert.deepEqual(ids('tv', [105000]), [5040], 'a number already added is not offered again')
})

test('addToText keeps the box sorted and without repeats, and leaves a broken box alone', () => {
  assert.equal(addToText('', 7120), '7120')
  assert.equal(addToText('7121', 7120), '7120, 7121')
  assert.equal(addToText('7120', 7120), '7120')
  assert.equal(addToText('oops', 7120), 'oops')
})
