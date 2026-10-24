import { formatBytes } from './format.ts'
import { t } from './i18n.ts'

// What a refused grab says about the download that is in the way ("Already
// downloading: ..."): enough to name it and offer to cancel it.
export interface BlockingDownload {
  id: number
  title: string
  progress: number
  sizeBytes: number
  paused: boolean
}

// The download in the way, if the error is a grab refused for that reason.
export function blockingOf(e: unknown): BlockingDownload | null {
  if (typeof e !== 'object' || e === null) return null
  const b = (e as { status?: unknown; blocking?: Partial<BlockingDownload> }).blocking
  if ((e as { status?: unknown }).status !== 409 || !b || typeof b.id !== 'number' || b.id <= 0 || typeof b.title !== 'string') return null
  return {
    id: b.id,
    title: b.title,
    progress: typeof b.progress === 'number' && b.progress > 0 ? Math.min(b.progress, 100) : 0,
    sizeBytes: typeof b.sizeBytes === 'number' && b.sizeBytes > 0 ? b.sizeBytes : 0,
    paused: b.paused === true,
  }
}

// How far the download has come, for the line under the message.
export function blockingDetail(b: BlockingDownload): string {
  const percent = Math.round(b.progress)
  const size = b.sizeBytes > 0 ? formatBytes(b.sizeBytes) : ''
  if (b.paused) return size ? t('Paused at {percent}% of {size}', { percent, size }) : t('Paused at {percent}%', { percent })
  return size ? t('{percent}% of {size} done', { percent, size }) : t('{percent}% done', { percent })
}
