import { useEffect, useState } from 'react'
import { api, type MusicFilterChoices } from '../api'
import { useModules } from '../ModulesContext'
import type { MusicFilter } from '../musicFilter'
import { t } from '../i18n'
import Icon from './Icon'
import MusicFilterEditor from './MusicFilterEditor'
import { useToast } from './Toast'

// Settings for the whole music library: which releases the artist pages list
// and following an artist adds. Every change is saved as it is made. Albums
// already in the library are never removed by a change here.
export default function MusicFiltersCard() {
  const on = useModules().on('music')
  const toast = useToast()
  const [choices, setChoices] = useState<MusicFilterChoices | null>(null)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!on) return
    api
      .musicFilters()
      .then(setChoices)
      .catch((e) => setError(e instanceof Error ? e.message : String(e)))
  }, [on])

  async function save(next: MusicFilter) {
    if (!choices) return
    const before = choices
    setChoices({ ...choices, filter: next })
    setSaving(true)
    try {
      setChoices(await api.setMusicFilters(next))
      toast.success(t('Saved.'))
    } catch (e) {
      setChoices(before)
      toast.error(e instanceof Error ? e.message : String(e))
    } finally {
      setSaving(false)
    }
  }

  if (!on) return null
  return (
    <section className="card span-all music-filters-card">
      <h2>
        <Icon name="music" size={18} /> {t('Music: what to show')}
      </h2>
      <p className="music-filters-intro">
        {t("Which releases an artist's page offers and which ones following an artist adds. Albums already in your library always stay, and each artist can have its own choice on its page.")}
      </p>
      {error && <p className="error-text">{error}</p>}
      {!choices && !error && <div className="skeleton" style={{ height: 160 }} />}
      {choices && <MusicFilterEditor idPrefix="lib" value={choices.filter} choices={choices} onChange={(f) => void save(f)} disabled={saving} />}
    </section>
  )
}
