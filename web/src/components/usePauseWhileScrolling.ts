import { useEffect } from 'react'

// While the page is being scrolled, the slow decorative animations (the light on the
// sidebar's edge, the shapes drifting behind the page) stand still and carry on when
// the scrolling stops. They are faint, so nobody sees the pause, and Chrome has
// less to paint while the page moves. The CSS side is `.is-scrolling` in design.css.
const QUIET_MS = 160

export function usePauseWhileScrolling(target: React.RefObject<HTMLElement | null>) {
  useEffect(() => {
    const el = target.current
    const shell = el?.closest('.app-shell')
    if (!el || !shell) return
    let timer = 0
    let paused = false
    const resume = () => {
      timer = 0
      paused = false
      shell.classList.remove('is-scrolling')
    }
    const onScroll = () => {
      if (!paused) {
        paused = true
        shell.classList.add('is-scrolling')
      }
      window.clearTimeout(timer)
      timer = window.setTimeout(resume, QUIET_MS)
    }
    el.addEventListener('scroll', onScroll, { passive: true })
    return () => {
      el.removeEventListener('scroll', onScroll)
      window.clearTimeout(timer)
      shell.classList.remove('is-scrolling')
    }
  }, [target])
}
