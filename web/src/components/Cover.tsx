import { useState } from 'react'
import { MusicFallback } from './PosterCard'
import { coverThumb, THUMB_SIDE } from '../coverThumb'

// An album cover (or artist picture). Not every release has one on the Cover
// Art Archive, so a missing or broken image falls back to a plain sleeve.
// thumb asks for the small version of a music cover, for a card or a row; leave it off where the picture is shown big.
export default function Cover({ src, alt = '', className, thumb }: { src?: string; alt?: string; className?: string; thumb?: boolean }) {
  // Remember which address failed, so a new cover (the same card showing another album) gets its own try.
  const [brokenSrc, setBrokenSrc] = useState<string | undefined>()
  return (
    <span className={`cover${className ? ` ${className}` : ''}`}>
      {src && brokenSrc !== src ? <img src={thumb ? coverThumb(src) : src} {...(thumb ? { width: THUMB_SIDE, height: THUMB_SIDE, fetchPriority: 'low' as const } : {})} alt={alt} loading="lazy" decoding="async" onError={() => setBrokenSrc(src)} /> : <MusicFallback />}
    </span>
  )
}
