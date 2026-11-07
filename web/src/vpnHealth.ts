import { plural, st, t } from './i18n.ts'
import { formatBytes } from './format.ts'

// What the VPN page shows about a tunnel: when the server last answered and
// how much the tunnel has carried. The fields come from /api/vpn/status.
export interface VPNHealthInput {
  tunnelUp?: boolean
  handshakeAgeSec?: number
  noHandshake?: boolean
  rxBytes?: number
  txBytes?: number
}

// The server counts a connection as down when its server has been quiet for four minutes.
const STALE_SEC = 240

// "12 seconds ago", "3 minutes ago", "2 hours ago".
export function agoText(sec: number): string {
  sec = Math.max(0, Math.floor(sec))
  if (sec < 60) return plural(sec, '{n} second ago', '{n} seconds ago')
  if (sec < 3600) return plural(Math.floor(sec / 60), '{n} minute ago', '{n} minutes ago')
  return plural(Math.floor(sec / 3600), '{n} hour ago', '{n} hours ago')
}

export interface HandshakeLine {
  text: string
  // true when the line is a warning: no handshake yet, or the server went quiet.
  warn: boolean
}

// The "last handshake" line for a connection, or null when there is no tunnel.
export function handshakeLine(s: VPNHealthInput | null | undefined): HandshakeLine | null {
  if (!s?.tunnelUp) return null
  if (s.handshakeAgeSec !== undefined) {
    return { text: t('Last handshake: {ago}', { ago: agoText(s.handshakeAgeSec) }), warn: s.handshakeAgeSec >= STALE_SEC }
  }
  if (s.noHandshake) {
    return { text: t('No handshake yet. The VPN server has not answered.'), warn: true }
  }
  return { text: t('Waiting for the first handshake…'), warn: false }
}

function bytesOrZero(n: number | undefined): string {
  return n ? formatBytes(n) : '0 B'
}

// "Received 1.2 MB, sent 340.0 KB since it connected", or null when there is no tunnel.
export function trafficLine(s: VPNHealthInput | null | undefined): string | null {
  if (!s?.tunnelUp) return null
  return t('Received {rx}, sent {tx} since it connected', { rx: bytesOrZero(s.rxBytes), tx: bytesOrZero(s.txBytes) })
}

export interface VPNTestResult {
  ok: boolean
  ip?: string
  reason?: string
  handshakeAgeSec?: number
}

// The answer to the Test button, as a sentence.
export function testSummary(r: VPNTestResult): string {
  if (!r.ok) return r.reason ? st(r.reason) : t("The test didn't work.")
  const ago = r.handshakeAgeSec === undefined ? '' : ' ' + t('Last handshake: {ago}', { ago: agoText(r.handshakeAgeSec) })
  return t('The tunnel works. A request came back through the VPN from {ip}.', { ip: r.ip ?? '' }) + ago
}
