// Which day an episode lands on, and at what time, in the viewer's own time
// zone. An episode with a known air time (`airAt`, the exact moment, from
// TVmaze) is placed by that moment; everything else stays on its date, so a
// show with no time known never moves. Nothing here changes the stored
// moment: choosing another zone only changes how it is read.

export const ZONE_KEY = 'mediarium.calendar.zone'

// '' = this device's zone; anything else is an IANA zone ("UTC" included).
export const COMMON_ZONES = [
  'UTC',
  'Europe/London',
  'Europe/Dublin',
  'Europe/Lisbon',
  'Europe/Paris',
  'Europe/Berlin',
  'Europe/Madrid',
  'Europe/Rome',
  'Europe/Amsterdam',
  'Europe/Warsaw',
  'Europe/Kyiv',
  'Europe/Moscow',
  'America/Halifax',
  'America/New_York',
  'America/Chicago',
  'America/Denver',
  'America/Los_Angeles',
  'America/Sao_Paulo',
  'Asia/Dubai',
  'Asia/Kolkata',
  'Asia/Bangkok',
  'Asia/Shanghai',
  'Asia/Tokyo',
  'Australia/Sydney',
  'Pacific/Auckland',
]

export function isZone(zone: string): boolean {
  try {
    new Intl.DateTimeFormat('en', { timeZone: zone })
    return true
  } catch {
    return false
  }
}

export function deviceZone(): string {
  try {
    const z = Intl.DateTimeFormat().resolvedOptions().timeZone
    return z && isZone(z) ? z : 'UTC'
  } catch {
    return 'UTC'
  }
}

/** The zone the person picked on this browser, '' for "this device". */
export function readZonePref(): string {
  try {
    const z = localStorage.getItem(ZONE_KEY) ?? ''
    return z && isZone(z) ? z : ''
  } catch {
    return ''
  }
}

export function writeZonePref(zone: string): void {
  try {
    if (zone) localStorage.setItem(ZONE_KEY, zone)
    else localStorage.removeItem(ZONE_KEY)
  } catch {
    // A private window may refuse; the choice just isn't remembered.
  }
}

/** The zone to read times in for a stored choice. */
export function resolveZone(pref: string): string {
  return pref && isZone(pref) ? pref : deviceZone()
}

const dayFormats = new Map<string, Intl.DateTimeFormat>()
const timeFormats = new Map<string, Intl.DateTimeFormat>()

/** "YYYY-MM-DD" of a moment as the wall calendar of a zone shows it. */
export function dayInZone(at: Date | number, zone: string): string {
  let f = dayFormats.get(zone)
  if (!f) {
    f = new Intl.DateTimeFormat('en-US', { timeZone: zone, year: 'numeric', month: '2-digit', day: '2-digit' })
    dayFormats.set(zone, f)
  }
  const p: Record<string, string> = {}
  for (const part of f.formatToParts(at)) p[part.type] = part.value
  return `${p.year}-${p.month}-${p.day}`
}

/** "21:00" (24-hour) of an RFC 3339 moment in a zone; '' if it can't be read. */
export function timeInZone(airAt: string, zone: string): string {
  const ms = Date.parse(airAt)
  if (Number.isNaN(ms)) return ''
  let f = timeFormats.get(zone)
  if (!f) {
    f = new Intl.DateTimeFormat('en-GB', { timeZone: zone, hour: '2-digit', minute: '2-digit', hourCycle: 'h23' })
    timeFormats.set(zone, f)
  }
  return f.format(ms)
}

interface Placed {
  kind: string
  releaseDate: string
  airAt?: string
}

/** The day an entry sits on in a zone: by its exact moment when it is an episode with one, else its date. */
export function entryDay(e: Placed, zone: string): string {
  if (e.kind === 'episode' && e.airAt) {
    const ms = Date.parse(e.airAt)
    if (!Number.isNaN(ms)) return dayInZone(ms, zone)
  }
  return e.releaseDate
}

/** The time to print on an entry in a zone, '' when only its date is known. */
export function entryTime(e: Placed, zone: string): string {
  return e.kind === 'episode' && e.airAt ? timeInZone(e.airAt, zone) : ''
}

/** Entries grouped by their day in a zone; within a day those without a time come first, then by time. */
export function groupByDay<T extends Placed>(entries: T[], zone: string): Map<string, T[]> {
  const m = new Map<string, T[]>()
  for (const e of entries) {
    const key = entryDay(e, zone)
    const list = m.get(key)
    if (list) list.push(e)
    else m.set(key, [e])
  }
  for (const list of m.values()) {
    if (list.length < 2) continue
    const at = (e: T) => (e.kind === 'episode' && e.airAt ? Date.parse(e.airAt) : Number.NEGATIVE_INFINITY)
    list.sort((a, b) => {
      const x = at(a)
      const y = at(b)
      return x === y ? 0 : x < y ? -1 : 1
    })
  }
  return m
}
