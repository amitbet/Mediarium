// What the details row of a release in the search list shows: where its score
// comes from and what else makes an automatic search skip it. Pure functions,
// so the table only draws what they return.

export interface WhyReason {
  kind: string
  text: string
}

export interface WhyFormat {
  name: string
  score: number
}

// A preferred term of the profile the release matched, with its score.
export interface WhyTerm {
  term: string
  score: number
}

export interface WhySource {
  formatScore?: number
  // The bonus of a fixed release (PROPER, REPACK) when bad releases are skipped.
  fixScore?: number
  formats?: WhyFormat[]
  terms?: WhyTerm[]
  minScore?: number
  upgradeUntil?: number
  reasons?: WhyReason[]
  rejections?: string[]
}

export interface WhyInfo {
  // The custom-format score, when the profile scores formats.
  score?: number
  formats: WhyFormat[]
  terms: WhyTerm[]
  // The bonus of a fixed release (PROPER, REPACK), 0 for none.
  fix: number
  // Formats, preferred terms and the fix bonus added up: what the pick
  // compares, and what the Score column shows.
  total: number
  minScore: number
  upgradeUntil: number
  belowMin: boolean
  // What makes an automatic search skip the release.
  reasons: WhyReason[]
  // Why another release was taken instead of this one (kind "notBest").
  notBest: string[]
}

// signed shows a score the way the lists do: +100, -50, 0.
export function signed(n: number): string {
  return n > 0 ? `+${n}` : String(n)
}

// whyOf collects the details for one release, or null when there is nothing
// to explain (no score, no matched term and no reason).
export function whyOf(r: WhySource): WhyInfo | null {
  // Lists that only send the plain rejection texts (music, books) still get them.
  const all = r.reasons?.length ? r.reasons : (r.rejections ?? []).map((text) => ({ kind: 'rule', text }))
  const reasons = all.filter((x) => x.kind !== 'notBest')
  const notBest = all.filter((x) => x.kind === 'notBest').map((x) => x.text)
  const terms = r.terms ?? []
  const fix = r.fixScore ?? 0
  if (r.formatScore === undefined && reasons.length === 0 && notBest.length === 0 && terms.length === 0 && fix === 0) return null
  const minScore = r.minScore ?? 0
  return {
    score: r.formatScore,
    formats: r.formats ?? [],
    terms,
    fix,
    total: (r.formatScore ?? 0) + terms.reduce((sum, x) => sum + x.score, 0) + fix,
    notBest,
    minScore,
    upgradeUntil: r.upgradeUntil ?? 0,
    belowMin: r.formatScore !== undefined && r.formatScore < minScore,
    reasons,
  }
}

const KEY = 'mediarium-show-why'

interface Store {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
}

function defaultStore(): Store | null {
  try {
    return globalThis.localStorage ?? null
  } catch {
    return null
  }
}

// loadShowWhy reads whether the list opens every details row; closed when
// nothing is stored or the browser will not say.
export function loadShowWhy(store: Store | null = defaultStore()): boolean {
  try {
    return store?.getItem(KEY) === '1'
  } catch {
    return false
  }
}

export function saveShowWhy(on: boolean, store: Store | null = defaultStore()) {
  try {
    store?.setItem(KEY, on ? '1' : '0')
  } catch {
    // a blocked store only means the choice is not remembered
  }
}
