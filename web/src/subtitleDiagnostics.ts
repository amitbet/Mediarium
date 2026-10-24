import type { SubtitleDiagnostic } from './api.ts'
import { plural, t } from './i18n.ts'

// One line of the "Details" under an empty subtitle search: which site was
// asked, how, and what came back, in plain words.
export function diagnosticLine(d: SubtitleDiagnostic): string {
  const parts = [d.source]
  switch (d.lookup) {
    case 'tmdb':
      parts.push(t('by TMDB id {value}', { value: d.value ?? '' }))
      break
    case 'imdb':
      parts.push(t('by IMDb id {value}', { value: d.value ?? '' }))
      break
    case 'title':
      parts.push(t('by title "{value}"', { value: d.value ?? '' }))
      break
  }
  if (d.bare) parts.push(t('with fewer filters'))
  const vars = { found: d.found, detail: d.detail ?? '' }
  if (d.note === 'error') parts.push(t('failed: {detail}', vars))
  else if (d.count > 0) parts.push(plural(d.count, '{n} result', '{n} results'))
  else if (d.note === 'no-match') parts.push(d.detail ? t('not found ({detail})', vars) : t('not found'))
  else if (d.note === 'wrong-language') parts.push(t('{found} found, all dropped: wrong language (found: {detail})', vars))
  else if (d.note === 'wrong-episode') parts.push(t('{found} found, all dropped: not for this episode', vars))
  else if (d.note === 'bad-link') parts.push(d.detail ? t('{found} found, all dropped: no usable download link (it looked like {detail})', vars) : t('{found} found, all dropped: no usable download link', vars))
  else if (d.found > 0) parts.push(t('{found} found, none usable', vars))
  else parts.push(t('0 results, the site answered normally'))
  return parts.join(': ')
}
