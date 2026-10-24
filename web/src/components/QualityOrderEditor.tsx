import { useRef, useState } from 'react'
import Icon from './Icon'
import { plural, t } from '../i18n'
import {
  MAX_GROUP_NAME,
  defaultGroupName,
  groupRows,
  isGroup,
  moveRow,
  renameGroup,
  rowLabel,
  shiftRow,
  splitOut,
  toggleRow,
  trashLayout,
  ungroupRow,
  webAndTvTogether,
  type OrderRow,
} from '../qualityOrder'

const tierLabel = (tier: string) => (tier === 'Unknown' ? t('Unknown (untagged releases)') : tier)

// The list of qualities of a profile, best first: tick the ones to accept, drag
// a row (or use its arrows) to change its rank, and pick several to put them
// in one group that counts as a single rank. Touch and keyboard work through
// the arrow buttons and the arrow keys on a row's handle.
export default function QualityOrderEditor({
  ladder,
  rows,
  onChange,
  onReset,
}: {
  // Every quality, worst first.
  ladder: string[]
  rows: OrderRow[]
  onChange: (rows: OrderRow[]) => void
  onReset: () => void
}) {
  const [picked, setPicked] = useState<string[]>([])
  const [dragging, setDragging] = useState<string | null>(null)
  const [said, setSaid] = useState('')
  const grips = useRef(new Map<string, HTMLButtonElement>())
  const live = picked.filter((k) => rows.some((r) => r.key === k && r.allowed))
  const acceptedCount = rows.filter((r) => r.allowed).length

  function announce(next: OrderRow[], key: string) {
    const row = next.find((r) => r.key === key)
    if (!row) return
    setSaid(t('{name} is now number {n} of {total}.', { name: rowLabel(row, tierLabel), n: next.indexOf(row) + 1, total: next.filter((r) => r.allowed).length }))
  }

  function move(key: string, delta: -1 | 1) {
    const next = shiftRow(rows, key, delta)
    onChange(next)
    announce(next, key)
    requestAnimationFrame(() => grips.current.get(key)?.focus())
  }

  function groupPicked() {
    const name = defaultGroupName(rows, rows.filter((r) => live.includes(r.key)).flatMap((r) => r.tiers), (n) => t('Group {n}', { n }))
    const g = groupRows(ladder, rows, live, name)
    if (!g) return
    onChange(g.rows)
    setPicked([])
    setSaid(t('Grouped as {name}. Give the group a name of your own if you like.', { name }))
    requestAnimationFrame(() => document.getElementById(`qo-name-${g.key}`)?.focus())
  }

  return (
    <div className="qo">
      <p className="qo-help">
        {t('The best quality is at the top. Drag a row, or use its arrows, to change its place. Qualities in a group count as one rank: none is an upgrade over another, and the usual order settles which is picked between them.')}
      </p>
      <div className="qo-tools">
        <button type="button" className="btn-sm" onClick={groupPicked} disabled={live.length < 2}>
          {live.length >= 2 ? t('Group selected ({n})', { n: live.length }) : t('Group selected')}
        </button>
        <button type="button" className="btn-sm" onClick={() => onChange(trashLayout(ladder, rows))} title={t('The usual order, with the web qualities named WEB 2160p, WEB 1080p and so on, as in the TRaSH guides.')}>
          {t('Use TRaSH layout')}
        </button>
        <button type="button" className="btn-sm" onClick={() => onChange(webAndTvTogether(ladder, rows, (res) => t('Web and TV {res}', { res })))} title={t('Puts the web and TV qualities of each resolution in one group, so a TV copy is not an upgrade over a web copy.')}>
          {t('Treat web and TV as equal')}
        </button>
        <button type="button" className="btn-sm" onClick={onReset}>
          {t('Reset to default order')}
        </button>
      </div>
      <ol className="qo-list">
        {rows.map((row, i) => {
          const group = isGroup(row)
          const place = rows.slice(0, i + 1).filter((r) => r.allowed).length
          const last = i === acceptedCount - 1
          const label = rowLabel(row, tierLabel)
          return (
            <li
              key={row.key}
              className={`qo-row${row.allowed ? ' on' : ' off'}${group ? ' group' : ''}${dragging === row.key ? ' dragging' : ''}`}
              onDragOver={(e) => {
                if (dragging === null || dragging === row.key) return
                e.preventDefault()
                onChange(moveRow(rows, dragging, i))
              }}
              onDrop={(e) => e.preventDefault()}
            >
              <button
                type="button"
                className="qo-grip"
                ref={(el) => {
                  if (el) grips.current.set(row.key, el)
                  else grips.current.delete(row.key)
                }}
                draggable={row.allowed}
                disabled={!row.allowed}
                aria-label={t('Move {name}. Use the up and down arrow keys.', { name: label })}
                onDragStart={(e) => {
                  setDragging(row.key)
                  e.dataTransfer.effectAllowed = 'move'
                  const li = e.currentTarget.closest('li')
                  if (li) e.dataTransfer.setDragImage(li, 12, 12)
                }}
                onDragEnd={() => setDragging(null)}
                onKeyDown={(e) => {
                  if (e.key === 'ArrowUp' || e.key === 'ArrowDown') {
                    e.preventDefault()
                    move(row.key, e.key === 'ArrowUp' ? -1 : 1)
                  }
                }}
              >
                <Icon name="menu" size={15} />
              </button>
              <span className="qo-step" aria-hidden="true">{row.allowed ? place : ''}</span>
              <input
                type="checkbox"
                checked={row.allowed}
                onChange={() => {
                  onChange(toggleRow(rows, row.key))
                  setPicked((p) => p.filter((k) => k !== row.key))
                }}
                aria-label={t('Accept {name}', { name: label })}
              />
              <span className="qo-name">
                {group ? (
                  <>
                    <input
                      id={`qo-name-${row.key}`}
                      className="qo-group-name"
                      value={row.name}
                      maxLength={MAX_GROUP_NAME}
                      aria-label={t('Group name')}
                      placeholder={t('Group name')}
                      onChange={(e) => onChange(renameGroup(rows, row.key, e.target.value))}
                    />
                    <span className="qo-members">
                      {row.tiers.map((tier) => (
                        <span key={tier} className="qo-chip">
                          {tierLabel(tier)}
                          {row.tiers.length > 1 && (
                            <button type="button" className="qo-chip-x" aria-label={t('Take {name} out of the group', { name: tierLabel(tier) })} title={t('Take {name} out of the group', { name: tierLabel(tier) })} onClick={() => onChange(splitOut(rows, row.key, tier))}>
                              <Icon name="x" size={11} />
                            </button>
                          )}
                        </span>
                      ))}
                    </span>
                  </>
                ) : (
                  <strong>{label}</strong>
                )}
              </span>
              <span className="qo-actions">
                {group && (
                  <button type="button" className="btn-sm" onClick={() => onChange(ungroupRow(rows, row.key))}>
                    {t('Ungroup')}
                  </button>
                )}
                {row.allowed && (
                  <label className="qo-pick" title={t('Pick this row to group it with others')}>
                    <input
                      type="checkbox"
                      checked={picked.includes(row.key)}
                      onChange={(e) => setPicked((p) => (e.target.checked ? [...p, row.key] : p.filter((k) => k !== row.key)))}
                      aria-label={t('Pick {name} to group it with others', { name: label })}
                    />
                    <span>{t('Pick')}</span>
                  </label>
                )}
                <span className="qo-arrows">
                  <button type="button" className="icon-btn" onClick={() => move(row.key, -1)} disabled={!row.allowed || i === 0} aria-label={t('Move {name} up', { name: label })}>
                    <Icon name="chevron-up" size={15} />
                  </button>
                  <button type="button" className="icon-btn" onClick={() => move(row.key, 1)} disabled={!row.allowed || last} aria-label={t('Move {name} down', { name: label })}>
                    <Icon name="chevron-down" size={15} />
                  </button>
                </span>
              </span>
            </li>
          )
        })}
      </ol>
      <p className="qo-count">{plural(acceptedCount, '{n} rank in use', '{n} ranks in use')}</p>
      <div className="sr-only" role="status" aria-live="polite">{said}</div>
    </div>
  )
}
