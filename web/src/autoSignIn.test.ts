import assert from 'node:assert/strict'
import { test } from 'node:test'
import { autoSignInUrl, returnPath, shouldAutoSignIn, type AutoSignInInput } from './autoSignIn.ts'

const base: AutoSignInInput = {
  method: 'oidc',
  oidcAvailable: true,
  search: '',
  problemCode: '',
  skipOnce: false,
  notice: false,
  lastTriedAt: null,
  now: 1_000_000,
}

test('shouldAutoSignIn: when the page redirects and when it stays', () => {
  const cases: { name: string; change: Partial<AutoSignInInput>; want: boolean }[] = [
    { name: 'chosen and working', change: {}, want: true },
    { name: 'off (no method)', change: { method: undefined }, want: false },
    { name: 'empty method', change: { method: '' }, want: false },
    { name: 'a method that is not a redirect', change: { method: 'ldap' }, want: false },
    { name: 'the provider button is not offered', change: { oidcAvailable: false }, want: false },
    { name: 'no-redirect in the address', change: { search: '?no-redirect=1' }, want: false },
    { name: 'no-redirect without a value', change: { search: '?no-redirect' }, want: false },
    { name: 'disable-auth-redirect in the address', change: { search: '?disable-auth-redirect' }, want: false },
    { name: 'bypass among other parameters', change: { search: '?a=1&no-redirect=1&b=2' }, want: false },
    { name: 'bypass without the question mark', change: { search: 'disable-auth-redirect=1' }, want: false },
    { name: 'an unrelated parameter', change: { search: '?page=2' }, want: true },
    { name: 'the provider sent the person back with an error', change: { problemCode: 'denied' }, want: false },
    { name: 'the person just signed out', change: { skipOnce: true }, want: false },
    { name: 'the page shows a notice', change: { notice: true }, want: false },
    { name: 'redirected a moment ago (a loop)', change: { lastTriedAt: 1_000_000 - 5_000 }, want: false },
    { name: 'redirected long ago', change: { lastTriedAt: 1_000_000 - 120_000 }, want: true },
    { name: 'a clock that went backwards', change: { lastTriedAt: 1_000_000 + 5_000 }, want: true },
  ]
  for (const c of cases) {
    assert.equal(shouldAutoSignIn({ ...base, ...c.change }), c.want, c.name)
  }
})

test('returnPath keeps a short path on this site and nothing else', () => {
  assert.equal(returnPath('/', ''), '')
  assert.equal(returnPath('/movies', ''), '/movies')
  assert.equal(returnPath('/title/597', '?tab=cast'), '/title/597?tab=cast')
  assert.equal(returnPath('//evil.example', ''), '')
  assert.equal(returnPath('/a\\b', ''), '')
  assert.equal(returnPath('/x', '?q=' + 'a'.repeat(600)), '')
})

test('autoSignInUrl asks the server to come back to the page', () => {
  assert.equal(autoSignInUrl('/api/auth/oidc/start', '/', ''), '/api/auth/oidc/start')
  assert.equal(autoSignInUrl('/api/auth/oidc/start', '/movies', '?sort=year'), '/api/auth/oidc/start?next=%2Fmovies%3Fsort%3Dyear')
  assert.equal(autoSignInUrl('/mediarium/api/auth/oidc/start', '/series/4', ''), '/mediarium/api/auth/oidc/start?next=%2Fseries%2F4')
})
