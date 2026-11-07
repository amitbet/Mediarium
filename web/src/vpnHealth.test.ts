// Run with: npm test (uses node's built-in test runner, no extra packages).
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { agoText, handshakeLine, testSummary, trafficLine } from './vpnHealth.ts'

test('ago text counts seconds, minutes and hours', () => {
  assert.equal(agoText(0), '0 seconds ago')
  assert.equal(agoText(1), '1 second ago')
  assert.equal(agoText(59), '59 seconds ago')
  assert.equal(agoText(60), '1 minute ago')
  assert.equal(agoText(185), '3 minutes ago')
  assert.equal(agoText(3600), '1 hour ago')
  assert.equal(agoText(7300), '2 hours ago')
  assert.equal(agoText(-5), '0 seconds ago')
})

test('no tunnel means no handshake or traffic lines', () => {
  assert.equal(handshakeLine(null), null)
  assert.equal(handshakeLine({}), null)
  assert.equal(handshakeLine({ tunnelUp: false, handshakeAgeSec: 3 }), null)
  assert.equal(trafficLine(undefined), null)
  assert.equal(trafficLine({ tunnelUp: false }), null)
})

test('a recent handshake is shown without a warning', () => {
  assert.deepEqual(handshakeLine({ tunnelUp: true, handshakeAgeSec: 12 }), { text: 'Last handshake: 12 seconds ago', warn: false })
})

test('an old handshake is a warning', () => {
  assert.deepEqual(handshakeLine({ tunnelUp: true, handshakeAgeSec: 600 }), { text: 'Last handshake: 10 minutes ago', warn: true })
  assert.equal(handshakeLine({ tunnelUp: true, handshakeAgeSec: 239 })?.warn, false)
  assert.equal(handshakeLine({ tunnelUp: true, handshakeAgeSec: 240 })?.warn, true)
})

test('no handshake yet only warns after the grace time', () => {
  const waiting = handshakeLine({ tunnelUp: true, noHandshake: false })
  assert.equal(waiting?.warn, false)
  assert.equal(waiting?.text, 'Waiting for the first handshake…')
  const stuck = handshakeLine({ tunnelUp: true, noHandshake: true })
  assert.equal(stuck?.warn, true)
  assert.equal(stuck?.text, 'No handshake yet. The VPN server has not answered.')
})

test('traffic line shows zero as 0 B', () => {
  assert.equal(trafficLine({ tunnelUp: true }), 'Received 0 B, sent 0 B since it connected')
  assert.equal(trafficLine({ tunnelUp: true, rxBytes: 1536, txBytes: 512 }), 'Received 1.5 KB, sent 512.0 B since it connected')
})

test('test summary says what worked or what is wrong', () => {
  assert.equal(
    testSummary({ ok: true, ip: '203.0.113.7', handshakeAgeSec: 5 }),
    'The tunnel works. A request came back through the VPN from 203.0.113.7. Last handshake: 5 seconds ago',
  )
  assert.equal(testSummary({ ok: true, ip: '203.0.113.7' }), 'The tunnel works. A request came back through the VPN from 203.0.113.7.')
  assert.equal(testSummary({ ok: false, reason: 'The VPN server did not answer.' }), 'The VPN server did not answer.')
  assert.equal(testSummary({ ok: false }), "The test didn't work.")
})
