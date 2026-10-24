import { useEffect, useState } from 'react'
import { useToast } from '../../components/Toast'
import { api, type DownloadsStatus } from '../../api'
import { portStatus } from './torrentPort'
import { firstError, numberRange } from '../../validate'
import { FieldError, FormProblem, useValidation } from '../../useValidation'
import { t } from '../../i18n'
import TipLabel from '../../components/TipLabel'
import { isZeroLimit } from '../../seedLimits'

// What the torrent port is doing right now, in plain words. The wording comes
// from portStatus so it can be tested.
function PortStatus({ status }: { status: DownloadsStatus['torrent'] | null }) {
  const info = portStatus(status)
  if (!info) return null
  return (
    <div className={`notice ${info.tone === 'warn' ? 'notice-warn' : 'notice-info'}`} role="status">
      <strong>{info.title}</strong> {info.text}
    </div>
  )
}

export default function TorrentSection({ status }: { status: DownloadsStatus['torrent'] | null }) {
  const [listenPort, setListenPort] = useState('58264')
  const [seedRatioLimit, setSeedRatioLimit] = useState('')
  const [seedTimeLimitH, setSeedTimeLimitH] = useState('')
  const toast = useToast()
  // Saving before the current values have arrived would overwrite them with the defaults.
  const [loaded, setLoaded] = useState(false)
  const [saving, setSaving] = useState(false)
  const [loadError, setLoadError] = useState('')
  const errors = {
    listenPort: firstError(
      listenPort.trim() === '' && t('Enter a port number, or 0 for the default (58264).'),
      numberRange(listenPort, t('The listen port'), { min: 0, max: 65535 }),
    ),
    seedRatioLimit: numberRange(seedRatioLimit, t('The seed ratio limit'), { min: 0, max: 1000, decimal: true }),
    seedTimeLimitH: numberRange(seedTimeLimitH, t('The seed time limit'), { min: 0, max: 87600, decimal: true }),
  }
  const v = useValidation(errors)

  useEffect(() => {
    api
      .getSettings()
      .then((s) => {
        setListenPort(s.torrentListenPort || '58264')
        setSeedRatioLimit(s.torrentSeedRatioLimit ?? '')
        setSeedTimeLimitH(s.torrentSeedTimeLimitH ?? '')
        setLoaded(true)
      })
      .catch((e) => setLoadError(e instanceof Error ? e.message : String(e)))
  }, [])

  async function save() {
    if (!v.attempt()) return
    setSaving(true)
    try {
      await api.putSettings({ torrentListenPort: listenPort, torrentSeedRatioLimit: seedRatioLimit, torrentSeedTimeLimitH: seedTimeLimitH })
      toast.success(t('Saved.'))
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e))
    } finally {
      setSaving(false)
    }
  }

  return (
    <section className="card grid-form">
      <h2>{t('Client settings')}</h2>
      <p style={{ color: 'var(--text-dim)' }}>
        {t('The defaults are fine for most people.')}
      </p>
      <label>
        <TipLabel text={t('The port other torrent users connect to. Keep 58264 unless it clashes; if you change it, publish the same number in your Docker ports.')} learnMore="downloads#torrent-port-58264">{t('Listen port (0 = the default, 58264)')}</TipLabel>
        <input aria-label={t('Listen port (0 = the default, 58264)')} inputMode="numeric" value={listenPort} onChange={(e) => setListenPort(e.target.value)} {...v.bind('listenPort', listenPort, setListenPort)} />
        <FieldError v={v} name="listenPort" />
      </label>
      <PortStatus status={status} />
      <label>
        <TipLabel text={t("A finished torrent keeps sharing until it has uploaded this many times its size, or reaches the time limit. Whichever comes first stops it. Empty means no limit; 0 means don't seed at all. An external client such as qBittorrent gets these limits too, and decides itself what to do when they are met.")} learnMore="downloads#seeding">{t('Seed ratio limit')}</TipLabel>
        <input aria-label={t('Seed ratio limit')} type="number" min={0} step="any" inputMode="decimal" placeholder={t('Unlimited')} value={seedRatioLimit} onChange={(e) => setSeedRatioLimit(e.target.value)} {...v.bind('seedRatioLimit', seedRatioLimit, setSeedRatioLimit)} />
        <FieldError v={v} name="seedRatioLimit" />
      </label>
      <label>
        <TipLabel text={t("A finished torrent stops sharing after this many hours, or when it reaches the ratio limit. Whichever comes first stops it. Empty means no limit; 0 means don't seed at all.")} learnMore="downloads#seeding">{t('Seed time limit, hours')}</TipLabel>
        <input aria-label={t('Seed time limit, hours')} type="number" min={0} step="any" inputMode="decimal" placeholder={t('Unlimited')} value={seedTimeLimitH} onChange={(e) => setSeedTimeLimitH(e.target.value)} {...v.bind('seedTimeLimitH', seedTimeLimitH, setSeedTimeLimitH)} />
        <FieldError v={v} name="seedTimeLimitH" />
      </label>
      <small className="hint">{t("Leave a box empty for no limit. 0 means don't seed: the torrent stops as soon as it has downloaded.")}</small>
      {(isZeroLimit(seedRatioLimit) || isZeroLimit(seedTimeLimitH)) && (
        <small className="hint">{t("Some trackers need you to share. Private trackers may ban accounts that don't seed.")}</small>
      )}
      {loadError && <p className="error-text">{t('Could not load the current values: {error}', { error: loadError })}</p>}
      <button className="primary" onClick={save} disabled={!loaded || saving}>
        {t('Save')}
      </button>
      <FormProblem v={v} />
    </section>
  )
}
