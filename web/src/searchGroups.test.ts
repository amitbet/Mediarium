import assert from 'node:assert/strict'
import { test } from 'node:test'
import { effectiveTab, LEAD_SCORE, planSettings, readSearchTab, rememberSearchTab, SETTINGS_CAP, type KeyValueStore, type SearchTab } from './searchGroups.ts'

function memory(): KeyValueStore & { data: Map<string, string> } {
  const data = new Map<string, string>()
  return { data, getItem: (k) => data.get(k) ?? null, setItem: (k, v) => void data.set(k, v) }
}

const broken: KeyValueStore = {
  getItem() {
    throw new Error('blocked')
  },
  setItem() {
    throw new Error('blocked')
  },
}

test('the settings group is capped on the All tab and never leads for a loose match', () => {
  const cases: { name: string; hits: number; top: number; tab: SearchTab; expanded: boolean; want: ReturnType<typeof planSettings> }[] = [
    { name: 'no matches', hits: 0, top: 0, tab: 'all', expanded: false, want: { media: true, show: false, lead: false, shown: 0, more: 0 } },
    { name: 'one loose match, after the titles', hits: 1, top: 40, tab: 'all', expanded: false, want: { media: true, show: true, lead: false, shown: 1, more: 0 } },
    { name: 'exactly the cap', hits: SETTINGS_CAP, top: 40, tab: 'all', expanded: false, want: { media: true, show: true, lead: false, shown: SETTINGS_CAP, more: 0 } },
    { name: 'five loose matches show three', hits: 5, top: 40, tab: 'all', expanded: false, want: { media: true, show: true, lead: false, shown: 3, more: 2 } },
    { name: 'five matches after Show all', hits: 5, top: 40, tab: 'all', expanded: true, want: { media: true, show: true, lead: false, shown: 5, more: 0 } },
    { name: 'a match that starts like the text leads, still capped', hits: 5, top: LEAD_SCORE, tab: 'all', expanded: false, want: { media: true, show: true, lead: true, shown: 3, more: 2 } },
    { name: 'just under the lead score does not lead', hits: 2, top: LEAD_SCORE - 1, tab: 'all', expanded: false, want: { media: true, show: true, lead: false, shown: 2, more: 0 } },
    { name: 'the Media tab hides settings', hits: 5, top: 100, tab: 'media', expanded: false, want: { media: true, show: false, lead: false, shown: 0, more: 0 } },
    { name: 'the Settings tab lists them all and no titles', hits: 5, top: 10, tab: 'settings', expanded: false, want: { media: false, show: true, lead: true, shown: 5, more: 0 } },
  ]
  for (const c of cases) {
    assert.deepEqual(planSettings(c.hits, c.top, c.tab, c.expanded), c.want, c.name)
  }
})

test('without settings matches every tab shows the plain list', () => {
  for (const tab of ['all', 'media', 'settings'] as const) {
    assert.equal(effectiveTab(tab, 0), 'all', tab)
    assert.equal(effectiveTab(tab, 2), tab, tab)
  }
})

test('an empty Settings tab never hides the titles', () => {
  const tab = effectiveTab('settings', 0)
  assert.equal(planSettings(0, 0, tab, false).media, true)
})

test('the tab is remembered', () => {
  const store = memory()
  assert.equal(readSearchTab(store), 'all')
  rememberSearchTab('media', store)
  assert.equal(readSearchTab(store), 'media')
  rememberSearchTab('settings', store)
  assert.equal(readSearchTab(store), 'settings')
})

test('a stored value that is not a tab is ignored', () => {
  const store = memory()
  store.data.set('mediarium-search-tab', 'sideways')
  assert.equal(readSearchTab(store), 'all')
})

test('blocked storage does not break the popup', () => {
  assert.equal(readSearchTab(broken), 'all')
  assert.doesNotThrow(() => rememberSearchTab('media', broken))
  assert.equal(readSearchTab(null), 'all')
  assert.doesNotThrow(() => rememberSearchTab('media', null))
})
