import assert from 'node:assert/strict'
import { test } from 'node:test'
import { folderToRemember, manualMode, MANUAL_STORE, parentOf, recall, remember, rememberedFolder, sourceKept } from './manualImport.ts'

const memory = (initial: Record<string, string> = {}) => {
  const data = new Map(Object.entries(initial))
  return { getItem: (k: string) => data.get(k) ?? null, setItem: (k: string, v: string) => void data.set(k, v) }
}
const broken = {
  getItem: () => {
    throw new Error('blocked')
  },
  setItem: () => {
    throw new Error('blocked')
  },
}

test('manualMode falls back to a hardlink', () => {
  const cases: [unknown, string][] = [
    ['link', 'link'],
    ['copy', 'copy'],
    ['move', 'move'],
    ['', 'link'],
    [null, 'link'],
    ['MOVE', 'link'],
    ['delete', 'link'],
  ]
  for (const [raw, want] of cases) assert.equal(manualMode(raw), want, String(raw))
})

test('remembering works, and storage that throws is ignored', () => {
  const store = memory()
  remember(MANUAL_STORE.folder, '/mnt/torrents', store)
  assert.equal(rememberedFolder(store), '/mnt/torrents')
  assert.equal(rememberedFolder(memory({ [MANUAL_STORE.folder]: '  /x  ' })), '/x')
  assert.equal(recall('nothing', store), '')
  assert.equal(recall('x', broken), '')
  assert.doesNotThrow(() => remember('x', 'y', broken))
  assert.equal(rememberedFolder(null), '')
})

test('the downloads folder is not stored as a choice', () => {
  assert.equal(folderToRemember('/downloads', '/downloads'), '')
  assert.equal(folderToRemember('/mnt/other', '/downloads'), '/mnt/other')
})

test('parentOf', () => {
  const cases: [string, string][] = [
    ['/mnt/media/downloads', '/mnt/media'],
    ['/mnt/media/downloads/', '/mnt/media'],
    ['/mnt', ''],
    ['/', ''],
    ['', ''],
  ]
  for (const [path, want] of cases) assert.equal(parentOf(path), want, path)
})

test('only a move can leave the original behind', () => {
  assert.equal(sourceKept('move', true), true)
  assert.equal(sourceKept('move', false), false)
  assert.equal(sourceKept('move', undefined), false)
  assert.equal(sourceKept('copy', true), false)
  assert.equal(sourceKept('link', true), false)
})
