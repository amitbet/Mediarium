import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, type ManualFile, type ManualFolders, type ManualMode, type Movie, type Series } from '../api'
import { formatBytes } from '../format'
import Icon from '../components/Icon'
import KindGlyph from '../components/KindGlyph'
import Loading from '../components/Loading'
import { useToast } from '../components/Toast'
import { t, tc } from '../i18n'
import { folderToRemember, MANUAL_STORE, manualMode, recall, remember, rememberedFolder, sourceKept } from '../manualImport'
import '../import.css'

const norm = (s: string) => s.toLowerCase().replace(/[^a-z0-9]+/g, ' ').trim()

const MODES: { id: ManualMode; label: () => string; hint: () => string }[] = [
  { id: 'link', label: () => t('Hardlink'), hint: () => t('No extra space. The original stays where it is. Where a hardlink is not possible (another drive) it is copied.') },
  { id: 'copy', label: () => t('Copy'), hint: () => t('A separate copy that uses its own space. The original stays where it is.') },
  { id: 'move', label: () => t('Move'), hint: () => t('The original is removed once the file is in your library.') },
]

// Import a file by hand: video files in a folder (the downloads folder, or any
// folder you choose) that were not matched, each with a guess of what it is.
// Pick the movie or the episode and Mediarium names it and brings it into the
// library, linked, copied or moved.
export default function ManualImport() {
  const [files, setFiles] = useState<ManualFile[] | null>(null)
  const [folder, setFolder] = useState('') // the folder that was read
  const [typed, setTyped] = useState(() => rememberedFolder()) // what the box says
  const [downloads, setDownloads] = useState('')
  const [hidden, setHidden] = useState(0)
  const [more, setMore] = useState(false)
  const [incomplete, setIncomplete] = useState(() => recall(MANUAL_STORE.incomplete) === '1')
  const [mode, setMode] = useState<ManualMode>(() => manualMode(recall(MANUAL_STORE.mode)))
  const [movies, setMovies] = useState<Movie[]>([])
  const [series, setSeries] = useState<Series[]>([])
  const [error, setError] = useState('')
  const [picking, setPicking] = useState(false)
  const [done, setDone] = useState<Set<string>>(new Set())

  const look = useCallback((where: string, withIncomplete: boolean) => {
    setError('')
    setFiles(null)
    setDone(new Set())
    api
      .manualImportList(where, withIncomplete)
      .then((r) => {
        setFiles(r.files)
        setFolder(r.folder)
        setTyped(r.folder === r.downloadsFolder ? '' : r.folder)
        setDownloads(r.downloadsFolder)
        setHidden(r.hiddenIncomplete ?? 0)
        setMore(!!r.more)
        remember(MANUAL_STORE.folder, folderToRemember(r.folder, r.downloadsFolder))
      })
      .catch((e) => {
        setFiles([])
        setHidden(0)
        setError(e instanceof Error ? e.message : String(e))
      })
  }, [])

  useEffect(() => {
    look(rememberedFolder(), recall(MANUAL_STORE.incomplete) === '1')
    api.listMovies().then(setMovies).catch(() => undefined)
    api.listSeries().then(setSeries).catch(() => undefined)
  }, [look])

  const left = (files ?? []).filter((f) => !done.has(f.path))
  return (
    <div className="import-page">
      <div className="page-header">
        <h1>{t('Import a file by hand')}</h1>
        <Link className="btn-link" to="/queue">
          <Icon name="activity" size={15} /> {t('Back to Activity')}
        </Link>
      </div>
      <p className="imp-lede">
        {folder
          ? t("Video files in {folder}, biggest first. Choose what each one is, and Mediarium names it and puts it in your library. The title must be in your library already; add it first if it isn't.", { folder })
          : t("Video files in the folder you choose, biggest first. Choose what each one is, and Mediarium names it and puts it in your library. The title must be in your library already; add it first if it isn't.")}
      </p>

      <div className="card manual-source">
        <div className="manual-folder">
          <label htmlFor="manual-folder">{t('Folder to look in')}</label>
          <div className="manual-folder-row">
            <input
              id="manual-folder"
              value={typed}
              placeholder={downloads || '/downloads'}
              spellCheck={false}
              onChange={(e) => setTyped(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && look(typed.trim(), incomplete)}
            />
            <button onClick={() => look(typed.trim(), incomplete)}>{t('Look here')}</button>
            <button onClick={() => setPicking((p) => !p)} aria-expanded={picking}>
              <Icon name="folder" size={15} /> {t('Choose a folder')}
            </button>
            {folder !== downloads && downloads !== '' && (
              <button className="btn-link" onClick={() => look('', incomplete)}>
                {t('Downloads folder')}
              </button>
            )}
          </div>
          {picking && (
            <FolderPicker
              start={folder}
              onPick={(p) => {
                setPicking(false)
                look(p, incomplete)
              }}
              onClose={() => setPicking(false)}
            />
          )}
        </div>
        <div className="manual-options">
          <div className="manual-mode">
            <span id="manual-mode-label">{t('How it comes in')}</span>
            <div className="seg" role="radiogroup" aria-labelledby="manual-mode-label">
              {MODES.map((m) => (
                <button
                  key={m.id}
                  role="radio"
                  aria-checked={mode === m.id}
                  className={mode === m.id ? 'active' : ''}
                  onClick={() => {
                    setMode(m.id)
                    remember(MANUAL_STORE.mode, m.id)
                  }}
                >
                  {m.label()}
                </button>
              ))}
            </div>
            <small>{MODES.find((m) => m.id === mode)?.hint()}</small>
          </div>
          <label className="check-inline">
            <input
              type="checkbox"
              checked={incomplete}
              onChange={(e) => {
                setIncomplete(e.target.checked)
                remember(MANUAL_STORE.incomplete, e.target.checked ? '1' : '0')
                look(folder, e.target.checked)
              }}
            />{' '}
            {t('Show incomplete downloads')}
          </label>
        </div>
      </div>

      {error && <p className="error-text">{error}</p>}
      {files === null && !error && <Loading height={140} />}
      {files !== null && !incomplete && hidden > 0 && <p className="imp-note">{t('Files in incomplete download folders are hidden. Turn on Show incomplete downloads to see them.')}</p>}
      {files !== null && !error && left.length === 0 && (
        <div className="empty-state">
          <Icon name="check" size={44} />
          <p>{t('There are no video files waiting in this folder.')}</p>
        </div>
      )}
      {more && <p className="imp-note">{t('There are more files here than the list shows. Choose a smaller folder to see the rest.')}</p>}
      <div className="manual-list">
        {left.map((f) => (
          <ManualRow
            key={f.path}
            file={f}
            folder={folder}
            mode={mode}
            movies={movies}
            series={series}
            onDone={() => setDone((d) => new Set(d).add(f.path))}
          />
        ))}
      </div>
    </div>
  )
}

// FolderPicker lists the folders inside a folder, to walk to the one to look in.
function FolderPicker({ start, onPick, onClose }: { start: string; onPick: (path: string) => void; onClose: () => void }) {
  const [here, setHere] = useState<ManualFolders | null>(null)
  const [error, setError] = useState('')

  const open = useCallback(async (path: string, fallback = true) => {
    setError('')
    try {
      setHere(await api.manualImportFolders(path))
    } catch (e) {
      if (path && fallback) return open('', false)
      setError(e instanceof Error ? e.message : String(e))
    }
  }, [])
  useEffect(() => {
    void open(start)
  }, [open, start])

  return (
    <div className="manual-picker" role="group" aria-label={t('Choose a folder')}>
      <div className="manual-picker-head">
        <code>{here?.path || '/'}</code>
        <button className="btn-link" onClick={onClose}>
          {t('Close')}
        </button>
      </div>
      {here?.shortcuts && here.shortcuts.length > 0 && (
        <div className="manual-picker-short">
          {here.shortcuts.map((s) => (
            <button key={s.path} className="chip" onClick={() => void open(s.path)}>
              {s.name}
            </button>
          ))}
        </div>
      )}
      {error && <p className="error-text">{error}</p>}
      {here === null && !error && <Loading height={60} />}
      {here && (
        <ul className="manual-picker-list">
          {here.parent && (
            <li>
              <button onClick={() => void open(here.parent ?? '')}>
                <Icon name="chevron-left" size={14} /> {t('Up')}
              </button>
            </li>
          )}
          {here.folders.map((f) => (
            <li key={f.path}>
              <button onClick={() => void open(f.path)}>
                <Icon name="folder" size={14} /> {f.name}
              </button>
            </li>
          ))}
          {here.folders.length === 0 && <li className="manual-picker-empty">{t('No folders in here.')}</li>}
        </ul>
      )}
      {here && here.path && (
        <button className="primary" onClick={() => onPick(here.path)}>
          {t('Use this folder')}
        </button>
      )}
    </div>
  )
}

function ManualRow({
  file: f,
  folder,
  mode,
  movies,
  series,
  onDone,
}: {
  file: ManualFile
  folder: string
  mode: ManualMode
  movies: Movie[]
  series: Series[]
  onDone: () => void
}) {
  const toast = useToast()
  const guessMovie = useMemo(() => movies.find((m) => norm(m.title) === norm(f.title ?? '') && (!f.year || m.year === f.year)), [movies, f])
  const guessShow = useMemo(() => series.find((s) => norm(s.title) === norm(f.title ?? '')), [series, f])
  const [kind, setKind] = useState<'movie' | 'episode'>(f.season || f.episode ? 'episode' : 'movie')
  const [movieId, setMovieId] = useState(0)
  const [seriesId, setSeriesId] = useState(0)
  const [season, setSeason] = useState(String(f.season ?? 1))
  const [episode, setEpisode] = useState(f.episode ? String(f.episode) : '')
  const [busy, setBusy] = useState(false)
  const chosenMovie = movieId || guessMovie?.id || 0
  const chosenShow = seriesId || guessShow?.id || 0

  async function run() {
    setBusy(true)
    try {
      const base = { folder, path: f.path, mode, incomplete: !!f.incomplete }
      const r =
        kind === 'movie'
          ? await api.manualImport({ ...base, movieId: chosenMovie })
          : await api.manualImport({ ...base, seriesId: chosenShow, season: Number(season), episode: Number(episode) })
      if (sourceKept(mode, r.sourceKept)) toast.info(t("Imported to {path}, but the original couldn't be removed.", { path: r.path }))
      else toast.success(t('Imported to {path}', { path: r.path }))
      onDone()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  const ready = kind === 'movie' ? chosenMovie > 0 : chosenShow > 0 && Number(episode) > 0 && Number(season) >= 0
  const guess = f.title
    ? ` · ${t('looks like {what}', { what: `${f.title}${f.year ? ` (${f.year})` : ''}${f.season ? ` S${String(f.season).padStart(2, '0')}E${String(f.episode ?? 0).padStart(2, '0')}` : ''}` })}`
    : ''
  const slash = f.path.lastIndexOf('/')
  return (
    <div className={`card manual-row k-${kind === 'movie' ? 'movie' : 'tv'}`}>
      <span className="manual-shape">
        <KindGlyph kind={kind === 'movie' ? 'movie' : 'tv'} size={30} variant="solid" />
      </span>
      <div className="manual-file">
        <strong>{f.path.slice(slash + 1)}</strong>
        {slash > 0 && <code>{f.path.slice(0, slash + 1)}</code>}
        <small>
          {formatBytes(f.size)}
          {f.quality ? ` · ${f.quality}` : ''}
          {guess}
          {f.incomplete ? ` · ${t('in an incomplete folder, it may not be finished')}` : ''}
        </small>
      </div>
      <div className="manual-pick">
        <div className="seg" role="radiogroup" aria-label={t('What it is')}>
          <button role="radio" aria-checked={kind === 'movie'} className={kind === 'movie' ? 'active' : ''} onClick={() => setKind('movie')}>
            {t('Movie')}
          </button>
          <button role="radio" aria-checked={kind === 'episode'} className={kind === 'episode' ? 'active' : ''} onClick={() => setKind('episode')}>
            {t('Episode')}
          </button>
        </div>
        {kind === 'movie' ? (
          <select aria-label={t('Movie')} value={chosenMovie} onChange={(e) => setMovieId(Number(e.target.value))}>
            <option value={0}>{t('Choose a movie…')}</option>
            {movies.map((m) => (
              <option key={m.id} value={m.id}>
                {m.title} ({m.year})
              </option>
            ))}
          </select>
        ) : (
          <>
            <select aria-label={tc('tv', 'Show')} value={chosenShow} onChange={(e) => setSeriesId(Number(e.target.value))}>
              <option value={0}>{t('Choose a show…')}</option>
              {series.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.title}
                </option>
              ))}
            </select>
            <label className="inline-field">
              {t('Season')} <input inputMode="numeric" value={season} onChange={(e) => setSeason(e.target.value)} style={{ width: 60 }} />
            </label>
            <label className="inline-field">
              {t('Episode')} <input inputMode="numeric" value={episode} onChange={(e) => setEpisode(e.target.value)} style={{ width: 60 }} />
            </label>
          </>
        )}
        <button className="primary" disabled={!ready || busy} onClick={() => void run()}>
          {busy ? t('Importing…') : t('Import')}
        </button>
      </div>
    </div>
  )
}
