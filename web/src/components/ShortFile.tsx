import type { ShortFileNote } from '../api'
import { t } from '../i18n'
import { shortFileText, shortFileTip } from '../shortFile'
import Icon from './Icon'

// A plain warning that the file is far shorter than the movie or episode
// should be ("Only 1 min long"), with a button for an administrator to delete
// it and search again. Nothing is drawn for a file that looks fine.
export default function ShortFile({ note, onRedo, busy = false, compact = false }: { note: ShortFileNote | null | undefined; onRedo?: () => void; busy?: boolean; compact?: boolean }) {
  if (!note) return null
  return (
    <div className={`short-file${compact ? ' compact' : ''}`}>
      <span className="file-chip file-chip-short" title={shortFileTip(note)}>
        <Icon name="warning" size={compact ? 12 : 14} /> {shortFileText(note)}
      </span>
      {onRedo && (
        <button type="button" className="btn-sm danger-ghost" disabled={busy} onClick={onRedo}>
          {t('Delete this file and search again')}
        </button>
      )}
    </div>
  )
}
