import { useCallback, useEffect, useMemo, useState } from 'react'
import { api } from '../api'
import {
  MAX_NAMES,
  MAX_RULES,
  addName,
  emptyDraft,
  fileLine,
  mergeRules,
  previewLine,
  rulesOf,
  seasonShortcut,
  toDraft,
  type RuleDraft,
} from '../customNumbering'
import { plural, t } from '../i18n'
import { rescanMessage } from '../rescanMessage'
import { useToast } from './Toast'

// "Custom numbering and names": for a show whose files and releases count the
// seasons and episodes differently from TMDB, or call the show by another
// name. Rules say what the library's seasons are called in names; extra names
// are searched for and accepted next to the title. Collapsed until opened.
export default function CustomNumbering({
  id,
  seasons,
  monitored,
  onRescanned,
}: {
  id: number
  // The show's seasons in the library (specials left out).
  seasons: number[]
  monitored: boolean
  onRescanned?: () => void
}) {
  const toast = useToast()
  const [rows, setRows] = useState<RuleDraft[]>([])
  const [names, setNames] = useState<string[]>([])
  const [saved, setSaved] = useState('')
  const [count, setCount] = useState(0)
  const [nameText, setNameText] = useState('')
  const [shortcut, setShortcut] = useState('')
  const [allSeasons, setAllSeasons] = useState(false)
  const [busy, setBusy] = useState<'' | 'save' | 'rescan'>('')
  const [error, setError] = useState('')
  const [result, setResult] = useState('')
  const [loaded, setLoaded] = useState(false)

  const load = useCallback(() => {
    api
      .customNumbering(id)
      .then((c) => {
        setRows(c.rules.map(toDraft))
        setNames(c.names)
        setSaved(JSON.stringify({ rules: c.rules, names: c.names }))
        setCount(c.rules.length + c.names.length)
        setLoaded(true)
      })
      .catch(() => setLoaded(true))
  }, [id])
  useEffect(load, [load])

  const { rules, skipped } = useMemo(() => rulesOf(rows), [rows])
  const dirty = JSON.stringify({ rules, names }) !== saved

  function setRow(i: number, patch: Partial<RuleDraft>) {
    setRows((cur) => cur.map((r, j) => (j === i ? { ...r, ...patch } : r)))
    setResult('')
  }

  function applyShortcut() {
    const n = Number(shortcut)
    const made = seasonShortcut(n, seasons, allSeasons)
    if (made.length === 0) {
      setError(t('Type the season number the releases use.'))
      return
    }
    setError('')
    setResult('')
    setRows((cur) => mergeRules(rulesOf(cur).rules, made).map(toDraft))
    setShortcut('')
  }

  function addNameNow() {
    setNames((cur) => addName(cur, nameText))
    setNameText('')
    setResult('')
  }

  async function save(thenRescan: boolean) {
    setError('')
    setResult('')
    setBusy(thenRescan ? 'rescan' : 'save')
    try {
      // A name still being typed counts.
      const finalNames = addName(names, nameText)
      const out = await api.saveCustomNumbering(id, { rules, names: finalNames })
      setRows(out.rules.map(toDraft))
      setNames(out.names)
      setNameText('')
      setSaved(JSON.stringify({ rules: out.rules, names: out.names }))
      setCount(out.rules.length + out.names.length)
      if (!thenRescan) {
        toast.success(t('Saved.'))
        return
      }
      const rep = await api.rescanSeries(id)
      setResult(rescanMessage(rep, monitored))
      onRescanned?.()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy('')
    }
  }

  const shown = rules.slice(0, 3)
  const several = seasons.length > 1

  return (
    <details className="custom-numbering">
      <summary>
        <h3 style={{ margin: 0, display: 'inline' }}>{t('Custom numbering and names')}</h3>
        {count > 0 && <span className="badge custom-count">{count}</span>}
      </summary>
      {!loaded ? null : (
        <div className="custom-body">
          <p className="field-hint">
            {t('Use this when the files and releases number the show differently from TMDB, or call it by another name. The library keeps its own numbering for folders and file names; only the recognising changes.')}
          </p>

          <div className="custom-shortcut">
            <label className="custom-nowrap" htmlFor={`cn-season-${id}`}>
              {t("This show's releases use season")}
            </label>
            <input id={`cn-season-${id}`} className="custom-num" type="number" min={1} inputMode="numeric" value={shortcut} placeholder="15" onChange={(e) => setShortcut(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && applyShortcut()} />
            {several && (
              <label className="custom-nowrap custom-check">
                <input type="checkbox" checked={allSeasons} onChange={(e) => setAllSeasons(e.target.checked)} /> {t('Count the later seasons on from it')}
              </label>
            )}
            <button className="btn-sm" onClick={applyShortcut} disabled={!shortcut.trim()}>
              {t('Make the rule')}
            </button>
          </div>

          <div className="custom-table-wrap">
            <table className="custom-table">
              <thead>
                <tr>
                  <th>{t('Library season')}</th>
                  <th>{t('Release season')}</th>
                  <th>{t('Episode offset')}</th>
                  <th>{t('First episode')}</th>
                  <th>{t('Last episode')}</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {rows.length === 0 && (
                  <tr>
                    <td colSpan={6} className="field-hint">
                      {t('No rules. Releases and files are read with the numbering TMDB uses.')}
                    </td>
                  </tr>
                )}
                {rows.map((r, i) => (
                  <tr key={i}>
                    <td>
                      <input className="custom-num" type="number" min={1} inputMode="numeric" value={r.librarySeason} aria-label={t('Library season')} onChange={(e) => setRow(i, { librarySeason: e.target.value })} />
                    </td>
                    <td>
                      <input className="custom-num" type="number" min={1} inputMode="numeric" value={r.releaseSeason} aria-label={t('Release season')} onChange={(e) => setRow(i, { releaseSeason: e.target.value })} />
                    </td>
                    <td>
                      <input className="custom-num" type="number" inputMode="numeric" value={r.offset} placeholder="0" aria-label={t('Episode offset')} onChange={(e) => setRow(i, { offset: e.target.value })} />
                    </td>
                    <td>
                      <input className="custom-num" type="number" min={1} inputMode="numeric" value={r.from} placeholder={t('any')} aria-label={t('First episode')} onChange={(e) => setRow(i, { from: e.target.value })} />
                    </td>
                    <td>
                      <input className="custom-num" type="number" min={1} inputMode="numeric" value={r.to} placeholder={t('any')} aria-label={t('Last episode')} onChange={(e) => setRow(i, { to: e.target.value })} />
                    </td>
                    <td>
                      <button className="btn-sm danger-ghost" onClick={() => { setRows((cur) => cur.filter((_, j) => j !== i)); setResult('') }}>
                        {t('Remove')}
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div>
            <button className="btn-sm" onClick={() => setRows((cur) => [...cur, { ...emptyDraft(), librarySeason: String(seasons[0] ?? 1) }])} disabled={rows.length >= MAX_RULES}>
              {t('Add a rule')}
            </button>
          </div>
          <p className="field-hint custom-nowrap-hint">{t('A release episode is the library episode plus the offset. Leave the first and last episode empty for the whole season.')}</p>

          <div className="custom-names">
            <strong className="custom-nowrap">{t('Extra search names')}</strong>
            <div className="custom-chips">
              {names.length === 0 && <span className="field-hint">{t('None. Only the title and the names found online are used.')}</span>}
              {names.map((n) => (
                <span key={n} className="chip custom-chip">
                  {n}
                  <button type="button" className="custom-chip-x" aria-label={t('Remove {name}', { name: n })} onClick={() => { setNames((cur) => cur.filter((x) => x !== n)); setResult('') }}>
                    ×
                  </button>
                </span>
              ))}
            </div>
            <div className="custom-add-name">
              <input
                type="text"
                value={nameText}
                maxLength={120}
                placeholder={t('Another name the releases use')}
                aria-label={t('Another name the releases use')}
                onChange={(e) => setNameText(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault()
                    addNameNow()
                  }
                }}
              />
              <button className="btn-sm" onClick={addNameNow} disabled={!nameText.trim() || names.length >= MAX_NAMES}>
                {t('Add a name')}
              </button>
            </div>
          </div>

          {shown.length > 0 && (
            <ul className="custom-preview" aria-label={t('What this does')}>
              {shown.map((r, i) => (
                <li key={i}>
                  <span>{previewLine(r)}</span>
                  <span>{fileLine(r)}</span>
                </li>
              ))}
              {rules.length > shown.length && <li className="field-hint">{plural(rules.length - shown.length, '{n} more rule', '{n} more rules')}</li>}
            </ul>
          )}
          {skipped > 0 && <p className="field-hint">{plural(skipped, '{n} row is not finished and is left out.', '{n} rows are not finished and are left out.')}</p>}

          {error && <p className="error-text">{error}</p>}
          {result && <p className="custom-result" role="status">{result}</p>}

          <div className="custom-actions">
            <button className="btn-sm" onClick={() => void save(false)} disabled={busy !== '' || !dirty}>
              {busy === 'save' ? t('Saving…') : t('Save')}
            </button>
            <button className="primary btn-sm" onClick={() => void save(true)} disabled={busy !== ''}>
              {busy === 'rescan' ? t('Saving and rescanning…') : t('Save and rescan')}
            </button>
          </div>
        </div>
      )}
    </details>
  )
}
