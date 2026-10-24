import { useEffect, useRef, useState } from 'react'
import { api, type NamingTokens } from '../api'
import NamingPreview from './NamingPreview'
import { FieldError, type Validation } from '../useValidation'
import { t } from '../i18n'
import TipLabel from './TipLabel'

// The box a clicked token goes into: a file name or a title's folder.
type Kind = 'movie' | 'tv' | 'movieFolder' | 'seriesFolder'

const EXAMPLES = { a: '{ - Edition Tags}', b: '{[Quality Full]}' }
const ORIGINAL_TOKENS = { file: '{Original Filename}', release: '{Original Title}', example: 'Example.Movie.2024.1080p.BluRay-GROUP' }

// The server names the token groups and ready-made formats in English.
function groupName(g: string): string {
  const names: Record<string, string> = {
    Movie: t('Movie'),
    Show: t('Show'),
    Episode: t('Episode'),
    Ids: t('Ids'),
    Quality: t('Quality'),
    'Media info': t('Media info'),
    Release: t('Release'),
  }
  return names[g] ?? g
}

function schemeName(id: string, name: string): string {
  if (id === 'detailed') return t('Detailed, like Radarr and Sonarr')
  if (id === 'plex-editions') return t('Plex with editions')
  if (id === 'trash') return t('TRaSH recommended')
  if (id === 'trash-plex-tmdb') return t('TRaSH recommended, Plex with TMDb id')
  if (id === 'trash-emby-tmdb') return t('TRaSH recommended, Emby with TMDb id')
  if (id === 'trash-jellyfin-tmdb') return t('TRaSH recommended, Jellyfin with TMDb id')
  return name
}

function folderSchemeName(id: string, name: string): string {
  if (id === 'default') return t('Mediarium default')
  return schemeName(id, name)
}

// The tokens that say something about the title itself, the only ones a folder name can use.
const FOLDER_GROUPS = ['Movie', 'Show', 'Ids']

// The custom naming builder: start from a ready-made format (or paste the one
// you use in Radarr or Sonarr), then edit it and click tokens to add them.
// The previews come from the real naming engine, so they match what imports
// produce.
export default function NamingBuilder({
  movieFormat,
  setMovieFormat,
  episodeFormat,
  setEpisodeFormat,
  movieFolder,
  setMovieFolder,
  seriesFolder,
  setSeriesFolder,
  v,
}: {
  movieFormat: string
  setMovieFormat: (s: string) => void
  episodeFormat: string
  setEpisodeFormat: (s: string) => void
  movieFolder: string
  setMovieFolder: (s: string) => void
  seriesFolder: string
  setSeriesFolder: (s: string) => void
  v: Validation
}) {
  const [data, setData] = useState<NamingTokens | null>(null)
  const [target, setTarget] = useState<Kind>('movie')
  const movieRef = useRef<HTMLInputElement>(null)
  const episodeRef = useRef<HTMLInputElement>(null)
  const movieFolderRef = useRef<HTMLInputElement>(null)
  const seriesFolderRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    api
      .namingTokens()
      .then(setData)
      .catch(() => setData(null))
  }, [])

  const current = data?.schemes.find((s) => s.movie === movieFormat && s.episode === episodeFormat)?.id ?? ''

  // An empty folder box means the usual "Title (Year)", so it matches the default.
  const folderSchemes = data?.folderSchemes ?? []
  const currentFolder = folderSchemes.find((s) => s.movie === (movieFolder.trim() || folderSchemes[0]?.movie) && s.series === (seriesFolder.trim() || folderSchemes[0]?.series))?.id ?? ''

  function pickFolderScheme(id: string) {
    const s = folderSchemes.find((x) => x.id === id)
    if (!s) return
    setMovieFolder(s.movie)
    setSeriesFolder(s.series)
  }

  function pickScheme(id: string) {
    const s = data?.schemes.find((x) => x.id === id)
    if (!s) return
    setMovieFormat(s.movie)
    setEpisodeFormat(s.episode)
  }

  // Put the token where the cursor is in the box that was used last.
  function insert(token: string) {
    const boxes = {
      movie: { el: movieRef.current, value: movieFormat, set: setMovieFormat },
      tv: { el: episodeRef.current, value: episodeFormat, set: setEpisodeFormat },
      movieFolder: { el: movieFolderRef.current, value: movieFolder, set: setMovieFolder },
      seriesFolder: { el: seriesFolderRef.current, value: seriesFolder, set: setSeriesFolder },
    }
    const { el, value, set } = boxes[target]
    const start = el?.selectionStart ?? value.length
    const end = el?.selectionEnd ?? value.length
    const before = value.slice(0, start)
    const gap = before !== '' && !/[\s([{.\-_]$/.test(before) ? ' ' : ''
    const next = before + gap + token + value.slice(end)
    set(next)
    const caret = start + gap.length + token.length
    requestAnimationFrame(() => {
      el?.focus()
      el?.setSelectionRange(caret, caret)
    })
  }

  const forFolder = target === 'movieFolder' || target === 'seriesFolder'
  const side = target === 'movie' || target === 'movieFolder' ? 'movie' : 'tv'
  const tokens = (data?.tokens ?? []).filter((t) => (t.kind === 'both' || t.kind === side) && (!forFolder || (FOLDER_GROUPS.includes(t.group) && t.token !== '{Edition Tags}')))
  const groups = [...new Set(tokens.map((t) => t.group))]

  return (
    <div className="naming-builder">
      <div className="naming-builder-forms">
        <label>
          <TipLabel text={t('Fills both patterns from a ready-made format that you can then change. Your own text is replaced when you pick one.')}>{t('Start from')}</TipLabel>
          <select value={current} onChange={(e) => pickScheme(e.target.value)}>
            <option value="" disabled>
              {t('Your own format')}
            </option>
            {data?.schemes.map((s) => (
              <option key={s.id} value={s.id}>
                {schemeName(s.id, s.name)}
              </option>
            ))}
          </select>
        </label>
        <small style={{ color: 'var(--text-dim)' }}>
          {t('Already have a format in Radarr or Sonarr? Copy it from Settings > Media Management there and paste it below. The same tokens work here.')}
        </small>
        <label>
          <TipLabel text={t('The pattern each movie file is named by. Words in curly braces are swapped for the title, year and so on; click a token below to add one.')}>{t('Movie file name')}</TipLabel>
          <input
            ref={movieRef}
            value={movieFormat}
            onChange={(e) => setMovieFormat(e.target.value)}
            onFocus={() => setTarget('movie')}
            placeholder="{Movie CleanTitle} ({Release Year}){ - Edition Tags}"
            spellCheck={false}
            {...v.bind('customFormat', movieFormat, setMovieFormat)}
          />
          <FieldError v={v} name="customFormat" />
        </label>
        <NamingPreview preset="custom" format={movieFormat || undefined} folder={movieFolder.trim() || undefined} />
        <label>
          <TipLabel text={t('The pattern each episode file is named by. Words in curly braces are swapped for the show, season and episode; click a token below to add one.')}>{t('Episode file name')}</TipLabel>
          <input
            ref={episodeRef}
            value={episodeFormat}
            onChange={(e) => setEpisodeFormat(e.target.value)}
            onFocus={() => setTarget('tv')}
            placeholder="{Series TitleYear} - S{Season:00}E{Episode:00} - {Episode CleanTitle}"
            spellCheck={false}
            {...v.bind('episodeFormat', episodeFormat, setEpisodeFormat)}
          />
          <FieldError v={v} name="episodeFormat" />
        </label>
        <NamingPreview preset={episodeFormat ? 'custom' : 'plex'} format={episodeFormat || undefined} kind="tv" folder={seriesFolder.trim() || undefined} />
        <label>
          <TipLabel text={t('Fills both folder patterns from a ready-made format that you can then change. Your own text is replaced when you pick one.')}>{t('Folders start from')}</TipLabel>
          <select value={currentFolder} onChange={(e) => pickFolderScheme(e.target.value)}>
            <option value="" disabled>
              {t('Your own format')}
            </option>
            {folderSchemes.map((s) => (
              <option key={s.id} value={s.id}>
                {folderSchemeName(s.id, s.name)}
              </option>
            ))}
          </select>
        </label>
        <div className="naming-folder-pair">
          <label>
            <TipLabel text={t("The pattern a new movie's folder is named by. A movie that already has a folder keeps it. Leave it empty for the usual Title (Year).")}>{t('Movie folder')}</TipLabel>
            <input
              ref={movieFolderRef}
              value={movieFolder}
              onChange={(e) => setMovieFolder(e.target.value)}
              onFocus={() => setTarget('movieFolder')}
              placeholder="{Movie CleanTitle} ({Release Year})"
              spellCheck={false}
              {...v.bind('movieFolder', movieFolder, setMovieFolder)}
            />
            <FieldError v={v} name="movieFolder" />
          </label>
          <label>
            <TipLabel text={t("The pattern a new show's folder is named by. A show that already has a folder keeps it. Leave it empty for the usual Title (Year).")}>{t('Show folder')}</TipLabel>
            <input
              ref={seriesFolderRef}
              value={seriesFolder}
              onChange={(e) => setSeriesFolder(e.target.value)}
              onFocus={() => setTarget('seriesFolder')}
              placeholder="{Series TitleYear}"
              spellCheck={false}
              {...v.bind('seriesFolder', seriesFolder, setSeriesFolder)}
            />
            <FieldError v={v} name="seriesFolder" />
          </label>
        </div>
      </div>
      {data && (
        <div className="naming-tokens">
          <small style={{ color: 'var(--text-dim)' }}>
            {target === 'movie' && t("Click a token to add it to the movie file name. Text inside the braces, like {a} or {b}, only shows up when there's a value.", EXAMPLES)}
            {target === 'tv' && t("Click a token to add it to the episode file name. Text inside the braces, like {a} or {b}, only shows up when there's a value.", EXAMPLES)}
            {forFolder && t("Click a token to add it to the folder name. Only tokens about the title are listed, since a folder holds many files. Text inside the braces, like {a}, only shows up when there's a value.", { a: '{tmdb-{TmdbId}}' })}
          </small>
          <small style={{ color: 'var(--text-dim)' }}>{t('{file} is the name the file came with, without its extension, like {example}. {release} is the name of the whole release.', ORIGINAL_TOKENS)}</small>
          {groups.map((g) => (
            <div key={g} className="naming-token-group">
              <span>{groupName(g)}</span>
              <div className="chip-row">
                {tokens
                  .filter((t) => t.group === g)
                  .map((t) => (
                    <button key={t.token} type="button" className="chip naming-token" onMouseDown={(e) => e.preventDefault()} onClick={() => insert(t.token)}>
                      {t.token}
                    </button>
                  ))}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
