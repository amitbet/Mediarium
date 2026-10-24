// What a folded part of a page remembers (components/Collapsible.tsx): whether
// the person left it open or shut, per section, in this browser. Reading and
// writing never throw: private windows, blocked site data and previews can
// all refuse, and the page must work without it.

const PREFIX = 'mediarium-open:'

export interface KeyValueStore {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
}

function browserStore(): KeyValueStore | null {
  try {
    return typeof localStorage === 'undefined' ? null : localStorage
  } catch {
    return null
  }
}

// readOpen is the remembered choice, or fallback when none was made (or it can't be read).
export function readOpen(id: string, fallback: boolean, store: KeyValueStore | null = browserStore()): boolean {
  try {
    const v = store?.getItem(PREFIX + id)
    if (v === '1') return true
    if (v === '0') return false
  } catch {
    // Not readable: the default is used.
  }
  return fallback
}

// rememberOpen keeps the choice; it is not an error when that isn't possible.
export function rememberOpen(id: string, open: boolean, store: KeyValueStore | null = browserStore()): void {
  try {
    store?.setItem(PREFIX + id, open ? '1' : '0')
  } catch {
    // Not remembered; the section still works.
  }
}
