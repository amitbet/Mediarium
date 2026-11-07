import { useCallback } from 'react'
import { api, isAdmin } from './api'
import { useAuth } from './AuthContext'
import { useToast } from './components/Toast'
import { t } from './i18n'
import { useModules } from './ModulesContext'
import { isTorrentsOff, switchOnChanges } from './torrentsOff'

// The pieces of "torrents are switched off": whether they are, whether this
// person can switch them on, the switching itself and the toast for a grab
// that was refused for that reason.
export function useTorrentsSwitch() {
  const toast = useToast()
  const admin = isAdmin(useAuth().user)
  const { torrentsOn, refresh } = useModules()

  // Switches torrents on the way Settings > Downloading > Torrents does, then
  // refreshes what every page knows. Returns false when it could not be saved.
  const switchOn = useCallback(async (): Promise<boolean> => {
    try {
      const current = await api.getSettings().catch(() => null)
      await api.putSettings(switchOnChanges(current?.defaultSources))
      await refresh()
      return true
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e))
      return false
    }
  }, [refresh, toast])

  // Shows why a grab failed. When it is because torrents are off, an
  // administrator gets a button that switches them on (and, once they are on,
  // one that grabs the same release again); anyone else is told who can.
  const grabError = useCallback(
    (e: unknown, retry?: () => void) => {
      const message = e instanceof Error ? e.message : String(e)
      if (!isTorrentsOff(e)) {
        toast.error(message)
        return
      }
      if (!admin) {
        toast.error(`${message} ${t('Ask an administrator to switch torrents on.')}`)
        return
      }
      toast.error(message, {
        label: t('Switch on torrents'),
        onClick: () => {
          void switchOn().then((ok) => {
            if (ok) toast.success(t('Torrents are switched on.'), retry ? { label: t('Grab now'), onClick: retry } : undefined)
          })
        },
      })
    },
    [admin, switchOn, toast],
  )

  return { admin, torrentsOn, switchOn, grabError }
}
