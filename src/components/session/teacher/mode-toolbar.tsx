"use client";

export type DashboardMode = "presentation" | "grid" | "collaborate" | "broadcast";

interface ModeToolbarProps {
  activeMode: DashboardMode;
  onModeChange: (mode: DashboardMode) => void;
}

const modes: { key: DashboardMode; label: string; icon: string }[] = [
  { key: "presentation", label: "Presentation", icon: "📋" },
  { key: "grid", label: "Student Grid", icon: "🔲" },
  { key: "collaborate", label: "Collaborate", icon: "✏️" },
  { key: "broadcast", label: "Broadcast", icon: "📡" },
];

export function ModeToolbar({ activeMode, onModeChange }: ModeToolbarProps) {
  return (
    <div className="flex gap-1 border-t px-3 py-2 bg-muted/30">
      {modes.map((m) => (
        <button
          key={m.key}
          onClick={() => onModeChange(m.key)}
          className={`flex items-center gap-1.5 px-3 py-1.5 rounded text-sm transition-colors ${
            activeMode === m.key
              ? "bg-primary text-primary-foreground"
              : "text-muted-foreground hover:text-foreground hover:bg-muted"
          }`}
        >
          <span>{m.icon}</span>
          <span>{m.label}</span>
        </button>
      ))}
    </div>
  );
}
