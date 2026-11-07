import { useEffect, useState } from 'react'
import { t } from '../i18n'
import Icon from './Icon'
import { showBackToTop } from './scrollTopButton'

// A small button, bottom right, that takes a long page back to the top (where
// the filters and the page's own controls are). It shows once the page has
// been scrolled about a screen and never on a short page.
export default function BackToTop({ target }: { target: React.RefObject<HTMLElement | null> }) {
  const [shown, setShown] = useState(false)
  useEffect(() => {
    const el = target.current
    if (!el) return
    let frame = 0
    let now = false
    const measure = () => {
      frame = 0
      const next = showBackToTop(el.scrollHeight, el.clientHeight, el.scrollTop, now)
      if (next !== now) {
        now = next
        setShown(next)
      }
    }
    const queue = () => {
      if (!frame) frame = requestAnimationFrame(measure)
    }
    measure()
    el.addEventListener('scroll', queue, { passive: true })
    window.addEventListener('resize', queue)
    // A page that is still filling in grows without scrolling.
    const size = new ResizeObserver(queue)
    size.observe(el)
    return () => {
      el.removeEventListener('scroll', queue)
      window.removeEventListener('resize', queue)
      size.disconnect()
      if (frame) cancelAnimationFrame(frame)
    }
  }, [target])
  const up = () => {
    const calm = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    target.current?.scrollTo({ top: 0, behavior: calm ? 'auto' : 'smooth' })
  }
  return (
    <button type="button" className={`to-top${shown ? ' shown' : ''}`} onClick={up} aria-label={t('Back to top')} title={t('Back to top')} tabIndex={shown ? 0 : -1} aria-hidden={shown ? undefined : true}>
      <Icon name="chevron-up" size={20} />
    </button>
  )
}
