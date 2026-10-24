// Seeding limits as people type them. In the global settings an empty box is
// "unlimited" and 0 is "don't seed". An indexer's own rule has one more answer:
// an empty box follows the global limit, and the word "unlimited" (saved as -1)
// means no limit of that kind even when the global one has a limit.

/** What the server keeps for "no limit, whatever the global limit says". */
export const UNLIMITED = -1

/**
 * Reads a rule box of one indexer. null follows the global limit, a number is
 * this indexer's own (0 = don't seed, -1 = unlimited) and undefined is no
 * number. `word` is the word for unlimited in the language of the page.
 */
export function parseRuleLimit(text: string, word = 'unlimited'): number | null | undefined {
  const v = text.trim().replace(',', '.')
  if (v === '') return null
  const w = v.toLowerCase()
  if (w === 'unlimited' || w === word.toLowerCase()) return UNLIMITED
  const n = Number(v)
  return Number.isFinite(n) && n >= 0 ? n : undefined
}

/** The text for a saved rule value. */
export function ruleLimitText(n: number | null | undefined, word = 'unlimited'): string {
  if (n === null || n === undefined) return ''
  return n === UNLIMITED ? word : String(n)
}

/**
 * The global upload limit in KB/s from its box: -1 for empty (no limit), a
 * whole number from 0 (0 switches uploading off), or undefined for anything
 * else (a fraction, a negative number, text).
 */
export function parseUploadLimit(text: string): number | undefined {
  const v = text.trim()
  if (v === '') return UNLIMITED
  if (!/^\d+$/.test(v)) return undefined
  const n = Number(v)
  return Number.isSafeInteger(n) ? n : undefined
}

/** True when a limit box holds a real 0, which switches seeding or uploading off. */
export function isZeroLimit(text: string): boolean {
  const v = text.trim().replace(',', '.')
  return v !== '' && Number.isFinite(Number(v)) && Number(v) === 0
}
