import { useCallback, useEffect, useRef, useState } from 'react'
import { api, type UsenetClientInput, type UsenetClientKind, type UsenetClientPriority, type UsenetClientSettings } from '../api'
import { t } from '../i18n'
import Icon from './Icon'
import { useToast } from './Toast'
import TipLabel from './TipLabel'
import { reloadForm } from './downloadClientForm'

const LABEL: Record<Exclude<UsenetClientKind, ''>, string> = {
  sabnzbd: 'SABnzbd',
  nzbget: 'NZBGet',
}

const blank: UsenetClientInput = {
  kind: '',
  use: false,
  url: '',
  apiKey: '',
  username: '',
  password: '',
  category: '',
  categoryMovies: '',
  categoryTv: '',
  categoryMusic: '',
  categoryBooks: '',
  priority: 'normal',
  sendUrl: false,
  remotePath: '',
  localPath: '',
  removeAfterImport: false,
  removeOnStop: true,
}

function toInput(s: UsenetClientSettings): UsenetClientInput {
  return {
    kind: s.kind,
    use: s.use,
    url: s.url,
    apiKey: '',
    username: s.username,
    password: '',
    category: s.category,
    categoryMovies: s.categoryMovies,
    categoryTv: s.categoryTv,
    categoryMusic: s.categoryMusic,
    categoryBooks: s.categoryBooks,
    priority: s.priority || 'normal',
    sendUrl: s.sendUrl,
    remotePath: s.remotePath,
    localPath: s.localPath,
    removeAfterImport: s.removeAfterImport,
    removeOnStop: s.removeOnStop,
  }
}

// Settings > Downloading > Usenet: send Usenet releases to a downloader you
// already run (SABnzbd or NZBGet) instead of the built-in one. The downloader
// fetches, repairs and unpacks; Mediarium imports the finished files from its
// folder without moving them. The built-in downloader stays the default.
export default function UsenetClientCard({ onChanged }: { onChanged?: () => void }) {
  const toast = useToast()
  const [form, setForm] = useState<UsenetClientInput>(blank)
  const [saved, setSaved] = useState<UsenetClientSettings | null>(null)
  const [busy, setBusy] = useState(false)
  const [result, setResult] = useState<{ ok: boolean; version?: string; error?: string } | null>(null)
  const [loadFailed, setLoadFailed] = useState(false)
  // The saved copy the form was last in step with, so a reload keeps what the
  // person typed and did not save.
  const savedRef = useRef<UsenetClientSettings | null>(null)

  const load = useCallback(() => {
    api
      .usenetClient()
      .then((d) => {
        const was = savedRef.current ? toInput(savedRef.current) : blank
        savedRef.current = d
        setSaved(d)
        setForm((f) => reloadForm(f, was, toInput(d)))
        setLoadFailed(false)
      })
      .catch(() => setLoadFailed(true))
  }, [])
  useEffect(load, [load])

  function set<K extends keyof UsenetClientInput>(key: K, value: UsenetClientInput[K]) {
    setForm((f) => ({ ...f, [key]: value }))
    setResult(null)
  }

  const isSab = form.kind === 'sabnzbd'
  const sameClient = !!saved && saved.kind === form.kind && saved.url === form.url
  const keyKept = isSab && sameClient && !!saved?.hasApiKey && form.apiKey === ''
  const passwordKept = !isSab && sameClient && !!saved?.hasPassword && form.password === ''
  const savedInput = saved ? toInput(saved) : blank
  const changed = (Object.keys(form) as (keyof UsenetClientInput)[]).some((k) => form[k] !== savedInput[k])
  const name = form.kind ? LABEL[form.kind] : ''
  const needsKey = isSab && !keyKept && form.apiKey === ''

  async function test() {
    setBusy(true)
    setResult(null)
    try {
      setResult(await api.testUsenetClient(form))
    } catch (e) {
      setResult({ ok: false, error: e instanceof Error ? e.message : String(e) })
    } finally {
      setBusy(false)
    }
  }

  async function save() {
    setBusy(true)
    try {
      const d = await api.saveUsenetClient(form)
      savedRef.current = d
      setSaved(d)
      setForm(toInput(d))
      onChanged?.()
      if (!d.kind) toast.success(t('Saved. Usenet releases are downloaded here again.'))
      else if (d.use) toast.success(t('Saved. Usenet releases now go to {name}.', { name: LABEL[d.kind as 'sabnzbd'] }))
      else toast.success(t('Saved. Usenet releases are still downloaded here.'))
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <fieldset className="group usenet span-all">
      <legend>
        <Icon name="server" size={14} /> <TipLabel help="downloads#sabnzbd-and-nzbget">{t('Use SABnzbd or NZBGet')}</TipLabel>
      </legend>
      <p style={{ marginTop: 0 }}>
        {t('Hand Usenet releases to a downloader you already run. SABnzbd or NZBGet does the downloading, repairing and unpacking, and Mediarium imports the finished files from its folder without moving them.')}
      </p>
      <p className="field-help">{t("The built-in downloader stays the default. It is only left out when you pick a downloader here and switch it on.")}</p>
      <div className="form-cols">
        <label>
          {t('Downloader')}
          <select
            value={form.kind}
            onChange={(e) => {
              const kind = e.target.value as UsenetClientKind
              setForm((f) => ({ ...f, kind, use: kind === '' ? false : f.use }))
              setResult(null)
            }}
          >
            <option value="">{t('None (use the built-in downloader)')}</option>
            <option value="sabnzbd">SABnzbd</option>
            <option value="nzbget">NZBGet</option>
          </select>
        </label>
        {form.kind && (
          <>
            <label>
              {t('Address')}
              <input value={form.url} onChange={(e) => set('url', e.target.value)} placeholder={isSab ? 'http://192.168.1.10:8080' : 'http://192.168.1.10:6789'} autoComplete="off" spellCheck={false} />
              <small className="field-help">{t('Where you open its web page.')}</small>
            </label>
            {isSab ? (
              <label>
                {t('API key')}
                <input
                  type="password"
                  value={form.apiKey}
                  onChange={(e) => set('apiKey', e.target.value)}
                  placeholder={keyKept ? t('Saved. Type a new one to replace it.') : ''}
                  autoComplete="new-password"
                  spellCheck={false}
                />
                <small className="field-help">{t('In SABnzbd under Config > General.')}</small>
              </label>
            ) : (
              <>
                <label>
                  {t('User name')}
                  <input value={form.username} onChange={(e) => set('username', e.target.value)} autoComplete="off" spellCheck={false} />
                </label>
                <label>
                  {t('Password')}
                  <input
                    type="password"
                    value={form.password}
                    onChange={(e) => set('password', e.target.value)}
                    placeholder={passwordKept ? t('Saved. Type a new one to replace it.') : ''}
                    autoComplete="new-password"
                  />
                  <small className="field-help">{t("The ControlUsername and ControlPassword in NZBGet's settings, under Security.")}</small>
                </label>
              </>
            )}
            <label>
              {t('Category')}
              <input value={form.category} onChange={(e) => set('category', e.target.value)} placeholder="mediarium" autoComplete="off" spellCheck={false} />
              <small className="field-help">
                {t('Downloads are added in this category. It has to exist in {name} already, which is also where its download folder is set. If you clear it and fill in a category below, a kind with none of its own gets no category. With everything empty, "mediarium" is used.', { name })}
              </small>
            </label>
            <label>
              {t('Category for movies')}
              <input value={form.categoryMovies} onChange={(e) => set('categoryMovies', e.target.value)} autoComplete="off" spellCheck={false} />
            </label>
            <label>
              {t('Category for TV shows')}
              <input value={form.categoryTv} onChange={(e) => set('categoryTv', e.target.value)} autoComplete="off" spellCheck={false} />
            </label>
            <label>
              {t('Category for music')}
              <input value={form.categoryMusic} onChange={(e) => set('categoryMusic', e.target.value)} autoComplete="off" spellCheck={false} />
            </label>
            <label>
              {t('Category for books')}
              <input value={form.categoryBooks} onChange={(e) => set('categoryBooks', e.target.value)} autoComplete="off" spellCheck={false} />
            </label>
            <label>
              {t('Priority')}
              <select value={form.priority} onChange={(e) => set('priority', e.target.value as UsenetClientPriority)}>
                <option value="low">{t('Low')}</option>
                <option value="normal">{t('Normal')}</option>
                <option value="high">{t('High')}</option>
                <option value="force">{t('Force')}</option>
              </select>
            </label>
            <small className="field-help" style={{ gridColumn: '1 / -1' }}>{t('The four categories above are optional. Fill one in to keep that kind of download apart in the downloader. An empty one uses the main category.')}</small>
          </>
        )}
      </div>
      {form.kind && (
        <>
          <label className="check-hint">
            <input type="checkbox" checked={form.use} onChange={(e) => set('use', e.target.checked)} />
            <span>
              {t('Send Usenet releases to {name}', { name })}
              <small>{t('Off: the built-in downloader is used and {name} is left alone.', { name })}</small>
            </span>
          </label>
          <label className="check-hint">
            <input type="checkbox" checked={form.sendUrl} onChange={(e) => set('sendUrl', e.target.checked)} />
            <span>
              {t('Let {name} fetch the NZB file itself', { name })}
              <small>{t("Off: Mediarium gets the NZB, checks it and sends the file on. On: only the link is sent, so Mediarium can't check the file first. Releases that need Mediarium's sign-in are always sent as a file.")}</small>
            </span>
          </label>
          <h3 style={{ margin: '18px 0 4px', fontSize: '1rem' }}><TipLabel help="downloads#path-mapping">{t('Path mapping')}</TipLabel></h3>
          <p className="field-help">
            {t("Only needed when {name} runs in another container or on another computer and sees its download folder under another path. Example: it saves to /downloads/complete, and Mediarium sees the same folder as /data/usenet/complete.", { name })}
          </p>
          <div className="form-cols">
            <label>
              {t("The downloader's folder")}
              <input value={form.remotePath} onChange={(e) => set('remotePath', e.target.value)} placeholder="/downloads/complete" autoComplete="off" spellCheck={false} />
            </label>
            <label>
              {t('The same folder in Mediarium')}
              <input value={form.localPath} onChange={(e) => set('localPath', e.target.value)} placeholder="/data/usenet/complete" autoComplete="off" spellCheck={false} />
            </label>
          </div>
          <label className="check-hint">
            <input type="checkbox" checked={form.removeOnStop} onChange={(e) => set('removeOnStop', e.target.checked)} />
            <span>
              {t('When I stop or remove a download, remove it from {name} too', { name })}
              <small>{t("Only for a download that Mediarium sent and that hasn't finished: it is removed with what it downloaded. A finished one stays.")}</small>
            </span>
          </label>
          <label className="check-hint">
            <input type="checkbox" checked={form.removeAfterImport} onChange={(e) => set('removeAfterImport', e.target.checked)} />
            <span>
              {t('Remove from {name} after import', { name })}
              <small>{t("Off: the finished download stays in {name} and its files are never touched. On: once it is in your library, the job is removed from {name}. SABnzbd deletes the files with it; NZBGet only takes it out of its history.", { name })}</small>
            </span>
          </label>
        </>
      )}
      <div className="form-actions" style={{ justifyContent: 'flex-start' }}>
        {form.kind && (
          <button onClick={() => void test()} disabled={busy || !form.url}>
            {busy ? t('Testing…') : t('Test')}
          </button>
        )}
        <button className="primary" onClick={() => void save()} disabled={busy || !changed || (form.kind !== '' && (!form.url || needsKey))}>
          {t('Save')}
        </button>
      </div>
      {loadFailed && (
        <div className="indexer-result bad" role="alert">
          <Icon name="warning" size={14} /> {t("Couldn't load the Usenet downloader settings. Try reloading the page.")}
        </div>
      )}
      {result && (
        <div className={`indexer-result ${result.ok ? 'ok' : 'bad'}`}>
          <Icon name={result.ok ? 'check' : 'warning'} size={14} />{' '}
          {result.ok ? t('Connected to {name} {version}.', { name, version: result.version ?? '' }) : result.error}
        </div>
      )}
    </fieldset>
  )
}
