import { t } from './i18n.ts'

// What adding custom formats to a quality profile did there, as the server
// counts it (see the "changes" of the add-to-profiles answer and the "profile"
// of a TRaSH import).
export interface ProfileScoreChange {
  profileId: number
  name: string
  added: number
  alreadyThere: number
  replaced: number
  removed: number
}

// changeCount is how many scores really changed in the profile.
export function changeCount(c: ProfileScoreChange): number {
  return c.added + c.replaced + c.removed
}

// profileChangeText says plainly what happened in one profile: "1080p: 3
// added, Already in the profile (skipped): 2." Formats the profile had are
// named as skipped, never as updated.
export function profileChangeText(c: ProfileScoreChange): string {
  const parts: string[] = []
  if (c.added > 0) parts.push(t('{n} added', { n: c.added }))
  if (c.alreadyThere > 0) parts.push(t('Already in the profile (skipped): {n}', { n: c.alreadyThere }))
  if (c.replaced > 0) parts.push(t('Scores replaced: {n}', { n: c.replaced }))
  if (c.removed > 0) parts.push(t('Taken out: {n}', { n: c.removed }))
  if (parts.length === 0) return t('{name}: nothing to change.', { name: c.name })
  return t('{name}: {parts}.', { name: c.name, parts: parts.join(', ') })
}

// profileChangesText joins the lines of several profiles.
export function profileChangesText(list: ProfileScoreChange[]): string {
  return list.map(profileChangeText).join(' ')
}
