import { t } from './i18n.ts'

/** showStatusLabel is where a show stands, in the screen language. */
export function showStatusLabel(status?: string): string {
  switch (status) {
    case 'ended':
      return t('Ended')
    case 'canceled':
      return t('Canceled')
    case 'continuing':
      return t('Continuing')
    case 'upcoming':
      return t('Upcoming')
    default:
      return ''
  }
}

/** isFinishedShow is a show with no more episodes coming (ended or canceled). */
export function isFinishedShow(status?: string): boolean {
  return status === 'ended' || status === 'canceled'
}

/** isRunningShow is a show that still has episodes coming, or soon will. */
export function isRunningShow(status?: string): boolean {
  return status === 'continuing' || status === 'upcoming'
}

/** showRibbon is the corner label on a finished show's poster, none for the others. */
export function showRibbon(status?: string): { label: string; tone: string } | undefined {
  return isFinishedShow(status) ? { label: showStatusLabel(status), tone: 'ended' } : undefined
}
