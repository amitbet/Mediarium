// How the calendar is drawn for each of its four views, on a wide panel and on a narrow one (a phone).
// Every view looks different at every width; none of them is swapped for another.

export type View = 'month' | 'posters' | 'backdrops' | 'agenda'

export const VIEW_KEY = 'mediarium.calendar.view'

/** What is drawn:
 *  grid       a month grid whose days hold the releases (wide Month, Posters and Backdrops: the days hold posters or stills)
 *  dots       a compact month grid with a dot per release; the picked day's list sits under it (narrow Month)
 *  day-tiles  one block per day with a grid of posters (narrow Posters)
 *  day-wide   one block per day with wide stills (narrow Backdrops)
 *  agenda     one block per day with plain cards (Agenda, at any width) */
export type Layout = 'grid' | 'dots' | 'day-tiles' | 'day-wide' | 'agenda'

export function parseView(v: string | null | undefined): View {
  return v === 'agenda' || v === 'posters' || v === 'backdrops' ? v : 'month'
}

export function layoutFor(view: View, narrow: boolean): Layout {
  if (view === 'agenda') return 'agenda'
  if (!narrow) return 'grid'
  if (view === 'posters') return 'day-tiles'
  if (view === 'backdrops') return 'day-wide'
  return 'dots'
}

/** The day whose list shows under the compact month: the one picked, else today when it is in the month, else the first day
 *  that has something, else the 1st. Days are YYYY-MM-DD; month is YYYY-MM. */
export function pickDay(picked: string | null, month: string, today: string, busyDays: string[]): string {
  if (picked && picked.startsWith(month)) return picked
  if (today.startsWith(month)) return today
  const first = busyDays.filter((d) => d.startsWith(month)).sort()[0]
  return first ?? `${month}-01`
}
