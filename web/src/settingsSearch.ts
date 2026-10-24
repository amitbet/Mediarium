// The matcher behind the settings part of the search bar. It works on plain
// text, so it is easy to test: no capital letters, no accents, and a typo or
// two does not hide a result.

export interface Searchable {
  label: string // the name as shown (already translated)
  trail: string // where it lives, for example "Library › Folders"
  words: string // other words people use for it
}

// Lower case, accents removed, anything that is not a letter or digit turned
// into a single space.
export function norm(s: string): string {
  return s
    .normalize('NFD')
    .replace(/\p{M}+/gu, '')
    .toLowerCase()
    .replace(/[^\p{L}\p{N}]+/gu, ' ')
    .trim()
}

// True when a and b are the same, or differ by one letter added, left out or
// changed.
export function withinOne(a: string, b: string): boolean {
  if (a === b) return true
  if (Math.abs(a.length - b.length) > 1) return false
  let i = 0
  while (i < a.length && i < b.length && a[i] === b[i]) i++
  if (a.length === b.length) return a.slice(i + 1) === b.slice(i + 1)
  return a.length > b.length ? a.slice(i + 1) === b.slice(i) : a.slice(i) === b.slice(i + 1)
}

// A typed word starts a word, or is that word with a plural s or two letters
// more ("backups" finds "backup").
const startsWord = (words: string[], token: string) => words.some((w) => w.startsWith(token) || (w.length >= 4 && token.startsWith(w) && token.length - w.length <= 2))
const nearWord = (words: string[], token: string) => token.length >= 4 && words.some((w) => withinOne(token, w.slice(0, token.length)) || withinOne(token, w))

// How well a query fits one entry, 0 for not at all:
//   100 the label is exactly what was typed
//    85 the label starts with it
//    65 every typed word starts a word of the label
//    50 it appears inside the label
//    35 every typed word starts a word of the label, the trail or the extra words
//    20 like the 65 case, allowing one wrong letter in each word
//    15 like the 35 case, allowing one wrong letter in each word
export function score(query: string, e: Searchable): number {
  const full = norm(query)
  if (!full) return 0
  const tokens = full.split(' ')
  const label = norm(e.label)
  const labelWords = label.split(' ')
  if (label === full) return 100
  if (label.startsWith(full)) return 85
  if (tokens.every((t) => startsWord(labelWords, t))) return 65
  if (full.length >= 3 && label.includes(full)) return 50
  const all = [...labelWords, ...norm(`${e.trail} ${e.words}`).split(' ')]
  if (tokens.every((t) => startsWord(all, t))) return 35
  if (tokens.every((t) => startsWord(labelWords, t) || nearWord(labelWords, t))) return 20
  if (tokens.every((t) => startsWord(all, t) || nearWord(all, t))) return 15
  return 0
}

// The best entries for a query, best first; entries that fit equally well
// keep the order they were listed in.
export function rank<T extends Searchable>(query: string, entries: readonly T[], limit = 6): { entry: T; score: number }[] {
  const scored: { entry: T; score: number; at: number }[] = []
  entries.forEach((entry, at) => {
    const s = score(query, entry)
    if (s > 0) scored.push({ entry, score: s, at })
  })
  scored.sort((a, b) => b.score - a.score || a.at - b.at)
  return scored.slice(0, limit).map(({ entry, score }) => ({ entry, score }))
}
