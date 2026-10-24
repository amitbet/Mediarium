// Run with: npm test (uses node's built-in test runner, no extra packages).
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { isZeroLimit, parseRuleLimit, parseUploadLimit, ruleLimitText, UNLIMITED } from './seedLimits.ts'

test('an indexer rule box: empty follows the global limit', () => {
  const cases: [string, number | null | undefined][] = [
    ['', null],
    ['   ', null],
    ['0', 0],
    ['1.5', 1.5],
    ['1,5', 1.5],
    [' 48 ', 48],
    ['unlimited', UNLIMITED],
    ['Unlimited', UNLIMITED],
    ['-1', undefined],
    ['abc', undefined],
    ['NaN', undefined],
  ]
  for (const [text, want] of cases) assert.equal(parseRuleLimit(text), want, JSON.stringify(text))
})

test('the word for unlimited can be in the page language', () => {
  assert.equal(parseRuleLimit('Ilimitado', 'ilimitado'), UNLIMITED)
  assert.equal(parseRuleLimit('unlimited', 'ilimitado'), UNLIMITED)
})

test('a saved rule value is shown back as typed', () => {
  assert.equal(ruleLimitText(null), '')
  assert.equal(ruleLimitText(undefined), '')
  assert.equal(ruleLimitText(0), '0')
  assert.equal(ruleLimitText(2.5), '2.5')
  assert.equal(ruleLimitText(UNLIMITED), 'unlimited')
  assert.equal(ruleLimitText(UNLIMITED, 'ilimitado'), 'ilimitado')
})

test('the upload limit: empty is none, 0 is off, fractions are refused', () => {
  const cases: [string, number | undefined][] = [
    ['', UNLIMITED],
    ['  ', UNLIMITED],
    ['0', 0],
    ['300', 300],
    [' 12 ', 12],
    ['0.5', undefined],
    ['1.0', undefined],
    ['-1', undefined],
    ['1e3', undefined],
    ['abc', undefined],
  ]
  for (const [text, want] of cases) assert.equal(parseUploadLimit(text), want, JSON.stringify(text))
})

test('only a real zero switches seeding off', () => {
  for (const text of ['0', '0.0', ' 0 ', '0,0']) assert.equal(isZeroLimit(text), true, text)
  for (const text of ['', '  ', '1', '0.5', 'abc', 'unlimited']) assert.equal(isZeroLimit(text), false, text)
})
