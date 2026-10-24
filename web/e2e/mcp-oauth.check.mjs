// Walks the MCP sign-in (OAuth with PKCE) against a running Mediarium, the way a
// program such as Home Assistant does, with a real browser for the consent page.
//
//   MEDIARIUM_URL=http://localhost:8264 MEDIARIUM_SETUP_CODE=AAAA-BBBB-CCCC node e2e/mcp-oauth.check.mjs
//
// It needs a running Mediarium with an empty config folder. It creates the
// administrator itself, switches MCP on, registers a client, signs in through
// the consent page, swaps the code for tokens, lists the tools with the token,
// refreshes, and revokes. Set PW_CHANNEL (default "chrome") to use an installed
// browser and SHOTS to a folder to keep screenshots of the consent page and the
// connected apps card.
import { chromium } from '@playwright/test'
import { createHash, randomBytes } from 'node:crypto'
import { mkdirSync } from 'node:fs'

const origin = process.env.MEDIARIUM_URL ?? 'http://localhost:8264'
const prefix = (process.env.URL_BASE ?? '').replace(/\/+$/, '')
const base = origin + prefix
const shots = process.env.SHOTS
if (shots) mkdirSync(shots, { recursive: true })
const password = 'Check-' + randomBytes(9).toString('base64url') + '-pw'
const redirect = 'http://localhost:6274/oauth/callback'

const problems = []
const ok = (m) => console.log('ok    ' + m)
const fail = (m) => {
  problems.push(m)
  console.log('FAIL  ' + m)
}
const check = (cond, m) => (cond ? ok(m) : fail(m))

const browser = await chromium.launch({ channel: process.env.PW_CHANNEL || 'chrome' })
const context = await browser.newContext({ viewport: { width: 1280, height: 900 } })
const page = await context.newPage()
page.on('pageerror', (e) => fail('page error: ' + e.message))
await context.route('http://localhost:6274/**', (r) => r.fulfill({ status: 200, contentType: 'text/plain', body: 'the app' }))

const shot = async (p, name) => {
  if (shots) await p.screenshot({ path: `${shots}/${name}.png` })
}

// An administrator, signed in, MCP on.
const made = await context.request.post(base + '/api/onboarding/admin', {
  data: { username: 'oauth-admin', password, firstName: 'Check', lastName: 'Admin', setupCode: process.env.MEDIARIUM_SETUP_CODE ?? '' },
})
check(made.status() === 201, 'administrator created')
// A new install shows the setup wizard to its administrator; skip it so the consent page can be reached.
await context.request.put(base + '/api/settings', { data: { onboardingDone: true } })
const on = await context.request.put(base + '/api/system/mcp', { data: { enabled: true } })
check(on.status() === 200, 'MCP switched on')

// Discovery.
const meta = await (await fetch(base + '/.well-known/oauth-authorization-server')).json()
check(meta.issuer === base && meta.authorization_endpoint === base + '/oauth/authorize', 'authorization server metadata')
const prm = await (await fetch(base + '/.well-known/oauth-protected-resource')).json()
check(prm.resource === base + '/mcp', 'protected resource metadata')
const unknown = await fetch(base + '/.well-known/nothing-here')
check(unknown.status === 404 && (unknown.headers.get('content-type') ?? '').includes('json'), 'unknown well-known path is a JSON 404')
const noAuth = await fetch(base + '/mcp', { method: 'POST', headers: { 'content-type': 'application/json' }, body: '{}' })
check(noAuth.status === 401 && /resource_metadata="/.test(noAuth.headers.get('www-authenticate') ?? ''), '401 points to the metadata')

// Registration and the authorization request.
const reg = await (
  await fetch(meta.registration_endpoint, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ client_name: 'Home Assistant', redirect_uris: [redirect] }),
  })
).json()
check(!!reg.client_id, 'client registered')
const verifier = randomBytes(40).toString('base64url')
const challenge = createHash('sha256').update(verifier).digest('base64url')
const authorize = new URL(meta.authorization_endpoint)
for (const [k, v] of Object.entries({ response_type: 'code', client_id: reg.client_id, redirect_uri: redirect, state: 'st-123', code_challenge: challenge, code_challenge_method: 'S256', scope: 'mcp:read mcp:write' })) {
  authorize.searchParams.set(k, v)
}

// A visitor with no session: sign in first, then come back to the consent page.
const fresh = await browser.newContext({ viewport: { width: 1280, height: 900 } })
const freshPage = await fresh.newPage()
await freshPage.goto(authorize.toString())
await freshPage.getByLabel('Username').fill('oauth-admin')
await freshPage.getByLabel('Password').fill(password)
await freshPage.getByRole('button', { name: 'Sign in', exact: true }).click()
await freshPage.getByRole('heading', { name: /Allow Home Assistant/ }).waitFor({ timeout: 15000 }).catch(async (e) => {
  console.log(freshPage.url(), (await freshPage.innerText('body')).slice(0, 400))
  throw e
})
ok('sign-in page first, then the consent page')
await shot(freshPage, 'consent-1280')
await freshPage.setViewportSize({ width: 390, height: 844 })
await shot(freshPage, 'consent-390')
await fresh.close()

// The consent page for the signed-in administrator.
await page.goto(authorize.toString())
await page.getByRole('heading', { name: /Allow Home Assistant/ }).waitFor({ timeout: 15000 })
await page.getByRole('button', { name: 'Allow', exact: true }).click()
await page.waitForURL(/localhost:6274/, { timeout: 15000 })
const back = new URL(page.url())
const code = back.searchParams.get('code')
check(!!code && back.searchParams.get('state') === 'st-123', 'the browser returns with a code and the state')

// Tokens.
const tokenRes = await fetch(meta.token_endpoint, {
  method: 'POST',
  headers: { 'content-type': 'application/x-www-form-urlencoded' },
  body: new URLSearchParams({ grant_type: 'authorization_code', code, redirect_uri: redirect, code_verifier: verifier, client_id: reg.client_id }),
})
const tokens = await tokenRes.json()
check(tokenRes.status === 200 && !!tokens.access_token && !!tokens.refresh_token && tokens.expires_in === 3600, 'code swapped for tokens')
const rpc = (token, method) =>
  fetch(base + '/mcp', { method: 'POST', headers: { 'content-type': 'application/json', authorization: 'Bearer ' + token }, body: JSON.stringify({ jsonrpc: '2.0', id: 1, method }) })
const list = await (await rpc(tokens.access_token, 'tools/list')).json()
const names = (list.result?.tools ?? []).map((t) => t.name)
check(names.includes('library_stats') && names.includes('add_movie'), `tools/list with the token (${names.length} tools)`)

// The card.
await page.goto(base + '/settings/other-apps')
await page.getByText('Connected apps').first().waitFor({ timeout: 15000 })
await page.getByText('Register a client for apps that ask for a client ID and secret').click()
await page.getByLabel('Name', { exact: true }).fill('Home Assistant')
await page.getByLabel('Redirect address').fill('https://my.home-assistant.io/redirect/oauth')
await page.getByRole('button', { name: 'Make client ID and secret' }).click()
await page.getByText('Client secret').waitFor({ timeout: 10000 })
await page.getByText('Connected apps').first().scrollIntoViewIfNeeded()
await shot(page, 'card-1280')
await page.setViewportSize({ width: 390, height: 844 })
await page.getByText('Connected apps').first().scrollIntoViewIfNeeded()
await shot(page, 'card-390')
await page.setViewportSize({ width: 1280, height: 900 })
check((await page.getByRole('cell', { name: 'Home Assistant' }).count()) > 0, 'the connected app is listed')

// Refresh, then revoke from the card.
const refreshed = await (
  await fetch(meta.token_endpoint, {
    method: 'POST',
    headers: { 'content-type': 'application/x-www-form-urlencoded' },
    body: new URLSearchParams({ grant_type: 'refresh_token', refresh_token: tokens.refresh_token, client_id: reg.client_id }),
  })
).json()
check(!!refreshed.access_token, 'refresh token swapped for a new pair')
await page.goto(base + '/settings/other-apps')
await page.getByRole('button', { name: 'Revoke' }).first().click()
await page.getByRole('button', { name: 'Disconnect' }).click()
await page.getByText('No app has connected with a sign-in yet.').waitFor({ timeout: 10000 })
check((await rpc(refreshed.access_token, 'tools/list')).status === 401, 'revoked token stops working')

await browser.close()
if (problems.length) {
  console.log(`\n${problems.length} problem(s)`)
  process.exit(1)
}
console.log('\nall good')
