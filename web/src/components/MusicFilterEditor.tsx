import type { MusicFilterChoices } from '../api'
import { secondaryName, sameFilter, toggleKey, typeName, type MusicFilter } from '../musicFilter'
import { t } from '../i18n'
import Switch from './Switch'
import '../musicFilter.css'

const TYPES = ['album', 'ep', 'single']

// The choices for what the music pages list: release types, secondary types to
// hide, and the bootleg test. Used in Settings (the whole library) and on an
// artist's page (that artist alone). It changes nothing by itself: the caller
// gets the new choice and decides when to save it.
export default function MusicFilterEditor({
  value,
  choices,
  onChange,
  disabled,
  idPrefix,
}: {
  value: MusicFilter
  choices: MusicFilterChoices
  onChange: (next: MusicFilter) => void
  disabled?: boolean
  idPrefix: string
}) {
  const secondary = choices.secondaryTypes.map((s) => s.key)
  const studio = sameFilter(value, choices.studio)
  const defaults = sameFilter(value, choices.defaults)
  return (
    <div className="music-filter-editor">
      <fieldset className="music-filter-group" disabled={disabled}>
        <legend>{t('Release types to show')}</legend>
        <div className="music-filter-checks">
          {TYPES.map((k) => (
            <label key={k} className="check-row">
              <input
                type="checkbox"
                id={`${idPrefix}-type-${k}`}
                checked={value.types.includes(k)}
                disabled={value.types.length === 1 && value.types.includes(k)}
                onChange={() => onChange({ ...value, types: toggleKey(value.types, k, TYPES) })}
              />
              <span>{typeName(k)}</span>
            </label>
          ))}
        </div>
      </fieldset>
      <fieldset className="music-filter-group" disabled={disabled}>
        <legend>{t('Kinds to hide')}</legend>
        <div className="music-filter-checks music-filter-wide">
          {secondary.map((k) => (
            <label key={k} className="check-row">
              <input
                type="checkbox"
                id={`${idPrefix}-hide-${k}`}
                checked={value.hideSecondary.includes(k)}
                onChange={() => onChange({ ...value, hideSecondary: toggleKey(value.hideSecondary, k, secondary) })}
              />
              <span>{secondaryName(k)}</span>
            </label>
          ))}
        </div>
      </fieldset>
      <Switch
        checked={value.hideBootlegs}
        disabled={disabled}
        onChange={(v) => onChange({ ...value, hideBootlegs: v })}
        label={t('Hide bootleg-only releases')}
        description={t("Leaves out releases that MusicBrainz only lists as bootlegs, promotions or pseudo-releases. Each release is checked once, one request a second, and the answer is kept. Until it is checked, a release stays visible.")}
      />
      <div className="music-filter-presets">
        <button type="button" className="btn-sm" disabled={disabled || studio} onClick={() => onChange(choices.studio)}>
          {t('Official studio releases only')}
        </button>
        <button type="button" className="btn-sm" disabled={disabled || defaults} onClick={() => onChange(choices.defaults)}>
          {t('Back to the defaults')}
        </button>
      </div>
    </div>
  )
}

