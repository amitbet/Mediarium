import { useState } from 'react'
import { api, type SeedRule } from '../api'
import { t } from '../i18n'
import { useToast } from './Toast'
import '../torrentcontrols.css'
import TipLabel from './TipLabel'
import { isZeroLimit, parseRuleLimit, ruleLimitText } from '../seedLimits'

// An empty box follows the global limit (null); a number, including 0 (don't
// seed), is this indexer's own, and the word for unlimited means no limit of
// that kind even when the global one has one. Anything else is no number.
const text = (n: number | null | undefined) => ruleLimitText(n, t('unlimited'))
const parse = (s: string) => parseRuleLimit(s, t('unlimited'))

// How long the built-in torrent client seeds what came from one torrent
// indexer. Private trackers set their own rules, so the global limit in
// Settings is not always right for them.
export default function SeedRules({ indexerId }: { indexerId: number }) {
  const toast = useToast()
  const [loaded, setLoaded] = useState(false)
  const [saved, setSaved] = useState<SeedRule>({ ratio: null, hours: null })
  const [ratio, setRatio] = useState('')
  const [hours, setHours] = useState('')
  const [busy, setBusy] = useState(false)

  function adopt(rule: SeedRule) {
    setSaved(rule)
    setRatio(text(rule.ratio))
    setHours(text(rule.hours))
  }

  function load() {
    if (loaded) return
    api
      .seedRules()
      .then((all) => {
        adopt(all[String(indexerId)] ?? { ratio: null, hours: null })
        setLoaded(true)
      })
      .catch(() => undefined)
  }

  const r = parse(ratio)
  const h = parse(hours)
  const bad = r === undefined || h === undefined
  const changed = !bad && (r !== saved.ratio || h !== saved.hours)

  async function save() {
    if (bad) return
    setBusy(true)
    try {
      adopt(await api.setSeedRule(indexerId, { ratio: r, hours: h }))
      toast.success(r === null && h === null ? t('This indexer follows the global seeding limit.') : t('Seeding rule saved.'))
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <details className="qdetails" onToggle={(e) => e.currentTarget.open && load()}>
      <summary><TipLabel text={t('How long a torrent from this tracker keeps sharing, in the built-in torrent client or in qBittorrent or Transmission. Private trackers often ask for a minimum ratio or time.')} learnMore="downloads#seeding-rules-for-one-indexer">{t('Seeding rules')}</TipLabel></summary>
      <div className="seed-rule">
        <label>
          <small>{t('Seed until ratio')}</small>
          <input inputMode="decimal" value={ratio} placeholder={t('Global')} onChange={(e) => setRatio(e.target.value)} aria-invalid={r === undefined} />
        </label>
        <label>
          <small>{t('or for hours')}</small>
          <input inputMode="decimal" value={hours} placeholder={t('Global')} onChange={(e) => setHours(e.target.value)} aria-invalid={h === undefined} />
        </label>
        <button className="btn-sm" disabled={busy || !loaded || !changed} onClick={() => void save()}>
          {t('Save')}
        </button>
      </div>
      {(isZeroLimit(ratio) || isZeroLimit(hours)) && <small className="seed-rule">{t("Some trackers need you to share. Private trackers may ban accounts that don't seed.")}</small>}
      <small className="seed-rule">{t('Torrents from this indexer stop seeding at the first one reached. Leave a box empty to use the limit in Settings, type 0 to not seed at all, or type unlimited for no limit of that kind even when Settings has one. It applies to torrents that start downloading from now on. An external client gets these limits when the torrent is added (Transmission only the ratio). What it does when they are met, such as pausing or removing the torrent, is set in the client itself.')}</small>
    </details>
  )
}
