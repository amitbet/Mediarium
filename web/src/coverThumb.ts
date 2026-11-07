// A grid of artists or albums shows each cover a couple of hundred pixels wide, so it asks the
// server for the small thumbnail (a few kilobytes, kept on disk by the server) instead of the
// full picture. Only the music cover addresses on this server take the size; anything else
// (a TMDB poster, a picture that already has a query) is left as it is.

/** The side, in pixels, of the longest edge of a thumbnail the server makes. */
export const THUMB_SIDE = 256

const MUSIC_COVER = /\/api\/music\/(?:artists\/\d+\/cover|albums\/\d+\/cover|covers\/release-group\/[0-9a-fA-F-]{36})$/

export function isMusicCover(url: string | undefined): url is string {
  return !!url && MUSIC_COVER.test(url)
}

/** The address of the thumbnail for a music cover address, or the address unchanged. */
export function coverThumb(url: string): string
export function coverThumb(url: string | undefined): string | undefined
export function coverThumb(url: string | undefined): string | undefined {
  return isMusicCover(url) ? `${url}?size=thumb` : url
}
