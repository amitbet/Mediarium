// When the "Back to top" button shows: once the page has been scrolled about
// one screen down, and only on a page that is clearly longer than a screen.
// The gap between the two limits stops the button flickering at the edge.
export function showBackToTop(scrollHeight: number, clientHeight: number, scrollTop: number, shown: boolean): boolean {
  if (clientHeight <= 0) return false
  if (scrollHeight < clientHeight * 1.8) return false
  const limit = clientHeight * (shown ? 0.7 : 0.9)
  return scrollTop > limit
}
