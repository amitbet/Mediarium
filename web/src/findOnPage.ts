import { useEffect } from 'react'
import { useLocation } from 'react-router-dom'
import { norm } from './settingsSearch.ts'

// Opened from the search bar, a settings page scrolls to the card or setting
// that was picked and lights it up for a moment. The card is found by its
// heading (the text the search result showed), so a page needs nothing added.
// The page may still be loading when the link is followed, so this keeps
// looking for a few seconds.

const HEADINGS = 'h2, h3, h4, legend, summary, label, dt'
const GIVE_UP_MS = 6000
const SHOWN_MS = 2600

function locate(root: ParentNode, want: string): HTMLElement | null {
  let partial: HTMLElement | null = null
  for (const el of root.querySelectorAll<HTMLElement>(HEADINGS)) {
    const text = norm(el.textContent ?? '')
    if (!text || !(el.offsetParent || el.getClientRects().length)) continue
    if (text === want) return el
    if (!partial && text.includes(want)) partial = el
  }
  return partial
}

function show(heading: HTMLElement): () => void {
  // A closed "details" holds what was asked for: open it.
  for (let d = heading.closest('details'); d; d = d.parentElement?.closest('details') ?? null) d.open = true
  const box = heading.closest<HTMLElement>('fieldset, details, section, .card, .group') ?? heading.parentElement ?? heading
  const calm = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
  box.scrollIntoView({ block: 'center', behavior: calm ? 'auto' : 'smooth' })
  box.classList.add('find-hit')
  const timer = window.setTimeout(() => box.classList.remove('find-hit'), SHOWN_MS)
  return () => {
    window.clearTimeout(timer)
    box.classList.remove('find-hit')
  }
}

// Call from the settings layout: follows `state.find`, set when the search
// bar navigates here.
export function useFindOnPage() {
  const loc = useLocation()
  const find = (loc.state as { find?: string } | null)?.find
  useEffect(() => {
    const want = norm(find ?? '')
    if (!want) return
    const root = document.querySelector('.settings-content') ?? document.body
    let undo: (() => void) | undefined
    let done = false
    const look = () => {
      const el = locate(root, want)
      if (!el) return false
      done = true
      undo = show(el)
      return true
    }
    let watcher: MutationObserver | undefined
    let giveUp = 0
    // A short wait first lets a page that is already drawn settle.
    const first = window.setTimeout(() => {
      if (look()) return
      watcher = new MutationObserver(() => {
        if (!done && look()) {
          watcher?.disconnect()
          window.clearTimeout(giveUp)
        }
      })
      watcher.observe(root, { childList: true, subtree: true })
      giveUp = window.setTimeout(() => watcher?.disconnect(), GIVE_UP_MS)
    }, 120)
    return () => {
      window.clearTimeout(first)
      window.clearTimeout(giveUp)
      watcher?.disconnect()
      undo?.()
    }
  }, [find, loc.key])
}
