// Extra categories of an indexer: category numbers searched besides the standard
// ones, per kind of media. Free of React so plain node can test it (see
// extraCategories.test.ts). The matching server-side rules live in
// internal/indexers/extra_categories.go.
import { t } from './i18n.ts'

export const EXTRA_KINDS = ['movies', 'tv', 'music', 'books', 'audiobooks'] as const
export type ExtraKind = (typeof EXTRA_KINDS)[number]
export type ExtraCategories = Partial<Record<ExtraKind, number[]>>
export type CapCategory = { id: number; name: string }

export const MAX_EXTRA_CATEGORY = 999999
export const MAX_EXTRA_PER_KIND = 30

/** The numbers Mediarium always asks for, per kind. */
const STANDARD: Record<ExtraKind, number[]> = {
  movies: [2000],
  tv: [5000],
  music: [3000, 3010, 3040, 3050],
  books: [7000, 7020],
  audiobooks: [3030],
}

/** Reads what a person typed: whole numbers separated by commas, semicolons or spaces. */
export function parseCategoryText(text: string): { list: number[]; error: string | null } {
  const list: number[] = []
  for (const part of text.split(/[,;\s]+/)) {
    if (part === '') continue
    if (!/^\d+$/.test(part) || Number(part) < 1 || Number(part) > MAX_EXTRA_CATEGORY) {
      return { list: [], error: t('"{value}" is not a category number. Use whole numbers from 1 to {max}, separated by commas.', { value: part.slice(0, 20), max: MAX_EXTRA_CATEGORY }) }
    }
    const n = Number(part)
    if (!list.includes(n)) list.push(n)
  }
  if (list.length > MAX_EXTRA_PER_KIND) {
    return { list: [], error: t('That is too many extra categories for one kind of media. Use at most {max}.', { max: MAX_EXTRA_PER_KIND }) }
  }
  return { list: list.sort((a, b) => a - b), error: null }
}

export function formatCategories(list?: number[]): string {
  return (list ?? []).join(', ')
}

/** The text boxes' contents for the categories an indexer has. */
export function textsFrom(extra?: ExtraCategories | null): Record<ExtraKind, string> {
  const out = {} as Record<ExtraKind, string>
  for (const k of EXTRA_KINDS) out[k] = formatCategories(extra?.[k])
  return out
}

/** The request body: only kinds with something in them; an empty object clears them all. */
export function buildExtra(texts: Record<ExtraKind, string>): { extra: ExtraCategories; errors: Partial<Record<ExtraKind, string>> } {
  const extra: ExtraCategories = {}
  const errors: Partial<Record<ExtraKind, string>> = {}
  for (const k of EXTRA_KINDS) {
    const r = parseCategoryText(texts[k] ?? '')
    if (r.error) errors[k] = r.error
    else if (r.list.length > 0) extra[k] = r.list
  }
  return { extra, errors }
}

const NAME_HINT: Record<ExtraKind, RegExp> = {
  movies: /movie|film/i,
  tv: /\btv\b|series|show|episode|serie/i,
  music: /music|mp3|flac|lossless|album/i,
  books: /book|ebook|epub|magazine|comic|buch/i,
  audiobooks: /audiobook|h(ö|oe)rbuch|spoken/i,
}

function fitsKind(kind: ExtraKind, c: CapCategory): boolean {
  const audiobook = NAME_HINT.audiobooks.test(c.name)
  if (kind === 'audiobooks') return audiobook
  if (audiobook) return false // 3xxx music ranges must not take an audiobook category
  if (c.id >= 100000) return NAME_HINT[kind].test(c.name)
  const range = Math.floor(c.id / 1000)
  switch (kind) {
    case 'movies': return range === 2
    case 'tv': return range === 5
    case 'music': return range === 3 && c.id !== 3030
    case 'books': return range === 7
  }
}

/** Categories an indexer lists that are worth offering for a kind: not already searched,
 *  not already added, the indexer's own numbers (100000 and up) first. */
export function suggestionsFor(kind: ExtraKind, caps: CapCategory[], have: number[], limit = 24): CapCategory[] {
  const skip = new Set([...STANDARD[kind], ...have])
  return caps
    .filter((c) => !skip.has(c.id) && (c.id >= 100000 || c.id % 1000 !== 0) && fitsKind(kind, c))
    .sort((a, b) => Number(b.id >= 100000) - Number(a.id >= 100000) || a.id - b.id)
    .slice(0, limit)
}

/** Adds a number to the text of a box, keeping the list sorted and free of repeats. */
export function addToText(text: string, id: number): string {
  const r = parseCategoryText(text)
  if (r.error) return text
  return formatCategories([...new Set([...r.list, id])].sort((a, b) => a - b))
}
