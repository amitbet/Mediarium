import type { FileInfo } from '../api'
import { fileChips } from '../fileInfo'

// A compact line of what a video file holds: "HEVC 10-bit HDR10", "2160p",
// "TrueHD 7.1 Atmos", "EN, DE audio", "EN, NL subs". Nothing is drawn for a
// file that has not been read yet.
export default function FileChips({ info, className = '' }: { info: FileInfo | null | undefined; className?: string }) {
  const chips = fileChips(info)
  if (chips.length === 0) return null
  return (
    <div className={`file-chips ${className}`.trim()}>
      {chips.map((c) => (
        <span key={c.key} className={`file-chip file-chip-${c.key}`} title={c.title}>
          {c.text}
        </span>
      ))}
    </div>
  )
}
