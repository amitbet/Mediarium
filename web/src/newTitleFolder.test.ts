// Run with: npm test (uses node's built-in test runner, no extra packages).
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { addChoices, keptRule, laterChoices, mostRoomNote, MOST_ROOM, ruleChoices } from './newTitleFolder.ts'

const folders = ['/movies', '/movies2', '/kids']

test('the Add dialog offers every folder explicitly and starts where new titles go', () => {
  const { options, initial } = addChoices(folders, { folder: '/movies2', mostRoom: false })
  assert.deepEqual(
    options.map((o) => o.value),
    folders,
  )
  assert.equal(options[0].label, '/movies (main)')
  assert.equal(options[1].label, '/movies2')
  assert.equal(initial, '/movies2')
})

test('the Add dialog starts on the main folder when the server says nothing useful', () => {
  assert.equal(addChoices(folders).initial, '/movies')
  assert.equal(addChoices(folders, { folder: '/gone', mostRoom: false }).initial, '/movies')
  assert.equal(addChoices([]).initial, '')
})

test('a form that adds titles later can leave the folder to the rule', () => {
  const options = laterChoices(folders, { folder: '/kids', mostRoom: true })
  assert.equal(options[0].value, '')
  assert.equal(options[0].label, 'Where new titles go (/kids right now)')
  assert.deepEqual(options.slice(1).map((o) => o.value), folders)
  // Without the server's answer the main folder is named.
  assert.equal(laterChoices(folders)[0].label, 'Where new titles go (/movies right now)')
})

test('the settings rule lists the main folder, the extra ones and most room', () => {
  const options = ruleChoices('/movies', ['/movies2', '/kids'])
  assert.deepEqual(
    options.map((o) => o.value),
    ['', '/movies2', '/kids', MOST_ROOM],
  )
  assert.equal(options[0].label, '/movies (main)')
  assert.equal(options[3].label, 'The folder with the most room')
})

test('a rule naming a folder that was removed falls back to the main folder', () => {
  assert.equal(keptRule('/kids', ['/movies2']), '')
  assert.equal(keptRule('/kids', ['/movies2', '/kids']), '/kids')
  assert.equal(keptRule(MOST_ROOM, []), MOST_ROOM)
  assert.equal(keptRule('', []), '')
})

test('the most-room note names the folder that has the most room now', () => {
  assert.equal(mostRoomNote({ folder: '/movies2', mostRoom: true }), 'New titles go to the folder with the most room; right now that is /movies2.')
  assert.equal(mostRoomNote({ folder: '/movies2', mostRoom: false }), '')
  assert.equal(mostRoomNote(undefined), '')
})
