import { useState } from 'react'
import { blockingDetail, type BlockingDownload } from '../grabBlocked'
import { t } from '../i18n'

// Shown when a grab was refused because another download for the same title
// is running: what is in the way, and a button that cancels it and grabs the
// release that was picked, with one question first.
export default function GrabBlocked({
  message,
  blocking,
  busy,
  onReplace,
  onDismiss,
}: {
  message: string
  blocking: BlockingDownload
  busy: boolean
  onReplace: () => void
  onDismiss: () => void
}) {
  const [asking, setAsking] = useState(false)
  return (
    <div className="grab-blocked" role="alert">
      <p className="error-text">{message}</p>
      <p className="field-hint">{blockingDetail(blocking)}</p>
      {asking ? (
        <div className="grab-blocked-actions">
          <span>{t('Cancel the running download and grab this release instead?')}</span>
          <button type="button" className="btn-sm btn-danger-solid" disabled={busy} onClick={onReplace}>
            {busy ? t('Working…') : t('Yes, cancel it and grab')}
          </button>
          <button type="button" className="btn-sm" disabled={busy} onClick={() => setAsking(false)}>
            {t('No, keep it')}
          </button>
        </div>
      ) : (
        <div className="grab-blocked-actions">
          <button type="button" className="btn-sm btn-primary" disabled={busy} onClick={() => setAsking(true)}>
            {t('Cancel it and grab this one')}
          </button>
          <button type="button" className="btn-sm" onClick={onDismiss}>
            {t('Dismiss')}
          </button>
        </div>
      )}
    </div>
  )
}
