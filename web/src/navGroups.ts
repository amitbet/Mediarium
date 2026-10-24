// Which sub menu of the left menu is open. Library and Settings each have one,
// and only one is open at a time. Nothing opens by itself: not because the page
// you are on belongs to a group, not on a change of page. Only a click on the
// group (or the arrow keys on it) opens it, and a reload starts with both
// closed.

export type NavGroup = 'library' | 'settings'

/** The open group, or null when both are closed. */
export type OpenGroup = NavGroup | null

/**
 * A click on a group's row. `inside` says whether the page is already one of the
 * group's: then the click only folds the sub menu up or down. From any other
 * page it goes to the group and opens it (closing the other group).
 */
export function afterClick(open: OpenGroup, group: NavGroup, inside: boolean): OpenGroup {
  if (!inside) return group
  return open === group ? null : group
}

/**
 * An arrow key on a group's row: Right opens its sub menu, Left folds it up.
 * Returns undefined for any other key, and when the key changes nothing.
 */
export function afterKey(open: OpenGroup, group: NavGroup, key: string): OpenGroup | undefined {
  if (key === 'ArrowRight' && open !== group) return group
  if (key === 'ArrowLeft' && open === group) return null
  return undefined
}
