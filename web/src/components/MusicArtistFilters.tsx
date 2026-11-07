import { useCallback, useEffect, useRef, useState } from 'react'
import { api, type MusicArtist, type MusicDiscography, type MusicFilterChoices, type MusicHiddenAlbum } from '../api'
import { hiddenSummary, reasonWord, toggleKey, typeName, type MusicFilter } from '../musicFilter'
import { plural, t } from '../i18n'
import Collapsible from './Collapsible'
import Icon from './Icon'
import MusicFilterEditor from './MusicFilterEditor'
import { useToast } from './Toast'
import '../musicFilter.css'

const TYPES = ['album', 'ep', 'single']

// Under an artist's header: the release types to show as chips, a folded
// "More filters" with the rest of the choice (administrators), and a line
// saying what the filter leaves out and why, with a way to see those releases
// for this visit and add one. Nothing is hidden without being counted.
export default function MusicArtistFilters({
  artist,
  admin,
  canManage,
  onArtist,
  onChanged,
  onLibraryHidden,
  leftOut,
  showLeftOut,
  onShowLeftOut,
}: {
  artist: MusicArtist
  admin: boolean
  canManage: boolean
  onArtist: (a: MusicArtist) => void
  onChanged: () => void
  // Told which albums of the library the filter leaves out of the list.
  onLibraryHidden: (list: MusicHiddenAlbum[]) => void
  // How many albums the list leaves out now, and whether they are shown anyway.
  leftOut: number
  showLeftOut: boolean
  onShowLeftOut: (show: boolean) => void
}) {
  const toast = useToast()
  const [disco, setDisco] = useState<MusicDiscography | null>(null)
  const [choices, setChoices] = useState<MusicFilterChoices | null>(null)
  const [show, setShow] = useState(false)
  const [saving, setSaving] = useState(false)
  const [adding, setAdding] = useState('')
  const [refreshing, setRefreshing] = useState(false)
  const timer = useRef<number | undefined>(undefined)
  const asked = useRef(0)

  const loadDisco = useCallback(() => {
    window.clearTimeout(timer.current)
    const mine = ++asked.current
    setRefreshing(true)
    api
      .artistDiscography(artist.id)
      .then((d) => {
        if (mine !== asked.current) return // a newer answer is on its way
        setRefreshing(false)
        setDisco(d)
        onLibraryHidden(d.library ?? [])
        // Releases still being checked: ask again in a few seconds until they are known.
        if (d.checking > 0) timer.current = window.setTimeout(loadDisco, 4000)
      })
      .catch(() => {
        if (mine === asked.current) setRefreshing(false)
      })
  }, [artist.id, onLibraryHidden])
  useEffect(() => {
    loadDisco()
    return () => window.clearTimeout(timer.current)
  }, [loadDisco])
  useEffect(() => {
    if (admin) api.musicFilters().then(setChoices).catch(() => undefined)
  }, [admin])

  const filter: MusicFilter | undefined = artist.filter ? { types: artist.filter.types, hideSecondary: artist.filter.hideSecondary, hideBootlegs: artist.filter.hideBootlegs } : undefined

  async function save(changes: { filter?: MusicFilter; useLibraryFilter?: boolean }) {
    setSaving(true)
    try {
      onArtist(await api.updateArtist(artist.id, changes))
      loadDisco()
      // A followed artist adds what the new choice lets in in the background.
      window.setTimeout(() => {
        loadDisco()
        onChanged()
      }, 4000)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e))
    } finally {
      setSaving(false)
    }
  }

  async function add(mbid: string, title: string) {
    setAdding(mbid)
    try {
      await api.addArtistAlbum(artist.id, mbid)
      toast.success(t('{title} added to your library.', { title }))
      setDisco((d) => (d ? { ...d, hidden: d.hidden.filter((g) => g.mbid !== mbid), counts: dropCount(d, mbid) } : d))
      onChanged()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e))
    } finally {
      setAdding('')
    }
  }

  const summary = disco ? hiddenSummary(disco.counts) : ''
  const checking = disco?.checking ?? 0
  if (!admin && !summary && checking === 0 && leftOut === 0) return null
  return (
    <section className="music-artist-filters" aria-label={t('What this page shows')}>
      {admin && filter && (
        <div className="music-filter-chips">
          <span className="music-filter-label">{t('Show:')}</span>
          {TYPES.map((k) => {
            const on = filter.types.includes(k)
            return (
              <button
                key={k}
                type="button"
                className={`filter-chip${on ? ' on' : ''}`}
                aria-pressed={on}
                disabled={saving || (on && filter.types.length === 1)}
                onClick={() => void save({ filter: { ...filter, types: toggleKey(filter.types, k, TYPES) } })}
              >
                {on && <Icon name="check" size={13} />} {typeName(k)}
              </button>
            )
          })}
          <span className="music-filter-source">{artist.filter?.useLibrary ? t('Using the library setting') : t("This artist's own choice")}</span>
        </div>
      )}
      {admin && filter && choices && (
        <Collapsible id="music-artist-more-filters" label={t('More filters')} showLabel>
          <MusicFilterEditor idPrefix={`artist-${artist.id}`} value={filter} choices={choices} disabled={saving} onChange={(f) => void save({ filter: f })} />
          <div className="music-filter-presets">
            <button type="button" className="btn-sm" disabled={saving || artist.filter?.useLibrary} onClick={() => void save({ useLibraryFilter: true })}>
              {t('Use the library setting')}
            </button>
          </div>
        </Collapsible>
      )}
      {(summary || checking > 0 || leftOut > 0 || refreshing) && (
        <div className="music-hidden-line">
          {summary && (
            <>
              <span>{summary}</span>
              <button type="button" className="btn-sm" aria-expanded={show} onClick={() => setShow(!show)}>
                {show ? t('Hide them again') : t('Show them')}
              </button>
            </>
          )}
          {leftOut > 0 && (
            <>
              <span>{plural(leftOut, '{n} in your library is hidden by this filter', '{n} in your library are hidden by this filter')}</span>
              <button type="button" className="btn-sm" aria-expanded={showLeftOut} onClick={() => onShowLeftOut(!showLeftOut)}>
                {showLeftOut ? t('Hide them again') : t('Show them')}
              </button>
            </>
          )}
          {refreshing && <span className="music-checking">{t('Updating the list…')}</span>}
          {checking > 0 && <span className="music-checking">{plural(checking, 'Checking {n} release…', 'Checking {n} releases…')}</span>}
        </div>
      )}
      {show && disco && disco.hidden.length > 0 && (
        <ul className="music-hidden-list">
          {disco.hidden.map((g) => (
            <li key={g.mbid}>
              <span className="music-hidden-title">{g.title}</span>
              <span className="music-hidden-meta">
                {g.releaseDate.slice(0, 4)} · {reasonWord(g.hiddenBy)}
              </span>
              {canManage && (
                <button type="button" className="btn-sm" disabled={adding === g.mbid} onClick={() => void add(g.mbid, g.title)}>
                  {t('Add to library')}
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}

function dropCount(d: MusicDiscography, mbid: string): Record<string, number> {
  const g = d.hidden.find((x) => x.mbid === mbid)
  if (!g) return d.counts
  const next = { ...d.counts, [g.hiddenBy]: (d.counts[g.hiddenBy] ?? 1) - 1 }
  if (next[g.hiddenBy] <= 0) delete next[g.hiddenBy]
  return next
}
