import { t } from './i18n.ts'

// A person's own numbering for a show: the rules that say what release and
// file names call the library's seasons and episodes, and the extra names to
// search for. These helpers build the rule rows, the sentences that preview
// them and the rules the "this show's releases use season N" shortcut makes.

export interface NumberingRule {
  librarySeason: number
  releaseSeason: number
  offset?: number
  from?: number
  to?: number
}

export interface CustomNumbering {
  rules: NumberingRule[]
  names: string[]
}

// A row being edited: what is typed in each box, as text.
export interface RuleDraft {
  librarySeason: string
  releaseSeason: string
  offset: string
  from: string
  to: string
}

export const MAX_RULES = 24
export const MAX_NAMES = 12

export function emptyDraft(): RuleDraft {
  return { librarySeason: '1', releaseSeason: '', offset: '', from: '', to: '' }
}

export function toDraft(r: NumberingRule): RuleDraft {
  return {
    librarySeason: String(r.librarySeason),
    releaseSeason: String(r.releaseSeason),
    offset: r.offset ? String(r.offset) : '',
    from: r.from ? String(r.from) : '',
    to: r.to ? String(r.to) : '',
  }
}

function whole(s: string): number | null {
  const v = s.trim()
  if (!/^-?\d+$/.test(v)) return null
  return Number(v)
}

// The rule a row stands for, or null while its seasons are not whole numbers
// of 1 or more. Empty optional boxes mean "not set".
export function toRule(d: RuleDraft): NumberingRule | null {
  const librarySeason = whole(d.librarySeason)
  const releaseSeason = whole(d.releaseSeason)
  if (librarySeason === null || releaseSeason === null || librarySeason < 1 || releaseSeason < 1) return null
  const rule: NumberingRule = { librarySeason, releaseSeason }
  const offset = d.offset.trim() === '' ? 0 : whole(d.offset)
  const from = d.from.trim() === '' ? 0 : whole(d.from)
  const to = d.to.trim() === '' ? 0 : whole(d.to)
  if (offset === null || from === null || to === null || from < 0 || to < 0) return null
  if (offset) rule.offset = offset
  if (from) rule.from = from
  if (to) rule.to = to
  return rule
}

// The rules to send, and how many rows were left out because they are not
// finished.
export function rulesOf(drafts: RuleDraft[]): { rules: NumberingRule[]; skipped: number } {
  const rules: NumberingRule[] = []
  let skipped = 0
  for (const d of drafts) {
    const r = toRule(d)
    if (r) rules.push(r)
    else skipped++
  }
  return { rules, skipped }
}

const two = (n: number) => String(n).padStart(2, '0')

// S01E03
export function code(season: number, episode: number): string {
  return `S${two(season)}E${two(episode)}`
}

// The episode the preview talks about: the rule's first one.
function sampleEpisode(r: NumberingRule): number {
  return r.from && r.from > 0 ? r.from : 1
}

// "Library S01E03 = release S15E03"
export function previewLine(r: NumberingRule): string {
  const e = sampleEpisode(r)
  return t('Library {library} = release {release}', { library: code(r.librarySeason, e), release: code(r.releaseSeason, e + (r.offset ?? 0)) })
}

// "File S15E01.mkv in the folder is found as library S01E01"
export function fileLine(r: NumberingRule): string {
  const e = sampleEpisode(r)
  const release = code(r.releaseSeason, e + (r.offset ?? 0))
  return t('File {file} in the folder is found as library {library}', { file: `${release}.mkv`, library: code(r.librarySeason, e) })
}

// The rules for "This show's releases use season N": a show with one season
// in the library maps it to N; with several, the first goes to N and the
// others follow on (2 to N+1 ...) unless only the first is asked for.
export function seasonShortcut(releaseSeason: number, librarySeasons: number[], allSeasons: boolean): NumberingRule[] {
  const seasons = [...new Set(librarySeasons.filter((s) => s > 0))].sort((a, b) => a - b)
  if (!Number.isInteger(releaseSeason) || releaseSeason < 1) return []
  const first = seasons[0] ?? 1
  if (!allSeasons || seasons.length <= 1) return [{ librarySeason: first, releaseSeason }]
  return seasons.map((s) => ({ librarySeason: s, releaseSeason: releaseSeason + (s - first) }))
}

// A rule row for a library season replaces an earlier row of the same season
// that covers the whole season; used when the shortcut is pressed twice.
export function mergeRules(current: NumberingRule[], add: NumberingRule[]): NumberingRule[] {
  const seasons = new Set(add.map((r) => r.librarySeason))
  return [...current.filter((r) => !seasons.has(r.librarySeason)), ...add].sort((a, b) => a.librarySeason - b.librarySeason || (a.from ?? 0) - (b.from ?? 0))
}

// Adds a name to the list unless it is there already (ignoring case) or empty.
export function addName(names: string[], raw: string): string[] {
  const name = raw.trim().replace(/\s+/g, ' ')
  if (!name || names.some((n) => n.toLowerCase() === name.toLowerCase()) || names.length >= MAX_NAMES) return names
  return [...names, name]
}
