import { useMemo, useState } from 'react'
import { api, type CustomFormat } from '../api'
import { plural, t } from '../i18n'
import { useToast } from './Toast'

// "Add formats" on a profile's edit form: tick several custom formats and give
// them all a score in one go, either the one the TRaSH Guides give each of them
// or one typed score. The scores land in the form; they are saved with the
// profile.
export default function AddFormatsPicker({
  candidates,
  total,
  onAdd,
}: {
  // The formats the profile doesn't score yet.
  candidates: CustomFormat[]
  // How many custom formats there are in all.
  total: number
  // Gets every format that was ticked, and the score for each one that has one.
  onAdd: (ids: number[], scores: Record<number, string>) => void
}) {
  const toast = useToast()
  const [open, setOpen] = useState(false)
  const [picked, setPicked] = useState<Set<number>>(new Set())
  const [filter, setFilter] = useState('')
  const [useTrash, setUseTrash] = useState(true)
  const [score, setScore] = useState('')
  const [busy, setBusy] = useState(false)

  const shown = useMemo(() => {
    const q = filter.trim().toLowerCase()
    return candidates.filter((f) => !q || f.name.toLowerCase().includes(q))
  }, [candidates, filter])
  const typedOk = /^-?\d{1,6}$/.test(score.trim())

  function toggle(id: number) {
    const next = new Set(picked)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    setPicked(next)
  }

  async function add() {
    const ids = [...picked]
    const scores: Record<number, string> = {}
    setBusy(true)
    try {
      if (useTrash) {
        const r = await api.trashDefaultScores(ids)
        for (const s of r.scores) scores[s.formatId] = String(s.score)
        const none = ids.filter((id) => scores[id] === undefined).length
        if (none > 0) toast.info(plural(none, "{n} of them has no TRaSH score, so it was added without one. Type a score for it.", "{n} of them have no TRaSH score, so they were added without one. Type a score for them."))
      } else {
        for (const id of ids) scores[id] = String(Number(score.trim()))
      }
      onAdd(ids, scores)
      setPicked(new Set())
      setOpen(false)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  if (candidates.length === 0) return total > 0 ? <small style={{ color: 'var(--text-dim)' }}>{t('Every custom format is already in this profile.')}</small> : null
  if (!open) {
    return (
      <div>
        <button type="button" className="btn-sm" onClick={() => setOpen(true)}>
          {t('Add formats')}
        </button>
      </div>
    )
  }
  return (
    <fieldset className="group add-formats">
      <legend>{t('Add formats')}</legend>
      <input type="search" placeholder={t('Filter {n} formats', { n: candidates.length })} aria-label={t('Filter formats')} value={filter} onChange={(e) => setFilter(e.target.value)} style={{ width: '100%', marginBottom: 8 }} />
      <ul className="trash-formats">
        {shown.map((f) => (
          <li key={f.id}>
            <label>
              <input type="checkbox" checked={picked.has(f.id)} onChange={() => toggle(f.id)} />
              <span>{f.name}</span>
              <small className="trash-score">{f.app === 'sonarr' ? t('TV') : t('movies')}</small>
            </label>
          </li>
        ))}
      </ul>
      <div className="trash-import-bar">
        <div className="seg" role="radiogroup" aria-label={t('Score to give them')}>
          <button type="button" role="radio" aria-checked={useTrash} className={useTrash ? 'active' : ''} onClick={() => setUseTrash(true)}>
            {t("TRaSH's score for each")}
          </button>
          <button type="button" role="radio" aria-checked={!useTrash} className={!useTrash ? 'active' : ''} onClick={() => setUseTrash(false)}>
            {t('The same score for all')}
          </button>
        </div>
        {!useTrash && <input inputMode="numeric" aria-label={t('Score')} placeholder="0" value={score} onChange={(e) => setScore(e.target.value)} style={{ width: 96, textAlign: 'right' }} />}
        <button type="button" className="primary" disabled={picked.size === 0 || busy || (!useTrash && !typedOk)} onClick={() => void add()}>
          {picked.size === 0 ? t('Tick the formats to add') : plural(picked.size, 'Add {n} format', 'Add {n} formats')}
        </button>
        <button type="button" onClick={() => setOpen(false)}>
          {t('Cancel')}
        </button>
      </div>
    </fieldset>
  )
}
