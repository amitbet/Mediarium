import { test } from 'node:test'
import assert from 'node:assert/strict'
import { norm, rank, score, withinOne, type Searchable } from './settingsSearch.ts'

const e = (label: string, words = '', trail = 'Library'): Searchable => ({ label, trail, words })

test('norm drops capitals, accents and punctuation', () => {
  const cases: [string, string][] = [
    ['Folders', 'folders'],
    ['  Información  del   servidor ', 'informacion del servidor'],
    ['Português (Brasil)', 'portugues brasil'],
    ['Wizdom (Hebrew)', 'wizdom hebrew'],
    ['4K requests', '4k requests'],
    ['Спасибо', 'спасибо'],
    ['Zażółć gęślą jaźń', 'zazołc gesla jazn'],
  ]
  for (const [input, want] of cases) assert.equal(norm(input), want, input)
})

test('withinOne allows one letter added, left out or changed', () => {
  const cases: [string, string, boolean][] = [
    ['folder', 'folder', true],
    ['foldr', 'folder', true],
    ['folderr', 'folder', true],
    ['faldet', 'folder', false],
    ['fildor', 'folder', false],
    ['fodler', 'folder', false], // two letters swapped is two changes
    ['password', 'passwrd', true],
    ['', 'a', true],
    ['ab', 'abcd', false],
  ]
  for (const [a, b, want] of cases) assert.equal(withinOne(a, b), want, `${a} ~ ${b}`)
})

test('an exact label beats a prefix, a word, a keyword and a typo', () => {
  const entry = e('Folders', 'paths directories')
  const cases: [string, number][] = [
    ['folders', 100],
    ['FOLDERS', 100],
    ['fold', 85],
    ['fo', 85],
    ['olders', 50], // inside a word
    ['paths', 35], // only in the extra words
    ['foldrs', 20], // one wrong letter
    ['pathz', 15], // one wrong letter in the extra words
    ['zebra', 0],
  ]
  for (const [q, want] of cases) assert.equal(score(q, entry), want, q)
  const words = e('Temporary download folder')
  assert.equal(score('folder', words), 65) // starts a later word of the label
  assert.equal(score('temp folder', words), 65) // every word starts a label word
  assert.equal(score('folder temp', words), 65) // in any order
  assert.equal(score('folders', words), 65) // a plural finds the singular
  assert.equal(score('backups', e('Backup and restore')), 65)
})

test('ranking puts exact first, then prefix, then word, then keyword, then typo', () => {
  const entries = [
    e('Download rules', 'limits'),
    e('Folders', 'paths'),
    e('Downloads folder', 'path'),
    e('Temporary download folder', 'temp'),
    e('Library', 'folders paths'),
    e('Quality', 'folderz'),
  ]
  const order = (q: string) => rank(q, entries).map((r) => r.entry.label)
  assert.deepEqual(order('folders'), ['Folders', 'Downloads folder', 'Temporary download folder', 'Library', 'Quality'])
  // "download" is a prefix of two labels and a later word of one
  assert.deepEqual(order('download'), ['Download rules', 'Downloads folder', 'Temporary download folder'])
  // "folder": prefix of Folders, a word of two others, only a keyword on Library, near folderz
  assert.deepEqual(order('folder'), ['Folders', 'Downloads folder', 'Temporary download folder', 'Library', 'Quality'])
})

test('equal scores keep the order they were listed in, and the limit applies', () => {
  const entries = [e('Alpha one'), e('Alpha two'), e('Alpha three'), e('Alpha four')]
  assert.deepEqual(rank('alpha', entries, 3).map((r) => r.entry.label), ['Alpha one', 'Alpha two', 'Alpha three'])
  assert.deepEqual(rank('', entries), [])
  assert.deepEqual(rank('   ', entries), [])
})

test('accents and capitals never get in the way', () => {
  const cases: [string, string, boolean][] = [
    ['informacion', 'Información de la película', true],
    ['INFORMACIÓN', 'Informacion de la pelicula', true],
    ['Zażółć', 'zazolc', true],
    ['uber', 'Über uns', true],
    ['cafe', 'Café', true],
    ['coffee', 'Café', false],
  ]
  for (const [q, label, want] of cases) assert.equal(score(q, e(label)) > 0, want, `${q} / ${label}`)
})

test('short typos in short words are not guessed at', () => {
  // under four letters only a real prefix counts, so "tv" never finds "ty"
  assert.equal(score('ty', e('TV folder')), 0)
  assert.equal(score('tv', e('TV folder')), 85)
  assert.equal(score('vpn', e('VPN and proxy')), 85)
  assert.equal(score('vpm', e('VPN and proxy')), 0)
})
