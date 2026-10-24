import { createPortal } from 'react-dom'
import { useEffect, useRef, useState } from 'react'
import { api, type BrowseAnswer } from '../api'
import { formatBytes } from '../format'
import { st, t } from '../i18n'
import { useFocusTrap } from '../useFocusTrap'
import { useToast } from './Toast'
import Icon from './Icon'

type Mode = 'file' | 'folder'

// Points a title at files you renamed or moved by hand. You browse the library
// folders and pick the video file (a movie), or the folder to read (a movie or
// a show). Nothing is moved, renamed or deleted: Mediarium only learns the new place.
export default function LocateDialog({
  kind,
  id,
  label = '',
  start,
  onClose,
  onDone,
}: {
  kind: 'movie' | 'series' | 'episode'
  id: number
  // The episode's name for the texts (kind 'episode').
  label?: string
  // The folder to open first (the title's own folder), when there is one.
  start?: string
  onClose: () => void
  onDone: () => void
}) {
  const toast = useToast()
  const box = useRef<HTMLDivElement>(null)
  useFocusTrap(box)
  const [mode, setMode] = useState<Mode>(kind === 'series' ? 'folder' : 'file')
  const [here, setHere] = useState<BrowseAnswer | null>(null)
  const [picked, setPicked] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function open(path: string) {
    setError('')
    setPicked('')
    try {
      setHere(await api.browseLibrary(path))
    } catch (e) {
      if (path) return open('')
      setError(e instanceof Error ? e.message : String(e))
    }
  }
  useEffect(() => {
    void open(start ?? '')
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && !busy && onClose()
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [busy, onClose])

  const atTop = !here || here.path === ''
  const target = mode === 'file' ? picked : here?.path ?? ''

  async function go() {
    if (!target) return
    setBusy(true)
    setError('')
    try {
      if (kind === 'series') {
        const rep = await api.changeSeriesFolder(id, target)
        if (rep.found > 0) toast.success(st(rep.message))
        else toast.info(st(rep.message))
      } else if (kind === 'episode') {
        const rep = await api.locateEpisodeFile(id, target)
        toast.success(t('Linked {file} to {episode}.', { file: rep.added ?? '', episode: label }))
      } else if (mode === 'file') {
        const rep = await api.locateMovieFile(id, target)
        toast.success(t('Found {file} and added it.', { file: rep.added ?? '' }))
      } else {
        const rep = await api.locateMovieFolder(id, target)
        if (rep.added) toast.success(t('Found {file} and added it.', { file: rep.added }))
        else if (rep.busy) toast.info(t('This movie is downloading right now, so nothing was changed.'))
        else {
          toast.info(t('No video file found in {folder}.', { folder: rep.looked ?? target }))
          setBusy(false)
          return
        }
      }
      onDone()
      onClose()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
      setBusy(false)
    }
  }

  const title = kind === 'movie' ? t('Locate the file') : kind === 'episode' ? t('Link a file to this episode') : t('Change folder')
  const goLabel = kind === 'series' ? t('Use this folder') : kind === 'episode' ? t('Link this file') : mode === 'file' ? t('Use this file') : t('Look in this folder')

  return createPortal(
    <div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && !busy && onClose()}>
      <div ref={box} tabIndex={-1} className="modal" role="dialog" aria-modal="true" aria-label={title}>
        <div className="modal-head">
          <div style={{ flex: 1, minWidth: 0 }}>
            <h2 style={{ margin: '0 0 4px' }}>{title}</h2>
            <p className="field-hint" style={{ margin: 0 }}>
              {kind === 'episode'
                ? t("Pick the video file of {episode}. It can be in the show's folder or anywhere else in your library folders. Nothing is moved, renamed or deleted.", { episode: label })
                : kind === 'series'
                ? t("Pick the show's new folder. Mediarium reads the episode files in it and attaches the ones it can place. Nothing is moved, renamed or deleted.")
                : mode === 'file'
                  ? t("Pick the video file of this title. Nothing is moved, renamed or deleted.")
                  : t('Pick a folder. The biggest video file in it becomes the file of this title. Nothing is moved, renamed or deleted.')}
            </p>
          </div>
          <button className="icon-btn" onClick={onClose} disabled={busy} aria-label={t('Close')}>
            <Icon name="x" />
          </button>
        </div>

        <div className="modal-body" style={{ display: 'grid', gap: 12 }}>
          {kind === 'movie' && (
            <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
              <button type="button" className={mode === 'file' ? 'primary btn-sm' : 'btn-sm'} onClick={() => { setMode('file'); setPicked('') }} disabled={busy}>
                {t('Pick the file')}
              </button>
              <button type="button" className={mode === 'folder' ? 'primary btn-sm' : 'btn-sm'} onClick={() => { setMode('folder'); setPicked('') }} disabled={busy}>
                {t('Look in a different folder')}
              </button>
            </div>
          )}
          <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
            <button type="button" className="btn-sm" onClick={() => void open(here?.parent ?? '')} disabled={busy || atTop}>
              {t('Up')}
            </button>
            <code style={{ minWidth: 0, overflowWrap: 'anywhere', fontSize: '0.82rem' }}>{atTop ? t('Your library folders') : here?.path}</code>
          </div>
          <ul className="locate-list" style={{ listStyle: 'none', margin: 0, padding: 0, maxHeight: 320, overflowY: 'auto', display: 'grid', gap: 2 }}>
            {here?.entries.length === 0 && <li className="field-hint">{t('This folder has no folders or video files in it.')}</li>}
            {here?.entries.map((en) => (
              <li key={en.path}>
                <button
                  type="button"
                  className={'locate-row' + (picked === en.path ? ' picked' : '')}
                  style={{ width: '100%', textAlign: 'left', display: 'flex', gap: 8, alignItems: 'center', background: picked === en.path ? 'var(--surface-2, rgba(128,128,128,0.18))' : 'transparent', border: 0, padding: '6px 8px', borderRadius: 8 }}
                  disabled={busy}
                  onClick={() => (en.dir ? void open(en.path) : setPicked(en.path))}
                  onDoubleClick={() => !en.dir && mode === 'file' && void go()}
                >
                  <Icon name={en.dir ? 'folder' : 'play'} size={14} />
                  <span style={{ flex: 1, minWidth: 0, overflowWrap: 'anywhere' }}>{en.name}</span>
                  {!en.dir && en.size !== undefined && <small className="field-hint">{formatBytes(en.size)}</small>}
                </button>
              </li>
            ))}
          </ul>
          {kind === 'episode' && picked && (
            <p className="field-hint" style={{ margin: 0, overflowWrap: 'anywhere' }}>
              {t('Link {file} to {episode}?', { file: picked.split(/[\\/]/).pop() ?? picked, episode: label })}
            </p>
          )}
          {error && <p className="error-text">{error}</p>}
        </div>

        <div className="modal-foot">
          <button onClick={onClose} disabled={busy}>
            {t('Cancel')}
          </button>
          <button className="primary" onClick={() => void go()} disabled={busy || !target}>
            {busy ? t('Working…') : goLabel}
          </button>
        </div>
      </div>
    </div>,
    document.body,
  )
}
