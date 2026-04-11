"use client";

import { Button } from "@/components/ui/button";

interface SidebarHeaderProps {
  collapsed: boolean;
  onToggle: () => void;
}

export function SidebarHeader({ collapsed, onToggle }: SidebarHeaderProps) {
  return (
    <div className="flex items-center justify-between p-4 border-b border-border/50">
      {!collapsed && <span className="font-bold text-lg">Bridge</span>}
      <Button variant="ghost" size="sm" onClick={onToggle} title="Toggle sidebar (Ctrl+B)">
        {collapsed ? "→" : "←"}
      </Button>
    </div>
  );
}
