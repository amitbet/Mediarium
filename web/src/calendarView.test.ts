import { test } from 'node:test'
import assert from 'node:assert/strict'
import { layoutFor, parseView, pickDay, type View } from './calendarView.ts'

test('a saved view name is read back, anything else is the month', () => {
  const cases: [string | null | undefined, View][] = [
    ['month', 'month'],
    ['posters', 'posters'],
    ['backdrops', 'backdrops'],
    ['agenda', 'agenda'],
    ['grid', 'month'],
    ['', 'month'],
    [null, 'month'],
    [undefined, 'month'],
  ]
  for (const [raw, want] of cases) assert.equal(parseView(raw), want, String(raw))
})

test('each view is drawn differently, at every width', () => {
  const views: View[] = ['month', 'posters', 'backdrops', 'agenda']
  for (const narrow of [false, true]) {
    const layouts = views.map((v) => layoutFor(v, narrow))
    // Wide: the three grid views differ by what their days hold, the agenda is a list. Narrow: four different layouts.
    assert.equal(new Set(layouts).size, narrow ? 4 : 2, `narrow=${narrow}: ${layouts.join(',')}`)
  }
  assert.equal(layoutFor('month', true), 'dots')
  assert.equal(layoutFor('posters', true), 'day-tiles')
  assert.equal(layoutFor('backdrops', true), 'day-wide')
  assert.equal(layoutFor('agenda', true), 'agenda')
  assert.equal(layoutFor('agenda', false), 'agenda')
  assert.equal(layoutFor('month', false), 'grid')
})

test('the compact month opens on a sensible day', () => {
  const cases: { picked: string | null; month: string; today: string; busy: string[]; want: string }[] = [
    { picked: '2026-10-07', month: '2026-10', today: '2026-10-23', busy: [], want: '2026-10-07' },
    { picked: '2026-09-07', month: '2026-10', today: '2026-10-23', busy: [], want: '2026-10-23' },
    { picked: null, month: '2026-11', today: '2026-10-23', busy: ['2026-11-20', '2026-11-04'], want: '2026-11-04' },
    { picked: null, month: '2026-11', today: '2026-10-23', busy: ['2026-10-02'], want: '2026-11-01' },
  ]
  for (const c of cases) assert.equal(pickDay(c.picked, c.month, c.today, c.busy), c.want)
})
