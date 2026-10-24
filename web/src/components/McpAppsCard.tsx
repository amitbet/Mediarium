import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { api, type McpApp, type McpClient, type McpClientMade } from '../api'
import { useConfirm } from './ConfirmProvider'
import { useToast } from './Toast'
import CopyBox from './CopyBox'
import { timeAgo } from '../format'
import { locale, t, tx } from '../i18n'

const HA_REDIRECT = 'https://my.home-assistant.io/redirect/oauth'

// The apps that connected to MCP with a sign-in (OAuth), and the form for apps
// that ask for a client ID and secret instead of finding the sign-in themselves.
export default function McpAppsCard() {
  const toast = useToast()
  const confirm = useConfirm()
  const [apps, setApps] = useState<McpApp[] | null>(null)
  const [clients, setClients] = useState<McpClient[]>([])
  const [name, setName] = useState('')
  const [redirect, setRedirect] = useState('')
  const [busy, setBusy] = useState(false)
  const [made, setMade] = useState<McpClientMade | null>(null)

  const load = useCallback(() => {
    api
      .mcpApps()
      .then((r) => {
        setApps(r.apps)
        setClients(r.clients)
      })
      .catch((e) => toast.error(e instanceof Error ? e.message : String(e)))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])
  useEffect(load, [load])

  async function revoke(a: McpApp) {
    if (!(await confirm({ title: t('Disconnect "{name}"?', { name: a.clientName }), body: <p>{t('It stops working straight away. To connect it again, someone signs in from the app once more.')}</p>, confirmLabel: t('Disconnect'), danger: true }))) return
    try {
      await api.revokeMcpApp(a.id)
      toast.success(t('Disconnected.'))
      load()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e))
    }
  }

  async function removeClient(c: McpClient) {
    if (!(await confirm({ title: t('Remove "{name}"?', { name: c.name }), body: <p>{t('Its client ID and secret stop working, and anything it was connected with is disconnected.')}</p>, confirmLabel: t('Remove'), danger: true }))) return
    try {
      await api.deleteMcpClient(c.clientId)
      toast.success(t('Removed.'))
      load()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e))
    }
  }

  async function make(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    try {
      setMade(await api.createMcpClient(name.trim(), redirect.trim()))
      setName('')
      setRedirect('')
      load()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <h3>{t('Connected apps')}</h3>
      <p style={{ color: 'var(--text-dim)' }}>
        {t('Apps that found the sign-in by themselves, such as Home Assistant, appear here after an administrator allowed them. Disconnect one and it stops working at once.')}
      </p>
      {apps === null ? null : apps.length === 0 ? (
        <p className="field-note">{t('No app has connected with a sign-in yet.')}</p>
      ) : (
        <div className="table-scroll">
          <table className="connected-apps">
            <thead>
              <tr>
                <th>{t('App')}</th>
                <th>{t('Account')}</th>
                <th>{t('Access')}</th>
                <th>{t('Connected')}</th>
                <th>{t('Last used')}</th>
                <th>
                  <span className="sr-only">{t('Actions')}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {apps.map((a) => (
                <tr key={a.id}>
                  <td className="app-name">{a.clientName}</td>
                  <td data-label={t('Account')}>{a.account}</td>
                  <td data-label={t('Access')}>{a.scope.includes('mcp:write') ? t('Read and change') : t('Read only')}</td>
                  <td data-label={t('Connected')}>{new Date(a.createdAt).toLocaleDateString(locale())}</td>
                  <td data-label={t('Last used')}>{a.lastUsedAt ? timeAgo(a.lastUsedAt) : t('never')}</td>
                  <td>
                    <button onClick={() => void revoke(a)}>{t('Revoke')}</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <details className="client-details">
        <summary>{t('Register a client for apps that ask for a client ID and secret')}</summary>
        <p style={{ color: 'var(--text-dim)' }}>
          {tx('Home Assistant can take these under Application Credentials. Use {url} as the redirect address for it.', { url: <code>{HA_REDIRECT}</code> })}
        </p>
        <form className="client-form" onSubmit={(e) => void make(e)}>
          <label>
            {t('Name')}
            <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Home Assistant" maxLength={100} required />
          </label>
          <label>
            {t('Redirect address')}
            <input value={redirect} onChange={(e) => setRedirect(e.target.value)} placeholder={HA_REDIRECT} required />
          </label>
          <button className="primary" type="submit" disabled={busy || !name.trim() || !redirect.trim()}>
            {busy ? t('Making…') : t('Make client ID and secret')}
          </button>
        </form>
        {made && (
          <div className="client-made">
            <dl className="other-app-fields">
              <dt>{t('Client ID')}</dt>
              <dd>
                <CopyBox text={made.clientId} />
              </dd>
              <dt>{t('Client secret')}</dt>
              <dd>
                <CopyBox text={made.clientSecret} />
              </dd>
            </dl>
            <span style={{ color: 'var(--warning)' }}>{t("Copy the secret now. You won't see it again.")}</span>
          </div>
        )}
        {clients.length > 0 && (
          <ul className="other-app-tips">
            {clients.map((c) => (
              <li key={c.clientId}>
                <strong>{c.name}</strong> <code>{c.clientId}</code> <span style={{ color: 'var(--text-dim)' }}>{c.redirectUris.join(', ')}</span>{' '}
                <button onClick={() => void removeClient(c)}>{t('Remove')}</button>
              </li>
            ))}
          </ul>
        )}
      </details>
    </>
  )
}
