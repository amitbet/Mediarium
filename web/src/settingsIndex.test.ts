import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { buildSettingsIndex, searchSettings, settingsRoutes } from './settingsIndex.ts'
import { SETTINGS_NAV, SETTINGS_REDIRECTS } from './settingsNav.ts'

const here = new URL('.', import.meta.url)

// The settings routes the router really has: the path="..." lines inside the
// /settings route in App.tsx.
function routerPaths(): Set<string> {
  const app = readFileSync(new URL('App.tsx', here), 'utf8')
  const start = app.indexOf('<Route path="/settings"')
  const end = app.indexOf('</Route>', start)
  assert.ok(start > 0 && end > start, 'the /settings route was not found in App.tsx')
  const paths = new Set<string>()
  for (const m of app.slice(start, end).matchAll(/<Route path="([a-z-]+)"/g)) paths.add(`/settings/${m[1]}`)
  return paths
}

function sourceText(): string {
  const out: string[] = []
  const walk = (dir: URL) => {
    for (const ent of readdirSync(dir, { withFileTypes: true })) {
      if (ent.isDirectory()) walk(new URL(`${ent.name}/`, dir))
      else if (/\.tsx$/.test(ent.name)) out.push(readFileSync(new URL(ent.name, dir), 'utf8'))
    }
  }
  walk(here)
  return out.join('\n')
}

test('every entry points at a settings route that exists', () => {
  const routes = routerPaths()
  for (const [admin, viewer] of [[true, false], [false, false], [false, true]] as const) {
    const entries = buildSettingsIndex(admin, viewer)
    for (const to of settingsRoutes(entries)) assert.ok(routes.has(to), `${to} is not a route (admin ${admin})`)
  }
  for (const p of SETTINGS_NAV) assert.ok(routes.has(p.to), `${p.to} is in the sidebar list but not in the router`)
  for (const [, to] of SETTINGS_REDIRECTS) assert.ok(routes.has(to), `${to} (a redirect target) is not a route`)
})

test('every settings page can be found, and nothing is listed twice', () => {
  const admin = buildSettingsIndex(true, false)
  const pages = admin.filter((e) => e.page).map((e) => e.to)
  assert.deepEqual(pages, SETTINGS_NAV.map((p) => p.to))
  const seen = new Set<string>()
  for (const e of admin) {
    const key = `${e.to}|${e.anchor ?? ''}`
    assert.ok(!seen.has(key), `listed twice: ${key}`)
    seen.add(key)
  }
  // every page can be reached by its own name
  for (const p of SETTINGS_NAV) {
    const top = searchSettings(p.label, admin, 3)[0]
    assert.ok(top && top.score >= 85, `${p.label} is not found by its name`)
  }
})

test('every card and setting is a heading the pages really show', () => {
  const src = sourceText()
  for (const e of buildSettingsIndex(true, false)) {
    if (e.page) continue
    assert.ok(src.includes(`'${e.anchor}'`) || src.includes(`"${e.anchor}"`), `no page shows "${e.anchor}" (${e.to})`)
  }
})

test('members and view-only accounts are only offered what they can open', () => {
  const member = buildSettingsIndex(false, false)
  assert.ok(member.every((e) => SETTINGS_NAV.find((p) => p.to === e.to)?.member), 'a member got an administrator page')
  assert.ok(!member.some((e) => e.anchor === 'Address of Mediarium'), 'an administrator card was offered to a member')
  const viewer = buildSettingsIndex(false, true)
  assert.ok(!viewer.some((e) => e.to === '/settings/services'), 'a view-only account got Linked services')
  // a media type that is off takes its folder with it
  const noMusic = buildSettingsIndex(true, false, (m) => m !== 'music')
  assert.ok(!noMusic.some((e) => e.anchor === 'Music folder'))
  assert.ok(buildSettingsIndex(true, false).some((e) => e.anchor === 'Music folder'))
})

test('typing what people look for lands on the right place', () => {
  const index = buildSettingsIndex(true, false)
  const first = (q: string) => {
    const hit = searchSettings(q, index, 1)[0]?.entry
    return hit ? `${hit.to}${hit.anchor ? ` > ${hit.anchor}` : ''}` : ''
  }
  const cases: [string, string][] = [
    ['Folders', '/settings/media'],
    ['temp folder', '/settings/download-rules > Temporary download folder'],
    ['Quality profiles', '/settings/quality > Quality profiles'],
    ['indexers', '/settings/indexers'],
    ['usenet', '/settings/usenet'],
    ['vpn', '/settings/vpn'],
    ['Notifications', '/settings/notifications'],
    ['backups', '/settings/backup'],
    ['api keys', '/settings/profile > API keys'],
    ['timezone', '/settings/general > Time zone'],
    ['dark mode', '/settings/general > Light or dark'],
    ['passkey', '/settings/security > Passkeys'],
    ['two factor', '/settings/security > Two-step sign-in'],
    ['flaresolverr', '/settings/indexers > How to add FlareSolverr to Docker'],
  ]
  for (const [q, want] of cases) assert.equal(first(q), want, q)
  // "Folders" is found as Library › Folders
  const folders = searchSettings('Folders', index, 1)[0].entry
  assert.equal(`${folders.trail} › ${folders.label}`, 'Library › Folders')
})
