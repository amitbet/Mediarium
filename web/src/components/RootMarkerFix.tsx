import { useEffect, useState } from 'react'
import { api, type RootMarkerInfo } from '../api'
import { plural, st, t } from '../i18n'
import { failedForFolder, folderName } from '../rootMarker'
import { useToast } from './Toast'

// For an administrator: a library folder has lost its .mediarium-root file (a new drive at the same path, a moved
// library, a tool that tidied hidden files) and imports into it are refused. This puts the file back after the
// person says it is the right folder, and then offers to retry the imports that were refused.
// `marker` is passed when the caller already has the folder check; otherwise the folder is checked here.
export default function RootMarkerFix({ path, marker, onFixed }: { path: string; marker?: RootMarkerInfo; onFixed?: () => void }) {
  const toast = useToast()
  const [info, setInfo] = useState<RootMarkerInfo | null | undefined>(marker)
  const [open, setOpen] = useState(false)
  const [ack, setAck] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [fixed, setFixed] = useState(false)
  const [failed, setFailed] = useState<number[]>([])

  useEffect(() => {
    if (marker !== undefined || fixed) {
      if (marker !== undefined && !fixed) setInfo(marker)
      return
    }
    let stale = false
    api
      .folderCheck(path)
      .then((c) => !stale && setInfo(c.marker ?? null))
      .catch(() => !stale && setInfo(null))
    return () => {
      stale = true
    }
  }, [path, marker, fixed])

  async function create() {
    setBusy(true)
    setError('')
    try {
      await api.createRootMarker(path, ack)
      setFixed(true)
      setOpen(false)
      onFixed?.()
      const items = await api.listQueue().catch(() => [])
      setFailed(failedForFolder(items, path))
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  async function retry() {
    setBusy(true)
    try {
      const r = await api.bulkQueue(failed, 'retry')
      toast.success(plural(r.done, 'Trying {n} download again.', 'Trying {n} downloads again.'))
      setFailed([])
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  if (fixed) {
    return (
      <div className="marker-fix">
        <p className="field-hint">{t('Done. Imports into {path} work again.', { path })}</p>
        {failed.length > 0 && (
          <button className="btn-sm primary" disabled={busy} onClick={() => void retry()}>
            {t('Retry the failed imports')} ({failed.length})
          </button>
        )}
      </div>
    )
  }
  if (!info) return null
  if (!info.canCreate) {
    return <p className="field-hint marker-fix">{info.why ? st(info.why) : null}</p>
  }
  const needsAck = info.strong
  return (
    <div className="marker-fix">
      {!open ? (
        <button className="btn-sm" onClick={() => setOpen(true)}>
          {t('This is the right folder: create the marker')}
        </button>
      ) : (
        <div className={`marker-confirm${needsAck ? ' strong' : ''}`} role="group" aria-label={t('Create the marker file')}>
          <p>{t('Mediarium keeps a small empty file named {name} in each library folder. It tells a real folder from the empty one left behind when a drive is not connected.', { name: info.name })}</p>
          {needsAck ? (
            <p className="error-text">
              {t("Titles are recorded in {path}, but none of their folders is there. That usually means the drive is not connected, and this is an empty folder on the machine's own disk. Making the file now would let new downloads fill that disk.", { path })}
            </p>
          ) : (
            <p>{t('Only press this if the drive is connected and {path} is the folder your library is in.', { path })}</p>
          )}
          {needsAck && (
            <label className="inline-field">
              <input type="checkbox" checked={ack} onChange={(e) => setAck(e.target.checked)} />
              {t('I know the drive is connected (the folder is {name})', { name: folderName(path) })}
            </label>
          )}
          {error && <p className="error-text">{error}</p>}
          <div className="row-actions">
            <button className="btn-sm primary" disabled={busy || (needsAck && !ack)} onClick={() => void create()}>
              {busy ? t('Creating…') : t('Create the marker')}
            </button>
            <button className="btn-sm" disabled={busy} onClick={() => setOpen(false)}>
              {t('Cancel')}
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
