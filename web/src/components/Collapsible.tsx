import { useEffect, useId, useState, type ReactNode } from 'react'
import { readOpen, rememberOpen } from '../collapsibleState'
import Icon from './Icon'

// A part of a page that can be folded away, for the long parts of the settings.
// The head is one button across the width: the name of the part (shown, or
// only read out when the card already has a heading above), a short summary
// such as "5 formats", and a chevron. The person's choice is remembered per
// section (the id), in this browser. The section opens by itself when
// something happens inside it: openSignal goes up (a format was added, say) or
// forceOpen is true (a mistake in it that needs to be seen). Those automatic
// openings are not remembered.
export default function Collapsible({
  id,
  label,
  showLabel = false,
  summary,
  defaultOpen = false,
  openSignal = 0,
  forceOpen = false,
  children,
}: {
  id: string
  // The name of the part, for the button.
  label: string
  // Show the name in the head (inside an editor); a card with its own heading leaves it out.
  showLabel?: boolean
  // Shown in the head, e.g. "5 formats".
  summary?: string
  defaultOpen?: boolean
  openSignal?: number
  forceOpen?: boolean
  children: ReactNode
}) {
  const [open, setOpen] = useState(() => readOpen(id, defaultOpen))
  const bodyId = useId()

  useEffect(() => {
    if (openSignal > 0) setOpen(true)
  }, [openSignal])
  useEffect(() => {
    if (forceOpen) setOpen(true)
  }, [forceOpen])

  function toggle() {
    const next = !open
    setOpen(next)
    rememberOpen(id, next)
  }

  return (
    <div className="collapsible" data-open={open ? 'true' : 'false'}>
      <button type="button" className="collapsible-head" aria-expanded={open} aria-controls={bodyId} onClick={toggle}>
        <span className={showLabel ? 'collapsible-label' : 'sr-only'}>{label}</span>
        {summary && <span className="collapsible-summary">{summary}</span>}
        <Icon name="chevron-down" size={16} className="collapsible-chevron" />
      </button>
      <div id={bodyId} className="collapsible-body" hidden={!open}>
        {children}
      </div>
    </div>
  )
}
