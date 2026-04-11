"use client";

import { signOut } from "next-auth/react";
import { Button } from "@/components/ui/button";
import { ThemeToggle } from "./theme-toggle";

interface SidebarFooterProps {
  userName: string;
  collapsed: boolean;
}

export function SidebarFooter({ userName, collapsed }: SidebarFooterProps) {
  return (
    <div className="border-t border-border/50 p-3 space-y-2">
      <div className="flex items-center justify-between">
        {!collapsed && (
          <span className="text-sm text-muted-foreground truncate">{userName}</span>
        )}
        <ThemeToggle collapsed={collapsed} />
      </div>
      <Button
        variant="ghost"
        size="sm"
        className="w-full justify-start"
        onClick={() => signOut({ callbackUrl: "/" })}
      >
        {collapsed ? "↩" : "Sign Out"}
      </Button>
    </div>
  );
}
