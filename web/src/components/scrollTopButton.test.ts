// Run with: npm test (uses node's built-in test runner, no extra packages).
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { showBackToTop } from './scrollTopButton.ts'

test('showBackToTop waits for about one screen of scrolling on a long page', () => {
  const cases: [string, number, number, number, boolean, boolean][] = [
    ['top of a long page', 5000, 800, 0, false, false],
    ['half a screen down', 5000, 800, 400, false, false],
    ['just under one screen', 5000, 800, 700, false, false],
    ['past one screen', 5000, 800, 900, false, true],
    ['already shown, a little above the limit', 5000, 800, 600, true, true],
    ['already shown, back near the top', 5000, 800, 300, true, false],
    ['short page never shows it', 1200, 800, 400, false, false],
    ['short page, even if scrolled', 1200, 800, 1000, true, false],
    ['hidden element has no height', 5000, 0, 900, false, false],
  ]
  for (const [name, scrollHeight, clientHeight, scrollTop, shown, want] of cases) {
    assert.equal(showBackToTop(scrollHeight, clientHeight, scrollTop, shown), want, name)
  }
})
