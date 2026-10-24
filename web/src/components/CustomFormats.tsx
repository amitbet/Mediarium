import { useCallback, useEffect, useMemo, useState } from 'react'
import Dropdown from './Dropdown'
import { useToast } from './Toast'
import { useConfirm } from './ConfirmProvider'
import { api, type CustomFormat, type FormatTestResult, type QualityProfile, type TrashFormat, type TrashProfile } from '../api'
import { plural, t, tx, st } from '../i18n'
import TipLabel from './TipLabel'
import Collapsible from './Collapsible'
import TrashUpdateBar from './TrashUpdateBar'
import { importScoreAction, profileForApp, scorePreview, signed } from '../trashScores'
import { changeCount, profileChangeText, profileChangesText } from '../formatChanges'

type Panel = 'list' | 'trash' | 'paste' | 'test'
type App = 'radarr' | 'sonarr'

const APP_LABEL: Record<App, string> = { radarr: t('Movies'), sonarr: t('TV') }
const errText = (e: unknown) => (e instanceof Error ? e.message : String(e))

// Custom formats: named conditions on a release name (HDR, a codec, a
// release group, a streaming service…) that quality profiles give a score.
// They come from the TRaSH Guides, from Radarr/Sonarr JSON, and are tested
// here against any release name.
export default function CustomFormatsSection({ rev = 0, onChanged }: { rev?: number; onChanged?: () => void }) {
  const toast = useToast()
  const confirm = useConfirm()
  const [panel, setPanel] = useState<Panel>('list')
  const [formats, setFormats] = useState<CustomFormat[]>([])
  const [profiles, setProfiles] = useState<QualityProfile[]>([])
  const [defaults, setDefaults] = useState({ id: 0, tvId: 0 })
  const [error, setError] = useState('')
  // The card is folded away until wanted; a change made in it (an import, say) keeps it open.
  const [openSignal, setOpenSignal] = useState(0)

  const load = useCallback(() => {
    api
      .listCustomFormats()
      .then((r) => setFormats(r.formats))
      .catch((e) => setError(errText(e)))
    api
      .listProfiles()
      .then((r) => {
        setProfiles(r.profiles)
        setDefaults({ id: r.defaultId, tvId: r.defaultTvId })
      })
      .catch(() => setProfiles([]))
  }, [])
  useEffect(load, [load, rev])

  const changed = () => {
    setOpenSignal((n) => n + 1)
    load()
    onChanged?.()
  }

  async function remove(f: CustomFormat) {
    const body = f.usedBy.length ? <p>{t('Its score is taken out of {profiles}.', { profiles: f.usedBy.join(', ') })}</p> : <p>{t('No profile uses it.')}</p>
    if (!(await confirm({ title: t('Delete "{name}"?', { name: f.name }), body, confirmLabel: t('Delete format'), danger: true }))) return
    try {
      await api.deleteCustomFormat(f.id)
      toast.success(t('Deleted {name}.', { name: f.name }))
      changed()
    } catch (e) {
      toast.error(errText(e))
    }
  }

  return (
    <section className="card wide">
      <h2><TipLabel help="quality#custom-formats">{t('Custom formats')}</TipLabel></h2>
      <p style={{ color: 'var(--text-dim)' }}>
        {t('A custom format spots something in a release name, like HDR, a codec or a release group. Each quality profile gives the formats it cares about a score: releases scoring higher win, and a negative score keeps a release out. Most people start from the TRaSH Guides.')}
      </p>
      {error && <p className="error-text">{error}</p>}
      <Collapsible id="custom-formats" label={t('Custom formats')} summary={plural(formats.length, '{n} format', '{n} formats')} openSignal={openSignal}>
      {formats.some((f) => f.trashId) && <TrashUpdateBar onUpdated={changed} />}
      <div className="seg" role="tablist" aria-label={t('Custom formats')} style={{ marginBottom: 14 }}>
        <button role="tab" aria-selected={panel === 'list'} className={panel === 'list' ? 'active' : ''} onClick={() => setPanel('list')}>
          {t('Your formats')} <small>({formats.length})</small>
        </button>
        <button role="tab" aria-selected={panel === 'trash'} className={panel === 'trash' ? 'active' : ''} onClick={() => setPanel('trash')}>
          {t('From the TRaSH Guides')}
        </button>
        <button role="tab" aria-selected={panel === 'paste'} className={panel === 'paste' ? 'active' : ''} onClick={() => setPanel('paste')}>
          {t('Paste JSON')}
        </button>
        <button role="tab" aria-selected={panel === 'test'} className={panel === 'test' ? 'active' : ''} onClick={() => setPanel('test')}>
          {t('Test a release')}
        </button>
      </div>
      {panel === 'list' && <FormatList formats={formats} profiles={profiles} onAssigned={changed} onRemove={remove} onRenamed={changed} onStart={() => setPanel('trash')} />}
      {panel === 'trash' && <TrashPanel profiles={profiles} defaults={defaults} onImported={changed} />}
      {panel === 'paste' && (
        <PastePanel
          onImported={() => {
            changed()
            setPanel('list')
          }}
        />
      )}
      {panel === 'test' && <TestPanel profiles={profiles} hasFormats={formats.length > 0} />}
      </Collapsible>
    </section>
  )
}

function FormatList({
  formats,
  profiles,
  onAssigned,
  onRemove,
  onRenamed,
  onStart,
}: {
  formats: CustomFormat[]
  profiles: QualityProfile[]
  onAssigned: () => void
  onRemove: (f: CustomFormat) => void
  onRenamed: () => void
  onStart: () => void
}) {
  const toast = useToast()
  const [editing, setEditing] = useState<{ id: number; name: string } | null>(null)
  const [ticked, setTicked] = useState<Set<number>>(new Set())
  const allTicked = formats.length > 0 && formats.every((f) => ticked.has(f.id))
  const tick = (id: number) =>
    setTicked((cur) => {
      const next = new Set(cur)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })

  async function rename() {
    if (!editing) return
    try {
      await api.renameCustomFormat(editing.id, editing.name)
      setEditing(null)
      onRenamed()
    } catch (e) {
      toast.error(errText(e))
    }
  }

  if (formats.length === 0) {
    return (
      <p>
        {t('No custom formats yet.')} <button onClick={onStart}>{t('Browse the TRaSH Guides…')}</button>
      </p>
    )
  }
  return (
    <>
      {ticked.size > 0 && (
        <AssignBar
          formats={formats.filter((f) => ticked.has(f.id))}
          profiles={profiles}
          onClear={() => setTicked(new Set())}
          onDone={() => {
            setTicked(new Set())
            onAssigned()
          }}
        />
      )}
    <div className="table-scroll">
      <table className="formats-table">
        <thead>
          <tr>
            <th>
              <input type="checkbox" checked={allTicked} aria-label={t('Tick all formats')} onChange={() => setTicked(allTicked ? new Set() : new Set(formats.map((f) => f.id)))} />
            </th>
            <th>{t('Name')}</th>
            <th>{t('For')}</th>
            <th>{t('Conditions')}</th>
            <th style={{ whiteSpace: 'nowrap' }}>{t('Scored by')}</th>
            <th>
              <span className="sr-only">{t('Actions')}</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {formats.map((f) => (
            <tr key={f.id}>
              <td>
                <input type="checkbox" checked={ticked.has(f.id)} aria-label={t('Tick {name}', { name: f.name })} onChange={() => tick(f.id)} />
              </td>
              <td>
                {editing?.id === f.id ? (
                  <form
                    style={{ display: 'flex', gap: 6 }}
                    onSubmit={(e) => {
                      e.preventDefault()
                      void rename()
                    }}
                  >
                    <input autoFocus aria-label={t('New name')} value={editing.name} maxLength={100} onChange={(e) => setEditing({ ...editing, name: e.target.value })} />
                    <button className="primary" type="submit">
                      {t('Save')}
                    </button>
                    <button type="button" onClick={() => setEditing(null)}>
                      {t('Cancel')}
                    </button>
                  </form>
                ) : (
                  <>
                    {f.name} {f.trashId && <span className="badge">TRaSH</span>}
                  </>
                )}
                {f.problems?.map((p) => (
                  <div key={p} className="format-problem">
                    {p}
                  </div>
                ))}
              </td>
              <td>{APP_LABEL[f.app]}</td>
              <td
                className="format-conditions"
                title={f.conditions
                  .map((c) => {
                    const name = c.negate ? t('not {name}', { name: c.name }) : c.name
                    return c.required ? t('{name} (required)', { name }) : name
                  })
                  .join('\n')}
              >
                {plural(f.conditions.length, '{n} condition', '{n} conditions')}
              </td>
              <td style={{ color: 'var(--text-dim)', fontSize: '0.85rem' }}>{f.usedBy.length ? f.usedBy.join(', ') : t('no profile yet')}</td>
              <td style={{ whiteSpace: 'nowrap' }}>
                <button onClick={() => setEditing({ id: f.id, name: f.name })}>{t('Rename')}</button> <button onClick={() => onRemove(f)}>{t('Delete')}</button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
    </>
  )
}

// Gives the ticked formats a score in one or more profiles: the one TRaSH
// gives each, or one typed score for all.
function AssignBar({ formats, profiles, onClear, onDone }: { formats: CustomFormat[]; profiles: QualityProfile[]; onClear: () => void; onDone: () => void }) {
  const toast = useToast()
  const [open, setOpen] = useState(false)
  const [chosen, setChosen] = useState<Set<number>>(new Set())
  const [useTrash, setUseTrash] = useState(true)
  const [score, setScore] = useState('')
  const [replace, setReplace] = useState(false)
  const [busy, setBusy] = useState(false)
  const typedOk = /^-?\d{1,5}$/.test(score.trim())

  async function assign() {
    setBusy(true)
    try {
      const r = await api.assignCustomFormats(formats.map((f) => f.id), [...chosen], useTrash ? { trash: true } : { score: Number(score.trim()) }, replace)
      const text = profileChangesText(r.changes)
      if (r.changes.some((c) => changeCount(c) > 0)) toast.success(text)
      else toast.info(text)
      if (r.unscored.length > 0) toast.info(t("TRaSH gives no score to {names}, so those weren't scored.", { names: r.unscored.join(', ') }))
      onDone()
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="assign-bar">
      <div className="assign-head">
        <strong>{plural(formats.length, '{n} format picked', '{n} formats picked')}</strong>
        <button type="button" className="primary" onClick={() => setOpen(!open)} aria-expanded={open}>
          {t('Add to profiles…')}
        </button>
        <button type="button" onClick={onClear}>
          {t('Clear')}
        </button>
      </div>
      {open && (
        <div className="assign-body">
          <fieldset className="group">
            <legend>{t('Profiles')}</legend>
            <ul className="trash-formats">
              {profiles.map((p) => (
                <li key={p.id}>
                  <label>
                    <input
                      type="checkbox"
                      checked={chosen.has(p.id)}
                      onChange={() =>
                        setChosen((cur) => {
                          const next = new Set(cur)
                          if (next.has(p.id)) next.delete(p.id)
                          else next.add(p.id)
                          return next
                        })
                      }
                    />
                    <span>{p.name}</span>
                  </label>
                </li>
              ))}
            </ul>
          </fieldset>
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
            <button type="button" className="primary" disabled={chosen.size === 0 || busy || (!useTrash && !typedOk)} onClick={() => void assign()}>
              {chosen.size === 0 ? t('Pick the profiles') : plural(chosen.size, 'Score in {n} profile', 'Score in {n} profiles')}
            </button>
          </div>
          <label className="check">
            <input type="checkbox" checked={replace} onChange={(e) => setReplace(e.target.checked)} />
            <span>{t('Replace the score if the profile already has the format')}</span>
          </label>
          <small style={{ color: 'var(--text-dim)' }}>{t('A format a profile already has is skipped and keeps its score, unless you tick the box. A score of 0 takes the format out of the profile.')}</small>
        </div>
      )}
    </div>
  )
}

function AppSwitch({ app, onChange }: { app: App; onChange: (a: App) => void }) {
  return (
    <div className="seg" role="radiogroup" aria-label={t('Movies or TV')}>
      {(['radarr', 'sonarr'] as App[]).map((a) => (
        <button key={a} role="radio" aria-checked={app === a} className={app === a ? 'active' : ''} onClick={() => onChange(a)}>
          {a === 'radarr' ? t('Movies (Radarr)') : t('TV (Sonarr)')}
        </button>
      ))}
    </div>
  )
}

function TrashPanel({ profiles, defaults, onImported }: { profiles: QualityProfile[]; defaults: { id: number; tvId: number }; onImported: () => void }) {
  const toast = useToast()
  const [app, setApp] = useState<App>('radarr')
  const [data, setData] = useState<{ formats: TrashFormat[]; profiles: TrashProfile[] } | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [reload, setReload] = useState(0)
  const [filter, setFilter] = useState('')
  const [picked, setPicked] = useState<Set<string>>(new Set())
  // The profile to score in: the default one for this kind of title until the person picks (or picks none, '').
  const [chosenIn, setScoreIn] = useState<string | null>(null)
  const defaultIn = profileForApp(app, defaults.id, defaults.tvId)
  const scoreIn = chosenIn ?? (profiles.some((p) => p.id === defaultIn) ? String(defaultIn) : '')
  const profileScores = useMemo(() => {
    const p = profiles.find((x) => String(x.id) === scoreIn)
    return p ? new Map((p.formatScores ?? []).map((s) => [s.formatId, s.score])) : null
  }, [profiles, scoreIn])
  const scoredProfile = profiles.find((p) => String(p.id) === scoreIn)
  const [replaceScores, setReplaceScores] = useState(false)
  const pickedFormats = (data?.formats ?? []).filter((f) => picked.has(f.trashId))
  const actions = pickedFormats.map((f) => ({ f, what: importScoreAction(f, profileScores, replaceScores) }))
  const scoredNow = actions.filter((a) => a.what === 'add' || a.what === 'replace')
  const skippedNow = actions.filter((a) => a.what === 'skip').length
  const zeroScored = scoredNow.filter((a) => a.f.defaultScore === 0)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    let live = true
    api
      .trashGuides(app)
      .then((d) => live && setData(d))
      .catch((e) => live && setError(errText(e)))
      .finally(() => live && setLoading(false))
    return () => {
      live = false
    }
  }, [app, reload])

  // Reads the list again (another app, after an import, or after an error).
  function load(a: App) {
    setLoading(true)
    setError('')
    setPicked(new Set())
    if (a !== app) {
      setData(null)
      setApp(a)
    } else setReload((n) => n + 1)
  }

  const shown = useMemo(() => {
    const q = filter.trim().toLowerCase()
    return (data?.formats ?? []).filter((f) => !q || f.name.toLowerCase().includes(q))
  }, [data, filter])

  function toggle(id: string) {
    const next = new Set(picked)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    setPicked(next)
  }

  async function importFormats() {
    setBusy(true)
    try {
      const r = await api.importTrashFormats(app, [...picked], scoreIn ? Number(scoreIn) : undefined, replaceScores)
      const changes = [r.added && t('{n} added', { n: r.added }), r.updated && t('{n} updated', { n: r.updated })].filter(Boolean).join(', ')
      const line = [changes && t('Custom formats {changes}.', { changes }), r.profile && profileChangeText(r.profile)].filter(Boolean).join(' ')
      if (changes || (r.profile && changeCount(r.profile) > 0)) toast.success(line)
      else toast.info(line || t('Nothing was added.'))
      onImported()
      load(app)
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }

  async function importProfile(tp: TrashProfile) {
    setBusy(true)
    try {
      const r = await api.importTrashProfile(app, tp.trashId)
      const vars = { name: r.profile.name, n: r.profile.formatScores?.length ?? 0, missing: r.formats.missing?.length ?? 0 }
      toast.success(
        vars.missing
          ? t('Created the "{name}" profile with {n} scored formats. {missing} of its formats weren\'t found.', vars)
          : t('Created the "{name}" profile with {n} scored formats.', vars),
      )
      onImported()
      load(app)
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div>
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 12, alignItems: 'center', marginBottom: 12 }}>
        <AppSwitch app={app} onChange={load} />
        <small style={{ color: 'var(--text-dim)' }}>
          {tx('Read from the {link} on GitHub. Importing a format again updates it.', {
            link: (
              <a href="https://trash-guides.info/" target="_blank" rel="noreferrer">
                TRaSH Guides
              </a>
            ),
          })}
        </small>
      </div>
      {loading && <p style={{ color: 'var(--text-dim)' }}>{t('Reading the TRaSH Guides… the first time takes a few seconds.')}</p>}
      {error && (
        <p className="error-text">
          {error} <button onClick={() => load(app)}>{t('Try again')}</button>
        </p>
      )}
      {data && (
        <div className="trash-columns">
          <fieldset className="group">
            <legend>{t('Ready-made profiles')}</legend>
            <p style={{ marginTop: 0 }}>{t("Creates a new quality profile with the qualities, formats and scores TRaSH suggests. Your own profiles aren't changed.")}</p>
            <ul className="trash-profiles">
              {data.profiles.map((tp) => (
                <li key={tp.trashId}>
                  <div>
                    <strong>{tp.name}</strong>
                    <div className="trash-meta">
                      {tp.qualities.join(', ')} · {plural(tp.formats, '{n} format', '{n} formats')}
                      {tp.minFormatScore ? ` · ${t('minimum {n}', { n: String(tp.minFormatScore) })}` : ''}
                    </div>
                  </div>
                  <button onClick={() => void importProfile(tp)} disabled={busy}>
                    {t('Create profile')}
                  </button>
                </li>
              ))}
            </ul>
          </fieldset>
          <fieldset className="group">
            <legend>{t('Formats')}</legend>
            <input type="search" placeholder={t('Filter {n} formats', { n: data.formats.length })} aria-label={t('Filter formats')} value={filter} onChange={(e) => setFilter(e.target.value)} style={{ width: '100%', marginBottom: 8 }} />
            <ul className="trash-formats">
              {shown.map((f) => (
                <li key={f.trashId}>
                  <label>
                    <input type="checkbox" checked={picked.has(f.trashId)} onChange={() => toggle(f.trashId)} />
                    <span>{f.name}</span>
                    {(() => {
                      const pv = scorePreview(f, profileScores)
                      return (
                        <>
                          {f.defaultScore !== 0 && <small className="trash-score">{signed(f.defaultScore)}</small>}
                          {pv.flag === 'zero' && (
                            <span className="badge conflict" title={t('This format is already added but scores 0 in {profile}, so it does nothing there. TRaSH scores it {score}. It stays at 0 unless you tick the replace box below.', { profile: scoredProfile?.name ?? '', score: signed(f.defaultScore) })}>
                              {t('0 in {profile}', { profile: scoredProfile?.name ?? '' })}
                            </span>
                          )}
                          {pv.flag === 'differs' && (
                            <span className="badge" title={t('This format is already scored {current} in {profile}. TRaSH scores it {score}. It keeps its score unless you tick the replace box below.', { current: signed(pv.current ?? 0), profile: scoredProfile?.name ?? '', score: signed(f.defaultScore) })}>
                              {t('{score} in {profile}', { score: signed(pv.current ?? 0), profile: scoredProfile?.name ?? '' })}
                            </span>
                          )}
                          {f.importedId > 0 && <span className="badge downloaded">{t('added')}</span>}
                        </>
                      )
                    })()}
                  </label>
                </li>
              ))}
            </ul>
            {picked.size > 0 && (
              <p className="trash-score-note">
                {scoredProfile
                  ? [
                      scoredNow.length > 0 ? plural(scoredNow.length - zeroScored.length, '{n} of the picked formats gets its TRaSH score in {profile}.', '{n} of the picked formats get their TRaSH score in {profile}.', { profile: scoredProfile.name }) : '',
                      zeroScored.length > 0 ? plural(zeroScored.length, "{n} has a TRaSH score of 0, so it is added with 0.", "{n} have a TRaSH score of 0, so they are added with 0.") : '',
                      skippedNow > 0 ? t('Already in {profile} and left as they are: {n}.', { profile: scoredProfile.name, n: skippedNow }) : '',
                    ]
                      .filter(Boolean)
                      .join(' ')
                  : t("Without a profile the picked formats are added without a score. They don't count anywhere until you score them.")}
              </p>
            )}
            <div className="trash-import-bar">
              <label className="field" style={{ minWidth: 220 }}>
                <span>{t("Give them TRaSH's default score in")}</span>
                <Dropdown
                  label={t('Profile to score them in')}
                  value={scoreIn}
                  options={[{ value: '', label: t("Don't score them") }, ...profiles.map((p) => ({ value: String(p.id), label: p.name }))]}
                  onChange={setScoreIn}
                />
              </label>
              {scoredProfile && (
                <label className="check">
                  <input type="checkbox" checked={replaceScores} onChange={(e) => setReplaceScores(e.target.checked)} />
                  <span>{t('Replace the score if the profile already has the format')}</span>
                </label>
              )}
              <button className="primary" disabled={picked.size === 0 || busy} onClick={() => void importFormats()}>
                {picked.size === 0 ? t('Pick formats to add') : plural(picked.size, 'Add {n} format', 'Add {n} formats')}
              </button>
            </div>
          </fieldset>
        </div>
      )}
    </div>
  )
}

function PastePanel({ onImported }: { onImported: () => void }) {
  const toast = useToast()
  const [app, setApp] = useState<App>('radarr')
  const [json, setJson] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit() {
    setBusy(true)
    try {
      const r = await api.importCustomFormats(json, app)
      toast.success([r.added && t('{n} added', { n: r.added }), r.updated && t('{n} updated', { n: r.updated })].filter(Boolean).join(', ') + '.')
      setJson('')
      onImported()
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="grid-form" style={{ maxWidth: 760 }}>
      <AppSwitch app={app} onChange={setApp} />
      <label>
        {t('One format or a list, as JSON')}
        <textarea rows={10} value={json} onChange={(e) => setJson(e.target.value)} placeholder='{"name": "HDR", "specifications": [ … ]}' spellCheck={false} style={{ fontFamily: 'var(--font-mono)', fontSize: '0.8rem' }} />
        <small style={{ color: 'var(--text-dim)' }}>
          {t('Use the Export button in a Radarr or Sonarr custom format, or copy a file from the TRaSH Guides. A format with the same name replaces the one you have.')}
        </small>
      </label>
      <div>
        <button className="primary" disabled={!json.trim() || busy} onClick={() => void submit()}>
          {t('Import')}
        </button>
      </div>
    </div>
  )
}

function TestPanel({ profiles, hasFormats }: { profiles: QualityProfile[]; hasFormats: boolean }) {
  const [title, setTitle] = useState('')
  const [profileId, setProfileId] = useState('')
  const [result, setResult] = useState<FormatTestResult | null>(null)
  const [error, setError] = useState('')

  async function run() {
    setError('')
    try {
      setResult(await api.testCustomFormats(title, profileId ? Number(profileId) : undefined))
    } catch (e) {
      setResult(null)
      setError(errText(e))
    }
  }

  return (
    <div className="grid-form" style={{ maxWidth: 760 }}>
      {!hasFormats && <p style={{ color: 'var(--text-dim)', margin: 0 }}>{t('You have no custom formats yet, so nothing will match.')}</p>}
      <form
        className="format-test-form"
        onSubmit={(e) => {
          e.preventDefault()
          void run()
        }}
      >
        <label style={{ flex: '3 1 320px' }}>
          {t('Release name')}
          <input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Night.Harbour.2021.1080p.BluRay.DDP5.1.x264-GRP" spellCheck={false} />
        </label>
        <label className="field" style={{ flex: '1 1 180px' }}>
          <span>{t('Score with profile')}</span>
          <Dropdown label={t('Profile')} value={profileId} options={[{ value: '', label: t('No profile') }, ...profiles.map((p) => ({ value: String(p.id), label: p.name }))]} onChange={setProfileId} />
        </label>
        <button className="primary" type="submit" disabled={!title.trim()}>
          {t('Test')}
        </button>
      </form>
      {error && <p className="error-text">{error}</p>}
      {result && (
        <div className="format-test-result">
          <p style={{ margin: 0 }}>
            {tx('Quality: {quality}', { quality: <strong>{result.quality}</strong> })}
            {result.score !== undefined && (
              <>
                {' '}
                · {tx('Score: {score}', { score: <strong>{result.score}</strong> })} ·{' '}
                {result.accepted ? <span className="badge downloaded">{t('would be taken')}</span> : <span className="badge conflict">{t('would be skipped')}</span>}
              </>
            )}
          </p>
          {result.reason && <p style={{ margin: '4px 0 0', color: 'var(--text-dim)' }}>{st(result.reason)}</p>}
          {result.matched.length === 0 ? (
            <p style={{ color: 'var(--text-dim)' }}>{t('No custom format matches this name.')}</p>
          ) : (
            <ul className="format-test-matches">
              {result.matched.map((m) => (
                <li key={m.id}>
                  {m.name}
                  {m.score !== undefined && <span className="trash-score">{m.score > 0 ? `+${m.score}` : m.score}</span>}
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  )
}
