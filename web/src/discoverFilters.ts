import { locale, plural, t } from './i18n.ts'

// The extra Discover filters for movies and shows: original language, lowest
// rating and runtime. Their values live in the address and go to the server
// as they are, so everything here is plain strings.

export interface ExtraFilters {
  language: string // original language, a code like "ko"; '' is any
  minRating: string // '6', '7', '8' or '9'; '' is any
  runtime: string // one of RUNTIMES; '' is any
}

export const RATINGS = ['6', '7', '8'] as const
export const RUNTIMES = ['under90', '90to120', 'over120'] as const

// The languages most people look for come first, in this order.
export const COMMON_LANGUAGES = ['en', 'ko', 'ja', 'es', 'fr', 'de', 'it', 'hi', 'zh', 'cn', 'pt', 'tr', 'th', 'da', 'sv', 'no', 'nl', 'pl', 'ru', 'uk'] as const

// The rest, as the movie database knows them (ISO 639-1; "cn" is its code for Cantonese).
const OTHER_LANGUAGES = [
  'af', 'ar', 'bg', 'bn', 'bs', 'ca', 'cs', 'cy', 'el', 'et', 'eu', 'fa', 'fi', 'ga', 'gl', 'gu', 'he', 'hr', 'hu', 'hy',
  'id', 'is', 'ka', 'kk', 'kn', 'ku', 'la', 'lt', 'lv', 'mk', 'ml', 'mn', 'mr', 'ms', 'my', 'ne', 'pa', 'ro', 'si', 'sk',
  'sl', 'sq', 'sr', 'sw', 'ta', 'te', 'tl', 'ur', 'uz', 'vi',
] as const

// Intl has no entry for the movie database's "cn"; Cantonese is "yue" there.
const DISPLAY_CODE: Record<string, string> = { cn: 'yue' }

// The name of a language in the reader's own language ("Korean", "Coreano"),
// so no per-language strings are needed. The code itself is the fallback.
export function languageName(code: string, loc: string | undefined = locale()): string {
  try {
    const name = new Intl.DisplayNames(loc ? [loc] : undefined, { type: 'language' }).of(DISPLAY_CODE[code] ?? code)
    if (name) return name
  } catch {
    // An old browser without Intl.DisplayNames, or a code it does not know.
  }
  return code.toUpperCase()
}

export interface LanguageOption {
  value: string
  label: string
}

// "Any language", then the common ones, then every other language by name.
export function languageOptions(anyLabel: string, loc: string | undefined = locale()): LanguageOption[] {
  const named = (codes: readonly string[]) => codes.map((c) => ({ value: c, label: languageName(c, loc) }))
  const rest = named(OTHER_LANGUAGES).sort((a, b) => a.label.localeCompare(b.label, loc))
  return [{ value: '', label: anyLabel }, ...named(COMMON_LANGUAGES), ...rest]
}

export function ratingLabel(rating: string): string {
  return t('Rated {n}+', { n: rating })
}

export function runtimeLabel(runtime: string): string {
  switch (runtime) {
    case 'under90':
      return t('Under 90 min')
    case 'over120':
      return t('Over 120 min')
    case '90to120':
      return t('90 to 120 min')
  }
  return runtime
}

// How many of the extra filters are picked, for the "More filters" header.
export function activeExtraFilters(f: ExtraFilters): number {
  return [f.language, f.minRating, f.runtime].filter(Boolean).length
}

export function extraFiltersSummary(f: ExtraFilters): string {
  const n = activeExtraFilters(f)
  return n === 0 ? '' : plural(n, '{n} filter on', '{n} filters on')
}

// The parts of a results heading that come from the extra filters:
// "Shows in Korean" (the language is folded into what is listed), then "rated 7+" and "under 90 min".
export function withLanguage(what: string, languageNameText: string | undefined): string {
  return languageNameText ? t('{what} in {language}', { what, language: languageNameText }) : what
}

export function extraTitleParts(f: Pick<ExtraFilters, 'minRating' | 'runtime'>): string[] {
  const out: string[] = []
  if (f.minRating) out.push(t('rated {n}+', { n: f.minRating }))
  if (f.runtime) out.push(runtimeLabel(f.runtime).toLowerCase())
  return out
}

// The heading of a filtered list: "Korean shows..." in words, years last.
// what is "Movies" or "Drama shows"; the years are the two Discover year filters.
export function browseTitle(what: string, f: Partial<ExtraFilters> & { yearFrom?: string; yearTo?: string }, languageNameText?: string): string {
  let main = withLanguage(what, f.language ? languageNameText : undefined)
  if (f.yearFrom && f.yearTo) main = t('{what} from {from} to {to}', { what: main, from: f.yearFrom, to: f.yearTo })
  else if (f.yearFrom) main = t('{what} from {from}', { what: main, from: f.yearFrom })
  else if (f.yearTo) main = t('{what} up to {to}', { what: main, to: f.yearTo })
  return [main, ...extraTitleParts({ minRating: f.minRating ?? '', runtime: f.runtime ?? '' })].join(', ')
}
