// Applying the sizes the TRaSH Guides recommend to the size limits table:
// pure functions, so the card only draws what they return.

export interface SizeText {
  quality: string
  minText: string
  maxText: string
}

export interface TrashSizeRow {
  quality: string
  min: number
  max: number // 0: no upper limit
}

export interface SizeChange {
  quality: string
  fromMin: number
  fromMax: number
  toMin: number
  toMax: number
}

const num = (text: string): number => {
  const n = Number(text.trim())
  return Number.isFinite(n) ? n : 0
}

// The table holds an empty box for "no limit".
export const maxBoxText = (n: number): string => (n === 0 ? '' : String(n))

// applyTrashSizes puts TRaSH's smallest and largest sizes into the rows it has
// a size for and says which rows that changed. The other rows are returned as
// they were; the original list is not touched.
export function applyTrashSizes<T extends SizeText>(rows: T[], trash: TrashSizeRow[]): { rows: T[]; changes: SizeChange[]; untouched: string[] } {
  const by = new Map(trash.map((r) => [r.quality, r]))
  const changes: SizeChange[] = []
  const untouched: string[] = []
  const out = rows.map((r) => {
    const sz = by.get(r.quality)
    if (!sz) {
      untouched.push(r.quality)
      return r
    }
    const fromMin = num(r.minText)
    const fromMax = num(r.maxText)
    if (fromMin !== sz.min || fromMax !== sz.max) changes.push({ quality: r.quality, fromMin, fromMax, toMin: sz.min, toMax: sz.max })
    return { ...r, minText: String(sz.min), maxText: maxBoxText(sz.max) }
  })
  return { rows: out, changes, untouched }
}
