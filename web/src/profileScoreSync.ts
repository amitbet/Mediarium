// An open profile form holds a copy of the profile's custom format scores. When
// formats are imported or added to the profile from the Custom formats card
// while the form is open, the copy goes stale, and saving it would drop the
// new scores. syncFormatScores brings the copy up to date without touching
// anything the person typed.

type Scores = Record<number, string>

// base: the scores the form was loaded with (or last synced to).
// draft: what the form holds now. server: the profile's scores now.
// A format the person has not edited (draft equals base) follows the server;
// one they edited keeps their text.
export function syncFormatScores(base: Scores, draft: Scores, server: Scores): Scores {
  const out: Scores = {}
  const ids = new Set([...Object.keys(base), ...Object.keys(draft), ...Object.keys(server)].map(Number))
  for (const id of ids) {
    const edited = (draft[id] ?? '') !== (base[id] ?? '')
    if (edited) {
      out[id] = draft[id] ?? ''
    } else if (server[id] !== undefined) {
      out[id] = server[id]
    }
  }
  return out
}

// hasScore: the format is in the profile's list. A score of 0 counts, since
// the format is added and does nothing until it is raised; only an empty box
// leaves it out.
export function hasScore(scores: Scores, id: number): boolean {
  return (scores[id] ?? '').trim() !== ''
}

// withoutScore takes one format out of the list (the bin button on its row).
export function withoutScore(scores: Scores, id: number): Scores {
  const out = { ...scores }
  delete out[id]
  return out
}
