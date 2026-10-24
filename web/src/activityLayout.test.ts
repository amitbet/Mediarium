import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const css = readFileSync(new URL('activity.css', import.meta.url), 'utf8')

// A release or file name ("MobLand.S02E06.1080p.10bit.WEBRip.6CH.x265-PSA.mkv") has no space to break at. Without
// overflow-wrap the text runs past the card's right edge on a phone (issue 121), and the row's text column (a grid
// item) never shrinks below the width of that one word unless it has min-width: 0.
test('the text of a download row may break anywhere', () => {
  const rule = css.match(/([^{}]*\.qerror[^{}]*\.qsteps li[^{}]*)\{([^}]*)\}/)
  assert.ok(rule, 'no rule that lists .qerror and .qsteps li together')
  for (const cls of ['.qrelease', '.qnote', '.qerror', '.qwhen', '.qhelp', '.qtitle', '.qmain']) {
    assert.ok(rule[1].includes(cls), `${cls} is missing from the overflow-wrap rule`)
  }
  assert.match(rule[2], /overflow-wrap:\s*anywhere/)
})

test('the text column and the Details box can shrink', () => {
  assert.match(css, /\.activity-page \.qmain\s*\{[^}]*min-width:\s*0/, '.qmain needs min-width: 0')
  assert.match(css, /\.qdetails\s*\{[^}]*min-width:\s*0/, '.qdetails needs min-width: 0')
  assert.match(css, /\.qdetails dl\s*\{[^}]*minmax\(0,\s*1fr\)/, 'the value column of Details must be allowed to shrink')
  assert.match(css, /\.qdetails dd\s*\{[^}]*overflow-wrap:\s*anywhere/, 'Details values must break anywhere')
})

test('each step in Details lines up: the time in its own column', () => {
  assert.match(css, /\.qsteps li\s*\{[^}]*display:\s*grid[^}]*grid-template-columns:\s*max-content minmax\(0,\s*1fr\)/)
})
