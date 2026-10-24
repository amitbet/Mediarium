import type { ManualMode } from './api'

// What the page "Import a file by hand" remembers between visits. Everything is
// best effort: private windows and blocked storage make the accessors throw,
// and the page works the same without them.
export const MANUAL_STORE = {
  folder: 'mediarium-manual-import-folder',
  mode: 'mediarium-manual-import-mode',
  incomplete: 'mediarium-manual-import-incomplete',
}

export const MANUAL_MODES: ManualMode[] = ['link', 'copy', 'move']

// manualMode reads a stored or typed value, falling back to the usual way (a
// hardlink, which is what every import does).
export function manualMode(raw: unknown): ManualMode {
  return MANUAL_MODES.find((m) => m === raw) ?? 'link'
}

type Store = Pick<Storage, 'getItem' | 'setItem'>

function browserStore(): Store | null {
  try {
    return localStorage
  } catch {
    return null
  }
}

export function recall(key: string, store: Store | null = browserStore()): string {
  try {
    return store?.getItem(key) ?? ''
  } catch {
    return ''
  }
}

export function remember(key: string, value: string, store: Store | null = browserStore()): void {
  try {
    store?.setItem(key, value)
  } catch {
    // best effort only
  }
}

// rememberedFolder is the folder to open first: the last one that worked, or
// '' for the downloads folder.
export function rememberedFolder(store?: Store | null): string {
  return recall(MANUAL_STORE.folder, store).trim()
}

// folderToRemember is what to store once a folder was read: nothing special
// for the downloads folder itself (it is the default and may be changed in
// Settings), the path for any other.
export function folderToRemember(folder: string, downloadsFolder: string): string {
  return folder === downloadsFolder ? '' : folder
}

// parentOf is the folder above a full path, '' at the top.
export function parentOf(path: string): string {
  const trimmed = path.replace(/\/+$/, '')
  const i = trimmed.lastIndexOf('/')
  return i <= 0 ? '' : trimmed.slice(0, i)
}

// sourceKept says the original is still there after a move, which is worth
// telling the person about.
export function sourceKept(mode: ManualMode, kept: boolean | undefined): boolean {
  return mode === 'move' && kept === true
}
