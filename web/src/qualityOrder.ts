// The quality order of a profile: its qualities listed best first, any of them
// put in a named group that counts as one rank. Pure helpers, so the editor
// only draws what they return. The server keeps the same list (see
// internal/quality/order.go): the accepted rows, best first, each a quality or
// a group.
//
// "ladder" below is the list of every quality as the API gives it, worst first.

export interface QualityItem {
  name?: string
  tiers: string[]
}

export interface OrderRow {
  // Stays the same while the row is edited. A quality is its own key.
  key: string
  // The group's name; '' for a single quality.
  name: string
  // The qualities in the row, best first.
  tiers: string[]
  allowed: boolean
}

export const MAX_GROUP_NAME = 40

export const isGroup = (r: OrderRow) => r.name !== ''

// Accepted rows first (in the order that counts), the ones left out after them.
// The sort is stable, so the order inside each part is kept.
function settle(rows: OrderRow[]): OrderRow[] {
  return [...rows.filter((r) => r.allowed), ...rows.filter((r) => !r.allowed)]
}

const single = (tier: string, allowed: boolean): OrderRow => ({ key: tier, name: '', tiers: [tier], allowed })

// Every quality as a row in the built-in order, best first; the accepted ones on top.
export function defaultRows(ladder: string[], allowed: string[]): OrderRow[] {
  return settle([...ladder].reverse().map((t) => single(t, allowed.includes(t))))
}

// The rows for a profile: its own order when it has one, else the built-in order.
export function rowsFromProfile(ladder: string[], allowed: string[], order?: QualityItem[]): OrderRow[] {
  if (!order || order.length === 0) return defaultRows(ladder, allowed)
  const rows: OrderRow[] = order.map((it, i) => ({
    key: it.name ? `group-${i + 1}` : it.tiers[0],
    name: it.name ?? '',
    tiers: sortBest(ladder, it.tiers),
    allowed: true,
  }))
  const listed = new Set(rows.flatMap((r) => r.tiers))
  const rest = [...ladder].reverse().filter((t) => !listed.has(t)).map((t) => single(t, false))
  return [...rows, ...rest]
}

function sortBest(ladder: string[], tiers: string[]): string[] {
  return [...tiers].sort((a, b) => ladder.indexOf(b) - ladder.indexOf(a))
}

// What goes to the server: the accepted rows, best first.
export function orderFromRows(rows: OrderRow[]): QualityItem[] {
  return rows.filter((r) => r.allowed).map((r) => (isGroup(r) ? { name: r.name.trim(), tiers: r.tiers } : { tiers: r.tiers }))
}

// The accepted qualities, worst first like the API lists them.
export function allowedFromRows(ladder: string[], rows: OrderRow[]): string[] {
  const on = new Set(rows.filter((r) => r.allowed).flatMap((r) => r.tiers))
  return ladder.filter((t) => on.has(t))
}

// Moves a row to a place in the list (0 = the top). Rows left out stay below the accepted ones.
export function moveRow(rows: OrderRow[], key: string, to: number): OrderRow[] {
  const from = rows.findIndex((r) => r.key === key)
  if (from < 0) return rows
  const next = rows.filter((r) => r.key !== key)
  next.splice(Math.max(0, Math.min(to, next.length)), 0, rows[from])
  return settle(next)
}

// Moves a row one place up (-1) or down (1).
export function shiftRow(rows: OrderRow[], key: string, delta: -1 | 1): OrderRow[] {
  const from = rows.findIndex((r) => r.key === key)
  if (from < 0) return rows
  return moveRow(rows, key, from + delta)
}

// Accepts or leaves out a row. An accepted row joins the end of the accepted
// ones; one left out goes to the top of those left out.
export function toggleRow(rows: OrderRow[], key: string): OrderRow[] {
  const flipped = rows.map((r) => (r.key === key ? { ...r, allowed: !r.allowed } : r))
  const row = flipped.find((r) => r.key === key)
  if (!row) return rows
  const others = flipped.filter((r) => r.key !== key)
  const accepted = others.filter((r) => r.allowed)
  const left = others.filter((r) => !r.allowed)
  return [...accepted, row, ...left]
}

// A name for a new group: its qualities joined, or "Group 2" when that is too long or taken.
export function defaultGroupName(rows: OrderRow[], tiers: string[], label: (n: number) => string): string {
  const taken = new Set(rows.filter(isGroup).map((r) => r.name.toLowerCase()))
  const joined = tiers.join(' + ')
  if (joined.length <= MAX_GROUP_NAME && !taken.has(joined.toLowerCase())) return joined
  let n = 1
  while (taken.has(label(n).toLowerCase())) n++
  return label(n)
}

function nextGroupKey(rows: OrderRow[]): string {
  let n = 1
  const used = new Set(rows.map((r) => r.key))
  while (used.has(`group-${n}`)) n++
  return `group-${n}`
}

// Puts the picked rows (two or more, all accepted) into one group, at the
// place of the topmost of them. Returns null when that is not possible.
export function groupRows(ladder: string[], rows: OrderRow[], keys: string[], name: string): { rows: OrderRow[]; key: string } | null {
  const picked = rows.filter((r) => keys.includes(r.key))
  if (picked.length < 2 || picked.some((r) => !r.allowed)) return null
  const key = nextGroupKey(rows)
  const merged: OrderRow = { key, name, tiers: sortBest(ladder, picked.flatMap((r) => r.tiers)), allowed: true }
  const at = rows.findIndex((r) => keys.includes(r.key))
  const next = rows.filter((r) => !keys.includes(r.key))
  next.splice(at, 0, merged)
  return { rows: settle(next), key }
}

// Breaks a group into its qualities, in the built-in order, where the group was.
export function ungroupRow(rows: OrderRow[], key: string): OrderRow[] {
  const at = rows.findIndex((r) => r.key === key)
  const row = rows[at]
  if (!row || !isGroup(row)) return rows
  const parts = row.tiers.map((t) => single(t, row.allowed))
  const next = [...rows]
  next.splice(at, 1, ...parts)
  return next
}

// Takes one quality out of a group; it becomes its own row just below the group.
export function splitOut(rows: OrderRow[], key: string, tier: string): OrderRow[] {
  const at = rows.findIndex((r) => r.key === key)
  const row = rows[at]
  if (!row || !isGroup(row) || !row.tiers.includes(tier)) return rows
  const rest = row.tiers.filter((t) => t !== tier)
  const next = [...rows]
  const alone = single(tier, row.allowed)
  if (rest.length === 0) next.splice(at, 1, alone)
  else next.splice(at, 1, { ...row, tiers: rest }, alone)
  return next
}

export function renameGroup(rows: OrderRow[], key: string, name: string): OrderRow[] {
  return rows.map((r) => (r.key === key ? { ...r, name } : r))
}

// "TRaSH layout": the built-in order, with the web qualities named WEB 2160p, WEB 1080p...
export function trashLayout(ladder: string[], rows: OrderRow[]): OrderRow[] {
  const allowed = allowedFromRows(ladder, rows)
  return defaultRows(ladder, allowed).map((r) => {
    const m = /^WEBDL-(\d+p)$/.exec(r.tiers[0])
    return m && r.allowed && !isGroup(r) ? { ...r, key: `group-${m[1]}`, name: `WEB ${m[1]}` } : r
  })
}

// Web and TV copies of one resolution count the same: each pair of accepted
// WEBDL and HDTV qualities of a resolution becomes one group.
export function webAndTvTogether(ladder: string[], rows: OrderRow[], label: (res: string) => string): OrderRow[] {
  const allowed = allowedFromRows(ladder, rows)
  let out = defaultRows(ladder, allowed)
  for (const res of ['2160p', '1080p', '720p']) {
    const keys = [`WEBDL-${res}`, `HDTV-${res}`].filter((k) => out.some((r) => r.key === k && r.allowed))
    if (keys.length < 2) continue
    const g = groupRows(ladder, out, keys, label(res))
    if (g) out = g.rows
  }
  return out
}

// What is wrong with the order, in words, or null: a group needs a name that is
// short and not used twice, and at least one row has to be accepted.
export function orderProblem(
  rows: OrderRow[],
  words: { noName: string; tooLong: (name: string) => string; twice: (name: string) => string; none: string },
): string | null {
  if (!rows.some((r) => r.allowed)) return words.none
  const seen = new Set<string>()
  for (const r of rows) {
    if (!isGroup(r) || !r.allowed) continue
    const name = r.name.trim()
    if (name === '') return words.noName
    if (name.length > MAX_GROUP_NAME) return words.tooLong(name)
    if (seen.has(name.toLowerCase())) return words.twice(name)
    seen.add(name.toLowerCase())
  }
  return null
}

// The cutoff stays when its quality is still in an accepted row; otherwise the
// best accepted row's first quality takes over.
export function fixedCutoff(rows: OrderRow[], cutoff: string): string {
  const open = rows.filter((r) => r.allowed)
  if (open.some((r) => r.tiers.includes(cutoff))) return cutoff
  return open[0]?.tiers[0] ?? ''
}

// The row the cutoff sits in.
export function cutoffRow(rows: OrderRow[], cutoff: string): OrderRow | undefined {
  return rows.find((r) => r.allowed && r.tiers.includes(cutoff))
}

export function rowLabel(r: OrderRow, tierLabel: (t: string) => string): string {
  return isGroup(r) ? r.name : tierLabel(r.tiers[0])
}

export function orderCounts(rows: OrderRow[]): { qualities: number; groups: number } {
  const on = rows.filter((r) => r.allowed)
  return { qualities: on.reduce((n, r) => n + r.tiers.length, 0), groups: on.filter(isGroup).length }
}

// Whether the rows are just the built-in order with nothing grouped.
export function isBuiltInOrder(ladder: string[], rows: OrderRow[]): boolean {
  const on = rows.filter((r) => r.allowed)
  if (on.some(isGroup)) return false
  const want = [...ladder].reverse().filter((t) => on.some((r) => r.tiers[0] === t))
  return on.every((r, i) => r.tiers[0] === want[i])
}

// A profile's qualities as one line, best first, a group as "Name (a, b)". Without
// its own order: the accepted qualities as the API lists them.
export function qualitiesLine(allowed: string[], order: QualityItem[] | undefined, tierLabel: (t: string) => string): string {
  if (!order || order.length === 0) return allowed.map(tierLabel).join(', ')
  return order.map((it) => (it.name ? `${it.name} (${it.tiers.map(tierLabel).join(', ')})` : tierLabel(it.tiers[0]))).join(' > ')
}

// How a profile's cutoff reads: the group it sits in, or the quality.
export function cutoffText(cutoff: string, order: QualityItem[] | undefined): string {
  const it = (order ?? []).find((x) => x.tiers.includes(cutoff))
  return it?.name ? it.name : cutoff
}
