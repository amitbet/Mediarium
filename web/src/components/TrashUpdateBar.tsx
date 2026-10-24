import { useCallback, useEffect, useState } from 'react'
import { api, type TrashCheck, type TrashFormatChange, type TrashUpdateStatus } from '../api'
import { timeAgo } from '../format'
import { locale, plural, st, t } from '../i18n'
import { signed } from '../trashScores'
import { useAutosaveSetting } from '../useAutosave'
import { useConfirm } from './ConfirmProvider'
import Dropdown from './Dropdown'
import Switch from './Switch'
import { useToast } from './Toast'

const errText = (e: unknown) => (e instanceof Error ? e.message : String(e))

// What one changed format is told as: "conditions updated", "TRaSH score 500 to 600".
function changeNotes(c: TrashFormatChange): string {
  const notes: string[] = []
  if (c.conditions) notes.push(t('conditions updated'))
  if (c.scoreChanged) {
    notes.push(t('TRaSH score {from} to {to}', { from: signed(c.scoreFrom), to: signed(c.scoreTo) }))
    if (c.profiles.length > 0) notes.push(t('changes in {profiles}', { profiles: c.profiles.join(', ') }))
  }
  return notes.join(', ')
}

// What the last check found, in one sentence: "Checked the TRaSH Guides: 3 formats
// updated, 12 already up to date, 2 profile scores changed."
function checkSummary(c: TrashCheck): string {
  if (c.error) return t("Couldn't check the TRaSH Guides: {error}", { error: st(c.error) })
  if (c.updated === 0) return t('Checked the TRaSH Guides: nothing to update. {upToDate}.', { upToDate: plural(c.upToDate, '{n} format is up to date', '{n} formats are up to date') })
  const parts = [plural(c.updated, '{n} format updated', '{n} formats updated'), t('{n} already up to date', { n: c.upToDate })]
  if (c.scores > 0) parts.push(t('{scores} in {profiles}', { scores: plural(c.scores, '{n} score changed', '{n} scores changed'), profiles: plural(c.profiles, '{n} profile', '{n} profiles') }))
  return t('Checked the TRaSH Guides: {parts}.', { parts: parts.join(', ') })
}

// "Next check: about Tue 21 Oct, 14:00", or "within the hour" when it is due.
function nextText(iso: string): string {
  const at = new Date(iso)
  if (Number.isNaN(at.getTime())) return ''
  if (at.getTime() <= Date.now()) return t('Next check: within the hour')
  return t('Next check: about {when}', { when: at.toLocaleString(locale(), { weekday: 'short', day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' }) })
}

// Keeps the custom formats that came from the TRaSH Guides up to date: a
// switch for the daily or weekly update, a button that shows what would change
// before it changes it, and what the last check found.
export default function TrashUpdateBar({ onUpdated }: { onUpdated: () => void }) {
  const toast = useToast()
  const confirm = useConfirm()
  const [busy, setBusy] = useState(false)
  const [status, setStatus] = useState<TrashUpdateStatus | null>(null)
  const loadStatus = useCallback(() => {
    api.trashUpdateStatus().then(setStatus).catch(() => undefined)
  }, [])
  useEffect(loadStatus, [loadStatus])
  const auto = useAutosaveSetting<boolean>(
    (s) => s.trashFormatsAuto ?? false,
    (v) => ({ trashFormatsAuto: v }),
    (saved) => (saved ? t('Saved: the formats from the TRaSH Guides are kept up to date.') : t("Saved: the formats from the TRaSH Guides are only updated when you press the button.")),
    false,
  )
  const every = useAutosaveSetting<number>(
    (s) => s.trashFormatsEveryDays ?? 1,
    (v) => ({ trashFormatsEveryDays: v }),
    (saved) => (saved === 7 ? t('Saved: the TRaSH Guides are checked every week.') : t('Saved: the TRaSH Guides are checked every day.')),
    1,
  )
  // The next check moves when the switch or the interval changes.
  useEffect(() => {
    if (auto.loaded && !auto.saving && !every.saving) loadStatus()
  }, [auto.value, auto.loaded, auto.saving, every.value, every.saving, loadStatus])

  async function update() {
    setBusy(true)
    try {
      const preview = await api.trashUpdatePreview()
      if (preview.changes.length === 0) {
        // Nothing to change: this still counts as a check, so it is written to the log and the Activity page.
        await api.trashUpdate()
        return
      }
      const body = (
        <>
          <p>{plural(preview.changes.length, '{n} format changed:', '{n} formats changed:')}</p>
          <ul>
            {preview.changes.map((c) => (
              <li key={c.id}>
                <strong>{c.name}</strong>: {changeNotes(c)}
              </li>
            ))}
          </ul>
          <p>{t("Scores you changed yourself stay as they are. Formats you pasted or made yourself aren't touched.")}</p>
        </>
      )
      if (!(await confirm({ title: t('Update from the TRaSH Guides?'), body, confirmLabel: t('Update') }))) return
      await api.trashUpdate()
      onUpdated()
    } catch (e) {
      toast.error(errText(e))
    } finally {
      loadStatus()
      setBusy(false)
    }
  }

  const last = status?.last
  return (
    <div className="trash-update">
      <div className="trash-update-bar">
        <Switch
          checked={auto.value}
          onChange={(v) => void auto.change(v)}
          disabled={!auto.loaded || auto.saving}
          label={t('Keep TRaSH formats up to date')}
          description={t('Mediarium reads the TRaSH Guides again every day or every week, as you choose, and updates the formats that came from them, and the scores that are still the ones TRaSH gave. Formats you pasted or made yourself are never changed.')}
          showState
        />
        <div className="trash-update-actions">
          {auto.value && (
            <label className="field trash-update-every">
              <span>{t('Check')}</span>
              <Dropdown
                label={t('How often to check the TRaSH Guides')}
                value={String(every.value)}
                options={[
                  { value: '1', label: t('Every day') },
                  { value: '7', label: t('Every week') },
                ]}
                disabled={!every.loaded || every.saving}
                onChange={(v) => void every.change(Number(v))}
              />
            </label>
          )}
          <button type="button" onClick={() => void update()} disabled={busy}>
            {busy ? t('Checking…') : t('Update from TRaSH')}
          </button>
        </div>
      </div>
      {status && (
        <p className="trash-update-when">
          {last ? t('Last checked {when}', { when: timeAgo(last.at) }) : t('Not checked yet.')}
          {auto.value && status.next && <> · {nextText(status.next)}</>}
        </p>
      )}
      {last && (
        <div className={last.error ? 'trash-update-result error' : 'trash-update-result'} role="status">
          <p>{checkSummary(last)}</p>
          {!last.error && last.names.length > 0 && <p className="trash-update-names">{t('Updated: {names}', { names: last.names.join(', ') })}</p>}
        </div>
      )}
    </div>
  )
}
