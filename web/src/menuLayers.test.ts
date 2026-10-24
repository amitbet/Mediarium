import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const here = new URL('.', import.meta.url)

// Cards are layers of their own (the rise-in animation, container queries), so
// a menu that drops out of one is drawn under the card below it unless the card
// holding the open menu is lifted. This keeps that rule in place for the cards
// that hold a menu: the download rows and their cards (Activity > Download
// clients), the settings cards and the other row cards.
test('a card with an open menu is lifted above the cards next to it', () => {
  const css = readFileSync(new URL('projection.css', here), 'utf8')
  const rule = css.match(/:is\(([^)]*)\):has\(\.dropdown\.open\)\s*\{([^}]*)\}/)
  assert.ok(rule, 'no ":has(.dropdown.open)" rule in projection.css')
  const selectors = rule[1].split(',').map((s) => s.trim())
  for (const cls of ['.qrow', '.client-card', '.card', '.ncard', '.info-card']) {
    assert.ok(selectors.includes(cls), `${cls} is missing from the open-menu rule`)
  }
  assert.match(rule[2], /position:\s*relative/, 'a lifted card must be positioned for z-index to apply')
  const z = Number(rule[2].match(/z-index:\s*(\d+)/)?.[1])
  // Above the cards (auto/0 to a few), below the sticky top bar (40).
  assert.ok(z >= 10 && z < 40, `the lifted card has z-index ${z}, it must stay under the top bar (40)`)
})

test('the menu list itself is above everything inside its card', () => {
  const css = readFileSync(new URL('design.css', here), 'utf8')
  const list = css.match(/\.dropdown-list\s*\{[^}]*z-index:\s*(\d+)/)
  assert.ok(list && Number(list[1]) >= 50, 'the .dropdown-list needs a high z-index')
})
