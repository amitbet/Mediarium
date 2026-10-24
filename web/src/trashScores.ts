// What adding TRaSH custom formats to a quality profile does to their scores:
// pure functions, so the Custom formats card only draws what they return.

export interface TrashScoreInput {
  defaultScore: number // what the guides score the format
  importedId: number // the id of the format when it is already added (0: not yet)
}

export interface ScorePreview {
  // The score the format will have in the profile after adding (null: the
  // format is left unscored because no profile is chosen, or the guides give
  // it no score and nothing is changed).
  willGet: number | null
  // The score it has in the profile now, for a format that is already added.
  current: number | null
  // 'zero': already added, scored 0 here although the guides score it.
  // 'differs': already added with another score than the guides give.
  flag: 'zero' | 'differs' | null
}

// scorePreview is what the import will do for one format in the chosen
// profile. scores maps a format id to its score in that profile; null means
// no profile is chosen.
export function scorePreview(f: TrashScoreInput, scores: Map<number, number> | null): ScorePreview {
  const current = f.importedId > 0 ? (scores?.get(f.importedId) ?? 0) : null
  if (scores === null) return { willGet: null, current, flag: null }
  const willGet = f.defaultScore
  let flag: ScorePreview['flag'] = null
  if (current !== null && f.defaultScore !== 0) {
    if (current === 0) flag = 'zero'
    else if (current !== f.defaultScore) flag = 'differs'
  }
  return { willGet, current, flag }
}

// signed writes a score with a plus for the positive ones.
export const signed = (n: number): string => (n > 0 ? `+${n}` : String(n))

// profileForApp is the quality profile a title of that kind uses when it has
// none of its own: shows have a default of their own when one is set.
export function profileForApp(app: 'radarr' | 'sonarr', defaultId: number, defaultTvId: number): number {
  return app === 'sonarr' && defaultTvId > 0 ? defaultTvId : defaultId
}

// importScoreAction is what adding one TRaSH format with a profile chosen does
// to its score there: 'add' (the profile does not have it yet), 'skip' (it has
// it, and keeps its score), 'replace' (it has it and the person asked for the
// TRaSH score), or 'none' (no profile chosen). scores maps a format id to its
// score in that profile; a score of 0 counts as being in it.
export function importScoreAction(f: TrashScoreInput, scores: Map<number, number> | null, replace: boolean): 'add' | 'skip' | 'replace' | 'none' {
  if (scores === null) return 'none'
  if (f.importedId <= 0 || !scores.has(f.importedId)) return 'add'
  return replace ? 'replace' : 'skip'
}
