/**
 * Icon character map for sidebar navigation.
 * Uses unicode/emoji as lightweight placeholders — will be replaced
 * with Lucide React components in the portal pages plan.
 */
export function getIconChar(iconName: string): string {
  const icons: Record<string, string> = {
    "layout-dashboard": "◻",
    "building-2": "🏢",
    "users": "👥",
    "settings": "⚙",
    "graduation-cap": "🎓",
    "book-open": "📖",
    "school": "🏫",
    "calendar": "📅",
    "bar-chart-3": "📊",
    "code": "⌨",
    "help-circle": "❓",
    "file-text": "📄",
    "video": "🎥",
    "puzzle": "🧩",
  };
  return icons[iconName] || "•";
}
