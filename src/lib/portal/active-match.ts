// Plan 067 phase 2 — active nav-item resolution.
//
// Both the desktop sidebar and the mobile bottom-nav need to highlight
// the item that "owns" the current pathname. The naive rule
// (`pathname === item || pathname.startsWith(item + "/")`) collides
// when one item's href is a prefix of another's — e.g., the teacher
// portal has both "Dashboard" (`/teacher`) and "Units" (`/teacher/units`).
// On `/teacher/units` the naive rule highlights both.
//
// Longest-match wins: among all items whose href is a prefix of the
// pathname, pick the one with the longest href. That uniquely
// identifies the deepest "owner" of the current page.

export interface ActiveMatchItem {
  href: string;
}

export function findActiveIndex(
  pathname: string,
  items: ActiveMatchItem[],
): number {
  let best = -1;
  let bestLen = -1;
  for (let i = 0; i < items.length; i++) {
    const itemPath = items[i].href.split("?")[0];
    const matches =
      pathname === itemPath || pathname.startsWith(itemPath + "/");
    if (!matches) continue;
    if (itemPath.length > bestLen) {
      bestLen = itemPath.length;
      best = i;
    }
  }
  return best;
}
