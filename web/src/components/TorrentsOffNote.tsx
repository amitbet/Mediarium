import { useState } from 'react'
import { t } from '../i18n'
import { showTorrentsOffHint } from '../torrentsOff'
import { useTorrentsSwitch } from '../useTorrentsSwitch'

// The line that tells a person why a torrent can't be downloaded, with the
// button that fixes it for an administrator and the name of who to ask for
// everyone else.
//
// As a release list's header ("results") it shows while the list has a torrent
// and torrents are off. On a download that failed ("failed") the caller decides
// when it applies. Once torrents are switched on here, it says so and what to
// press next instead of just vanishing.
export default function TorrentsOffNote({
  results,
  failed = false,
  className,
}: {
  results?: readonly { protocol?: string }[]
  failed?: boolean
  className?: string
}) {
  const { admin, torrentsOn, switchOn } = useTorrentsSwitch()
  const [busy, setBusy] = useState(false)
  const [switched, setSwitched] = useState(false)
  const applies = failed ? !torrentsOn : showTorrentsOffHint(torrentsOn, results ?? [])
  if (!applies && !switched) return null
  const cls = `notice notice-warn torrents-off-note${className ? ` ${className}` : ''}`
  if (switched) {
    return (
      <div className={cls} role="status">
        <span className="torrents-off-text">{failed ? t('Torrents are switched on. Press Retry.') : t('Torrents are switched on. Press Grab again.')}</span>
      </div>
    )
  }
  async function press() {
    setBusy(true)
    try {
      if (await switchOn()) setSwitched(true)
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className={cls} role="status">
      <span className="torrents-off-text">{failed ? t("Torrents are switched off, so this download can't start.") : t("Torrents are switched off, so torrent releases can't be downloaded.")}</span>
      {admin ? (
        <button type="button" className="btn-sm btn-primary" disabled={busy} onClick={() => void press()}>
          {busy ? t('Working…') : t('Switch on torrents')}
        </button>
      ) : (
        <span className="torrents-off-ask">{t('Ask an administrator to switch torrents on.')}</span>
      )}
    </div>
  )
}
